package transport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testServer(t *testing.T) (*httptest.Server, string, []byte) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "Credentials=token==" {
			http.Error(w, `{"code":"40100","message":"missing cookie"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":"ok"}`))
	}))
	t.Cleanup(server.Close)
	certificate := server.Certificate()
	sum := sha256.Sum256(certificate.Raw)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	return server, hex.EncodeToString(sum[:]), ca
}

func TestPinnedTransportAndCookie(t *testing.T) {
	server, fingerprint, _ := testServer(t)
	client, err := New(server.URL, TrustConfig{CertificateSHA256: fingerprint}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetCredential("token=="); err != nil {
		t.Fatal(err)
	}
	var value struct {
		Value string `json:"value"`
	}
	if err := client.DoJSON(context.Background(), http.MethodGet, "/test", nil, nil, &value, true); err != nil {
		t.Fatal(err)
	}
	if value.Value != "ok" {
		t.Fatalf("value = %q", value.Value)
	}
}

func TestCATransportAndTrustFailures(t *testing.T) {
	server, _, ca := testServer(t)
	client, err := New(server.URL, TrustConfig{CAPEM: ca}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(context.Background(), http.MethodGet, "/", nil, nil, true); err == nil {
		t.Fatal("authenticated request without credential succeeded")
	}
	if _, err := New(server.URL, TrustConfig{}, time.Second); err == nil {
		t.Fatal("transport without trust anchor succeeded")
	}
	wrong := make([]byte, sha256.Size)
	wrongClient, err := New(server.URL, TrustConfig{CertificateSHA256: hex.EncodeToString(wrong)}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := wrongClient.SetCredential("token"); err != nil {
		t.Fatal(err)
	}
	if _, err := wrongClient.Do(context.Background(), http.MethodGet, "/", nil, nil, true); err == nil {
		t.Fatal("wrong certificate fingerprint succeeded")
	}
}

func TestInspectUntrustedCertificate(t *testing.T) {
	server, fingerprint, _ := testServer(t)
	result, err := InspectUntrustedCertificate(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if result.SHA256 != fingerprint || len(result.PEM) == 0 {
		t.Fatalf("inspection = %#v", result)
	}
}

func TestHTTPErrorIsBoundedAndTyped(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":40401,"message":"missing"}`))
	}))
	defer server.Close()
	sum := sha256.Sum256(server.Certificate().Raw)
	client, err := New(server.URL, TrustConfig{CertificateSHA256: hex.EncodeToString(sum[:])}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(context.Background(), http.MethodGet, "/missing", nil, nil, false)
	httpError, ok := err.(*HTTPError)
	if !ok || httpError.Code != "40401" {
		t.Fatalf("error = %#v", err)
	}
}

func TestDoWithAccept(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/pdf" {
			t.Errorf("Accept = %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	sum := sha256.Sum256(server.Certificate().Raw)
	client, err := New(server.URL, TrustConfig{CertificateSHA256: hex.EncodeToString(sum[:])}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.DoWithAccept(context.Background(), http.MethodGet, "/file", nil, nil, false, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if _, err := client.DoWithAccept(context.Background(), http.MethodGet, "/file", nil, nil, false, "bad\r\nheader"); err == nil {
		t.Fatal("invalid Accept header succeeded")
	}
}
