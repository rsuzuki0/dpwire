package dptest

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
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
