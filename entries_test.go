package dpwire

import (
	"strings"
	"testing"
)

func canonicalWireDocument() wireEntry {
	return wireEntry{
		ID:             "document-1",
		Name:           "paper.pdf",
		Path:           "Document/Documents/paper.pdf",
		Type:           "document",
		Created:        "2026-08-06T11:00:00Z",
		Modified:       "2026-08-06T12:00:00.123Z",
		Size:           "42",
		ParentFolderID: "folder-1",
	}
}

func TestDecodeEntryStrictValidation(t *testing.T) {
	raw := canonicalWireDocument()
	entry, err := decodeEntry(raw, true)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Name != raw.Name || entry.Path.String() != raw.Path {
		t.Fatalf("entry = %+v", entry)
	}

	for _, test := range []struct {
		name   string
		change func(*wireEntry)
		want   string
	}{
		{"timestamp", func(entry *wireEntry) { entry.Modified = "August 6" }, "noncanonical modified_date"},
		{"size", func(entry *wireEntry) { entry.Size = "42 bytes" }, "noncanonical file_size"},
		{"missing time", func(entry *wireEntry) { entry.Modified = "" }, "no modification time"},
		{"missing size", func(entry *wireEntry) { entry.Size = "" }, "no file size"},
		{"name mismatch", func(entry *wireEntry) { entry.Name = "other.pdf" }, "name does not match"},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := raw
			test.change(&changed)
			if _, err := decodeEntry(changed, true); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("decodeEntry error = %v, want %q", err, test.want)
			}
			if _, err := decodeEntry(changed, false); err != nil {
				t.Fatalf("normal validation rejected safe legacy metadata: %v", err)
			}
		})
	}
}

func TestDecodeEntryAlwaysRejectsUnsafeIdentity(t *testing.T) {
	raw := canonicalWireDocument()
	for _, test := range []struct {
		name   string
		change func(*wireEntry)
	}{
		{"ID control", func(entry *wireEntry) { entry.ID = "document\n1" }},
		{"name control", func(entry *wireEntry) { entry.Name = "paper\t.pdf" }},
		{"path control", func(entry *wireEntry) { entry.Path = "Document/Documents/paper\x1b.pdf" }},
		{"parent control", func(entry *wireEntry) { entry.ParentFolderID = "folder\r1" }},
		{"display metadata control", func(entry *wireEntry) { entry.Size = "42\n" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := raw
			test.change(&changed)
			if _, err := decodeEntry(changed, false); err == nil {
				t.Fatal("unsafe entry was accepted in normal mode")
			}
		})
	}
}
