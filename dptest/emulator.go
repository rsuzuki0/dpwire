package dptest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"
)

// Simulator is an in-process HTTPS Digital Paper protocol simulator.
type Simulator struct {
	State  *State
	server *httptest.Server
}

// Start starts a loopback TLS simulator.
func Start(state *State) *Simulator {
	if state == nil {
		state = NewState("DP-SIM", "0.0-p0")
	}
	sim := &Simulator{State: state}
	sim.server = httptest.NewTLSServer(http.HandlerFunc(sim.serveHTTP))
	return sim
}

// URL returns the simulator base URL.
func (s *Simulator) URL() string { return s.server.URL }

// Client returns an HTTP client that validates and trusts this simulator's
// certificate only. It never enables InsecureSkipVerify.
func (s *Simulator) Client() *http.Client {
	pool := x509.NewCertPool()
	pool.AddCert(s.server.Certificate())
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    pool,
		}},
	}
}

// DeviceCAPEM returns the simulator's ephemeral certificate as PEM.
func (s *Simulator) DeviceCAPEM() []byte {
	cert := s.server.Certificate()
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

// CertificateSHA256 returns the TLS certificate fingerprint.
func (s *Simulator) CertificateSHA256() string {
	sum := sha256.Sum256(s.server.Certificate().Raw)
	return hex.EncodeToString(sum[:])
}

// Close stops the simulator.
func (s *Simulator) Close() { s.server.Close() }

func (s *Simulator) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if fault, ok := s.State.takeFault(r.Method + " " + r.URL.Path); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(fault.Status)
		_, _ = w.Write([]byte(fault.Body))
		return
	}

	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/auth/nonce/"):
		s.serveNonce(w, strings.TrimPrefix(r.URL.Path, "/auth/nonce/"))
		return
	case r.Method == http.MethodPut && r.URL.Path == "/auth":
		s.serveAuth(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/register"):
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error_code": "P0_PAIRING_RESERVED", "message": "registration is reserved for phase P3",
		})
		return
	}
	if s.State.authenticationRequired() && !s.authenticated(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error_code": "AUTH_REQUIRED", "message": "valid Credentials cookie required"})
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/ping":
		writeJSON(w, http.StatusOK, map[string]string{"value": "pong"})
	case r.Method == http.MethodGet && r.URL.Path == "/api_version":
		writeJSON(w, http.StatusOK, map[string]string{"value": "0.6.0"})
	case r.Method == http.MethodGet && r.URL.Path == "/system/status/firmware_version":
		model, firmware := s.State.Device()
		writeJSON(w, http.StatusOK, map[string]string{"value": firmware, "model_name": model})
	case r.Method == http.MethodGet && r.URL.Path == "/system/status/battery":
		writeJSON(w, http.StatusOK, map[string]string{"level": "87", "icon_type": "level_bar_4", "status": "discharging", "health": "good", "plugged": "not_plugged", "pen": "64"})
	case r.Method == http.MethodGet && r.URL.Path == "/system/status/storage":
		writeJSON(w, http.StatusOK, map[string]string{"capacity": "11811160064", "available": "9663676416"})
	case r.Method == http.MethodGet && r.URL.Path == "/documents2":
		s.writeEntryList(w, r, s.State.Documents())
	case r.Method == http.MethodPost && r.URL.Path == "/folders2":
		var request struct {
			ParentID string `json:"parent_folder_id"`
			Name     string `json:"folder_name"`
		}
		if !decodeRequest(w, r, &request) {
			return
		}
		folder, err := s.State.createFolder(request.ParentID, request.Name)
		if err != nil {
			writeStateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"folder_id": folder.ID})
	case strings.HasPrefix(r.URL.Path, "/folders2/"):
		id := strings.TrimPrefix(r.URL.Path, "/folders2/")
		if r.Method == http.MethodGet {
			entry, ok := s.State.entry(id)
			if !ok || entry.Type != "folder" {
				writeJSON(w, http.StatusNotFound, map[string]string{"error_code": "40401", "message": "folder not found"})
				return
			}
			writeJSON(w, http.StatusOK, entry)
			return
		}
		if r.Method == http.MethodPut {
			var request struct {
				ParentID string `json:"parent_folder_id"`
				Name     string `json:"folder_name"`
			}
			if !decodeRequest(w, r, &request) {
				return
			}
			if _, err := s.State.updateEntry(id, request.ParentID, request.Name, true); err != nil {
				writeStateError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error_code": "METHOD_NOT_ALLOWED"})
	case r.Method == http.MethodPost && r.URL.Path == "/documents2":
		var request struct {
			ParentID string `json:"parent_folder_id"`
			Name     string `json:"file_name"`
		}
		if !decodeRequest(w, r, &request) {
			return
		}
		document, err := s.State.createDocument(request.ParentID, request.Name)
		if err != nil {
			writeStateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"document_id": document.ID})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/folders/") && strings.HasSuffix(r.URL.Path, "/entries2"):
		folderID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/folders/"), "/entries2")
		s.writeEntryList(w, r, s.State.folderEntries(folderID))
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/documents2/"):
		id := strings.TrimPrefix(r.URL.Path, "/documents2/")
		document, _, ok := s.State.document(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error_code": "40401", "message": "document not found"})
			return
		}
		writeJSON(w, http.StatusOK, document)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/documents2/"):
		id := strings.TrimPrefix(r.URL.Path, "/documents2/")
		var request struct {
			ParentID string `json:"parent_folder_id"`
			Name     string `json:"file_name"`
		}
		if !decodeRequest(w, r, &request) {
			return
		}
		if _, err := s.State.updateEntry(id, request.ParentID, request.Name, false); err != nil {
			writeStateError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/resolve/entry/path/"):
		path := strings.TrimPrefix(r.URL.Path, "/resolve/entry/path/")
		document, ok := s.State.documentByPath(path)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error_code": "40401", "message": "entry not found"})
			return
		}
		writeJSON(w, http.StatusOK, document)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/documents/") && strings.HasSuffix(r.URL.Path, "/file"):
		if accept := r.Header.Get("Accept"); accept != "" && !strings.Contains(accept, "application/pdf") && !strings.Contains(accept, "*/*") {
			writeJSON(w, http.StatusNotAcceptable, map[string]string{"error_code": "NOT_ACCEPTABLE"})
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/documents/"), "/file")
		document, content, ok := s.State.document(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error_code": "40401", "message": "document not found"})
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("ETag", `"`+document.Revision+":"+document.FileHash+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/documents/") && strings.HasSuffix(r.URL.Path, "/file"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/documents/"), "/file")
		if r.URL.Query().Has("file_hash") && s.State.uploadFileHashRejected() {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "40006", "message": "file_hash is not accepted by this profile"})
			return
		}
		content, ok := readMultipartPDF(w, r)
		if !ok {
			return
		}
		hash := sha256.Sum256(content)
		fileHash := r.URL.Query().Get("file_hash")
		if fileHash != "" && !strings.EqualFold(fileHash, hex.EncodeToString(hash[:])) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "40006", "message": "file hash mismatch"})
			return
		}
		document, err := s.State.uploadDocument(id, content, fileHash, r.URL.Query().Get("target_revision"))
		if err != nil {
			writeStateError(w, err)
			return
		}
		size := strconv.Itoa(len(content))
		writeJSON(w, http.StatusOK, map[string]string{"received_bytes": size, "current_bytes": size, "completed": "yes", "file_revision": document.Revision})
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/documents/") && strings.HasSuffix(r.URL.Path, "/copy"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/documents/"), "/copy")
		var request struct {
			ParentID string `json:"parent_folder_id"`
			Name     string `json:"file_name"`
		}
		if !decodeRequest(w, r, &request) {
			return
		}
		copy, err := s.State.copyDocument(id, request.ParentID, request.Name)
		if err != nil {
			writeStateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"document_id": copy.ID})
	case r.Method == http.MethodPut && r.URL.Path == "/viewer/controls/open2":
		var request struct {
			DocumentID string `json:"document_id"`
			Page       string `json:"page"`
		}
		if !decodeRequest(w, r, &request) {
			return
		}
		if entry, ok := s.State.entry(request.DocumentID); !ok || entry.Type != "document" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error_code": "40401", "message": "document not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error_code": "NOT_FOUND"})
	}
}

func decodeRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "40001", "message": "invalid request"})
		return false
	}
	return true
}

func readMultipartPDF(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	reader, err := r.MultipartReader()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "40010", "message": "multipart file required"})
		return nil, false
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "file" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "40010", "message": "file part required"})
		return nil, false
	}
	defer part.Close()
	content, err := io.ReadAll(io.LimitReader(part, (1<<30)+1))
	if err != nil || len(content) > 1<<30 {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error_code": "50701", "message": "file too large"})
		return nil, false
	}
	if next, err := reader.NextPart(); err != io.EOF || next != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "40011", "message": "unexpected multipart part"})
		return nil, false
	}
	return content, true
}

func writeStateError(w http.ResponseWriter, err error) {
	switch err.Error() {
	case "duplicate":
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "40007", "message": "duplicate name"})
	case "parent not found":
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "40012", "message": "parent folder not found"})
	case "conflict":
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "40017", "message": "revision conflict"})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error_code": "40401", "message": "entry not found"})
	}
}

func (s *Simulator) serveNonce(w http.ResponseWriter, clientID string) {
	if _, ok := s.State.clientKey(clientID); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error_code": "AUTH_UNKNOWN_CLIENT", "message": "unknown client"})
		return
	}
	value := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error_code": "RANDOM_FAILED"})
		return
	}
	nonce := base64.RawURLEncoding.EncodeToString(value)
	s.State.setNonce(clientID, nonce)
	writeJSON(w, http.StatusOK, map[string]string{"nonce": nonce})
}

func (s *Simulator) serveAuth(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ClientID    string `json:"client_id"`
		NonceSigned string `json:"nonce_signed"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "AUTH_BAD_REQUEST"})
		return
	}
	key, ok := s.State.clientKey(request.ClientID)
	nonce, nonceOK := s.State.takeNonce(request.ClientID)
	signature, err := base64.StdEncoding.DecodeString(request.NonceSigned)
	if !ok || !nonceOK || err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error_code": "AUTH_FAILED"})
		return
	}
	digest := sha256.Sum256([]byte(nonce))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error_code": "AUTH_FAILED"})
		return
	}
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, tokenBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error_code": "RANDOM_FAILED"})
		return
	}
	token := base64.StdEncoding.EncodeToString(tokenBytes)
	s.State.addSession(token)
	w.Header().Add("Set-Cookie", "Credentials="+token+"; Secure; Path=/")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Simulator) authenticated(r *http.Request) bool {
	for _, header := range r.Header.Values("Cookie") {
		for _, item := range strings.Split(header, ";") {
			name, value, ok := strings.Cut(strings.TrimSpace(item), "=")
			if ok && name == "Credentials" && s.State.hasSession(value) {
				return true
			}
		}
	}
	return false
}

func (s *Simulator) writeEntryList(w http.ResponseWriter, r *http.Request, entries []Document) {
	offset, err := queryInteger(r, "offset", 0)
	if err != nil || offset < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "40002"})
		return
	}
	limit, err := queryInteger(r, "limit", 100)
	if err != nil || limit < 1 || limit > 1000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "40002"})
		return
	}
	encoded, _ := json.Marshal(entries)
	hash := sha256.Sum256(encoded)
	end := offset + limit
	if offset > len(entries) {
		offset = len(entries)
	}
	if end > len(entries) {
		end = len(entries)
	}
	writeJSON(w, http.StatusOK, struct {
		Count int        `json:"count"`
		Hash  string     `json:"entry_list_hash"`
		Docs  []Document `json:"entry_list"`
	}{len(entries), hex.EncodeToString(hash[:]), entries[offset:end]})
}

func queryInteger(r *http.Request, key string, fallback int) (int, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New("invalid integer")
	}
	return parsed, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
