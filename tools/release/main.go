// Command release creates deterministic DPWire binary and source
// archives. It refuses dirty or untagged production releases by default.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	semverNumeric       = `(0|[1-9][0-9]*)`
	semverPrereleaseID  = `(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)`
	semverBuildMetadata = `[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*`
)

var semanticVersion = regexp.MustCompile(`^` + semverNumeric + `\.` + semverNumeric + `\.` + semverNumeric + `(-` + semverPrereleaseID + `(\.` + semverPrereleaseID + `)*)?(\+` + semverBuildMetadata + `)?$`)

var targets = []target{
	{OS: "darwin", Arch: "arm64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
	{OS: "linux", Arch: "amd64"},
}

var distributionFiles = []string{
	"LICENSE",
	"NOTICE",
	"README.md",
	"docs/cli.md",
	"docs/compatibility.md",
	"docs/install.md",
	"docs/project-rationale-and-comparison.ja.md",
	"docs/project-rationale-and-comparison.md",
	"docs/recovery.md",
	"docs/release-notes.md",
}

type target struct {
	OS   string
	Arch string
}

type options struct {
	Version           string
	Output            string
	VerifyTag         bool
	AllowDirty        bool
	CheckReproducible bool
}

type archiveEntry struct {
	Name string
	Mode fs.FileMode
	Data []byte
}

type artifactRecord struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	OS     string `json:"os,omitempty"`
	Arch   string `json:"arch,omitempty"`
	SHA256 string `json:"sha256"`
}

type releaseManifest struct {
	SchemaVersion int              `json:"schema_version"`
	Version       string           `json:"version"`
	Commit        string           `json:"commit"`
	GoVersion     string           `json:"go_version"`
	Artifacts     []artifactRecord `json:"artifacts"`
}

func main() {
	version := flag.String("version", "", "semantic version, with optional leading v")
	output := flag.String("output", "dist", "new output directory")
	verifyTag := flag.Bool("verify-tag", false, "require HEAD to have the exact version tag")
	allowDirty := flag.Bool("allow-dirty", false, "allow a modified worktree (testing only)")
	checkReproducible := flag.Bool("check-reproducible", false, "build twice in temporary directories and compare")
	flag.Parse()

	err := run(context.Background(), options{
		Version: *version, Output: *output, VerifyTag: *verifyTag,
		AllowDirty: *allowDirty, CheckReproducible: *checkReproducible,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opts options) error {
	version, err := normalizeVersion(opts.Version)
	if err != nil {
		return err
	}
	if !opts.AllowDirty {
		if err := requireCleanWorktree(ctx); err != nil {
			return err
		}
	}
	commit, err := commandOutput(ctx, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("identify commit: %w", err)
	}
	if opts.VerifyTag {
		tag, err := commandOutput(ctx, "git", "describe", "--tags", "--exact-match", "HEAD")
		if err != nil || tag != "v"+version {
			return fmt.Errorf("HEAD must have exact tag v%s", version)
		}
	}

	build := func(directory string) error {
		return buildRelease(ctx, directory, version, commit)
	}
	if opts.CheckReproducible {
		first, err := os.MkdirTemp("", "dpwire-release-a-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(first)
		second, err := os.MkdirTemp("", "dpwire-release-b-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(second)
		if err := build(first); err != nil {
			return err
		}
		if err := build(second); err != nil {
			return err
		}
		if err := compareDirectories(first, second); err != nil {
			return err
		}
		fmt.Println("release archives are reproducible")
		return nil
	}

	output, err := filepath.Abs(opts.Output)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(output); err == nil {
		return fmt.Errorf("output already exists: %s", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".dpwire-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := build(staging); err != nil {
		return err
	}
	if err := os.Rename(staging, output); err != nil {
		return err
	}
	fmt.Println(output)
	return nil
}

func normalizeVersion(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	if !semanticVersion.MatchString(value) {
		return "", errors.New("version must be a valid semantic version")
	}
	return value, nil
}

func requireCleanWorktree(ctx context.Context) error {
	status, err := commandOutput(ctx, "git", "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return fmt.Errorf("inspect worktree: %w", err)
	}
	if status != "" {
		return errors.New("worktree is not clean")
	}
	return nil
}

func commandOutput(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", strings.Join(append([]string{name}, args...), " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func buildRelease(ctx context.Context, output, version, commit string) error {
	records := make([]artifactRecord, 0, len(targets)+1)
	for _, item := range targets {
		binaryDirectory, err := os.MkdirTemp("", "dpwire-binary-")
		if err != nil {
			return err
		}
		binaryPath := filepath.Join(binaryDirectory, "dp")
		err = buildBinary(ctx, binaryPath, version, item)
		if err != nil {
			os.RemoveAll(binaryDirectory)
			return err
		}
		entries, err := binaryArchiveEntries(binaryPath)
		os.RemoveAll(binaryDirectory)
		if err != nil {
			return err
		}
		base := fmt.Sprintf("dpwire-v%s-%s-%s", version, item.OS, item.Arch)
		name := base + ".tar.gz"
		if err := writeArchive(filepath.Join(output, name), base, entries); err != nil {
			return err
		}
		digest, err := fileSHA256(filepath.Join(output, name))
		if err != nil {
			return err
		}
		records = append(records, artifactRecord{Name: name, Kind: "binary", OS: item.OS, Arch: item.Arch, SHA256: digest})
	}

	sourceEntries, err := trackedSourceEntries(ctx)
	if err != nil {
		return err
	}
	sourceBase := fmt.Sprintf("dpwire-v%s-source", version)
	sourceName := sourceBase + ".tar.gz"
	if err := writeArchive(filepath.Join(output, sourceName), sourceBase, sourceEntries); err != nil {
		return err
	}
	digest, err := fileSHA256(filepath.Join(output, sourceName))
	if err != nil {
		return err
	}
	records = append(records, artifactRecord{Name: sourceName, Kind: "source", SHA256: digest})
	sort.Slice(records, func(i, j int) bool { return records[i].Name < records[j].Name })

	manifest := releaseManifest{SchemaVersion: 1, Version: version, Commit: commit, GoVersion: runtime.Version(), Artifacts: records}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := writeNew(filepath.Join(output, "release.json"), append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	return writeChecksums(output)
}

func buildBinary(ctx context.Context, output, version string, item target) error {
	ldflags := "-s -w -buildid= -X main.version=" + version
	command := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-ldflags", ldflags, "-o", output, "./cmd/dp")
	command.Env = append(os.Environ(), targetEnvironment(item)...)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("build dp for %s/%s: %w: %s", item.OS, item.Arch, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func targetEnvironment(item target) []string {
	environment := []string{"CGO_ENABLED=0", "GOOS=" + item.OS, "GOARCH=" + item.Arch}
	switch item.Arch {
	case "amd64":
		environment = append(environment, "GOAMD64=v1")
	case "arm64":
		environment = append(environment, "GOARM64=v8.0")
	}
	return environment
}

func binaryArchiveEntries(binaryPath string) ([]archiveEntry, error) {
	entries := make([]archiveEntry, 0, len(distributionFiles)+1)
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		return nil, err
	}
	entries = append(entries, archiveEntry{Name: "dp", Mode: 0o755, Data: binary})
	for _, name := range distributionFiles {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read distribution file %s: %w", name, err)
		}
		entries = append(entries, archiveEntry{Name: name, Mode: 0o644, Data: data})
	}
	return entries, nil
}

func trackedSourceEntries(ctx context.Context) ([]archiveEntry, error) {
	command := exec.CommandContext(ctx, "git", "ls-files", "-z")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("list tracked source: %w", err)
	}
	names := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
	sort.Strings(names)
	entries := make([]archiveEntry, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		info, err := os.Lstat(name)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("tracked source is not a regular file: %s", name)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		mode := fs.FileMode(0o644)
		if info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		entries = append(entries, archiveEntry{Name: filepath.ToSlash(name), Mode: mode, Data: data})
	}
	return entries, nil
}

func writeArchive(filename, root string, entries []archiveEntry) (err error) {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if !complete || err != nil {
			_ = os.Remove(filename)
		}
	}()

	zipper, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return err
	}
	zipper.Header.ModTime = time.Unix(0, 0).UTC()
	zipper.Header.OS = 255
	writer := tar.NewWriter(zipper)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	for _, entry := range entries {
		name := path.Clean(entry.Name)
		if name == "." || strings.HasPrefix(name, "../") || path.IsAbs(name) {
			return fmt.Errorf("unsafe archive path: %s", entry.Name)
		}
		header := &tar.Header{
			Name: path.Join(root, name), Mode: int64(entry.Mode.Perm()), Size: int64(len(entry.Data)),
			ModTime: time.Unix(0, 0).UTC(), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR,
		}
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if _, err := writer.Write(entry.Data); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if err := zipper.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func writeChecksums(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	var lines []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "SHA256SUMS" {
			continue
		}
		digest, err := fileSHA256(filepath.Join(directory, entry.Name()))
		if err != nil {
			return err
		}
		lines = append(lines, digest+"  "+entry.Name())
	}
	sort.Strings(lines)
	return writeNew(filepath.Join(directory, "SHA256SUMS"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func fileSHA256(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeNew(filename string, data []byte, mode fs.FileMode) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func compareDirectories(first, second string) error {
	read := func(root string) (map[string][sha256.Size]byte, error) {
		result := make(map[string][sha256.Size]byte)
		err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			data, err := os.ReadFile(filename)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, filename)
			if err != nil {
				return err
			}
			result[filepath.ToSlash(relative)] = sha256.Sum256(data)
			return nil
		})
		return result, err
	}
	a, err := read(first)
	if err != nil {
		return err
	}
	b, err := read(second)
	if err != nil {
		return err
	}
	if len(a) != len(b) {
		return errors.New("release output file counts differ")
	}
	for name, digest := range a {
		if other, ok := b[name]; !ok || !bytes.Equal(digest[:], other[:]) {
			return fmt.Errorf("release output is not reproducible: %s", name)
		}
	}
	return nil
}
