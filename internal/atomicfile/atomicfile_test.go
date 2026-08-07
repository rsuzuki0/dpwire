package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteNewAndReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value")
	if err := WriteNew(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(path, []byte("second"), 0o600); err == nil {
		t.Fatal("WriteNew replaced an existing file")
	}
	if err := Replace(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := os.ReadFile(path)
	if err != nil || string(value) != "second" {
		t.Fatalf("value = %q, err = %v", value, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, err = %v", info.Mode(), err)
	}
	if err := WriteNew(filepath.Join(t.TempDir(), "wide"), nil, 0o644); err == nil {
		t.Fatal("owner-public permission was accepted")
	}
	if err := Replace(path, nil, 0o644); err == nil {
		t.Fatal("owner-public replacement permission was accepted")
	}
}
