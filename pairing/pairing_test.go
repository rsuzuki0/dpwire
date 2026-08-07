package pairing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"net/http/httptest"
	"testing"
)

func TestAPIAddress(t *testing.T) {
	for input, want := range map[string]string{
		"digitalpaper.local":              "https://digitalpaper.local:8443",
		"http://digitalpaper.local:8080":  "https://digitalpaper.local:8443",
		"https://digitalpaper.local:8443": "https://digitalpaper.local:8443",
		"http://[fe80::1%25en12]:8080":    "https://[fe80::1%25en12]:8443",
	} {
		got, err := APIAddress(input)
		if err != nil || got != want {
			t.Fatalf("APIAddress(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, invalid := range []string{"https://127.0.0.1:58443", "ftp://host", "https://host/path"} {
		if _, err := APIAddress(invalid); err == nil {
			t.Fatalf("invalid address %q accepted", invalid)
		}
	}
}

func TestCertificateFingerprint(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	certificate := server.Certificate()
	value := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	got, err := certificateFingerprint(value)
	want := sha256.Sum256(certificate.Raw)
	if err != nil || got != hex.EncodeToString(want[:]) {
		t.Fatalf("fingerprint=%q err=%v", got, err)
	}
	if _, err := certificateFingerprint([]byte("bad")); err == nil {
		t.Fatal("invalid certificate accepted")
	}
}
