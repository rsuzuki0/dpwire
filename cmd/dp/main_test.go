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

	"github.com/rsuzuki0/dpwire"
	"github.com/rsuzuki0/dpwire/dptest"
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
	entries := []dpwire.Entry{
		{ID: "folder-1", Name: "Inbox", Type: dpwire.EntryFolder},
		{ID: "doc-1", Name: "paper.pdf", Type: dpwire.EntryDocument, Size: "42", Modified: "2026-08-06T12:00:00Z"},
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
		{[]string{"Documents"}, false, "Documents", true},
		{[]string{"-l", "Documents"}, true, "Documents", true},
		{[]string{"-x"}, false, "", false},
	} {
		long, target, ok := parseListArguments(test.arguments)
		if long != test.long || target != test.target || ok != test.ok {
			t.Fatalf("parseListArguments(%q) = %v, %q, %v", test.arguments, long, target, ok)
		}
	}
}

func TestDevicePathsHideProtocolRoot(t *testing.T) {
	path, err := parseDevicePath("Documents/paper.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if got := path.String(); got != "Document/Documents/paper.pdf" {
		t.Fatalf("path = %q", got)
	}
	if got := devicePathString(path); got != "Documents/paper.pdf" {
		t.Fatalf("devicePathString = %q", got)
	}
	directory, err := parseDevicePath("Documents///")
	if err != nil || directory.String() != "Document/Documents" {
		t.Fatalf("trailing slash path = %q, %v", directory.String(), err)
	}
	root, err := parseDevicePath("./")
	if err != nil || root.String() != "Document" {
		t.Fatalf("trailing slash root = %q, %v", root.String(), err)
	}
	if _, err := parseDevicePath("/"); err == nil {
		t.Fatal("slash was accepted as the device root")
	}
	if _, err := parseDevicePath("Document/Documents/paper.pdf"); err == nil {
		t.Fatal("protocol-internal Document prefix was accepted by CLI")
	}
	if _, err := parseDevicePath("Document/"); err == nil {
		t.Fatal("protocol-internal Document root with slash was accepted by CLI")
	}
	parent, name, err := splitRemoteTarget("Documents/new.pdf")
	if err != nil || parent.String() != "Document/Documents" || name != "new.pdf" {
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
	state := dptest.NewState("DPT-RP1", "test-write")
	state.RegisterClient("cli-client", &key.PublicKey)
	state.RequireAuthentication(true)
	root := state.AddFolder("Document/Documents", "Documents", "root", time.Now())
	sourceContent := []byte("%PDF-1.4\nsource\n")
	state.AddDocument("Document/Documents/source.pdf", "source.pdf", root.ID, sourceContent, time.Now())
	simulator := dptest.Start(state)
	defer simulator.Close()

	temporary := t.TempDir()
	keyPath := filepath.Join(temporary, "privatekey.dat")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(temporary, "profile.json")
	if err := dpwire.SaveProfile(profilePath, dpwire.DeviceProfile{
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
	if output := invoke("ls"); !strings.Contains(output, "Documents/") {
		t.Fatalf("root ls = %q", output)
	}
	if output := invoke("ls", "-l", "Documents/"); !strings.Contains(output, "source.pdf") || !strings.Contains(output, "doc-") {
		t.Fatalf("long ls = %q", output)
	}
	invoke("file", "Documents/source.pdf")
	invoke("stat", "Documents/source.pdf")
	invoke("mkdir", "Documents/Write")
	invoke("cp", "Documents/source.pdf", "Documents/Write/")
	invoke("mv", "Documents/Write/source.pdf", "Documents/Write/renamed.pdf")

	localUpload := filepath.Join(temporary, "local.pdf")
	uploadContent := []byte("%PDF-1.7\nupload\n")
	if err := os.WriteFile(localUpload, uploadContent, 0o600); err != nil {
		t.Fatal(err)
	}
	invoke("put", localUpload, "Documents/Write/")
	downloadPath := filepath.Join(temporary, "download.pdf")
	invoke("get", "Documents/Write/local.pdf", downloadPath)
	if downloaded, err := os.ReadFile(downloadPath); err != nil || !bytes.Equal(downloaded, uploadContent) {
		t.Fatalf("downloaded = %q, err = %v", downloaded, err)
	}
	invoke("open", "Documents/Write/renamed.pdf", "1")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-profile", profilePath, "put", localUpload, "Documents/Write"}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "write conflict") {
		t.Fatalf("duplicate put: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var removeOutput, removeErrors bytes.Buffer
	if code := run([]string{"-profile", profilePath, "rmdir", "Documents/Write"}, &removeOutput, &removeErrors); code == 0 || !strings.Contains(removeErrors.String(), "folder not empty") {
		t.Fatalf("non-empty rmdir: code=%d stdout=%q stderr=%q", code, removeOutput.String(), removeErrors.String())
	}
	invoke("rm", "Documents/Write/local.pdf")
	invoke("rm", "Documents/Write/renamed.pdf")
	invoke("rmdir", "Documents/Write")
	if output := invoke("ls", "Documents"); strings.Contains(output, "Write/") {
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
	if output := profileInvoke("ls", "Documents"); !strings.Contains(output, "source.pdf") {
		t.Fatalf("default-profile ls = %q", output)
	}
}
