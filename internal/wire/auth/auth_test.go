package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rsuzuki0/dpwire/internal/wire/transport"
)

func TestAuthenticate(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	nonce := "nonce-value"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/nonce/client-id":
			_ = json.NewEncoder(w).Encode(map[string]string{"nonce": nonce})
		case "/auth":
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			signature, err := base64.StdEncoding.DecodeString(request["nonce_signed"])
			if err != nil {
				t.Error(err)
			}
			digest := sha256.Sum256([]byte(nonce))
			if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Add("Set-Cookie", "Credentials=abc==; Secure; Path=/")
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	sum := sha256.Sum256(server.Certificate().Raw)
	client, err := transport.New(server.URL, transport.TrustConfig{CertificateSHA256: hex.EncodeToString(sum[:])}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := Authenticate(context.Background(), client, "client-id", key); err != nil {
		t.Fatal(err)
	}
}

func TestParseCredentialsCookie(t *testing.T) {
	value, err := ParseCredentialsCookie([]string{"Other=x", "Credentials=YWJjZA==; Path=/; Secure"})
	if err != nil {
		t.Fatal(err)
	}
	if value != "YWJjZA==" {
		t.Fatalf("value = %q", value)
	}
	for _, headers := range [][]string{nil, {"Credentials="}, {"Credentials=a", "Credentials=b"}} {
		if _, err := ParseCredentialsCookie(headers); err == nil {
			t.Fatalf("headers %#v unexpectedly succeeded", headers)
		}
	}
}
