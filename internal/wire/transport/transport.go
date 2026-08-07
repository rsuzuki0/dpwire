package transport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	maxJSONBody  = 8 << 20
	maxErrorBody = 1 << 20
)

// TrustConfig specifies device-specific TLS trust anchors.
type TrustConfig struct {
	CAPEM             []byte
	CertificateSHA256 string
}

// Client sends bounded requests to one device.
type Client struct {
	base       *url.URL
	httpClient *http.Client
	mu         sync.RWMutex
	credential string
}

// HTTPError is the protocol-level error returned internally to the public API.
type HTTPError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *HTTPError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("digitalpaper: HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("digitalpaper: HTTP %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

// New constructs a verified HTTPS transport.
func New(address string, trust TrustConfig, timeout time.Duration) (*Client, error) {
	base, err := baseURL(address)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := tlsConfig(trust)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &Client{
		base: base,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				TLSClientConfig:       tlsConfig,
				ForceAttemptHTTP2:     false,
				MaxIdleConnsPerHost:   2,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
			},
		},
	}, nil
}

// SetCredential sets the opaque Credentials cookie value.
func (c *Client) SetCredential(value string) error {
	if value == "" || strings.ContainsAny(value, ";\r\n\x00") {
		return errors.New("digitalpaper: invalid Credentials cookie")
	}
	c.mu.Lock()
	c.credential = value
	c.mu.Unlock()
	return nil
}

// DoJSON performs a JSON request and bounds response memory.
func (c *Client) DoJSON(ctx context.Context, method, endpoint string, query url.Values, input, output any, authenticated bool) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	response, err := c.do(ctx, method, endpoint, query, body, authenticated, "application/json", "application/json", -1)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if output == nil {
		written, err := io.Copy(io.Discard, io.LimitReader(response.Body, maxJSONBody+1))
		if err != nil {
			return err
		}
		if written > maxJSONBody {
			return errors.New("digitalpaper: JSON response exceeds limit")
		}
		return nil
	}
	limited := io.LimitReader(response.Body, maxJSONBody+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(raw) > maxJSONBody {
		return errors.New("digitalpaper: JSON response exceeds limit")
	}
	if err := json.Unmarshal(raw, output); err != nil {
		return fmt.Errorf("digitalpaper: decode response: %w", err)
	}
	return nil
}

// Do performs a request. The caller owns a successful response body.
func (c *Client) Do(ctx context.Context, method, endpoint string, query url.Values, body io.Reader, authenticated bool) (*http.Response, error) {
	return c.do(ctx, method, endpoint, query, body, authenticated, "application/json", "", -1)
}

// DoWithAccept performs a request with an explicit response media type.
func (c *Client) DoWithAccept(ctx context.Context, method, endpoint string, query url.Values, body io.Reader, authenticated bool, accept string) (*http.Response, error) {
	if strings.ContainsAny(accept, "\r\n\x00") {
		return nil, errors.New("digitalpaper: invalid Accept header")
	}
	return c.do(ctx, method, endpoint, query, body, authenticated, "application/json", accept, -1)
}

// DoMedia performs a request with explicit media types and content length. It
// is used for bounded streaming bodies such as multipart PDF uploads.
func (c *Client) DoMedia(ctx context.Context, method, endpoint string, query url.Values, body io.Reader, authenticated bool, contentType, accept string, contentLength int64) (*http.Response, error) {
	if strings.ContainsAny(contentType+accept, "\r\n\x00") {
		return nil, errors.New("digitalpaper: invalid media type header")
	}
	if contentLength < 0 {
		return nil, errors.New("digitalpaper: negative content length")
	}
	response, err := c.do(ctx, method, endpoint, query, body, authenticated, contentType, accept, contentLength)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) do(ctx context.Context, method, endpoint string, query url.Values, body io.Reader, authenticated bool, contentType, accept string, contentLength int64) (*http.Response, error) {
	requestURL, err := c.resolve(endpoint, query)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		request.Header.Set("Accept", accept)
	}
	if body != nil {
		request.Header.Set("Content-Type", contentType)
		if contentLength >= 0 {
			request.ContentLength = contentLength
		}
	}
	if authenticated {
		c.mu.RLock()
		credential := c.credential
		c.mu.RUnlock()
		if credential == "" {
			return nil, errors.New("digitalpaper: request requires authentication")
		}
		request.Header.Set("Cookie", "Credentials="+credential)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response, nil
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
	var problem struct {
		Code      any    `json:"code"`
		ErrorCode any    `json:"error_code"`
		Message   string `json:"message"`
	}
	_ = json.Unmarshal(raw, &problem)
	code := ""
	if problem.ErrorCode != nil {
		code = fmt.Sprint(problem.ErrorCode)
	} else if problem.Code != nil {
		code = fmt.Sprint(problem.Code)
	}
	return nil, &HTTPError{StatusCode: response.StatusCode, Code: code, Message: problem.Message}
}

func (c *Client) resolve(endpoint string, query url.Values) (*url.URL, error) {
	if !strings.HasPrefix(endpoint, "/") {
		return nil, errors.New("digitalpaper: endpoint must be absolute")
	}
	reference, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	resolved := c.base.ResolveReference(reference)
	resolved.RawQuery = query.Encode()
	return resolved, nil
}

func baseURL(address string) (*url.URL, error) {
	if !strings.Contains(address, "://") {
		host := address
		if _, _, err := net.SplitHostPort(address); err != nil {
			host = net.JoinHostPort(strings.Trim(address, "[]"), "8443")
		}
		address = "https://" + host
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("digitalpaper: address must identify an HTTPS device")
	}
	parsed.Path = "/"
	return parsed, nil
}

func tlsConfig(trust TrustConfig) (*tls.Config, error) {
	fingerprint, err := decodeFingerprint(trust.CertificateSHA256)
	if err != nil {
		return nil, err
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	var roots *x509.CertPool
	if len(trust.CAPEM) > 0 {
		roots = x509.NewCertPool()
		if !roots.AppendCertsFromPEM(trust.CAPEM) {
			return nil, errors.New("digitalpaper: invalid device CA PEM")
		}
	}
	if len(fingerprint) > 0 {
		// Verification is replaced, not skipped: VerifyConnection below requires
		// the exact leaf. This also supports legacy device certificates that have
		// a matching Common Name but no Subject Alternative Name extension.
		config.InsecureSkipVerify = true
	} else if roots != nil {
		config.RootCAs = roots
	} else {
		return nil, errors.New("digitalpaper: no TLS trust anchor")
	}
	if len(fingerprint) > 0 {
		config.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("digitalpaper: peer sent no certificate")
			}
			sum := sha256.Sum256(state.PeerCertificates[0].Raw)
			if subtle.ConstantTimeCompare(sum[:], fingerprint) != 1 {
				return errors.New("digitalpaper: certificate fingerprint mismatch")
			}
			return nil
		}
	}
	return config, nil
}

func decodeFingerprint(value string) ([]byte, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), ":", "")
	if value == "" {
		return nil, nil
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return nil, errors.New("digitalpaper: certificate fingerprint must be SHA-256")
	}
	return decoded, nil
}
