package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/rsuzuki0/digitalpaper/credentials"
	"github.com/rsuzuki0/digitalpaper/dptest"
)

func TestCheckAgainstAuthenticatedSimulator(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	state := dptest.NewState("DPT-RP1", "test-firmware")
	state.RegisterClient("device-check", &key.PublicKey)
	state.RequireAuthentication(true)
	state.AddDocument("Document/Inbox/資料.pdf", "資料.pdf", "inbox", []byte("%PDF-test"), time.Now())
	simulator := dptest.Start(state)
	defer simulator.Close()
	privateKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	result, err := check(context.Background(), configuration{
		Address: simulator.URL(), Fingerprint: simulator.CertificateSHA256(), PageSize: 1,
		DownloadPath: "Document/Inbox/資料.pdf",
	}, credentials.Credentials{ClientID: "device-check", PrivateKeyPEM: privateKey})
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "DPT-RP1" || !result.FolderListChecked || !result.UnicodeResolveChecked ||
		!result.MetadataByIDChecked || !result.DownloadChecked || !result.ETagPresent || !result.RevisionPresent {
		t.Fatalf("report = %#v", result)
	}
}

func TestCheckRejectsInvalidPageSize(t *testing.T) {
	if _, err := check(context.Background(), configuration{PageSize: 0}, credentials.Credentials{}); err == nil {
		t.Fatal("invalid page size succeeded")
	}
}

func TestParseSize(t *testing.T) {
	if parseSize("bad") != -1 || parseSize("-1") != -1 || parseSize("3") != 3 {
		t.Fatal("parseSize returned unexpected result")
	}
}
