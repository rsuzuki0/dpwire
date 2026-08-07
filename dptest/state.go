// Package dptest provides a stateful, TLS-enabled protocol simulator for tests.
// It is not a production device server.
package dptest

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Document is the simulator's protocol-facing document metadata.
type Document struct {
	ID           string `json:"entry_id"`
	Name         string `json:"entry_name"`
	Path         string `json:"entry_path"`
	Type         string `json:"entry_type"`
	Created      string `json:"created_date"`
	Modified     string `json:"modified_date"`
	MIMEType     string `json:"mime_type"`
	Size         string `json:"file_size"`
	DocumentType string `json:"document_type"`
	ParentID     string `json:"parent_folder_id"`
	IsNew        string `json:"is_new"`
	Source       string `json:"document_source"`
	ExternalID   string `json:"ext_id"`
	FileHash     string `json:"file_hash"`
	Revision     string `json:"file_revision"`
}

// Fault describes one deterministic injected HTTP failure.
type Fault struct {
	Status int
	Body   string
	Once   bool
}

// State stores mutable simulator data. All methods are safe for concurrent use.
type State struct {
	mu                    sync.RWMutex
	model                 string
	firmware              string
	nextID                uint64
	documents             map[string]Document
	folders               map[string]Document
	contents              map[string][]byte
	faults                map[string]Fault
	clients               map[string]*rsa.PublicKey
	nonces                map[string]string
	sessions              map[string]bool
	requireAuth           bool
	rejectHash            bool
	lastFolderDeleteForce *bool
}

// NewState constructs an empty simulated device.
func NewState(model, firmware string) *State {
	return &State{
		model: model, firmware: firmware, nextID: 1,
		documents: make(map[string]Document), folders: make(map[string]Document), contents: make(map[string][]byte), faults: make(map[string]Fault),
		clients: make(map[string]*rsa.PublicKey), nonces: make(map[string]string), sessions: make(map[string]bool),
	}
}

// AddFolder inserts a deterministic folder and returns its metadata.
func (s *State) AddFolder(path, name, parent string, at time.Time) Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addFolderLocked(path, name, parent, at)
}

func (s *State) addFolderLocked(path, name, parent string, at time.Time) Document {
	id := fmt.Sprintf("folder-%06d", s.nextID)
	s.nextID++
	folder := Document{ID: id, Name: name, Path: path, Type: "folder", Created: at.UTC().Format(time.RFC3339), ParentID: parent, IsNew: "false", ExternalID: id}
	s.folders[id] = folder
	return folder
}

// Device reports the configured model and firmware.
func (s *State) Device() (model, firmware string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model, s.firmware
}

// AddDocument inserts a deterministic document and returns its metadata.
func (s *State) AddDocument(path, name, parent string, content []byte, at time.Time) Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("doc-%06d", s.nextID)
	s.nextID++
	sum := sha256.Sum256(content)
	stamp := at.UTC().Format(time.RFC3339)
	doc := Document{
		ID: id, Name: name, Path: path, Type: "document", Created: stamp,
		Modified: stamp, MIMEType: "application/pdf", Size: fmt.Sprint(len(content)),
		DocumentType: "normal", ParentID: parent, IsNew: "false",
		Source: "dp-sim", ExternalID: id, FileHash: hex.EncodeToString(sum[:]), Revision: "1",
	}
	s.documents[id] = doc
	s.contents[id] = append([]byte(nil), content...)
	return doc
}

// RequireAuthentication controls whether protected emulator endpoints require
// a valid Credentials session. It is opt-in so unauthenticated fixtures remain usable.
func (s *State) RequireAuthentication(required bool) {
	s.mu.Lock()
	s.requireAuth = required
	s.mu.Unlock()
}

// RejectUploadFileHash makes uploads fail if the optional file_hash query is
// present. Polaris does not document its digest algorithm, so this mode guards
// clients that intentionally omit the parameter.
func (s *State) RejectUploadFileHash(reject bool) {
	s.mu.Lock()
	s.rejectHash = reject
	s.mu.Unlock()
}

func (s *State) uploadFileHashRejected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rejectHash
}

// RegisterClient adds a pre-paired public key. Registration uses the separate
// registration emulator.
func (s *State) RegisterClient(clientID string, key *rsa.PublicKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if clientID != "" && key != nil {
		copy := &rsa.PublicKey{N: new(big.Int).Set(key.N), E: key.E}
		s.clients[clientID] = copy
	}
}

func (s *State) authenticationRequired() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.requireAuth
}

func (s *State) clientKey(clientID string) (*rsa.PublicKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.clients[clientID]
	return key, ok
}

func (s *State) setNonce(clientID, nonce string) {
	s.mu.Lock()
	s.nonces[clientID] = nonce
	s.mu.Unlock()
}

func (s *State) takeNonce(clientID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nonce, ok := s.nonces[clientID]
	delete(s.nonces, clientID)
	return nonce, ok
}

func (s *State) addSession(token string) {
	s.mu.Lock()
	s.sessions[token] = true
	s.mu.Unlock()
}

func (s *State) hasSession(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[token]
}

func (s *State) document(id string) (Document, []byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	document, ok := s.documents[id]
	return document, append([]byte(nil), s.contents[id]...), ok
}

func (s *State) documentByPath(path string) (Document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.EqualFold(path, "Document") {
		return rootDocument(), true
	}
	for _, document := range s.documents {
		if strings.EqualFold(document.Path, path) {
			return document, true
		}
	}
	for _, folder := range s.folders {
		if strings.EqualFold(folder.Path, path) {
			return folder, true
		}
	}
	return Document{}, false
}

func (s *State) folderEntries(parentID string) []Document {
	documents := s.Documents()
	entries := make([]Document, 0, len(documents))
	s.mu.RLock()
	for _, folder := range s.folders {
		if folder.ParentID == parentID {
			entries = append(entries, folder)
		}
	}
	s.mu.RUnlock()
	for _, document := range documents {
		if document.ParentID == parentID {
			entries = append(entries, document)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

func (s *State) entry(id string) (Document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id == "root" {
		return rootDocument(), true
	}
	if document, ok := s.documents[id]; ok {
		return document, true
	}
	folder, ok := s.folders[id]
	return folder, ok
}

func rootDocument() Document {
	return Document{ID: "root", Name: ".", Path: "Document", Type: "folder", ExternalID: "root"}
}

func (s *State) createFolder(parentID, name string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	parentPath, ok := s.folderPathLocked(parentID)
	if !ok {
		return Document{}, errors.New("parent not found")
	}
	path := parentPath + "/" + name
	if s.pathExistsLocked(path) {
		return Document{}, errors.New("duplicate")
	}
	return s.addFolderLocked(path, name, parentID, time.Now()), nil
}

func (s *State) createDocument(parentID, name string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	parentPath, ok := s.folderPathLocked(parentID)
	if !ok {
		return Document{}, errors.New("parent not found")
	}
	path := parentPath + "/" + name
	if s.pathExistsLocked(path) {
		return Document{}, errors.New("duplicate")
	}
	id := fmt.Sprintf("doc-%06d", s.nextID)
	s.nextID++
	stamp := time.Now().UTC().Format(time.RFC3339)
	document := Document{ID: id, Name: name, Path: path, Type: "document", Created: stamp, Modified: stamp,
		MIMEType: "application/pdf", Size: "0", DocumentType: "normal", ParentID: parentID,
		IsNew: "false", Source: "dp-sim", ExternalID: id, Revision: "0"}
	s.documents[id] = document
	s.contents[id] = nil
	return document, nil
}

func (s *State) uploadDocument(id string, content []byte, fileHash, targetRevision string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	document, ok := s.documents[id]
	if !ok {
		return Document{}, errors.New("not found")
	}
	if targetRevision != "" && targetRevision != document.Revision {
		return Document{}, errors.New("conflict")
	}
	revision, _ := strconv.Atoi(document.Revision)
	document.Revision = strconv.Itoa(revision + 1)
	document.Size = strconv.Itoa(len(content))
	document.FileHash = fileHash
	document.Modified = time.Now().UTC().Format(time.RFC3339)
	s.documents[id] = document
	s.contents[id] = append([]byte(nil), content...)
	return document, nil
}

func (s *State) updateEntry(id, parentID, name string, folder bool) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	collection := s.documents
	if folder {
		collection = s.folders
	}
	entry, ok := collection[id]
	if !ok {
		return Document{}, errors.New("not found")
	}
	oldPath := entry.Path
	if parentID != "" {
		if _, ok := s.folderPathLocked(parentID); !ok {
			return Document{}, errors.New("parent not found")
		}
		entry.ParentID = parentID
	}
	if name != "" {
		entry.Name = name
	}
	parentPath, _ := s.folderPathLocked(entry.ParentID)
	entry.Path = parentPath + "/" + entry.Name
	if entry.Path != oldPath && s.pathExistsExceptLocked(entry.Path, id) {
		return Document{}, errors.New("duplicate")
	}
	collection[id] = entry
	if folder && entry.Path != oldPath {
		prefix := oldPath + "/"
		for childID, child := range s.folders {
			if strings.HasPrefix(child.Path, prefix) {
				child.Path = entry.Path + strings.TrimPrefix(child.Path, oldPath)
				s.folders[childID] = child
			}
		}
		for childID, child := range s.documents {
			if strings.HasPrefix(child.Path, prefix) {
				child.Path = entry.Path + strings.TrimPrefix(child.Path, oldPath)
				s.documents[childID] = child
			}
		}
	}
	return entry, nil
}

func (s *State) copyDocument(id, parentID, name string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, ok := s.documents[id]
	if !ok {
		return Document{}, errors.New("not found")
	}
	parentPath, ok := s.folderPathLocked(parentID)
	if !ok {
		return Document{}, errors.New("parent not found")
	}
	if name == "" {
		name = source.Name
	}
	path := parentPath + "/" + name
	if s.pathExistsLocked(path) {
		return Document{}, errors.New("duplicate")
	}
	newID := fmt.Sprintf("doc-%06d", s.nextID)
	s.nextID++
	copy := source
	copy.ID, copy.ExternalID, copy.Name, copy.Path, copy.ParentID = newID, newID, name, path, parentID
	copy.Created = time.Now().UTC().Format(time.RFC3339)
	s.documents[newID] = copy
	s.contents[newID] = append([]byte(nil), s.contents[id]...)
	return copy, nil
}

func (s *State) deleteDocument(id, targetRevision string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	document, ok := s.documents[id]
	if !ok {
		return errors.New("not found")
	}
	if targetRevision != "" && targetRevision != document.Revision {
		return errors.New("conflict")
	}
	delete(s.documents, id)
	delete(s.contents, id)
	return nil
}

func (s *State) deleteFolder(id string, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	recordedForce := force
	s.lastFolderDeleteForce = &recordedForce
	folder, ok := s.folders[id]
	if !ok {
		return errors.New("not found")
	}
	prefix := folder.Path + "/"
	for _, child := range s.folders {
		if child.ParentID == id && !force {
			return errors.New("not empty")
		}
	}
	for _, child := range s.documents {
		if child.ParentID == id && !force {
			return errors.New("not empty")
		}
	}
	if force {
		for childID, child := range s.folders {
			if strings.HasPrefix(child.Path, prefix) {
				delete(s.folders, childID)
			}
		}
		for childID, child := range s.documents {
			if strings.HasPrefix(child.Path, prefix) {
				delete(s.documents, childID)
				delete(s.contents, childID)
			}
		}
	}
	delete(s.folders, id)
	return nil
}

// LastFolderDeleteForced reports the force flag received by the most recent
// folder deletion request.
func (s *State) LastFolderDeleteForced() (bool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastFolderDeleteForce == nil {
		return false, false
	}
	return *s.lastFolderDeleteForce, true
}

func (s *State) folderPathLocked(id string) (string, bool) {
	if id == "root" {
		return "Document", true
	}
	folder, ok := s.folders[id]
	return folder.Path, ok
}

func (s *State) pathExistsLocked(path string) bool { return s.pathExistsExceptLocked(path, "") }

func (s *State) pathExistsExceptLocked(path, exceptID string) bool {
	for id, document := range s.documents {
		if id != exceptID && document.Path == path {
			return true
		}
	}
	for id, folder := range s.folders {
		if id != exceptID && folder.Path == path {
			return true
		}
	}
	return false
}

// Documents returns a path-sorted snapshot.
func (s *State) Documents() []Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	docs := make([]Document, 0, len(s.documents))
	for _, doc := range s.documents {
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs
}

// InjectFault installs a fault for a key such as "GET /documents2".
func (s *State) InjectFault(key string, fault Fault) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults[key] = fault
}

func (s *State) takeFault(key string) (Fault, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fault, ok := s.faults[key]
	matchedKey := key
	if !ok {
		for pattern, candidate := range s.faults {
			if strings.Contains(pattern, "*") {
				matched, _ := path.Match(pattern, key)
				if matched {
					fault, ok, matchedKey = candidate, true, pattern
					break
				}
			}
		}
	}
	if ok && fault.Once {
		delete(s.faults, matchedKey)
	}
	return fault, ok
}
