package credentials

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func testKey(t *testing.T, pkcs8 bool) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if pkcs8 {
		encoded, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})
	}
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func TestImportSonyAndFindCandidates(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "DigitalPaperApp", "workspace")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	clientPath := filepath.Join(directory, "deviceid.dat")
	keyPath := filepath.Join(directory, "privatekey.dat")
	if err := os.WriteFile(clientPath, []byte("client-123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, testKey(t, false), 0o600); err != nil {
		t.Fatal(err)
	}
	candidates, err := FindSonyCandidates(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Directory != directory {
		t.Fatalf("candidates = %#v", candidates)
	}
	credentials, err := ImportSony(clientPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.ClientID != "client-123" {
		t.Fatalf("client ID = %q", credentials.ClientID)
	}
}

func TestParsePKCS8AndRejectTrailingPEM(t *testing.T) {
	key := testKey(t, true)
	if _, err := ParseRSAPrivateKey(key); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRSAPrivateKey(append(key, key...)); err == nil {
		t.Fatal("accepted multiple PEM blocks")
	}
}
