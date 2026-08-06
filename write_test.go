package digitalpaper

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"testing"

	"github.com/rsuzuki0/digitalpaper/internal/wire/transport"
)

func TestMultipartPDFBody(t *testing.T) {
	content := []byte("%PDF-1.7\nfixture\n")
	body, contentType, contentLength, err := multipartBody("資料.pdf", bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(raw)) != contentLength {
		t.Fatalf("length = %d, want %d", len(raw), contentLength)
	}
	_, parameters, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(bytes.NewReader(raw), parameters["boundary"])
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(part)
	if err != nil {
		t.Fatal(err)
	}
	if part.FormName() != "file" || part.FileName() != "資料.pdf" || !bytes.Equal(got, content) {
		t.Fatalf("part name=%q filename=%q content=%q", part.FormName(), part.FileName(), got)
	}
	if _, err := reader.NextPart(); !errors.Is(err, io.EOF) {
		t.Fatalf("second part error = %v", err)
	}
}

func TestPreparePDFAndNames(t *testing.T) {
	content := []byte("%PDF-1.4\ntest")
	prepared, err := preparePDF(bytes.NewReader(content))
	if err != nil || prepared.size != int64(len(content)) || len(prepared.hash) != 64 {
		t.Fatalf("prepared = %#v, err = %v", prepared, err)
	}
	if _, err := preparePDF(bytes.NewReader([]byte("not a PDF"))); err == nil {
		t.Fatal("non-PDF source succeeded")
	}
	for _, test := range []struct {
		name     string
		document bool
		valid    bool
	}{
		{"folder", false, true}, {"paper.pdf", true, true}, {"paper.txt", true, false},
		{"../bad.pdf", true, false}, {"bad/name", false, false},
	} {
		_, err := validateEntryName(test.name, test.document)
		if (err == nil) != test.valid {
			t.Fatalf("name %q document=%v err=%v", test.name, test.document, err)
		}
	}
}

func TestWriteErrorTypes(t *testing.T) {
	api := &APIError{StatusCode: 400, Code: "40017", Message: "changed"}
	conflict := &ConflictError{Cause: api}
	if !errors.Is(conflict, ErrConflict) {
		t.Fatal("conflict does not match ErrConflict")
	}
	var gotAPI *APIError
	if !errors.As(conflict, &gotAPI) || gotAPI != api {
		t.Fatal("conflict does not preserve APIError")
	}
	partial := &PartialFailureError{Operation: "upload", EntryID: "doc-1", Cause: conflict}
	if !errors.Is(partial, ErrConflict) || partial.Error() == "" {
		t.Fatal("partial failure does not preserve cause")
	}
	if (&VerificationError{Field: "size", Expected: "1", Actual: "2"}).Error() == "" {
		t.Fatal("verification error is empty")
	}
}

func TestDuplicateNameIsConflict(t *testing.T) {
	for _, code := range []string{"40007", "40017"} {
		err := publicError(&transport.HTTPError{StatusCode: 400, Code: code, Message: "conflict"})
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("code %s did not map to ErrConflict: %v", code, err)
		}
	}
}
