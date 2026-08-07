package dpwire

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// EntryType is the protocol type of an entry.
type EntryType string

const (
	EntryDocument EntryType = "document"
	EntryFolder   EntryType = "folder"
)

// Entry contains document or folder metadata returned by the device.
type Entry struct {
	ID             string
	Name           string
	Path           RemotePath
	Type           EntryType
	Created        string
	Modified       string
	MIMEType       string
	Size           string
	DocumentType   string
	Author         string
	Title          string
	TotalPages     string
	CurrentPage    string
	ReadingDate    string
	ParentFolderID string
	IsNew          string
	DocumentSource string
	ExternalID     string
	FileHash       string
	Revision       string
}

type wireEntry struct {
	ID             string `json:"entry_id"`
	Name           string `json:"entry_name"`
	Path           string `json:"entry_path"`
	Type           string `json:"entry_type"`
	Created        string `json:"created_date"`
	Modified       string `json:"modified_date"`
	MIMEType       string `json:"mime_type"`
	Size           string `json:"file_size"`
	DocumentType   string `json:"document_type"`
	Author         string `json:"author"`
	Title          string `json:"title"`
	TotalPages     string `json:"total_page"`
	CurrentPage    string `json:"current_page"`
	ReadingDate    string `json:"reading_date"`
	ParentFolderID string `json:"parent_folder_id"`
	IsNew          string `json:"is_new"`
	DocumentSource string `json:"document_source"`
	ExternalID     string `json:"ext_id"`
	FileHash       string `json:"file_hash"`
	Revision       string `json:"file_revision"`
}

type wireEntryList struct {
	Count   int         `json:"count"`
	Hash    string      `json:"entry_list_hash"`
	Entries []wireEntry `json:"entry_list"`
}

// ListOptions controls one logical, automatically paginated listing.
type ListOptions struct {
	PageSize int
	Fields   []string
}

func (c *Client) listEntries(ctx context.Context, endpoint, entryType string, options ListOptions) ([]Entry, error) {
	pageSize := options.PageSize
	if pageSize == 0 {
		pageSize = 100
	}
	if pageSize < 1 || pageSize > 1000 {
		return nil, errors.New("dpwire: page size must be between 1 and 1000")
	}
	for _, field := range options.Fields {
		if field == "" || strings.ContainsAny(field, ",\r\n\x00") {
			return nil, errors.New("dpwire: invalid field name")
		}
	}
	var entries []Entry
	for offset := 0; ; {
		query := url.Values{"offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(pageSize)}}
		if entryType != "" {
			query.Set("entry_type", entryType)
		}
		if len(options.Fields) > 0 {
			query.Set("fields", strings.Join(options.Fields, ","))
		}
		var page wireEntryList
		if err := c.wire.DoJSON(ctx, http.MethodGet, endpoint, query, nil, &page, true); err != nil {
			return nil, publicError(err)
		}
		for _, raw := range page.Entries {
			entry, err := decodeEntry(raw)
			if err != nil {
				return nil, err
			}
			entries = append(entries, entry)
		}
		if len(entries) >= page.Count {
			return entries, nil
		}
		if len(page.Entries) == 0 {
			return nil, fmt.Errorf("dpwire: paginated list stopped at %d of %d entries", len(entries), page.Count)
		}
		offset += len(page.Entries)
	}
}

func decodeEntry(raw wireEntry) (Entry, error) {
	path, err := ParseRemotePath(strings.TrimSuffix(raw.Path, "/"))
	if err != nil {
		return Entry{}, fmt.Errorf("dpwire: entry %q has invalid path: %w", raw.ID, err)
	}
	entryType := EntryType(raw.Type)
	if entryType != EntryDocument && entryType != EntryFolder {
		return Entry{}, fmt.Errorf("dpwire: entry %q has unknown type %q", raw.ID, raw.Type)
	}
	return Entry{ID: raw.ID, Name: raw.Name, Path: path, Type: entryType, Created: raw.Created,
		Modified: raw.Modified, MIMEType: raw.MIMEType, Size: raw.Size, DocumentType: raw.DocumentType,
		Author: raw.Author, Title: raw.Title, TotalPages: raw.TotalPages, CurrentPage: raw.CurrentPage,
		ReadingDate: raw.ReadingDate, ParentFolderID: raw.ParentFolderID, IsNew: raw.IsNew,
		DocumentSource: raw.DocumentSource, ExternalID: raw.ExternalID, FileHash: raw.FileHash,
		Revision: raw.Revision}, nil
}

func validateID(id string) error {
	if id == "" || strings.ContainsAny(id, "/\\\r\n\x00") || id == "." || id == ".." {
		return errors.New("dpwire: invalid entry ID")
	}
	return nil
}
