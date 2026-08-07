package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rsuzuki0/digitalpaper"
	"github.com/rsuzuki0/digitalpaper/dptest"
)

func TestVersionAndUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"version"}, &stdout, &stderr); code != 0 || strings.TrimSpace(stdout.String()) != version {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestUnixStyleListOutput(t *testing.T) {
	entries := []digitalpaper.Entry{
		{ID: "folder-1", Name: "Inbox", Type: digitalpaper.EntryFolder},
		{ID: "doc-1", Name: "paper.pdf", Type: digitalpaper.EntryDocument, Size: "42", Modified: "2026-08-06T12:00:00Z"},
	}
	var output bytes.Buffer
	if code := printEntries(&output, entries, false); code != 0 || output.String() != "Inbox/\npaper.pdf\n" {
		t.Fatalf("short listing code=%d output=%q", code, output.String())
	}
	output.Reset()
	if code := printEntries(&output, entries, true); code != 0 {
		t.Fatalf("long listing code=%d", code)
	}
	if value := output.String(); !strings.Contains(value, "folder-1") || !strings.Contains(value, "doc-1") || !strings.Contains(value, "paper.pdf") {
		t.Fatalf("long listing = %q", value)
	}
	for _, test := range []struct {
		arguments []string
		long      bool
		target    string
		ok        bool
	}{
		{nil, false, "", true},
		{[]string{"-l"}, true, "", true},
		{[]string{"Codex_dp"}, false, "Codex_dp", true},
		{[]string{"-l", "Codex_dp"}, true, "Codex_dp", true},
		{[]string{"-x"}, false, "", false},
	} {
		long, target, ok := parseListArguments(test.arguments)
		if long != test.long || target != test.target || ok != test.ok {
			t.Fatalf("parseListArguments(%q) = %v, %q, %v", test.arguments, long, target, ok)
		}
	}
}

func TestDevicePathsHideProtocolRoot(t *testing.T) {
	path, err := parseDevicePath("Codex_dp/paper.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if got := path.String(); got != "Document/Codex_dp/paper.pdf" {
		t.Fatalf("path = %q", got)
	}
	if got := devicePathString(path); got != "Codex_dp/paper.pdf" {
		t.Fatalf("devicePathString = %q", got)
	}
	if _, err := parseDevicePath("Document/Codex_dp/paper.pdf"); err == nil {
		t.Fatal("protocol-internal Document prefix was accepted by CLI")
	}
	parent, name, err := splitRemoteTarget("Codex_dp/new.pdf")
	if err != nil || parent.String() != "Document/Codex_dp" || name != "new.pdf" {
		t.Fatalf("splitRemoteTarget = %q, %q, %v", parent.String(), name, err)
	}
}

func TestCredentialsFindDoesNotChooseCandidate(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "workspace")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"deviceid.dat", "privatekey.dat"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"credentials", "find", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), directory) {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestUnixAndFTPCommandsEndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	state := dptest.NewState("DPT-RP1", "test-p2")
	state.RegisterClient("cli-client", &key.PublicKey)
	state.RequireAuthentication(true)
	root := state.AddFolder("Document/Codex_dp", "Codex_dp", "root", time.Now())
	sourceContent := []byte("%PDF-1.4\nsource\n")
	state.AddDocument("Document/Codex_dp/source.pdf", "source.pdf", root.ID, sourceContent, time.Now())
	simulator := dptest.Start(state)
	defer simulator.Close()

	temporary := t.TempDir()
	keyPath := filepath.Join(temporary, "privatekey.dat")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(temporary, "profile.json")
	if err := digitalpaper.SaveProfile(profilePath, digitalpaper.DeviceProfile{
		Name: "cli-test", Address: simulator.URL(), ClientID: "cli-client",
		PrivateKeyRef: keyPath, CertificateSHA256: simulator.CertificateSHA256(),
	}); err != nil {
		t.Fatal(err)
	}

	invoke := func(arguments ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		all := append([]string{"-profile", profilePath}, arguments...)
		if code := run(all, &stdout, &stderr); code != 0 {
			t.Fatalf("dp %s: code=%d stdout=%q stderr=%q", strings.Join(arguments, " "), code, stdout.String(), stderr.String())
		}
		if strings.Contains(stdout.String(), "Document/") {
			t.Fatalf("dp %s exposed protocol root: %q", strings.Join(arguments, " "), stdout.String())
		}
		return stdout.String()
	}

	invoke("auth")
	invoke("device")
	if output := invoke("ls"); !strings.Contains(output, "Codex_dp/") {
		t.Fatalf("root ls = %q", output)
	}
	if output := invoke("ls", "-l", "Codex_dp"); !strings.Contains(output, "source.pdf") || !strings.Contains(output, "doc-") {
		t.Fatalf("long ls = %q", output)
	}
	invoke("file", "Codex_dp/source.pdf")
	invoke("stat", "Codex_dp/source.pdf")
	invoke("mkdir", "Codex_dp/P2")
	invoke("cp", "Codex_dp/source.pdf", "Codex_dp/P2")
	invoke("mv", "Codex_dp/P2/source.pdf", "Codex_dp/P2/renamed.pdf")

	localUpload := filepath.Join(temporary, "local.pdf")
	uploadContent := []byte("%PDF-1.7\nupload\n")
	if err := os.WriteFile(localUpload, uploadContent, 0o600); err != nil {
		t.Fatal(err)
	}
	invoke("put", localUpload, "Codex_dp/P2")
	downloadPath := filepath.Join(temporary, "download.pdf")
	invoke("get", "Codex_dp/P2/local.pdf", downloadPath)
	if downloaded, err := os.ReadFile(downloadPath); err != nil || !bytes.Equal(downloaded, uploadContent) {
		t.Fatalf("downloaded = %q, err = %v", downloaded, err)
	}
	invoke("open", "Codex_dp/P2/renamed.pdf", "1")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-profile", profilePath, "put", localUpload, "Codex_dp/P2"}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "write conflict") {
		t.Fatalf("duplicate put: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var removeOutput, removeErrors bytes.Buffer
	if code := run([]string{"-profile", profilePath, "rmdir", "Codex_dp/P2"}, &removeOutput, &removeErrors); code == 0 || !strings.Contains(removeErrors.String(), "folder not empty") {
		t.Fatalf("non-empty rmdir: code=%d stdout=%q stderr=%q", code, removeOutput.String(), removeErrors.String())
	}
	invoke("rm", "Codex_dp/P2/local.pdf")
	invoke("rm", "Codex_dp/P2/renamed.pdf")
	invoke("rmdir", "Codex_dp/P2")
	if output := invoke("ls", "Codex_dp"); strings.Contains(output, "P2/") {
		t.Fatalf("deleted folder remains in listing: %q", output)
	}

	sonyDirectory := filepath.Join(temporary, "sony-credentials")
	if err := os.Mkdir(sonyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sonyDirectory, "deviceid.dat"), []byte("cli-client\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sonyDirectory, "privatekey.dat"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	configRoot := filepath.Join(temporary, "managed-config")
	profileInvoke := func(arguments ...string) string {
		t.Helper()
		var output, errorsOutput bytes.Buffer
		all := append([]string{"-config-dir", configRoot}, arguments...)
		if code := run(all, &output, &errorsOutput); code != 0 {
			t.Fatalf("dp %s: code=%d stdout=%q stderr=%q", strings.Join(arguments, " "), code, output.String(), errorsOutput.String())
		}
		return output.String()
	}
	profileInvoke("profile", "import-sony", "rp1", simulator.URL(), simulator.CertificateSHA256(), sonyDirectory)
	if output := profileInvoke("profile", "list"); !strings.Contains(output, "*  rp1") || strings.Contains(output, "cli-client") {
		t.Fatalf("profile list = %q", output)
	}
	if output := profileInvoke("profile", "show"); !strings.Contains(output, simulator.URL()) || strings.Contains(output, "privatekey") || strings.Contains(output, "cli-client") {
		t.Fatalf("profile show = %q", output)
	}
	profileInvoke("profile", "use", "rp1")
	if output := profileInvoke("ls", "Codex_dp"); !strings.Contains(output, "source.pdf") {
		t.Fatalf("default-profile ls = %q", output)
	}
}
