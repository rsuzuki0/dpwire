package dpwire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const maxDocumentSize = int64(1 << 30)

// DocumentsService exposes document metadata, transfer, and safe-write operations.
type DocumentsService struct{ client *Client }

// List retrieves all documents using bounded, automatic pagination.
func (s *DocumentsService) List(ctx context.Context, options ListOptions) ([]Entry, error) {
	return s.client.listEntries(ctx, "/documents2", "documents", options)
}

// Get retrieves document metadata by device ID.
func (s *DocumentsService) Get(ctx context.Context, id string) (Entry, error) {
	if err := validateID(id); err != nil {
		return Entry{}, err
	}
	var raw wireEntry
	endpoint := "/documents2/" + url.PathEscape(id)
	if err := s.client.wire.DoJSON(ctx, http.MethodGet, endpoint, nil, nil, &raw, true); err != nil {
		return Entry{}, publicError(err)
	}
	return decodeEntry(raw)
}

// Resolve retrieves document or folder metadata by normalized device path.
func (s *DocumentsService) Resolve(ctx context.Context, path RemotePath) (Entry, error) {
	if path.String() == "" {
		return Entry{}, errors.New("dpwire: zero remote path")
	}
	var raw wireEntry
	endpoint := "/resolve/entry/path/" + path.EscapedValue()
	if err := s.client.wire.DoJSON(ctx, http.MethodGet, endpoint, nil, nil, &raw, true); err != nil {
		return Entry{}, publicError(err)
	}
	return decodeEntry(raw)
}

// DownloadResult describes one streamed PDF response.
type DownloadResult struct {
	Bytes int64
	ETag  string
}

// Download streams a PDF to destination without buffering it in memory.
func (s *DocumentsService) Download(ctx context.Context, id string, destination io.Writer) (DownloadResult, error) {
	if err := validateID(id); err != nil {
		return DownloadResult{}, err
	}
	if destination == nil {
		return DownloadResult{}, errors.New("dpwire: nil download destination")
	}
	endpoint := "/documents/" + url.PathEscape(id) + "/file"
	response, err := s.client.wire.DoWithAccept(ctx, http.MethodGet, endpoint, nil, nil, true, "application/pdf")
	if err != nil {
		return DownloadResult{}, publicError(err)
	}
	defer response.Body.Close()
	written, err := io.Copy(destination, io.LimitReader(response.Body, maxDocumentSize))
	if err != nil {
		return DownloadResult{}, err
	}
	var extra [1]byte
	count, readErr := response.Body.Read(extra[:])
	if count != 0 {
		return DownloadResult{}, fmt.Errorf("dpwire: document exceeds %d-byte limit", maxDocumentSize)
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return DownloadResult{}, readErr
	}
	return DownloadResult{Bytes: written, ETag: response.Header.Get("ETag")}, nil
}
