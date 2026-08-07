package profiles

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/rsuzuki0/dpwire"
	"github.com/rsuzuki0/dpwire/internal/atomicfile"
)

const (
	objectReferenceSchema  = 1
	minimumHexDigits       = 6
	maxObjectReferenceSize = 16 << 20
)

// ObjectReference is a profile-local, persistent reference to one device
// object. Number values are never reused. Hex is a shortened SHA-256 reference
// and therefore does not depend on the vendor's object-ID format.
type ObjectReference struct {
	Number   uint64
	Hex      string
	DeviceID string
	Type     dpwire.EntryType
}

// ObjectReferenceStore manages references for one device identity.
type ObjectReferenceStore struct {
	path     string
	lockPath string

	cacheMu    sync.Mutex
	cacheValid bool
	cacheInfo  os.FileInfo
	cacheState objectReferenceFile
}

type objectReferenceFile struct {
	SchemaVersion int                     `json:"schema_version"`
	Next          uint64                  `json:"next"`
	Objects       []objectReferenceRecord `json:"objects"`
}

type objectReferenceRecord struct {
	Number   uint64           `json:"number"`
	DeviceID string           `json:"device_id"`
	Type     dpwire.EntryType `json:"type"`
}

// ObjectReferences returns the reference store for profile's device identity.
func (m *Manager) ObjectReferences(profile dpwire.DeviceProfile) (*ObjectReferenceStore, error) {
	if profile.Address == "" || profile.ClientID == "" {
		return nil, errors.New("profiles: object references require a complete profile identity")
	}
	if err := m.ensureRoot(); err != nil {
		return nil, err
	}
	directory := filepath.Join(m.root, "object-references")
	identity := sha256.Sum256([]byte(profile.Address + "\x00" + profile.ClientID + "\x00" + profile.CertificateSHA256))
	name := hex.EncodeToString(identity[:16])
	return &ObjectReferenceStore{
		path: filepath.Join(directory, name+".json"), lockPath: filepath.Join(directory, name+".lock"),
	}, nil
}

// Assign returns references for entries, assigning persistent numbers to
// previously unseen device objects.
func (s *ObjectReferenceStore) Assign(entries []dpwire.Entry) (map[string]ObjectReference, error) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	var result map[string]ObjectReference
	err := s.withLock(func() error {
		state, err := s.loadCached()
		if err != nil {
			return err
		}
		byID := make(map[string]int, len(state.Objects))
		for index := range state.Objects {
			byID[state.Objects[index].DeviceID] = index
		}
		changed := false
		for _, entry := range entries {
			if entry.ID == "" || (entry.Type != dpwire.EntryDocument && entry.Type != dpwire.EntryFolder) {
				return errors.New("profiles: invalid object reference entry")
			}
			if index, ok := byID[entry.ID]; ok {
				if state.Objects[index].Type != entry.Type {
					state.Objects[index].Type = entry.Type
					changed = true
				}
				continue
			}
			if state.Next == ^uint64(0) {
				return errors.New("profiles: object reference number space is exhausted")
			}
			state.Objects = append(state.Objects, objectReferenceRecord{Number: state.Next, DeviceID: entry.ID, Type: entry.Type})
			byID[entry.ID] = len(state.Objects) - 1
			state.Next++
			changed = true
		}
		if changed {
			if err := s.save(state); err != nil {
				s.cacheValid = false
				return err
			}
		}
		s.remember(state)
		all := references(state.Objects)
		result = make(map[string]ObjectReference, len(entries))
		for _, entry := range entries {
			result[entry.ID] = all[entry.ID]
		}
		return nil
	})
	return result, err
}

// Candidates resolves a decimal number or 0x-prefixed hexadecimal reference.
// Hexadecimal prefixes may return multiple candidates; the caller must apply
// type and existence checks before requiring uniqueness.
func (s *ObjectReferenceStore) Candidates(selector string) ([]ObjectReference, error) {
	state, err := s.load()
	if err != nil {
		return nil, err
	}
	all := references(state.Objects)
	if strings.HasPrefix(selector, "0x") || strings.HasPrefix(selector, "0X") {
		prefix := strings.ToLower(selector[2:])
		if len(prefix) < 4 || len(prefix) > sha256.Size*2 {
			return nil, errors.New("profiles: hexadecimal object ID must contain 4 to 64 digits")
		}
		for _, digit := range prefix {
			if !strings.ContainsRune("0123456789abcdef", digit) {
				return nil, errors.New("profiles: hexadecimal object ID contains a non-hexadecimal digit")
			}
		}
		var matches []ObjectReference
		for _, reference := range all {
			if strings.HasPrefix(reference.Hex[2:], prefix) {
				matches = append(matches, reference)
			}
		}
		sort.Slice(matches, func(i, j int) bool { return matches[i].Number < matches[j].Number })
		return matches, nil
	}
	number, err := strconv.ParseUint(selector, 10, 64)
	if err != nil {
		return nil, errors.New("profiles: object ID must be a nonnegative integer or start with 0x")
	}
	for _, reference := range all {
		if reference.Number == number {
			return []ObjectReference{reference}, nil
		}
	}
	return []ObjectReference{}, nil
}

func references(records []objectReferenceRecord) map[string]ObjectReference {
	type digestRecord struct {
		record objectReferenceRecord
		digest string
	}
	ordered := make([]digestRecord, len(records))
	for index, record := range records {
		sum := sha256.Sum256([]byte(record.DeviceID))
		ordered[index] = digestRecord{record: record, digest: hex.EncodeToString(sum[:])}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].digest < ordered[j].digest })
	result := make(map[string]ObjectReference, len(records))
	for index, item := range ordered {
		digits := minimumHexDigits
		if index > 0 {
			digits = max(digits, commonPrefixLength(item.digest, ordered[index-1].digest)+1)
		}
		if index+1 < len(ordered) {
			digits = max(digits, commonPrefixLength(item.digest, ordered[index+1].digest)+1)
		}
		if digits > sha256.Size*2 {
			digits = sha256.Size * 2
		}
		result[item.record.DeviceID] = ObjectReference{
			Number: item.record.Number, Hex: "0x" + item.digest[:digits], DeviceID: item.record.DeviceID, Type: item.record.Type,
		}
	}
	return result
}

func commonPrefixLength(left, right string) int {
	limit := min(len(left), len(right))
	for index := 0; index < limit; index++ {
		if left[index] != right[index] {
			return index
		}
	}
	return limit
}

func (s *ObjectReferenceStore) loadCached() (objectReferenceFile, error) {
	info, err := os.Stat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		if s.cacheValid && s.cacheInfo == nil {
			return cloneObjectReferenceFile(s.cacheState), nil
		}
		return objectReferenceFile{SchemaVersion: objectReferenceSchema}, nil
	}
	if err != nil {
		return objectReferenceFile{}, err
	}
	if s.cacheValid && s.cacheInfo != nil && os.SameFile(s.cacheInfo, info) && s.cacheInfo.Size() == info.Size() && s.cacheInfo.ModTime() == info.ModTime() {
		return cloneObjectReferenceFile(s.cacheState), nil
	}
	return s.load()
}

func (s *ObjectReferenceStore) remember(state objectReferenceFile) {
	info, err := os.Stat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		info = nil
	} else if err != nil {
		s.cacheValid = false
		return
	}
	s.cacheState = cloneObjectReferenceFile(state)
	s.cacheInfo = info
	s.cacheValid = true
}

func cloneObjectReferenceFile(state objectReferenceFile) objectReferenceFile {
	copy := state
	copy.Objects = append([]objectReferenceRecord(nil), state.Objects...)
	return copy
}

func (s *ObjectReferenceStore) load() (objectReferenceFile, error) {
	state := objectReferenceFile{SchemaVersion: objectReferenceSchema}
	info, err := os.Stat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return objectReferenceFile{}, err
	}
	if info.Size() > maxObjectReferenceSize {
		return objectReferenceFile{}, errors.New("profiles: object reference file exceeds size limit")
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return objectReferenceFile{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return objectReferenceFile{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return objectReferenceFile{}, errors.New("profiles: object reference file contains trailing data")
	}
	if err := validateObjectReferenceFile(state); err != nil {
		return objectReferenceFile{}, err
	}
	return state, nil
}

func validateObjectReferenceFile(state objectReferenceFile) error {
	if state.SchemaVersion != objectReferenceSchema {
		return errors.New("profiles: unsupported object reference schema")
	}
	numbers := make(map[uint64]struct{}, len(state.Objects))
	ids := make(map[string]struct{}, len(state.Objects))
	for _, record := range state.Objects {
		if record.DeviceID == "" || strings.ContainsAny(record.DeviceID, "\r\n\x00") {
			return errors.New("profiles: invalid object reference device ID")
		}
		if record.Type != dpwire.EntryDocument && record.Type != dpwire.EntryFolder {
			return errors.New("profiles: invalid object reference type")
		}
		if record.Number >= state.Next {
			return errors.New("profiles: invalid object reference sequence")
		}
		if _, ok := numbers[record.Number]; ok {
			return errors.New("profiles: duplicate object reference number")
		}
		if _, ok := ids[record.DeviceID]; ok {
			return errors.New("profiles: duplicate object reference device ID")
		}
		numbers[record.Number] = struct{}{}
		ids[record.DeviceID] = struct{}{}
	}
	return nil
}

func (s *ObjectReferenceStore) save(state objectReferenceFile) error {
	sort.Slice(state.Objects, func(i, j int) bool { return state.Objects[i].Number < state.Objects[j].Number })
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Replace(s.path, append(encoded, '\n'), 0o600)
}

func (s *ObjectReferenceStore) withLock(operation func() error) error {
	file, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := os.Chmod(s.lockPath, 0o600); err != nil {
		return err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("profiles: lock object references: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return operation()
}
