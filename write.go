package dpwire

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const maxUploadResponse = 1 << 20

// UploadResult describes one completed PDF body upload.
type UploadResult struct {
	ReceivedBytes int64
	CurrentBytes  int64
	Completed     bool
	Revision      string
	SHA256        string
}

// CreateFolder creates one child folder and verifies its metadata.
func (s *FoldersService) Create(ctx context.Context, parentID, name string) (Entry, error) {
	if err := validateID(parentID); err != nil {
		return Entry{}, err
	}
	name, err := validateEntryName(name, false)
	if err != nil {
		return Entry{}, err
	}
	var response struct {
		FolderID string `json:"folder_id"`
	}
	payload := map[string]string{"parent_folder_id": parentID, "folder_name": name}
	if err := s.client.wire.DoJSON(ctx, http.MethodPost, "/folders2", nil, payload, &response, true); err != nil {
		return Entry{}, publicError(err)
	}
	if err := validateID(response.FolderID); err != nil {
		return Entry{}, errors.New("dpwire: device returned an invalid folder ID")
	}
	entry, err := s.Get(ctx, response.FolderID)
	if err != nil {
		return Entry{ID: response.FolderID}, &PartialFailureError{Operation: "folder creation verification", EntryID: response.FolderID, Cause: err}
	}
	if entry.Name != name || entry.ParentFolderID != parentID {
		cause := &VerificationError{Field: "folder metadata", Expected: parentID + "/" + name, Actual: entry.ParentFolderID + "/" + entry.Name}
		return entry, &PartialFailureError{Operation: "folder creation verification", EntryID: response.FolderID, Cause: cause}
	}
	return entry, nil
}

// Get retrieves folder metadata by device ID.
func (s *FoldersService) Get(ctx context.Context, id string) (Entry, error) {
	if err := validateID(id); err != nil {
		return Entry{}, err
	}
	var raw wireEntry
	if err := s.client.wire.DoJSON(ctx, http.MethodGet, "/folders2/"+url.PathEscape(id), nil, nil, &raw, true); err != nil {
		return Entry{}, publicError(err)
	}
	return decodeEntry(raw)
}

// Update renames and/or moves a folder, then verifies the resulting metadata.
func (s *FoldersService) Update(ctx context.Context, id, parentID, newName string) (Entry, error) {
	if err := validateID(id); err != nil {
		return Entry{}, err
	}
	payload := make(map[string]string)
	if parentID != "" {
		if err := validateID(parentID); err != nil {
			return Entry{}, err
		}
		payload["parent_folder_id"] = parentID
	}
	if newName != "" {
		name, err := validateEntryName(newName, false)
		if err != nil {
			return Entry{}, err
		}
		payload["folder_name"] = name
	}
	if len(payload) == 0 {
		return Entry{}, errors.New("dpwire: folder update has no changes")
	}
	endpoint := "/folders2/" + url.PathEscape(id)
	if err := s.client.wire.DoJSON(ctx, http.MethodPut, endpoint, nil, payload, nil, true); err != nil {
		return Entry{}, publicError(err)
	}
	entry, err := s.Get(ctx, id)
	if err != nil {
		return Entry{}, err
	}
	if parentID != "" && entry.ParentFolderID != parentID {
		return Entry{}, &VerificationError{Field: "parent_folder_id", Expected: parentID, Actual: entry.ParentFolderID}
	}
	if newName != "" && entry.Name != norm.NFC.String(newName) {
		return Entry{}, &VerificationError{Field: "folder_name", Expected: norm.NFC.String(newName), Actual: entry.Name}
	}
	return entry, nil
}

// CreateMetadata creates an empty document entry. Prefer CreateAndUpload,
// which reports partial failure if the body upload does not complete.
func (s *DocumentsService) CreateMetadata(ctx context.Context, parentID, filename string) (Entry, error) {
	if err := validateID(parentID); err != nil {
		return Entry{}, err
	}
	filename, err := validateEntryName(filename, true)
	if err != nil {
		return Entry{}, err
	}
	var response struct {
		DocumentID string `json:"document_id"`
	}
	payload := map[string]string{"parent_folder_id": parentID, "file_name": filename}
	if err := s.client.wire.DoJSON(ctx, http.MethodPost, "/documents2", nil, payload, &response, true); err != nil {
		return Entry{}, publicError(err)
	}
	if err := validateID(response.DocumentID); err != nil {
		return Entry{}, errors.New("dpwire: device returned an invalid document ID")
	}
	entry, err := s.Get(ctx, response.DocumentID)
	if err != nil {
		return Entry{ID: response.DocumentID}, &PartialFailureError{Operation: "document creation verification", EntryID: response.DocumentID, Cause: err}
	}
	if entry.Name != filename || entry.ParentFolderID != parentID {
		cause := &VerificationError{Field: "document metadata", Expected: parentID + "/" + filename, Actual: entry.ParentFolderID + "/" + entry.Name}
		return entry, &PartialFailureError{Operation: "document creation verification", EntryID: response.DocumentID, Cause: cause}
	}
	return entry, nil
}

// CreateAndUpload creates metadata, streams one PDF body, and verifies size and
// revision. It never silently removes a partially created entry.
func (s *DocumentsService) CreateAndUpload(ctx context.Context, parentID, filename string, source io.ReadSeeker) (Entry, UploadResult, error) {
	prepared, err := preparePDF(source)
	if err != nil {
		return Entry{}, UploadResult{}, err
	}
	entry, err := s.CreateMetadata(ctx, parentID, filename)
	if err != nil {
		return entry, UploadResult{}, err
	}
	result, err := s.upload(ctx, entry.ID, filename, prepared, "")
	if err != nil {
		return entry, UploadResult{}, &PartialFailureError{Operation: "create-and-upload", EntryID: entry.ID, Cause: err}
	}
	verified, err := s.Get(ctx, entry.ID)
	if err != nil {
		return entry, result, &PartialFailureError{Operation: "post-upload verification", EntryID: entry.ID, Cause: err}
	}
	if verified.Size != strconv.FormatInt(prepared.size, 10) {
		cause := &VerificationError{Field: "file_size", Expected: strconv.FormatInt(prepared.size, 10), Actual: verified.Size}
		return verified, result, &PartialFailureError{Operation: "post-upload verification", EntryID: entry.ID, Cause: cause}
	}
	if result.Revision == "" || verified.Revision != result.Revision {
		cause := &VerificationError{Field: "file_revision", Expected: result.Revision, Actual: verified.Revision}
		return verified, result, &PartialFailureError{Operation: "post-upload verification", EntryID: entry.ID, Cause: cause}
	}
	return verified, result, nil
}

// Replace uploads a new PDF body only if targetRevision is still current.
func (s *DocumentsService) Replace(ctx context.Context, id, filename, targetRevision string, source io.ReadSeeker) (Entry, UploadResult, error) {
	if targetRevision == "" {
		return Entry{}, UploadResult{}, errors.New("dpwire: target revision is required for replacement")
	}
	prepared, err := preparePDF(source)
	if err != nil {
		return Entry{}, UploadResult{}, err
	}
	result, err := s.upload(ctx, id, filename, prepared, targetRevision)
	if err != nil {
		return Entry{}, UploadResult{}, err
	}
	entry, err := s.Get(ctx, id)
	if err != nil {
		return Entry{}, result, err
	}
	if entry.Size != strconv.FormatInt(prepared.size, 10) || entry.Revision != result.Revision {
		return entry, result, &VerificationError{Field: "replacement metadata", Expected: strconv.FormatInt(prepared.size, 10) + "/" + result.Revision, Actual: entry.Size + "/" + entry.Revision}
	}
	return entry, result, nil
}

// Update renames and/or moves a document and verifies the result.
func (s *DocumentsService) Update(ctx context.Context, id, parentID, newName string) (Entry, error) {
	if err := validateID(id); err != nil {
		return Entry{}, err
	}
	payload := make(map[string]string)
	if parentID != "" {
		if err := validateID(parentID); err != nil {
			return Entry{}, err
		}
		payload["parent_folder_id"] = parentID
	}
	if newName != "" {
		name, err := validateEntryName(newName, true)
		if err != nil {
			return Entry{}, err
		}
		payload["file_name"] = name
	}
	if len(payload) == 0 {
		return Entry{}, errors.New("dpwire: document update has no changes")
	}
	endpoint := "/documents2/" + url.PathEscape(id)
	if err := s.client.wire.DoJSON(ctx, http.MethodPut, endpoint, nil, payload, nil, true); err != nil {
		return Entry{}, publicError(err)
	}
	entry, err := s.Get(ctx, id)
	if err != nil {
		return Entry{}, err
	}
	if parentID != "" && entry.ParentFolderID != parentID {
		return Entry{}, &VerificationError{Field: "parent_folder_id", Expected: parentID, Actual: entry.ParentFolderID}
	}
	if newName != "" && entry.Name != norm.NFC.String(newName) {
		return Entry{}, &VerificationError{Field: "file_name", Expected: norm.NFC.String(newName), Actual: entry.Name}
	}
	return entry, nil
}

// Copy duplicates a document into a destination folder and verifies the copy.
func (s *DocumentsService) Copy(ctx context.Context, id, parentID, newName string) (Entry, error) {
	if err := validateID(id); err != nil {
		return Entry{}, err
	}
	if err := validateID(parentID); err != nil {
		return Entry{}, err
	}
	payload := map[string]string{"parent_folder_id": parentID}
	if newName != "" {
		name, err := validateEntryName(newName, true)
		if err != nil {
			return Entry{}, err
		}
		payload["file_name"] = name
	}
	var response struct {
		DocumentID string `json:"document_id"`
	}
	endpoint := "/documents/" + url.PathEscape(id) + "/copy"
	if err := s.client.wire.DoJSON(ctx, http.MethodPost, endpoint, nil, payload, &response, true); err != nil {
		return Entry{}, publicError(err)
	}
	if err := validateID(response.DocumentID); err != nil {
		return Entry{}, errors.New("dpwire: device returned an invalid copied document ID")
	}
	entry, err := s.Get(ctx, response.DocumentID)
	if err != nil {
		return Entry{ID: response.DocumentID}, &PartialFailureError{Operation: "document copy verification", EntryID: response.DocumentID, Cause: err}
	}
	if entry.ParentFolderID != parentID || (newName != "" && entry.Name != norm.NFC.String(newName)) {
		cause := &VerificationError{Field: "copied document metadata", Expected: parentID + "/" + newName, Actual: entry.ParentFolderID + "/" + entry.Name}
		return entry, &PartialFailureError{Operation: "document copy verification", EntryID: response.DocumentID, Cause: cause}
	}
	return entry, nil
}

// Open asks the device viewer to display a document. Page zero means the
// device-selected page; positive pages are one-based.
func (s *DocumentsService) Open(ctx context.Context, id string, page int) error {
	if err := validateID(id); err != nil {
		return err
	}
	if page < 0 {
		return errors.New("dpwire: page must not be negative")
	}
	payload := map[string]string{"document_id": id}
	if page > 0 {
		payload["page"] = strconv.Itoa(page)
	}
	return publicError(s.client.wire.DoJSON(ctx, http.MethodPut, "/viewer/controls/open2", nil, payload, nil, true))
}

type preparedPDF struct {
	reader io.Reader
	size   int64
	hash   string
}

func preparePDF(source io.ReadSeeker) (preparedPDF, error) {
	if source == nil {
		return preparedPDF{}, errors.New("dpwire: nil PDF source")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return preparedPDF{}, fmt.Errorf("dpwire: seek PDF: %w", err)
	}
	var magic [5]byte
	if _, err := io.ReadFull(source, magic[:]); err != nil || string(magic[:]) != "%PDF-" {
		return preparedPDF{}, errors.New("dpwire: source is not a PDF")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return preparedPDF{}, err
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, io.LimitReader(source, maxDocumentSize+1))
	if err != nil {
		return preparedPDF{}, err
	}
	if size > maxDocumentSize {
		return preparedPDF{}, fmt.Errorf("dpwire: PDF exceeds %d-byte limit", maxDocumentSize)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return preparedPDF{}, err
	}
	return preparedPDF{reader: io.LimitReader(source, size), size: size, hash: hex.EncodeToString(hasher.Sum(nil))}, nil
}

func (s *DocumentsService) upload(ctx context.Context, id, filename string, prepared preparedPDF, targetRevision string) (UploadResult, error) {
	if err := validateID(id); err != nil {
		return UploadResult{}, err
	}
	filename, err := validateEntryName(filename, true)
	if err != nil {
		return UploadResult{}, err
	}
	body, contentType, contentLength, err := multipartBody(filename, prepared.reader, prepared.size)
	if err != nil {
		return UploadResult{}, err
	}
	// Polaris defines file_hash as optional but does not identify its digest
	// algorithm. Neither preserved reference client sends it. Keep the SHA-256
	// in UploadResult for local verification without asserting an undocumented
	// wire format.
	query := make(url.Values)
	if targetRevision != "" {
		query.Set("target_revision", targetRevision)
	}
	endpoint := "/documents/" + url.PathEscape(id) + "/file"
	response, err := s.client.wire.DoMedia(ctx, http.MethodPut, endpoint, query, body, true, contentType, "application/json", contentLength)
	if err != nil {
		return UploadResult{}, publicError(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxUploadResponse+1))
	if err != nil {
		return UploadResult{}, err
	}
	if len(raw) > maxUploadResponse {
		return UploadResult{}, errors.New("dpwire: upload response exceeds limit")
	}
	var wire struct {
		Received string `json:"received_bytes"`
		Current  string `json:"current_bytes"`
		Complete string `json:"completed"`
		Revision string `json:"file_revision"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return UploadResult{}, fmt.Errorf("dpwire: decode upload response: %w", err)
	}
	received, err := strconv.ParseInt(wire.Received, 10, 64)
	if err != nil {
		return UploadResult{}, errors.New("dpwire: invalid received_bytes")
	}
	current, err := strconv.ParseInt(wire.Current, 10, 64)
	if err != nil {
		return UploadResult{}, errors.New("dpwire: invalid current_bytes")
	}
	result := UploadResult{ReceivedBytes: received, CurrentBytes: current, Completed: wire.Complete == "yes", Revision: wire.Revision, SHA256: prepared.hash}
	if !result.Completed || received != prepared.size || current != prepared.size || result.Revision == "" {
		return result, &VerificationError{Field: "upload result", Expected: fmt.Sprintf("%d/%d/yes/revision", prepared.size, prepared.size), Actual: fmt.Sprintf("%d/%d/%s/%s", received, current, wire.Complete, wire.Revision)}
	}
	return result, nil
}

func multipartBody(filename string, content io.Reader, size int64) (io.Reader, string, int64, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	header := make(textproto.MIMEHeader)
	escaped := strings.NewReplacer("\\", "\\\\", `"`, `\"`, "\r", "%0D", "\n", "%0A").Replace(filename)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+escaped+`"`)
	header.Set("Content-Type", "application/pdf")
	if _, err := writer.CreatePart(header); err != nil {
		return nil, "", 0, err
	}
	headerLength := buffer.Len()
	if err := writer.Close(); err != nil {
		return nil, "", 0, err
	}
	all := buffer.Bytes()
	prefix := append([]byte(nil), all[:headerLength]...)
	suffix := append([]byte(nil), all[headerLength:]...)
	length := int64(len(prefix)) + size + int64(len(suffix))
	return io.MultiReader(bytes.NewReader(prefix), io.LimitReader(content, size), bytes.NewReader(suffix)), writer.FormDataContentType(), length, nil
}

func validateEntryName(name string, document bool) (string, error) {
	name = norm.NFC.String(name)
	if name == "" || name == "." || name == ".." || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\\r\n\x00") {
		return "", errors.New("dpwire: invalid entry name")
	}
	if document && !strings.HasSuffix(strings.ToLower(name), ".pdf") {
		return "", errors.New("dpwire: document name must end in .pdf")
	}
	return name, nil
}
