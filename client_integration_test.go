package dpwire_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/rsuzuki0/dpwire"
	"github.com/rsuzuki0/dpwire/credentials"
	"github.com/rsuzuki0/dpwire/dptest"
	wireregistration "github.com/rsuzuki0/dpwire/internal/wire/registration"
)

func TestAuthenticatedReadOnlyClient(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	state := dptest.NewState("DPT-RP1", "1.6.02")
	state.RegisterClient("integration-client", &key.PublicKey)
	state.RequireAuthentication(true)
	firstContent := []byte("%PDF-1.4\nfirst\n")
	first := state.AddDocument("Document/Inbox/資料 one.pdf", "資料 one.pdf", "inbox", firstContent, time.Unix(1_700_000_000, 0))
	state.AddDocument("Document/Inbox/two.pdf", "two.pdf", "inbox", []byte("%PDF-1.4\nsecond\n"), time.Unix(1_700_000_001, 0))
	simulator := dptest.Start(state)
	defer simulator.Close()

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	client, err := dpwire.NewClient(dpwire.DeviceProfile{
		Name: "integration", Address: simulator.URL(), ClientID: "integration-client",
		CertificateSHA256: simulator.CertificateSHA256(),
	}, dpwire.WithCredentials(credentials.Credentials{ClientID: "integration-client", PrivateKeyPEM: keyPEM}))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := client.Device.Firmware(ctx); err == nil {
		t.Fatal("protected request succeeded before Authenticate")
	}
	if err := client.Authenticate(ctx); err != nil {
		t.Fatal(err)
	}

	documents, err := client.Documents.List(ctx, dpwire.ListOptions{PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 2 || (documents[0].ID != first.ID && documents[1].ID != first.ID) {
		t.Fatalf("documents = %#v", documents)
	}
	metadata, err := client.Documents.Get(ctx, first.ID)
	if err != nil || metadata.Path.String() != "Document/Inbox/資料 one.pdf" {
		t.Fatalf("metadata = %#v, err = %v", metadata, err)
	}
	remotePath := dpwire.MustRemotePath("Document/Inbox/資料 one.pdf")
	resolved, err := client.Documents.Resolve(ctx, remotePath)
	if err != nil || resolved.ID != first.ID {
		t.Fatalf("resolved = %#v, err = %v", resolved, err)
	}
	entries, err := client.Folders.List(ctx, "inbox", dpwire.ListOptions{PageSize: 1})
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %#v, err = %v", entries, err)
	}
	var downloaded bytes.Buffer
	result, err := client.Documents.Download(ctx, first.ID, &downloaded)
	if err != nil || !bytes.Equal(downloaded.Bytes(), firstContent) || result.Bytes != int64(len(firstContent)) || result.ETag == "" {
		t.Fatalf("download result = %#v, content = %q, err = %v", result, downloaded.Bytes(), err)
	}
	state.InjectFault("GET /documents/*/file", dptest.Fault{Status: http.StatusOK, Body: "not a PDF", Once: true})
	var invalidDownload bytes.Buffer
	if _, err := client.Documents.Download(ctx, first.ID, &invalidDownload); err == nil || invalidDownload.Len() != 0 {
		t.Fatalf("non-PDF download content = %q, err = %v", invalidDownload.Bytes(), err)
	}
	firmware, err := client.Device.Firmware(ctx)
	if err != nil || firmware.Model != "DPT-RP1" || firmware.Version != "1.6.02" {
		t.Fatalf("firmware = %#v, err = %v", firmware, err)
	}
	battery, err := client.Device.Battery(ctx)
	if err != nil || battery.Status != "discharging" {
		t.Fatalf("battery = %#v, err = %v", battery, err)
	}
	storage, err := client.Device.Storage(ctx)
	if err != nil || storage.Capacity == "" {
		t.Fatalf("storage = %#v, err = %v", storage, err)
	}
	_, err = client.Documents.Get(ctx, "missing")
	var apiError *dpwire.APIError
	if !errors.As(err, &apiError) || apiError.Code != "40401" {
		t.Fatalf("missing document error = %#v", err)
	}
}

func TestSafeWriteLifecycle(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	state := dptest.NewState("DPT-RP1", "1.6.50")
	state.RegisterClient("write-client", &key.PublicKey)
	state.RequireAuthentication(true)
	state.RejectUploadFileHash(true)
	root := state.AddFolder("Document/Documents", "Documents", "root", time.Now())
	baselineContent := []byte("%PDF-1.4\nbaseline\n")
	baseline := state.AddDocument("Document/Documents/baseline.pdf", "baseline.pdf", root.ID, baselineContent, time.Now())
	simulator := dptest.Start(state)
	defer simulator.Close()
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	client, err := dpwire.NewClient(dpwire.DeviceProfile{
		Name: "write-integration", Address: simulator.URL(), ClientID: "write-client",
		CertificateSHA256: simulator.CertificateSHA256(),
	}, dpwire.WithCredentials(credentials.Credentials{ClientID: "write-client", PrivateKeyPEM: keyPEM}))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.Authenticate(ctx); err != nil {
		t.Fatal(err)
	}
	deviceRoot, err := client.Documents.Resolve(ctx, dpwire.MustRemotePath("Document"))
	if err != nil || deviceRoot.Type != dpwire.EntryFolder {
		t.Fatalf("device root = %#v, err = %v", deviceRoot, err)
	}
	rootEntries, err := client.Folders.List(ctx, deviceRoot.ID, dpwire.ListOptions{})
	if err != nil || len(rootEntries) != 1 || rootEntries[0].ID != root.ID {
		t.Fatalf("root entries = %#v, err = %v", rootEntries, err)
	}

	runFolder, err := client.Folders.Create(ctx, root.ID, "write-run")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Folders.Create(ctx, root.ID, "write-run"); err == nil {
		t.Fatal("duplicate folder creation succeeded")
	} else {
		var apiError *dpwire.APIError
		if !errors.As(err, &apiError) || apiError.Code != "40007" {
			t.Fatalf("duplicate error = %#v", err)
		}
	}
	state.InjectFault("GET /folders2/*", dptest.Fault{Status: http.StatusInternalServerError, Body: `{"error_code":"TEST_VERIFY","message":"verification failed"}`, Once: true})
	partiallyCreated, err := client.Folders.Create(ctx, root.ID, "verification-fails")
	var createPartial *dpwire.PartialFailureError
	if !errors.As(err, &createPartial) || partiallyCreated.ID == "" || createPartial.EntryID != partiallyCreated.ID {
		t.Fatalf("partially verified folder = %#v, error = %#v", partiallyCreated, err)
	}
	copy, err := client.Documents.Copy(ctx, baseline.ID, runFolder.ID, "copy.pdf")
	if err != nil {
		t.Fatal(err)
	}
	copy, err = client.Documents.Update(ctx, copy.ID, "", "renamed.pdf")
	if err != nil || copy.Name != "renamed.pdf" {
		t.Fatalf("renamed copy = %#v, err = %v", copy, err)
	}

	uploadContent := []byte("%PDF-1.7\ncreated\n")
	uploaded, upload, err := client.Documents.CreateAndUpload(ctx, runFolder.ID, "uploaded.pdf", bytes.NewReader(uploadContent))
	if err != nil || upload.Revision == "" || uploaded.Size == "0" {
		t.Fatalf("uploaded = %#v, result = %#v, err = %v", uploaded, upload, err)
	}
	wantHash := sha256.Sum256(uploadContent)
	if upload.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("upload SHA-256 = %q", upload.SHA256)
	}
	if _, _, err := client.Documents.Replace(ctx, uploaded.ID, uploaded.Name, "wrong-revision", bytes.NewReader([]byte("%PDF-1.7\nreplacement\n"))); !errors.Is(err, dpwire.ErrConflict) {
		t.Fatalf("replacement conflict = %#v", err)
	}
	replaced, replacement, err := client.Documents.Replace(ctx, uploaded.ID, uploaded.Name, uploaded.Revision, bytes.NewReader([]byte("%PDF-1.7\nreplacement\n")))
	if err != nil || replaced.Revision != replacement.Revision {
		t.Fatalf("replaced = %#v, result = %#v, err = %v", replaced, replacement, err)
	}
	if err := client.Documents.Open(ctx, replaced.ID, 1); err != nil {
		t.Fatal(err)
	}

	renamedFolder, err := client.Folders.Update(ctx, runFolder.ID, "", "write-renamed")
	if err != nil || renamedFolder.Name != "write-renamed" {
		t.Fatalf("renamed folder = %#v, err = %v", renamedFolder, err)
	}
	resolved, err := client.Documents.Resolve(ctx, dpwire.MustRemotePath("Document/Documents/write-renamed/renamed.pdf"))
	if err != nil || resolved.ID != copy.ID {
		t.Fatalf("resolved moved child = %#v, err = %v", resolved, err)
	}

	state.InjectFault("PUT /documents/*/file", dptest.Fault{Status: http.StatusInternalServerError, Body: `{"code":"TEST_UPLOAD_FAILURE"}`, Once: true})
	partialEntry, _, err := client.Documents.CreateAndUpload(ctx, renamedFolder.ID, "partial.pdf", bytes.NewReader([]byte("%PDF-1.4\npartial\n")))
	var partial *dpwire.PartialFailureError
	if !errors.As(err, &partial) || partial.EntryID == "" || partialEntry.ID != partial.EntryID {
		t.Fatalf("partial entry = %#v, error = %#v", partialEntry, err)
	}

	state.InjectFault("POST /documents2", dptest.Fault{Status: http.StatusInsufficientStorage, Body: `{"code":"50701","message":"storage full"}`, Once: true})
	if _, _, err := client.Documents.CreateAndUpload(ctx, renamedFolder.ID, "full.pdf", bytes.NewReader([]byte("%PDF-1.4\nfull\n"))); err == nil {
		t.Fatal("storage-full upload succeeded")
	} else {
		var apiError *dpwire.APIError
		if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusInsufficientStorage || apiError.Code != "50701" {
			t.Fatalf("storage-full error = %#v", err)
		}
	}

	state.InjectFault("PUT /documents/*/file", dptest.Fault{Status: http.StatusOK, Body: `{not-json`, Once: true})
	if entry, _, err := client.Documents.CreateAndUpload(ctx, renamedFolder.ID, "malformed.pdf", bytes.NewReader([]byte("%PDF-1.4\nmalformed\n"))); err == nil {
		t.Fatal("malformed upload response succeeded")
	} else if !errors.As(err, &partial) || partial.EntryID != entry.ID {
		t.Fatalf("malformed response entry = %#v, error = %#v", entry, err)
	}

	state.InjectFault("PUT /documents/*/file", dptest.Fault{Status: http.StatusOK, Body: `{"received_bytes":"1","current_bytes":"1","completed":"no","file_revision":"1"}`, Once: true})
	if entry, _, err := client.Documents.CreateAndUpload(ctx, renamedFolder.ID, "incomplete.pdf", bytes.NewReader([]byte("%PDF-1.4\nincomplete\n"))); err == nil {
		t.Fatal("incomplete upload response succeeded")
	} else {
		var verification *dpwire.VerificationError
		if !errors.As(err, &partial) || partial.EntryID != entry.ID || !errors.As(err, &verification) {
			t.Fatalf("incomplete response entry = %#v, error = %#v", entry, err)
		}
	}
}

func TestSafeDeleteLifecycle(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	state := dptest.NewState("DPT-RP1", "test-delete")
	state.RegisterClient("delete-client", &key.PublicKey)
	state.RequireAuthentication(true)
	folder := state.AddFolder("Document/delete-test", "delete-test", "root", time.Now())
	document := state.AddDocument("Document/delete-test/paper.pdf", "paper.pdf", folder.ID, []byte("%PDF-1.4\noriginal\n"), time.Now())
	simulator := dptest.Start(state)
	defer simulator.Close()

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	client, err := dpwire.NewClient(dpwire.DeviceProfile{
		Name: "delete-integration", Address: simulator.URL(), ClientID: "delete-client",
		CertificateSHA256: simulator.CertificateSHA256(),
	}, dpwire.WithCredentials(credentials.Credentials{ClientID: "delete-client", PrivateKeyPEM: keyPEM}))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.Authenticate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Documents.Delete(ctx, document.ID, ""); err == nil {
		t.Fatal("document deletion without a revision succeeded")
	}
	updated, _, err := client.Documents.Replace(ctx, document.ID, document.Name, document.Revision, bytes.NewReader([]byte("%PDF-1.4\nupdated\n")))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Documents.Delete(ctx, document.ID, document.Revision); !errors.Is(err, dpwire.ErrConflict) {
		t.Fatalf("stale-revision deletion error = %#v", err)
	}
	if err := client.Folders.DeleteEmpty(ctx, folder.ID); !errors.Is(err, dpwire.ErrNotEmpty) {
		t.Fatalf("non-empty folder deletion error = %#v", err)
	}
	if err := client.Documents.Delete(ctx, updated.ID, updated.Revision); err != nil {
		t.Fatal(err)
	}
	if err := client.Folders.DeleteEmpty(ctx, folder.ID); err != nil {
		t.Fatal(err)
	}
	if forced, ok := state.LastFolderDeleteForced(); !ok || forced {
		t.Fatalf("folder delete force flag = %v, recorded = %v", forced, ok)
	}
	if err := client.Folders.DeleteEmpty(ctx, "root"); err == nil {
		t.Fatal("device root deletion succeeded")
	}
}

func TestFreshPairingEmulatorLifecycle(t *testing.T) {
	state := dptest.NewState("DPT-RP1", "test-pairing")
	state.RequireAuthentication(true)
	simulator := dptest.Start(state)
	defer simulator.Close()

	registrationClient, err := wireregistration.NewExact(simulator.RegistrationURL(), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registrationClient.Register(context.Background(), func(context.Context) (string, error) {
		return "000000", nil
	}); err == nil {
		t.Fatal("wrong-PIN registration succeeded")
	}
	if _, err := registrationClient.Register(context.Background(), func(context.Context) (string, error) {
		return "", context.Canceled
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted registration error = %v", err)
	}
	registered, err := registrationClient.Register(context.Background(), func(context.Context) (string, error) {
		return "123456", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := dpwire.NewClient(dpwire.DeviceProfile{
		Name: "fresh", Address: simulator.URL(), Connection: dpwire.ConnectionDirect,
		ClientID: registered.ClientID, DeviceCAPEM: registered.DeviceCAPEM,
	}, dpwire.WithCredentials(credentials.Credentials{ClientID: registered.ClientID, PrivateKeyPEM: registered.PrivateKeyPEM}))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Authenticate(context.Background()); err != nil {
		t.Fatal(err)
	}
	firmware, err := client.Device.Firmware(context.Background())
	if err != nil || firmware.Version != "test-pairing" {
		t.Fatalf("firmware=%#v err=%v", firmware, err)
	}
	repeated, err := registrationClient.Register(context.Background(), func(context.Context) (string, error) {
		return "123456", nil
	})
	if err != nil || repeated.ClientID == registered.ClientID {
		t.Fatalf("repeated registration result=%q err=%v", repeated.ClientID, err)
	}
}
