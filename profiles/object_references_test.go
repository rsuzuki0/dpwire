package profiles

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rsuzuki0/dpwire"
)

func TestOptimizedReferencePrefixesMatchNaiveDefinition(t *testing.T) {
	records := make([]objectReferenceRecord, 500)
	digests := make(map[string]string, len(records))
	for index := range records {
		id := fmt.Sprintf("object-%04d", index)
		records[index] = objectReferenceRecord{Number: uint64(index), DeviceID: id, Type: dpwire.EntryDocument}
		sum := sha256.Sum256([]byte(id))
		digests[id] = hex.EncodeToString(sum[:])
	}
	got := references(records)
	for _, record := range records {
		digits := minimumHexDigits
		for digits < sha256.Size*2 {
			prefix := digests[record.DeviceID][:digits]
			unique := true
			for otherID, digest := range digests {
				if otherID != record.DeviceID && strings.HasPrefix(digest, prefix) {
					unique = false
					break
				}
			}
			if unique {
				break
			}
			digits++
		}
		want := "0x" + digests[record.DeviceID][:digits]
		if got[record.DeviceID].Hex != want {
			t.Fatalf("reference %q = %q, want %q", record.DeviceID, got[record.DeviceID].Hex, want)
		}
	}
}

func TestObjectReferencesAreStableAndNeverReused(t *testing.T) {
	manager, err := New(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := manager.ObjectReferences(dpwire.DeviceProfile{
		Address: "https://device.example:8443", ClientID: "client", CertificateSHA256: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	first := []dpwire.Entry{{ID: "folder-one", Type: dpwire.EntryFolder}, {ID: "document-one", Type: dpwire.EntryDocument}}
	references, err := store.Assign(first)
	if err != nil {
		t.Fatal(err)
	}
	if references["folder-one"].Number != 0 || references["document-one"].Number != 1 {
		t.Fatalf("references = %#v", references)
	}
	if !strings.HasPrefix(references["folder-one"].Hex, "0x") || len(references["folder-one"].Hex) < 8 {
		t.Fatalf("hex reference = %q", references["folder-one"].Hex)
	}
	second, err := store.Assign([]dpwire.Entry{{ID: "document-one", Type: dpwire.EntryDocument}, {ID: "document-two", Type: dpwire.EntryDocument}})
	if err != nil {
		t.Fatal(err)
	}
	if second["document-one"].Number != 1 || second["document-two"].Number != 2 {
		t.Fatalf("second references = %#v", second)
	}
	byNumber, err := store.Candidates("1")
	if err != nil || len(byNumber) != 1 || byNumber[0].DeviceID != "document-one" {
		t.Fatalf("number candidates = %#v, %v", byNumber, err)
	}
	byHex, err := store.Candidates(references["folder-one"].Hex)
	if err != nil || len(byHex) != 1 || byHex[0].DeviceID != "folder-one" {
		t.Fatalf("hex candidates = %#v, %v", byHex, err)
	}
	oddPrefix, err := store.Candidates(references["folder-one"].Hex[:7])
	if err != nil || len(oddPrefix) != 1 || oddPrefix[0].DeviceID != "folder-one" {
		t.Fatalf("odd hexadecimal prefix candidates = %#v, %v", oddPrefix, err)
	}
	info, err := os.Stat(store.path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("reference file mode = %v, err = %v", info.Mode(), err)
	}
}

func TestObjectReferenceRejectsInvalidSelectors(t *testing.T) {
	manager, _ := New(filepath.Join(t.TempDir(), "config"))
	store, err := manager.ObjectReferences(dpwire.DeviceProfile{Address: "https://device.example", ClientID: "client"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Assign([]dpwire.Entry{{ID: "document", Type: dpwire.EntryDocument}}); err != nil {
		t.Fatal(err)
	}
	for _, selector := range []string{"", "-1", "xyz", "0x", "0x123", "0xzzzz"} {
		if _, err := store.Candidates(selector); err == nil {
			t.Fatalf("selector %q accepted", selector)
		}
	}
}

func TestObjectReferencesSerializeConcurrentAssignments(t *testing.T) {
	manager, err := New(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	profile := dpwire.DeviceProfile{Address: "https://device.example", ClientID: "client"}
	const count = 32
	var wait sync.WaitGroup
	errorsFound := make(chan error, count)
	for index := 0; index < count; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			store, storeErr := manager.ObjectReferences(profile)
			if storeErr != nil {
				errorsFound <- storeErr
				return
			}
			_, storeErr = store.Assign([]dpwire.Entry{{
				ID: fmt.Sprintf("document-%02d", index), Name: fmt.Sprintf("private name %02d.pdf", index), Type: dpwire.EntryDocument,
			}})
			if storeErr != nil {
				errorsFound <- storeErr
			}
		}(index)
	}
	wait.Wait()
	close(errorsFound)
	for assignmentErr := range errorsFound {
		t.Error(assignmentErr)
	}
	store, err := manager.ObjectReferences(profile)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[uint64]struct{}, count)
	for index := 0; index < count; index++ {
		matches, candidateErr := store.Candidates(fmt.Sprintf("%d", index))
		if candidateErr != nil || len(matches) != 1 {
			t.Fatalf("reference %d = %#v, %v", index, matches, candidateErr)
		}
		seen[matches[0].Number] = struct{}{}
	}
	if len(seen) != count {
		t.Fatalf("unique reference count = %d", len(seen))
	}
	raw, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private name") {
		t.Fatalf("reference store contains an entry name: %s", raw)
	}
}
