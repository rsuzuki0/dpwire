package dptest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rsuzuki0/dpwire/internal/wire/auth"
	"github.com/rsuzuki0/dpwire/internal/wire/transport"
)

func TestDocumentsGolden(t *testing.T) {
	state := NewState("DPT-RP1", "1.6.02")
	state.AddDocument("Document/Inbox/example.pdf", "example.pdf", "root", []byte("%PDF-test"), time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC))
	sim := Start(state)
	defer sim.Close()

	response, err := sim.Client().Get(sim.URL() + "/documents2")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	got, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	goldenPath := filepath.Join("..", "testdata", "protocol", "documents2.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("protocol response differs from golden\nwant: %s\n got: %s", want, got)
	}
}

func TestAuthenticatedEndpoints(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	state := NewState("DPT-RP1", "1.6.02")
	state.RegisterClient("client", &key.PublicKey)
	state.RequireAuthentication(true)
	document := state.AddDocument("Document/Inbox/資料.pdf", "資料.pdf", "inbox", []byte("%PDF-fixture"), time.Now())
	sim := Start(state)
	defer sim.Close()

	unauthenticated, err := sim.Client().Get(sim.URL() + "/system/status/battery")
	if err != nil {
		t.Fatal(err)
	}
	unauthenticated.Body.Close()
	if unauthenticated.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.StatusCode)
	}

	client, err := transport.New(sim.URL(), transport.TrustConfig{CertificateSHA256: sim.CertificateSHA256()}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := auth.Authenticate(ctx, client, "client", key); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{
		"/system/status/firmware_version", "/system/status/battery", "/system/status/storage",
		"/documents2?offset=0&limit=1", "/folders/inbox/entries2?offset=0&limit=1",
		"/documents2/" + document.ID, "/resolve/entry/path/Document%2FInbox%2F%E8%B3%87%E6%96%99.pdf",
	} {
		response, err := client.Do(ctx, http.MethodGet, endpoint, nil, nil, true)
		if err != nil {
			t.Fatalf("GET %s: %v", endpoint, err)
		}
		response.Body.Close()
	}
	response, err := client.Do(ctx, http.MethodGet, "/documents/"+document.ID+"/file", nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || !bytes.Equal(content, []byte("%PDF-fixture")) || response.Header.Get("ETag") == "" {
		t.Fatalf("content=%q etag=%q err=%v", content, response.Header.Get("ETag"), err)
	}
}

func TestRegistrationIsExplicitlyUnavailable(t *testing.T) {
	sim := Start(nil)
	defer sim.Close()
	response, err := sim.Client().Get(sim.URL() + "/register/information")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNotImplemented)
	}
}

func TestClientValidatesTLS(t *testing.T) {
	sim := Start(nil)
	defer sim.Close()
	client := sim.Client()
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil {
		t.Fatal("simulator client has no explicit TLS configuration")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("simulator client disables TLS verification")
	}
	if len(sim.DeviceCAPEM()) == 0 || len(sim.CertificateSHA256()) != 64 {
		t.Fatal("simulator did not expose valid trust material")
	}
}

func TestFaultInjectionOnce(t *testing.T) {
	state := NewState("DP-SIM", "0")
	state.InjectFault("GET /ping", Fault{Status: http.StatusInternalServerError, Body: `{"code":"TEST"}`, Once: true})
	sim := Start(state)
	defer sim.Close()
	for index, want := range []int{http.StatusInternalServerError, http.StatusOK} {
		response, err := sim.Client().Get(sim.URL() + "/ping")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("request %d status = %d, want %d", index, response.StatusCode, want)
		}
	}
}

func TestDeleteEndpointsGuardRevisionAndFolderContents(t *testing.T) {
	state := NewState("DP-SIM", "test-delete")
	folder := state.AddFolder("Document/delete-test", "delete-test", "root", time.Now())
	document := state.AddDocument("Document/delete-test/paper.pdf", "paper.pdf", folder.ID, []byte("%PDF-test"), time.Now())
	sim := Start(state)
	defer sim.Close()

	request := func(method, endpoint, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, sim.URL()+endpoint, bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := sim.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}

	response := request(http.MethodDelete, "/documents/"+document.ID, `{"target_revision":"stale"}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("stale delete status = %d", response.StatusCode)
	}
	response.Body.Close()
	response = request(http.MethodDelete, "/folders/"+folder.ID, `{"force_delete_flag":"false"}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-empty rmdir status = %d", response.StatusCode)
	}
	response.Body.Close()
	response = request(http.MethodDelete, "/documents/"+document.ID, `{"target_revision":"1"}`)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("document delete status = %d", response.StatusCode)
	}
	response.Body.Close()
	response = request(http.MethodDelete, "/folders/"+folder.ID, `{"force_delete_flag":"false"}`)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("empty rmdir status = %d", response.StatusCode)
	}
	response.Body.Close()
	if _, ok := state.documentByPath("Document/delete-test"); ok {
		t.Fatal("deleted folder still resolves")
	}
}
