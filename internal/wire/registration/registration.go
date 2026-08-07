package registration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseSize = 1 << 20

// PINProvider obtains the PIN after the device has displayed it.
type PINProvider func(context.Context) (string, error)

// Result contains a newly registered client identity and the device CA.
type Result struct {
	ClientID      string
	PrivateKeyPEM []byte
	DeviceCAPEM   []byte
}

// HTTPError reports a registration endpoint error without exposing transcript
// or credential material.
type HTTPError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *HTTPError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("registration: HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("registration: HTTP %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

// Client performs one registration sequence over the device's dedicated HTTP
// endpoint. It never carries existing credentials or follows redirects.
type Client struct {
	base       *url.URL
	httpClient *http.Client
	random     io.Reader
}

// New constructs a registration client. Address may be a host, an HTTP
// registration URL, or an HTTPS API URL; the registration endpoint is always
// normalized to HTTP port 8080.
func New(address string, timeout time.Duration) (*Client, error) {
	base, err := registrationBaseURL(address)
	if err != nil {
		return nil, err
	}
	return newClient(base, timeout), nil
}

// NewExact constructs a client for an exact HTTP URL. It exists for the
// in-process protocol emulator, whose ephemeral port cannot be 8080.
func NewExact(address string, timeout time.Duration) (*Client, error) {
	base, err := url.Parse(address)
	if err != nil || base.Scheme != "http" || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return nil, errors.New("registration: invalid exact emulator address")
	}
	base.Path = "/"
	return newClient(base, timeout), nil
}

func newClient(base *url.URL, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &Client{
		base:   base,
		random: rand.Reader,
		httpClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("registration: redirects are disabled")
			},
			Transport: &http.Transport{
				Proxy:                 nil,
				ForceAttemptHTTP2:     false,
				MaxIdleConnsPerHost:   1,
				IdleConnTimeout:       15 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
			},
		},
	}
}

// Register performs the complete M1-M6 registration exchange. Cleanup is
// attempted after every sequence, including failures.
func (c *Client) Register(ctx context.Context, providePIN PINProvider) (result Result, err error) {
	if providePIN == nil {
		return Result{}, errors.New("registration: nil PIN provider")
	}
	if err := c.cleanup(ctx); err != nil {
		return Result{}, fmt.Errorf("registration: initial cleanup: %w", err)
	}
	defer func() {
		_ = c.cleanup(context.WithoutCancel(ctx))
	}()

	var m1 wireMessage
	if err := c.doJSON(ctx, http.MethodPost, "/register/pin", nil, &m1); err != nil {
		return Result{}, err
	}
	nonce1, err := decodeField("M1.a", m1.A, 16)
	if err != nil {
		return Result{}, err
	}
	mac, err := decodeField("M1.b", m1.B, 6)
	if err != nil {
		return Result{}, err
	}
	otherRaw, err := decodeFieldRange("M1.c", m1.C, 256, 257)
	if err != nil {
		return Result{}, err
	}

	dh, err := newDH(c.random)
	if err != nil {
		return Result{}, err
	}
	publicKey := dh.publicBytes()
	sharedKey, err := dh.sharedBytes(otherRaw)
	if err != nil {
		return Result{}, err
	}
	nonce2 := make([]byte, 16)
	if _, err := io.ReadFull(c.random, nonce2); err != nil {
		return Result{}, err
	}
	authKey, wrapKey := deriveKeys(sharedKey, join(nonce1, mac, nonce2))
	m2hmac := hashMAC(authKey, nonce1, mac, otherRaw, nonce1, nonce2, mac, publicKey)
	m2 := wireMessage{A: encode(nonce1), B: encode(nonce2), C: encode(mac), D: encode(publicKey), E: encode(m2hmac)}
	var m3 wireMessage
	if err := c.doJSON(ctx, http.MethodPost, "/register/hash", m2, &m3); err != nil {
		return Result{}, err
	}
	returnedNonce2, err := decodeField("M3.a", m3.A, 16)
	if err != nil || subtle.ConstantTimeCompare(returnedNonce2, nonce2) != 1 {
		return Result{}, errors.New("registration: M3 nonce mismatch")
	}
	eHash, err := decodeField("M3.b", m3.B, 32)
	if err != nil {
		return Result{}, err
	}
	m3hmac, err := decodeField("M3.e", m3.E, 32)
	if err != nil {
		return Result{}, err
	}
	wantM3 := hashMAC(authKey, nonce1, nonce2, mac, publicKey, m2hmac, nonce2, eHash)
	if subtle.ConstantTimeCompare(m3hmac, wantM3) != 1 {
		return Result{}, errors.New("registration: M3 transcript authentication failed")
	}

	pin, err := providePIN(ctx)
	if err != nil {
		return Result{}, err
	}
	if pin == "" || len(pin) > 64 || strings.ContainsAny(pin, "\r\n\x00") {
		return Result{}, errors.New("registration: invalid PIN")
	}
	psk := hashMAC(authKey, []byte(pin))
	rs := make([]byte, 16)
	if _, err := io.ReadFull(c.random, rs); err != nil {
		return Result{}, err
	}
	rHash := hashMAC(authKey, rs, psk, otherRaw, publicKey)
	wrappedRS, err := wrap(rs, authKey, wrapKey, c.random)
	if err != nil {
		return Result{}, err
	}
	m4hmac := hashMAC(authKey, nonce2, eHash, m3hmac, nonce1, rHash, wrappedRS)
	m4 := wireMessage{A: encode(nonce1), B: encode(rHash), D: encode(wrappedRS), E: encode(m4hmac)}
	var m5 wireMessage
	if err := c.doJSON(ctx, http.MethodPost, "/register/ca", m4, &m5); err != nil {
		return Result{}, err
	}
	returnedNonce2, err = decodeField("M5.a", m5.A, 16)
	if err != nil || subtle.ConstantTimeCompare(returnedNonce2, nonce2) != 1 {
		return Result{}, errors.New("registration: M5 nonce mismatch")
	}
	wrappedCertificate, err := decodeFieldRange("M5.d", m5.D, 32, maxResponseSize/2)
	if err != nil {
		return Result{}, err
	}
	m5hmac, err := decodeField("M5.e", m5.E, 32)
	if err != nil {
		return Result{}, err
	}
	wantM5 := hashMAC(authKey, nonce1, rHash, wrappedRS, m4hmac, nonce2, wrappedCertificate)
	if subtle.ConstantTimeCompare(m5hmac, wantM5) != 1 {
		return Result{}, errors.New("registration: M5 transcript authentication failed")
	}
	esCertificate, err := unwrap(wrappedCertificate, authKey, wrapKey)
	if err != nil || len(esCertificate) <= 16 {
		return Result{}, errors.New("registration: invalid wrapped device certificate")
	}
	es, certificate := esCertificate[:16], esCertificate[16:]
	wantEHash := hashMAC(authKey, es, psk, otherRaw, publicKey)
	if subtle.ConstantTimeCompare(eHash, wantEHash) != 1 {
		return Result{}, errors.New("registration: PIN or device certificate authentication failed")
	}
	if err := validateCertificatePEM(certificate); err != nil {
		return Result{}, err
	}

	privateKey, err := rsa.GenerateKey(c.random, 2048)
	if err != nil {
		return Result{}, fmt.Errorf("registration: generate RSA key: %w", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return Result{}, err
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return Result{}, err
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	clientID, err := randomUUID(c.random)
	if err != nil {
		return Result{}, err
	}
	wrappedIdentity, err := wrap(append([]byte(clientID), publicPEM...), authKey, wrapKey, c.random)
	if err != nil {
		return Result{}, err
	}
	m6hmac := hashMAC(authKey, nonce2, wrappedCertificate, m5hmac, nonce1, wrappedIdentity)
	m6 := wireMessage{A: encode(nonce1), D: encode(wrappedIdentity), E: encode(m6hmac)}
	if err := c.doJSON(ctx, http.MethodPost, "/register", m6, nil); err != nil {
		return Result{}, err
	}
	return Result{ClientID: clientID, PrivateKeyPEM: privatePEM, DeviceCAPEM: append([]byte(nil), certificate...)}, nil
}

type wireMessage struct {
	A string `json:"a,omitempty"`
	B string `json:"b,omitempty"`
	C string `json:"c,omitempty"`
	D string `json:"d,omitempty"`
	E string `json:"e,omitempty"`
}

func (c *Client) cleanup(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPut, "/register/cleanup", nil, nil)
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	requestURL := c.base.ResolveReference(&url.URL{Path: endpoint})
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return err
	}
	if len(raw) > maxResponseSize {
		return errors.New("registration: response exceeds size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var problem struct {
			Code      any    `json:"code"`
			ErrorCode any    `json:"error_code"`
			Message   string `json:"message"`
		}
		_ = json.Unmarshal(raw, &problem)
		var code string
		if problem.ErrorCode != nil {
			code = fmt.Sprint(problem.ErrorCode)
		} else if problem.Code != nil {
			code = fmt.Sprint(problem.Code)
		}
		return &HTTPError{StatusCode: response.StatusCode, Code: code, Message: problem.Message}
	}
	if output == nil {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("registration: decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("registration: response contains trailing data")
	}
	return nil
}

func registrationBaseURL(address string) (*url.URL, error) {
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("registration: invalid device address")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("registration: address must use HTTP or HTTPS")
	}
	parsed.Scheme = "http"
	parsed.Host = net.JoinHostPort(parsed.Hostname(), "8080")
	parsed.Path = "/"
	return parsed, nil
}

func decodeField(name, value string, length int) ([]byte, error) {
	decoded, err := decodeFieldRange(name, value, length, length)
	return decoded, err
}

func decodeFieldRange(name, value string, minimum, maximum int) ([]byte, error) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) < minimum || len(decoded) > maximum {
		return nil, fmt.Errorf("registration: invalid %s", name)
	}
	return decoded, nil
}

func encode(value []byte) string { return base64.StdEncoding.EncodeToString(value) }

func join(values ...[]byte) []byte {
	length := 0
	for _, value := range values {
		length += len(value)
	}
	result := make([]byte, 0, length)
	for _, value := range values {
		result = append(result, value...)
	}
	return result
}

func validateCertificatePEM(value []byte) error {
	rest := value
	found := false
	for len(strings.TrimSpace(string(rest))) != 0 {
		block, trailing := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" {
			return errors.New("registration: device returned invalid certificate PEM")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return errors.New("registration: device returned invalid X.509 certificate")
		}
		found = true
		rest = trailing
	}
	if !found {
		return errors.New("registration: device returned no certificate")
	}
	return nil
}

func randomUUID(random io.Reader) (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
