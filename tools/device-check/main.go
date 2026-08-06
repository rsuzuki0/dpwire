// Command device-check runs a redacted, read-only physical-device validation.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rsuzuki0/digitalpaper"
	"github.com/rsuzuki0/digitalpaper/credentials"
)

type configuration struct {
	Address      string
	Fingerprint  string
	PageSize     int
	DownloadPath string
}

type report struct {
	Model                 string `json:"model"`
	Firmware              string `json:"firmware"`
	TestedAt              string `json:"tested_at"`
	BatteryStatus         string `json:"battery_status"`
	DocumentCount         int    `json:"document_count"`
	PaginationPageSize    int    `json:"pagination_page_size"`
	FolderListChecked     bool   `json:"folder_list_checked"`
	UnicodeResolveChecked bool   `json:"unicode_resolve_checked"`
	MetadataByIDChecked   bool   `json:"metadata_by_id_checked"`
	DownloadChecked       bool   `json:"download_checked"`
	DownloadBytes         int64  `json:"download_bytes,omitempty"`
	ETagPresent           bool   `json:"etag_present"`
	RevisionPresent       bool   `json:"revision_present"`
	MetadataHashPresent   bool   `json:"metadata_hash_present"`
	MetadataHashMatches   bool   `json:"metadata_sha256_matches"`
}

func main() {
	address := flag.String("address", "", "device or local relay HTTPS address")
	fingerprint := flag.String("fingerprint", "", "verified SHA-256 TLS certificate fingerprint")
	clientIDFile := flag.String("client-id-file", "", "existing deviceid.dat path")
	privateKeyFile := flag.String("private-key-file", "", "existing privatekey.dat path")
	pageSize := flag.Int("page-size", 10, "pagination size (1-1000)")
	downloadPath := flag.String("download-path", "", "explicit safe PDF path to validate")
	flag.Parse()
	if *address == "" || *fingerprint == "" || *clientIDFile == "" || *privateKeyFile == "" {
		fmt.Fprintln(os.Stderr, "device-check: address, fingerprint, client-id-file, and private-key-file are required")
		os.Exit(2)
	}
	creds, err := credentials.ImportSony(*clientIDFile, *privateKeyFile)
	if err != nil {
		fatal("credential import", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, err := check(ctx, configuration{
		Address: *address, Fingerprint: *fingerprint, PageSize: *pageSize, DownloadPath: *downloadPath,
	}, creds)
	if err != nil {
		fatal("validation", err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fatal("report encoding", err)
	}
}

func check(ctx context.Context, config configuration, creds credentials.Credentials) (report, error) {
	if config.PageSize < 1 || config.PageSize > 1000 {
		return report{}, errors.New("page size must be between 1 and 1000")
	}
	client, err := digitalpaper.NewClient(digitalpaper.DeviceProfile{
		Name: "physical-device-check", Address: config.Address, ClientID: creds.ClientID,
		CertificateSHA256: config.Fingerprint,
	}, digitalpaper.WithCredentials(creds), digitalpaper.WithTimeout(90*time.Second))
	if err != nil {
		return report{}, fmt.Errorf("client creation: %w", err)
	}
	if err := client.Authenticate(ctx); err != nil {
		return report{}, fmt.Errorf("authentication: %w", err)
	}
	firmware, err := client.Device.Firmware(ctx)
	if err != nil {
		return report{}, fmt.Errorf("firmware status: %w", err)
	}
	battery, err := client.Device.Battery(ctx)
	if err != nil {
		return report{}, fmt.Errorf("battery status: %w", err)
	}
	if _, err := client.Device.Storage(ctx); err != nil {
		return report{}, fmt.Errorf("storage status: %w", err)
	}
	documents, err := client.Documents.List(ctx, digitalpaper.ListOptions{PageSize: config.PageSize})
	if err != nil {
		return report{}, fmt.Errorf("document listing: %w", err)
	}
	result := report{
		Model: firmware.Model, Firmware: firmware.Version, TestedAt: time.Now().UTC().Format(time.RFC3339),
		BatteryStatus: battery.Status, DocumentCount: len(documents), PaginationPageSize: config.PageSize,
	}
	for _, document := range documents {
		if !result.FolderListChecked && document.ParentFolderID != "" {
			if _, err := client.Folders.List(ctx, document.ParentFolderID, digitalpaper.ListOptions{PageSize: config.PageSize}); err != nil {
				return report{}, fmt.Errorf("folder listing: %w", err)
			}
			result.FolderListChecked = true
		}
		if !result.UnicodeResolveChecked && strings.IndexFunc(document.Path.String(), func(r rune) bool { return r > unicode.MaxASCII }) >= 0 {
			resolved, err := client.Documents.Resolve(ctx, document.Path)
			if err != nil || resolved.ID != document.ID {
				return report{}, errors.New("unicode path resolution failed")
			}
			result.UnicodeResolveChecked = true
		}
	}
	if config.DownloadPath == "" {
		return result, nil
	}
	path, err := digitalpaper.ParseRemotePath(config.DownloadPath)
	if err != nil {
		return report{}, fmt.Errorf("download path: %w", err)
	}
	candidate, err := client.Documents.Resolve(ctx, path)
	if err != nil {
		return report{}, fmt.Errorf("download path resolution: %w", err)
	}
	if candidate.Type != digitalpaper.EntryDocument {
		return report{}, errors.New("download path is not a document")
	}
	metadata, err := client.Documents.Get(ctx, candidate.ID)
	if err != nil || metadata.ID != candidate.ID {
		return report{}, errors.New("metadata by ID failed")
	}
	result.MetadataByIDChecked = true
	hasher := sha256.New()
	download, err := client.Documents.Download(ctx, candidate.ID, io.MultiWriter(io.Discard, hasher))
	if err != nil {
		return report{}, fmt.Errorf("PDF download: %w", err)
	}
	result.DownloadChecked = true
	result.DownloadBytes = download.Bytes
	result.ETagPresent = download.ETag != ""
	result.RevisionPresent = candidate.Revision != ""
	result.MetadataHashPresent = candidate.FileHash != ""
	if len(candidate.FileHash) == sha256.Size*2 {
		result.MetadataHashMatches = strings.EqualFold(candidate.FileHash, hex.EncodeToString(hasher.Sum(nil)))
	}
	if size := parseSize(candidate.Size); size >= 0 && download.Bytes != size {
		return report{}, errors.New("metadata size and download size differ")
	}
	return result, nil
}

func parseSize(value string) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return -1
	}
	return parsed
}

func fatal(stage string, err error) {
	fmt.Fprintf(os.Stderr, "device-check: %s: %v\n", stage, err)
	os.Exit(1)
}
