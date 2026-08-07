package dpwire

import (
	"encoding/json"
	"testing"
)

func TestRemotePathValidationAndNFC(t *testing.T) {
	path, err := ParseRemotePath("Document/Cafe\u0301/paper.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := path.String(), "Document/Café/paper.pdf"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got, want := path.EscapedValue(), "Document%2FCaf%C3%A9%2Fpaper.pdf"; got != want {
		t.Fatalf("EscapedValue() = %q, want %q", got, want)
	}
	encoded, err := json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RemotePath
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != path {
		t.Fatalf("round trip = %q, want %q", decoded, path)
	}
}

func TestRemotePathRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"", "/Document/a", "Document/", "Other/a", "Document//a", "Document/../a", "Document/./a", "Document/a\\b", "Document/a\x00b"} {
		if _, err := ParseRemotePath(value); err == nil {
			t.Errorf("ParseRemotePath(%q) unexpectedly succeeded", value)
		}
	}
}
