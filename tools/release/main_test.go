package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeVersion(t *testing.T) {
	for input, want := range map[string]string{
		"1.2.3-rc.1": "1.2.3-rc.1",
		"v1.2.3":     "1.2.3",
		"1.0.0+go":   "1.0.0+go",
	} {
		got, err := normalizeVersion(input)
		if err != nil || got != want {
			t.Fatalf("normalizeVersion(%q) = %q, %v", input, got, err)
		}
	}
	for _, input := range []string{"", "1", "01.2.3", "1.2.3-01", "1.2.3/../../bad", "v1.2"} {
		if _, err := normalizeVersion(input); err == nil {
			t.Fatalf("invalid version %q accepted", input)
		}
	}
}

func TestArchiveIsDeterministicAndNormalized(t *testing.T) {
	directory := t.TempDir()
	entries := []archiveEntry{
		{Name: "docs/readme.txt", Mode: 0o644, Data: []byte("read me\n")},
		{Name: "dp", Mode: 0o755, Data: []byte("binary")},
	}
	first := filepath.Join(directory, "first.tar.gz")
	second := filepath.Join(directory, "second.tar.gz")
	if err := writeArchive(first, "dpwire-test", append([]archiveEntry(nil), entries...)); err != nil {
		t.Fatal(err)
	}
	if err := writeArchive(second, "dpwire-test", append([]archiveEntry(nil), entries...)); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
	if !bytes.Equal(a, b) {
		t.Fatal("archives differ")
	}

	zipper, err := gzip.NewReader(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(zipper)
	seen := map[string]string{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !header.ModTime.Equal(time.Unix(0, 0)) || header.Uid != 0 || header.Gid != 0 {
			t.Fatalf("non-normalized header: %+v", header)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		seen[header.Name] = string(data)
	}
	if seen["dpwire-test/dp"] != "binary" || seen["dpwire-test/docs/readme.txt"] != "read me\n" {
		t.Fatalf("archive contents = %#v", seen)
	}
}

func TestChecksumsAndDirectoryComparison(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	for _, directory := range []string{first, second} {
		if err := os.WriteFile(filepath.Join(directory, "a"), []byte("same"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := compareDirectories(first, second); err != nil {
		t.Fatal(err)
	}
	if err := writeChecksums(first); err != nil {
		t.Fatal(err)
	}
	checksum, err := os.ReadFile(filepath.Join(first, "SHA256SUMS"))
	if err != nil || !bytes.Contains(checksum, []byte("  a\n")) {
		t.Fatalf("checksum = %q, %v", checksum, err)
	}
	if err := os.WriteFile(filepath.Join(second, "a"), []byte("different"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := compareDirectories(first, second); err == nil {
		t.Fatal("different directories compare equal")
	}
}

func TestCommandAndBinaryArchiveEntries(t *testing.T) {
	output, err := commandOutput(context.Background(), "go", "version")
	if err != nil || output == "" {
		t.Fatalf("go version = %q, %v", output, err)
	}
	if _, err := commandOutput(context.Background(), "go", "definitely-not-a-command"); err == nil {
		t.Fatal("failed command succeeded")
	}
	// The result depends on whether the caller is testing a clean checkout; both
	// paths are valid, but the command and status interpretation must not panic.
	_ = requireCleanWorktree(context.Background())

	directory := t.TempDir()
	binary := filepath.Join(directory, "dp")
	document := filepath.Join(directory, "README")
	if err := os.WriteFile(binary, []byte("program"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(document, []byte("documentation"), 0o644); err != nil {
		t.Fatal(err)
	}
	previous := distributionFiles
	distributionFiles = []string{document}
	t.Cleanup(func() { distributionFiles = previous })
	entries, err := binaryArchiveEntries(binary)
	if err != nil || len(entries) != 2 || entries[0].Name != "dp" || string(entries[0].Data) != "program" {
		t.Fatalf("binary entries = %#v, %v", entries, err)
	}
}

func TestWritersRefuseUnsafePathsAndOverwrite(t *testing.T) {
	directory := t.TempDir()
	archive := filepath.Join(directory, "unsafe.tar.gz")
	if err := writeArchive(archive, "root", []archiveEntry{{Name: "../secret", Mode: 0o644, Data: []byte("x")}}); err == nil {
		t.Fatal("unsafe archive path accepted")
	}
	filename := filepath.Join(directory, "new")
	if err := writeNew(filename, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(filename, []byte("second"), 0o600); err == nil {
		t.Fatal("existing file overwritten")
	}
}
