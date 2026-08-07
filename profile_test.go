package digitalpaper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	want := DeviceProfile{Name: "lab", Address: "digitalpaper.local", ClientID: "client", PrivateKeyRef: "key.pem", CertificateSHA256: strings.Repeat("a", 64)}
	if err := SaveProfile(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	got, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || got.Address != want.Address {
		t.Fatalf("profile = %#v, want %#v", got, want)
	}
	if got.EffectiveConnection() != ConnectionDirect {
		t.Fatalf("connection = %q", got.EffectiveConnection())
	}
	if err := SaveProfile(path, want); err == nil {
		t.Fatal("SaveProfile overwrote existing file")
	}
}

func TestConnectionModeInferenceAndValidation(t *testing.T) {
	for address, want := range map[string]ConnectionMode{
		"https://127.0.0.1:58443":         ConnectionRelay,
		"https://[::1]:58443":             ConnectionRelay,
		"https://localhost:58443":         ConnectionRelay,
		"https://digitalpaper.local:8443": ConnectionDirect,
	} {
		if got := InferConnectionMode(address); got != want {
			t.Fatalf("InferConnectionMode(%q) = %q, want %q", address, got, want)
		}
	}
	profile := DeviceProfile{Name: "bad", Address: "https://host", Connection: "unknown", ClientID: "id", CertificateSHA256: strings.Repeat("a", 64)}
	if err := profile.validate(); err == nil {
		t.Fatal("invalid connection mode accepted")
	}
}

func TestProfileRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	data := `{"name":"lab","address":"host","client_id":"id","certificate_sha256":"abc","unknown":true}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfile(path); err == nil {
		t.Fatal("LoadProfile accepted unknown field")
	}
}
