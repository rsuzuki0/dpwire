package digitalpaper_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/rsuzuki0/digitalpaper"
	"github.com/rsuzuki0/digitalpaper/credentials"
	"github.com/rsuzuki0/digitalpaper/dptest"
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
	client, err := digitalpaper.NewClient(digitalpaper.DeviceProfile{
		Name: "integration", Address: simulator.URL(), ClientID: "integration-client",
		CertificateSHA256: simulator.CertificateSHA256(),
	}, digitalpaper.WithCredentials(credentials.Credentials{ClientID: "integration-client", PrivateKeyPEM: keyPEM}))
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

	documents, err := client.Documents.List(ctx, digitalpaper.ListOptions{PageSize: 1})
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
	remotePath := digitalpaper.MustRemotePath("Document/Inbox/資料 one.pdf")
	resolved, err := client.Documents.Resolve(ctx, remotePath)
	if err != nil || resolved.ID != first.ID {
		t.Fatalf("resolved = %#v, err = %v", resolved, err)
	}
	entries, err := client.Folders.List(ctx, "inbox", digitalpaper.ListOptions{PageSize: 1})
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %#v, err = %v", entries, err)
	}
	var downloaded bytes.Buffer
	result, err := client.Documents.Download(ctx, first.ID, &downloaded)
	if err != nil || !bytes.Equal(downloaded.Bytes(), firstContent) || result.Bytes != int64(len(firstContent)) || result.ETag == "" {
		t.Fatalf("download result = %#v, content = %q, err = %v", result, downloaded.Bytes(), err)
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
	var apiError *digitalpaper.APIError
	if !errors.As(err, &apiError) || apiError.Code != "40401" {
		t.Fatalf("missing document error = %#v", err)
	}
}
