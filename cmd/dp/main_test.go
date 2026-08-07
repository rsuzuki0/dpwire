package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rsuzuki0/dpwire"
	"github.com/rsuzuki0/dpwire/dptest"
	"github.com/rsuzuki0/dpwire/profiles"
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

func TestUsageColumns(t *testing.T) {
	var output bytes.Buffer
	usage(&output)
	if !strings.HasPrefix(output.String(), "usage: dp [-profile NAME|FILE] COMMAND [ARG...]\n\ncommands:\n\n") {
		t.Fatalf("usage section spacing:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "print version\n\ndevice paths use the fixed root /") {
		t.Fatalf("usage block spacing:\n%s", output.String())
	}
	ordered := []string{"  ls ", "  open ", "  device ", "  auth ", "  profile ", "  credentials ", "  inspect-cert ", "  version "}
	previous := -1
	for _, command := range ordered {
		index := strings.Index(output.String(), command)
		if index < 0 || index <= previous {
			t.Fatalf("usage command order at %q:\n%s", command, output.String())
		}
		previous = index
	}
	lines := strings.Split(output.String(), "\n")
	descriptions := []string{"print version", "import an existing Sony credential pair", "display a PDF on the device"}
	column := -1
	for _, description := range descriptions {
		found := false
		for _, line := range lines {
			index := strings.Index(line, description)
			if index < 0 {
				continue
			}
			found = true
			if column < 0 {
				column = index
			} else if index != column {
				t.Fatalf("description %q starts at column %d, want %d:\n%s", description, index, column, output.String())
			}
		}
		if !found {
			t.Fatalf("missing usage description %q", description)
		}
	}
	longestCommand := "  profile import-sony NAME ADDRESS SHA256 CREDENTIAL_DIR"
	if column-len(longestCommand) < 4 {
		t.Fatalf("usage description gap = %d, want at least 4:\n%s", column-len(longestCommand), output.String())
	}
}

func TestUnixStyleListOutput(t *testing.T) {
	entries := []dpwire.Entry{
		{ID: "doc-1", Name: "paper.pdf", Type: dpwire.EntryDocument, Size: "42", Modified: "2026-08-06T12:00:00Z"},
		{ID: "folder-1", Name: "Inbox", Type: dpwire.EntryFolder},
	}
	sortDeviceEntries(entries, false)
	var output bytes.Buffer
	if code := printEntries(&output, entries, nil, false); code != 0 || output.String() != "Inbox/\npaper.pdf\n" {
		t.Fatalf("short listing code=%d output=%q", code, output.String())
	}
	output.Reset()
	references := map[string]profiles.ObjectReference{
		"folder-1": {Number: 0, Hex: "0x123456"},
		"doc-1":    {Number: 1, Hex: "0xabcdef"},
	}
	if code := printEntries(&output, entries, references, true); code != 0 {
		t.Fatalf("long listing code=%d", code)
	}
	if value := output.String(); !strings.Contains(value, "0  0x123456") || !strings.Contains(value, "1  0xabcdef") || !strings.Contains(value, "folder-1") || !strings.Contains(value, "doc-1") || !strings.Contains(value, "paper.pdf") {
		t.Fatalf("long listing = %q", value)
	}
	timed := []dpwire.Entry{
		{ID: "old", Name: "old.pdf", Modified: "2026-08-05T12:00:00Z"},
		{ID: "missing", Name: "missing.pdf"},
		{ID: "invalid", Name: "broken.pdf", Modified: "not-a-time"},
		{ID: "tie-b", Name: "bravo.pdf", Modified: "2026-08-07T12:00:00Z"},
		{ID: "new", Name: "new.pdf", Modified: "2026-08-08T12:00:00Z"},
		{ID: "offset", Name: "offset.pdf", Modified: "2026-08-08T10:00:00-05:00"},
		{ID: "tie-a", Name: "alpha.pdf", Modified: "2026-08-07T12:00:00Z"},
	}
	sortDeviceEntries(timed, true)
	got := make([]string, len(timed))
	for index := range timed {
		got[index] = timed[index].Name
	}
	if !slices.Equal(got, []string{"offset.pdf", "new.pdf", "alpha.pdf", "bravo.pdf", "old.pdf", "broken.pdf", "missing.pdf"}) {
		t.Fatalf("time-sorted listing = %q", got)
	}
	for _, test := range []struct {
		arguments []string
		long      bool
		recursive bool
		newest    bool
		target    string
		ok        bool
	}{
		{nil, false, false, false, "", true},
		{[]string{"-l"}, true, false, false, "", true},
		{[]string{"-t"}, false, false, true, "", true},
		{[]string{"-lt", "Documents"}, true, false, true, "Documents", true},
		{[]string{"-tl", "Documents"}, true, false, true, "Documents", true},
		{[]string{"-R"}, false, true, false, "", true},
		{[]string{"-lRt", "Documents"}, true, true, true, "Documents", true},
		{[]string{"-Rl", "Documents"}, true, true, false, "Documents", true},
		{[]string{"-l", "-R", "Documents"}, true, true, false, "Documents", true},
		{[]string{"-R", "-l", "Documents"}, true, true, false, "Documents", true},
		{[]string{"Documents"}, false, false, false, "Documents", true},
		{[]string{"-l", "Documents"}, true, false, false, "Documents", true},
		{[]string{"--id", "23"}, false, false, false, "23", true},
		{[]string{"-l", "--glob", "*.pdf"}, true, false, false, "*.pdf", true},
		{[]string{"-x"}, false, false, false, "", false},
	} {
		options, target, ok := parseListArguments(test.arguments)
		gotTarget := target.value
		if gotTarget == "." && test.target == "" {
			gotTarget = ""
		}
		if options.long != test.long || options.recursive != test.recursive || options.newest != test.newest || gotTarget != test.target || ok != test.ok {
			t.Fatalf("parseListArguments(%q) = %+v, %q, %v", test.arguments, options, gotTarget, ok)
		}
	}
	for _, value := range []string{"0", "-1", "1x", "1 2", ""} {
		if _, err := parsePage(value); err == nil {
			t.Fatalf("page %q was accepted", value)
		}
	}
	if page, err := parsePage("23"); err != nil || page != 23 {
		t.Fatalf("page = %d, %v", page, err)
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
	if got := devicePathString(path); got != "/Documents/paper.pdf" {
		t.Fatalf("devicePathString = %q", got)
	}
	directory, err := parseDevicePath("Documents///")
	if err != nil || directory.String() != "Document/Documents" {
		t.Fatalf("trailing slash path = %q, %v", directory.String(), err)
	}
	prefixed, err := parseDevicePath("././Documents/paper.pdf")
	if err != nil || prefixed.String() != "Document/Documents/paper.pdf" {
		t.Fatalf("root-prefixed path = %q, %v", prefixed.String(), err)
	}
	root, err := parseDevicePath("./")
	if err != nil || root.String() != "Document" {
		t.Fatalf("trailing slash root = %q, %v", root.String(), err)
	}
	absolute, err := parseDevicePath("/Documents/paper.pdf")
	if err != nil || absolute.String() != "Document/Documents/paper.pdf" {
		t.Fatalf("absolute device path = %q, %v", absolute.String(), err)
	}
	absoluteRoot, err := parseDevicePath("/")
	if err != nil || absoluteRoot.String() != "Document" {
		t.Fatalf("absolute device root = %q, %v", absoluteRoot.String(), err)
	}
	if _, err := parseDevicePath("//Documents/paper.pdf"); err == nil {
		t.Fatal("double-leading-slash device path was accepted")
	}
	if _, err := parseDevicePath("Document/Documents/paper.pdf"); err == nil {
		t.Fatal("protocol-internal Document prefix was accepted by CLI")
	}
	if _, err := parseDevicePath("Document/"); err == nil {
		t.Fatal("protocol-internal Document root with slash was accepted by CLI")
	}
	if _, err := parseDevicePath("./Document/Documents/paper.pdf"); err == nil {
		t.Fatal("root-prefixed internal Document path was accepted by CLI")
	}
	if _, err := parseDevicePath("/Document/Documents/paper.pdf"); err == nil {
		t.Fatal("absolute protocol-internal Document path was accepted by CLI")
	}
	for _, value := range []string{"document/Documents/paper.pdf", "/DOCUMENT/Documents/paper.pdf", "./DoCuMeNt/Documents/paper.pdf"} {
		if _, err := parseDevicePath(value); err == nil {
			t.Fatalf("case-variant protocol path %q was accepted", value)
		}
	}
	parent, name, err := splitRemoteTarget("Documents/new.pdf")
	if err != nil || parent.String() != "Document/Documents" || name != "new.pdf" {
		t.Fatalf("splitRemoteTarget = %q, %q, %v", parent.String(), name, err)
	}
}

func TestQuestionableGlobRequiresConfirmation(t *testing.T) {
	selector := objectSelector{kind: selectorGlob, value: "paper.pdf"}
	var output bytes.Buffer
	if err := confirmQuestionableGlob(selector, strings.NewReader(""), &output); err == nil || !strings.Contains(output.String(), "y/[N]") {
		t.Fatalf("default confirmation error=%v output=%q", err, output.String())
	}
	output.Reset()
	if err := confirmQuestionableGlob(selector, strings.NewReader("y\n"), &output); err != nil {
		t.Fatalf("affirmative confirmation: %v", err)
	}
	output.Reset()
	if err := confirmQuestionableGlob(objectSelector{kind: selectorGlob, value: "*.pdf"}, strings.NewReader(""), &output); err != nil || output.Len() != 0 {
		t.Fatalf("ordinary glob confirmation error=%v output=%q", err, output.String())
	}
}

func TestSelectedProfileRejectsBareNameAmbiguity(t *testing.T) {
	working := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(working); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	configRoot := filepath.Join(t.TempDir(), "config")
	manager, err := profiles.New(configRoot)
	if err != nil {
		t.Fatal(err)
	}
	save := func(path, name, address string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := dpwire.SaveProfile(path, dpwire.DeviceProfile{
			Name: name, Address: address, ClientID: "client", CertificateSHA256: strings.Repeat("a", 64),
		}); err != nil {
			t.Fatal(err)
		}
	}
	save(filepath.Join(configRoot, "profiles", "same", "profile.json"), "same", "https://saved.example")
	save(filepath.Join(working, "same"), "external", "https://external.example")
	if _, err := selectedProfile(manager, "same"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("same-name profile selection: %v", err)
	}
	if profile, err := selectedProfile(manager, "./same"); err != nil || profile.Address != "https://external.example" {
		t.Fatalf("explicit file profile = %#v, %v", profile, err)
	}
	save(filepath.Join(working, "external"), "external", "https://external-only.example")
	if profile, err := selectedProfile(manager, "external"); err != nil || profile.Address != "https://external-only.example" {
		t.Fatalf("bare file profile = %#v, %v", profile, err)
	}
	if err := os.WriteFile(filepath.Join(working, "invalid"), []byte("not JSON\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := selectedProfile(manager, "invalid"); err == nil || !strings.Contains(err.Error(), "not a valid profile") {
		t.Fatalf("invalid bare file profile: %v", err)
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
	baseTime := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	root := state.AddFolder("Document/Documents", "Documents", "root", baseTime)
	archive := state.AddFolder("Document/Documents/Archive", "Archive", root.ID, baseTime)
	state.AddFolder("Document/Examples", "Examples", "root", baseTime)
	sourceContent := []byte("%PDF-1.4\nsource\n")
	state.AddDocument("Document/root.pdf", "root.pdf", "root", sourceContent, baseTime)
	state.AddDocument("Document/Documents/source.pdf", "source.pdf", root.ID, sourceContent, baseTime.Add(-4*time.Hour))
	state.AddDocument("Document/Documents/Archive/old.pdf", "old.pdf", archive.ID, sourceContent, baseTime.Add(-6*time.Hour))
	state.AddDocument("Document/Documents/年次 報告 2026.pdf", "年次 報告 2026.pdf", root.ID, sourceContent, baseTime.Add(-3*time.Hour))
	state.AddDocument("Document/Documents/zebra.pdf", "zebra.pdf", root.ID, sourceContent, baseTime.Add(-2*time.Hour))
	state.AddDocument("Document/Documents/zenith.pdf", "zenith.pdf", root.ID, sourceContent, baseTime.Add(-1*time.Hour))
	state.AddDocument("Document/Documents/literal*.pdf", "literal*.pdf", root.ID, sourceContent, baseTime.Add(-5*time.Hour))
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
		all := append([]string{"-config-dir", filepath.Join(temporary, "external-profile-config"), "-profile", profilePath}, arguments...)
		if code := run(all, &stdout, &stderr); code != 0 {
			t.Fatalf("dp %s: code=%d stdout=%q stderr=%q", strings.Join(arguments, " "), code, stdout.String(), stderr.String())
		}
		if strings.Contains(stdout.String(), "Document/") {
			t.Fatalf("dp %s exposed protocol root: %q", strings.Join(arguments, " "), stdout.String())
		}
		return stdout.String()
	}
	referenceFor := func(listing, name string) (string, string) {
		t.Helper()
		for _, line := range strings.Split(listing, "\n") {
			if !strings.HasSuffix(strings.TrimSpace(line), name) {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[0], fields[1]
			}
		}
		t.Fatalf("no reference for %q in %q", name, listing)
		return "", ""
	}

	invoke("auth")
	invoke("device")
	if output := invoke("ls"); !strings.Contains(output, "Documents/") {
		t.Fatalf("root ls = %q", output)
	}
	if output := invoke("ls", "-l", "*"); !strings.Contains(output, "Documents/") || !strings.Contains(output, "root.pdf") {
		t.Fatalf("root wildcard ls = %q", output)
	}
	if output := invoke("ls", "-l", "*/"); !strings.Contains(output, "Documents/") || strings.Contains(output, "root.pdf") {
		t.Fatalf("root directory-only wildcard ls = %q", output)
	}
	if output := invoke("ls", "*"); strings.Contains(output, "source.pdf") || strings.Contains(output, "old.pdf") {
		t.Fatalf("ordinary glob recursed unexpectedly = %q", output)
	}
	for _, flags := range [][]string{{"-t"}, {"-lt"}, {"-tl"}} {
		arguments := append(append([]string{"ls"}, flags...), "/Documents")
		output := invoke(arguments...)
		previous := -1
		for _, name := range []string{"zenith.pdf", "zebra.pdf", "年次 報告 2026.pdf", "source.pdf", "literal*.pdf"} {
			index := strings.Index(output, name)
			if index <= previous {
				t.Fatalf("time-sorted ls %v at %q: %q", flags, name, output)
			}
			previous = index
		}
	}
	for _, flags := range [][]string{{"-R"}, {"-lR"}, {"-Rl"}, {"-l", "-R"}, {"-R", "-l"}, {"-ltR"}, {"-Rtl"}, {"-l", "-t", "-R"}} {
		arguments := append(append([]string{"ls"}, flags...), "/Documents")
		output := invoke(arguments...)
		if !strings.Contains(output, "/Documents:\n") || !strings.Contains(output, "/Documents/Archive:\n") || !strings.Contains(output, "old.pdf") {
			t.Fatalf("recursive ls %v = %q", flags, output)
		}
	}
	if output := invoke("ls", "-lR", "/"); !strings.Contains(output, "/:\n") || !strings.Contains(output, "/Documents:\n") || !strings.Contains(output, "root.pdf") {
		t.Fatalf("recursive root ls = %q", output)
	}
	if output := invoke("file", "*/"); !strings.Contains(output, `"type": "folder"`) || strings.Contains(output, `"name": "root.pdf"`) {
		t.Fatalf("root directory-only file glob = %q", output)
	}
	var quotingOutput, quotingErrors bytes.Buffer
	quotingArgs := []string{"-config-dir", filepath.Join(temporary, "external-profile-config"), "-profile", profilePath, "ls", "-l", "host-one", "host-two"}
	if code := run(quotingArgs, &quotingOutput, &quotingErrors); code != 2 || !strings.Contains(quotingErrors.String(), "quote glob patterns") {
		t.Fatalf("unquoted glob hint: code=%d stdout=%q stderr=%q", code, quotingOutput.String(), quotingErrors.String())
	}
	longListing := invoke("ls", "-l", "Documents/")
	if !strings.Contains(longListing, "source.pdf") || !strings.Contains(longListing, "doc-") {
		output := longListing
		t.Fatalf("long ls = %q", output)
	}
	number, hexID := referenceFor(longListing, "source.pdf")
	if _, err := strconv.ParseUint(number, 10, 64); err != nil || !strings.HasPrefix(hexID, "0x") {
		t.Fatalf("long ls references = %q", longListing)
	}
	invoke("file", "--id", number)
	invoke("stat", "--id", hexID)
	if output := invoke("file", "--glob", "/Documents/*source.pdf"); !strings.HasPrefix(strings.TrimSpace(output), "[") {
		t.Fatalf("explicit file glob is not a JSON array: %q", output)
	}
	if output := invoke("file", "/Documents/z*.pdf"); !strings.HasPrefix(strings.TrimSpace(output), "[") || !strings.Contains(output, "/Documents/zebra.pdf") || !strings.Contains(output, "/Documents/zenith.pdf") {
		t.Fatalf("automatic file glob = %q", output)
	}
	if output := invoke("stat", "/Documents/z*.pdf"); !strings.HasPrefix(strings.TrimSpace(output), "[") {
		t.Fatalf("automatic stat glob is not a JSON array: %q", output)
	}
	if output := invoke("file", "/Documents/literal*.pdf"); !strings.HasPrefix(strings.TrimSpace(output), "{") {
		t.Fatalf("literal metacharacter path did not win over glob: %q", output)
	}
	if output := invoke("ls", "-l", "/Documents/z*.pdf"); !strings.Contains(output, "zebra.pdf") || !strings.Contains(output, "zenith.pdf") {
		t.Fatalf("multiple ls glob = %q", output)
	}
	invoke("file", "--glob", "Documents/*報告*2026.PDF")
	if output := invoke("file", "--glob", "e*"); !strings.Contains(output, `"path": "/Examples"`) {
		t.Fatalf("root-scoped glob = %q", output)
	}
	if output := invoke("file", "--glob", "./E*"); !strings.Contains(output, `"path": "/Examples"`) {
		t.Fatalf("dot-prefixed root glob = %q", output)
	}
	if output := invoke("file", "--glob", "/E*"); !strings.Contains(output, `"path": "/Examples"`) {
		t.Fatalf("absolute root glob = %q", output)
	}
	var ambiguousOutput, ambiguousErrors bytes.Buffer
	ambiguousArgs := []string{"-config-dir", filepath.Join(temporary, "external-profile-config"), "-profile", profilePath, "get", "--glob", "Documents/z*.pdf"}
	if code := run(ambiguousArgs, &ambiguousOutput, &ambiguousErrors); code == 0 || !strings.Contains(ambiguousErrors.String(), "multiple device objects") || !strings.Contains(ambiguousErrors.String(), "Documents/zebra.pdf") || !strings.Contains(ambiguousErrors.String(), "Documents/zenith.pdf") {
		t.Fatalf("ambiguous glob: code=%d stdout=%q stderr=%q", code, ambiguousOutput.String(), ambiguousErrors.String())
	}
	invoke("file", "dOCUMENTS/SOURCE.PDF")
	invoke("file", "Documents/source.pdf")
	invoke("stat", "Documents/source.pdf")
	var confirmedOutput, confirmedErrors bytes.Buffer
	confirmedArgs := []string{"-config-dir", filepath.Join(temporary, "external-profile-config"), "-profile", profilePath, "file", "--glob", "Documents/source.pdf"}
	if code := runWithInput(confirmedArgs, strings.NewReader("y\n"), &confirmedOutput, &confirmedErrors); code != 0 || !strings.Contains(confirmedOutput.String(), `"name": "source.pdf"`) {
		t.Fatalf("confirmed metacharacter-free glob: code=%d stdout=%q stderr=%q", code, confirmedOutput.String(), confirmedErrors.String())
	}
	var cancelledOutput, cancelledErrors bytes.Buffer
	cancelledArgs := []string{"-config-dir", filepath.Join(temporary, "external-profile-config"), "-profile", profilePath, "rm", "--glob", "Documents/source.pdf"}
	if code := runWithInput(cancelledArgs, strings.NewReader(""), &cancelledOutput, &cancelledErrors); code == 0 || !strings.Contains(cancelledErrors.String(), "operation cancelled") {
		t.Fatalf("default-cancelled metacharacter-free glob: code=%d stdout=%q stderr=%q", code, cancelledOutput.String(), cancelledErrors.String())
	}
	invoke("file", "Documents/source.pdf")
	invoke("mkdir", "Documents/Write")
	writeNumber, _ := referenceFor(invoke("ls", "-l", "Documents"), "Write/")
	invoke("cp", "--id", number, "Documents/Write/")
	copyNumber, _ := referenceFor(invoke("ls", "-l", "Documents/Write"), "source.pdf")
	invoke("mv", "--id", copyNumber, "Documents/Write/renamed.pdf")
	if output := invoke("file", "--id", copyNumber); !strings.Contains(output, "Documents/Write/renamed.pdf") {
		t.Fatalf("reference did not survive move: %q", output)
	}

	localUpload := filepath.Join(temporary, "local.pdf")
	uploadContent := []byte("%PDF-1.7\nupload\n")
	if err := os.WriteFile(localUpload, uploadContent, 0o600); err != nil {
		t.Fatal(err)
	}
	invalidDownloadPath := filepath.Join(temporary, "invalid-download.pdf")
	state.InjectFault("GET /documents/*/file", dptest.Fault{Status: 200, Body: "not a PDF", Once: true})
	var invalidOutput, invalidErrors bytes.Buffer
	invalidArgs := []string{"-config-dir", filepath.Join(temporary, "external-profile-config"), "-profile", profilePath, "get", "Documents/source.pdf", invalidDownloadPath}
	if code := run(invalidArgs, &invalidOutput, &invalidErrors); code == 0 || !strings.Contains(invalidErrors.String(), "not a PDF") {
		t.Fatalf("invalid PDF download: code=%d stdout=%q stderr=%q", code, invalidOutput.String(), invalidErrors.String())
	}
	if _, err := os.Stat(invalidDownloadPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid PDF local file remains: %v", err)
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
	invoke("rmdir", "--id", writeNumber)
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
