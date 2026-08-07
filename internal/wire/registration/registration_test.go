package registration

import (
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRegistrationSequence(t *testing.T) {
	server := newRegistrationServer(t, "123456", false)
	defer server.Close()
	base, _ := url.Parse(server.URL + "/")
	client := &Client{base: base, httpClient: server.Client(), random: cryptorand.Reader}
	result, err := client.Register(context.Background(), func(context.Context) (string, error) { return "123456", nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ClientID) != 36 || len(result.PrivateKeyPEM) == 0 || len(result.DeviceCAPEM) == 0 {
		t.Fatalf("incomplete result: client ID length=%d key=%d CA=%d", len(result.ClientID), len(result.PrivateKeyPEM), len(result.DeviceCAPEM))
	}
	if err := validateCertificatePEM(result.DeviceCAPEM); err != nil {
		t.Fatal(err)
	}
	serverState := server.Config.BaseContext(nil).Value(registrationStateKey{}).(*registrationServerState)
	serverState.mu.Lock()
	defer serverState.mu.Unlock()
	if serverState.cleanupCount < 2 || !serverState.registered {
		t.Fatalf("cleanup=%d registered=%v", serverState.cleanupCount, serverState.registered)
	}
}

func TestRegistrationRejectsWrongPINAndCorruptTranscript(t *testing.T) {
	for _, test := range []struct {
		name, entered string
		corrupt       bool
	}{
		{name: "wrong PIN", entered: "000000"},
		{name: "corrupt M3", entered: "123456", corrupt: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newRegistrationServer(t, "123456", test.corrupt)
			defer server.Close()
			base, _ := url.Parse(server.URL + "/")
			client := &Client{base: base, httpClient: server.Client(), random: cryptorand.Reader}
			if _, err := client.Register(context.Background(), func(context.Context) (string, error) { return test.entered, nil }); err == nil {
				t.Fatal("invalid registration succeeded")
			}
		})
	}
}

func TestRegistrationAddressAndUUID(t *testing.T) {
	for input, want := range map[string]string{
		"digitalpaper.local":              "http://digitalpaper.local:8080/",
		"https://digitalpaper.local:8443": "http://digitalpaper.local:8080/",
		"http://[fe80::1%25en12]:8080":    "http://[fe80::1%25en12]:8080/",
	} {
		got, err := registrationBaseURL(input)
		if err != nil || got.String() != want {
			t.Fatalf("registrationBaseURL(%q) = %v, %v; want %s", input, got, err, want)
		}
	}
	for _, invalid := range []string{"ftp://host", "https://host/path", "https://user@host"} {
		if _, err := registrationBaseURL(invalid); err == nil {
			t.Fatalf("invalid address %q accepted", invalid)
		}
	}
	uuid, err := randomUUID(strings.NewReader(strings.Repeat("x", 16)))
	if err != nil || len(uuid) != 36 || uuid[14] != '4' || !strings.Contains("89ab", string(uuid[19])) {
		t.Fatalf("UUID = %q, %v", uuid, err)
	}
}

func TestDoJSONRejectsTrailingResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{} {}`))
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL + "/")
	client := &Client{base: base, httpClient: server.Client(), random: cryptorand.Reader}
	var output wireMessage
	if err := client.doJSON(context.Background(), http.MethodGet, "/test", nil, &output); err == nil || !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("trailing response error = %v", err)
	}
}

type registrationStateKey struct{}

type registrationServerState struct {
	mu           sync.Mutex
	pin          string
	corruptM3    bool
	cleanupCount int
	registered   bool
	nonce1       []byte
	mac          []byte
	dh           *dhExchange
	otherRaw     []byte
	nonce2       []byte
	clientPublic []byte
	authKey      []byte
	wrapKey      []byte
	m2hmac       []byte
	eHash        []byte
	m3hmac       []byte
	es           []byte
	certificate  []byte
	wrappedCert  []byte
	m5hmac       []byte
}

func newRegistrationServer(t *testing.T, pin string, corruptM3 bool) *httptest.Server {
	t.Helper()
	dh, err := newDH(cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	state := &registrationServerState{
		pin: pin, corruptM3: corruptM3, nonce1: []byte("0123456789abcdef"), mac: []byte{1, 2, 3, 4, 5, 6},
		dh: dh, otherRaw: dh.publicBytes(), es: []byte("fedcba9876543210"), certificate: testCertificate(t),
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.serve(w, r)
	}))
	server.Config.BaseContext = func(net.Listener) context.Context {
		return context.WithValue(context.Background(), registrationStateKey{}, state)
	}
	server.Start()
	return server
}

func (s *registrationServerState) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.Method + " " + r.URL.Path {
	case "PUT /register/cleanup":
		s.cleanupCount++
		w.WriteHeader(http.StatusNoContent)
	case "POST /register/pin":
		_ = json.NewEncoder(w).Encode(wireMessage{A: encode(s.nonce1), B: encode(s.mac), C: encode(s.otherRaw)})
	case "POST /register/hash":
		var m2 wireMessage
		if json.NewDecoder(r.Body).Decode(&m2) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		s.nonce2, _ = decodeField("M2.b", m2.B, 16)
		s.clientPublic, _ = decodeField("M2.d", m2.D, 257)
		s.m2hmac, _ = decodeField("M2.e", m2.E, 32)
		shared, err := s.dh.sharedBytes(s.clientPublic)
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		s.authKey, s.wrapKey = deriveKeys(shared, join(s.nonce1, s.mac, s.nonce2))
		psk := hashMAC(s.authKey, []byte(s.pin))
		s.eHash = hashMAC(s.authKey, s.es, psk, s.otherRaw, s.clientPublic)
		s.m3hmac = hashMAC(s.authKey, s.nonce1, s.nonce2, s.mac, s.clientPublic, s.m2hmac, s.nonce2, s.eHash)
		if s.corruptM3 {
			s.m3hmac[0] ^= 1
		}
		_ = json.NewEncoder(w).Encode(wireMessage{A: encode(s.nonce2), B: encode(s.eHash), E: encode(s.m3hmac)})
	case "POST /register/ca":
		var m4 wireMessage
		_ = json.NewDecoder(r.Body).Decode(&m4)
		rHash, _ := decodeField("M4.b", m4.B, 32)
		wrappedRS, _ := decodeFieldRange("M4.d", m4.D, 32, 128)
		m4hmac, _ := decodeField("M4.e", m4.E, 32)
		s.wrappedCert, _ = wrap(append(append([]byte(nil), s.es...), s.certificate...), s.authKey, s.wrapKey, cryptorand.Reader)
		s.m5hmac = hashMAC(s.authKey, s.nonce1, rHash, wrappedRS, m4hmac, s.nonce2, s.wrappedCert)
		_ = json.NewEncoder(w).Encode(wireMessage{A: encode(s.nonce2), D: encode(s.wrappedCert), E: encode(s.m5hmac)})
	case "POST /register":
		s.registered = true
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func testCertificate(t *testing.T) []byte {
	t.Helper()
	public, private, err := ed25519.GenerateKey(cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "digitalpaper.local"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(cryptorand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
