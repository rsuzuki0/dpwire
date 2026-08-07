package profiles

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsuzuki0/dpwire"
)

func TestImportListUseAndCurrent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config")
	manager, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	first := sonyCredentials(t, "first-client")
	profile, err := manager.ImportSony("first", "https://127.0.0.1:58443", strings.Repeat("a", 64), first)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "first" || !filepath.IsAbs(profile.PrivateKeyRef) {
		t.Fatalf("profile = %#v", profile)
	}
	if profile.Connection != dpwire.ConnectionRelay {
		t.Fatalf("first connection = %q", profile.Connection)
	}
	second := sonyCredentials(t, "second-client")
	if _, err := manager.ImportSony("second", "https://192.0.2.1:8443", strings.Repeat("b", 64), second); err != nil {
		t.Fatal(err)
	}
	items, err := manager.List()
	if err != nil || len(items) != 2 || !items[0].Current || items[0].Name != "first" {
		t.Fatalf("items = %#v, err = %v", items, err)
	}
	if items[1].Connection != dpwire.ConnectionDirect {
		t.Fatalf("second connection = %q", items[1].Connection)
	}
	if err := manager.Use("second"); err != nil {
		t.Fatal(err)
	}
	name, current, err := manager.Current()
	if err != nil || name != "second" || current.Name != "second" {
		t.Fatalf("current = %q %#v, err = %v", name, current, err)
	}
	if _, err := manager.ImportSony("second", "https://192.0.2.1:8443", strings.Repeat("b", 64), second); err == nil {
		t.Fatal("existing profile was overwritten")
	}
	pinRequested := false
	if _, err := manager.Pair(context.Background(), "first", "digitalpaper.local", func(context.Context) (string, error) {
		pinRequested = true
		return "123456", nil
	}); err == nil || pinRequested {
		t.Fatalf("existing profile pairing error=%v pinRequested=%v", err, pinRequested)
	}
	if _, err := manager.Pair(context.Background(), "failed-pair", "https://127.0.0.1:58443", func(context.Context) (string, error) {
		return "123456", nil
	}); err == nil {
		t.Fatal("loopback pairing succeeded")
	}
	if _, err := os.Stat(filepath.Join(root, "profiles", "failed-pair")); !os.IsNotExist(err) {
		t.Fatalf("failed pairing directory remains: %v", err)
	}
	for _, path := range []string{
		filepath.Join(root, "config.json"),
		filepath.Join(root, "profiles", "first", "profile.json"),
		filepath.Join(root, "profiles", "first", "privatekey.pem"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %v", path, info.Mode())
		}
	}
}

func TestRejectsUnsafeNamesAndConfig(t *testing.T) {
	if _, err := New("relative"); err == nil {
		t.Fatal("relative root accepted")
	}
	manager, _ := New(filepath.Join(t.TempDir(), "config"))
	for _, name := range []string{"", ".", "..", "bad/name", "bad name"} {
		if _, err := manager.Load(name); err == nil {
			t.Fatalf("unsafe name %q accepted", name)
		}
	}
}

func TestDefaultRootCompatibility(t *testing.T) {
	base := t.TempDir()
	current := filepath.Join(base, "dpwire")
	legacy := filepath.Join(base, "digitalpaper")

	root, err := defaultRootIn(base)
	if err != nil || root != current {
		t.Fatalf("new root = %q, err = %v", root, err)
	}
	if err := os.Mkdir(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err = defaultRootIn(base)
	if err != nil || root != legacy {
		t.Fatalf("legacy root = %q, err = %v", root, err)
	}
	if err := os.Mkdir(current, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err = defaultRootIn(base)
	if err != nil || root != current {
		t.Fatalf("current root = %q, err = %v", root, err)
	}
}

func TestDefaultRootRejectsLegacySymlink(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, "digitalpaper")
	if err := os.Symlink(t.TempDir(), legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := defaultRootIn(base); err == nil {
		t.Fatal("legacy configuration symlink accepted")
	}
}

func sonyCredentials(t *testing.T, clientID string) string {
	t.Helper()
	directory := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(filepath.Join(directory, "deviceid.dat"), []byte(clientID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "privatekey.dat"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return directory
}
