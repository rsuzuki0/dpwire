package main

import (
	"path/filepath"
	"testing"
)

func TestForbiddenDependencies(t *testing.T) {
	tests := []struct {
		file, imported string
		forbidden      bool
	}{
		{"client.go", module + "/workflow/send", true},
		{"credentials/store.go", module + "/internal/wire/auth", false},
		{"internal/wire/auth/auth.go", module + "/dptest", true},
		{"workflow/send/send.go", module, false},
		{"workflow/send/send.go", module + "/internal/wire/documents", true},
	}
	for _, test := range tests {
		got := forbidden(test.file, test.imported) != ""
		if got != test.forbidden {
			t.Errorf("forbidden(%q, %q) = %v, want %v", test.file, test.imported, got, test.forbidden)
		}
	}
}

func TestRepositoryArchitecture(t *testing.T) {
	violations, err := check(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %#v", violations)
	}
}
