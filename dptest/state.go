// Package dptest provides a stateful, TLS-enabled protocol simulator for tests.
// It is not a production device server.
package dptest

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
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
	mu          sync.RWMutex
	model       string
	firmware    string
	nextID      uint64
	documents   map[string]Document
	contents    map[string][]byte
	faults      map[string]Fault
	clients     map[string]*rsa.PublicKey
	nonces      map[string]string
	sessions    map[string]bool
	requireAuth bool
}

// NewState constructs an empty simulated device.
func NewState(model, firmware string) *State {
	return &State{
		model: model, firmware: firmware, nextID: 1,
		documents: make(map[string]Document), contents: make(map[string][]byte), faults: make(map[string]Fault),
		clients: make(map[string]*rsa.PublicKey), nonces: make(map[string]string), sessions: make(map[string]bool),
	}
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
// a valid Credentials session. It is opt-in so P0 protocol fixtures remain usable.
func (s *State) RequireAuthentication(required bool) {
	s.mu.Lock()
	s.requireAuth = required
	s.mu.Unlock()
}

// RegisterClient adds a pre-paired public key. Registration itself remains a
// reserved P3 operation.
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
	for _, document := range s.documents {
		if document.Path == path {
			return document, true
		}
	}
	return Document{}, false
}

func (s *State) folderEntries(parentID string) []Document {
	documents := s.Documents()
	entries := documents[:0]
	for _, document := range documents {
		if document.ParentID == parentID {
			entries = append(entries, document)
		}
	}
	return entries
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
	if ok && fault.Once {
		delete(s.faults, key)
	}
	return fault, ok
}
