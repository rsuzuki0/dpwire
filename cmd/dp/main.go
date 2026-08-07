package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/rsuzuki0/dpwire"
	"github.com/rsuzuki0/dpwire/credentials"
	"github.com/rsuzuki0/dpwire/profiles"
)

var version = "0.3.0"

func main() { os.Exit(runWithInput(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(arguments []string, stdout, stderr io.Writer) int {
	return runWithInput(arguments, strings.NewReader(""), stdout, stderr)
}

func runWithInput(arguments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("dp", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profileSelection := flags.String("profile", "", "named profile or profile JSON file")
	configDirectory := flags.String("config-dir", "", "profile configuration directory")
	flags.Usage = func() { usage(stderr) }
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	args := flags.Args()
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	if args[0] == "version" && len(args) == 1 {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if args[0] == "inspect-cert" && len(args) == 2 {
		certificate, err := dpwire.InspectUntrustedCertificate(context.Background(), args[1])
		if err != nil {
			return report(stderr, err)
		}
		return encode(stdout, map[string]string{"sha256": certificate.SHA256, "subject_common_name": certificate.SubjectCommonName})
	}
	if args[0] == "credentials" && len(args) == 3 && args[1] == "find" {
		candidates, err := credentials.FindSonyCandidates(args[2])
		if err != nil {
			return report(stderr, err)
		}
		return encode(stdout, candidates)
	}
	manager, err := profileManager(*configDirectory)
	if err != nil {
		return report(stderr, err)
	}
	ctx := context.Background()
	if args[0] == "profile" {
		return profileCommand(ctx, manager, args, stdin, stdout, stderr)
	}
	profile, err := selectedProfile(manager, *profileSelection)
	if err != nil {
		return report(stderr, err)
	}
	client, err := dpwire.NewClient(profile)
	if err != nil {
		return report(stderr, err)
	}
	if err := client.Authenticate(ctx); err != nil {
		return report(stderr, err)
	}

	switch args[0] {
	case "auth":
		if len(args) != 1 {
			usage(stderr)
			return 2
		}
		fmt.Fprintln(stdout, "authenticated")
		return 0
	case "device":
		if len(args) != 1 {
			usage(stderr)
			return 2
		}
		firmware, err := client.Device.Firmware(ctx)
		if err != nil {
			return report(stderr, err)
		}
		battery, err := client.Device.Battery(ctx)
		if err != nil {
			return report(stderr, err)
		}
		storage, err := client.Device.Storage(ctx)
		if err != nil {
			return report(stderr, err)
		}
		return encode(stdout, struct {
			Firmware dpwire.FirmwareStatus `json:"firmware"`
			Battery  dpwire.BatteryStatus  `json:"battery"`
			Storage  dpwire.StorageStatus  `json:"storage"`
		}{firmware, battery, storage})
	case "ls":
		long, target, ok := parseListArguments(args[1:])
		if !ok {
			usage(stderr)
			return 2
		}
		var entries []dpwire.Entry
		if target != "" {
			path, pathErr := parseDevicePath(target)
			if pathErr != nil {
				return report(stderr, pathErr)
			}
			folder, resolveErr := client.Documents.Resolve(ctx, path)
			if resolveErr != nil {
				return report(stderr, resolveErr)
			}
			if folder.Type == dpwire.EntryDocument {
				entries = []dpwire.Entry{folder}
			} else {
				entries, err = client.Folders.List(ctx, folder.ID, dpwire.ListOptions{})
			}
		} else {
			root, resolveErr := client.Documents.Resolve(ctx, dpwire.MustRemotePath("Document"))
			if resolveErr != nil {
				return report(stderr, resolveErr)
			}
			entries, err = client.Folders.List(ctx, root.ID, dpwire.ListOptions{})
		}
		if err != nil {
			return report(stderr, err)
		}
		return printEntries(stdout, entries, long)
	case "stat", "file":
		if len(args) != 2 {
			usage(stderr)
			return 2
		}
		path, err := parseDevicePath(args[1])
		if err != nil {
			return report(stderr, err)
		}
		entry, err := client.Documents.Resolve(ctx, path)
		if err != nil {
			return report(stderr, err)
		}
		return encode(stdout, presentEntry(entry))
	case "get":
		if len(args) < 2 || len(args) > 3 {
			usage(stderr)
			return 2
		}
		local := ""
		if len(args) == 3 {
			local = args[2]
		}
		return get(ctx, client, args[1], local, stdout, stderr)
	case "mkdir":
		if len(args) != 2 {
			usage(stderr)
			return 2
		}
		parent, name, err := splitRemoteTarget(args[1])
		if err != nil {
			return report(stderr, err)
		}
		parentEntry, err := client.Documents.Resolve(ctx, parent)
		if err != nil {
			return report(stderr, err)
		}
		entry, err := client.Folders.Create(ctx, parentEntry.ID, name)
		if err != nil {
			return report(stderr, err)
		}
		return encode(stdout, presentEntry(entry))
	case "put":
		if len(args) < 2 || len(args) > 3 {
			usage(stderr)
			return 2
		}
		remote := filepath.Base(args[1])
		if len(args) == 3 {
			remote = args[2]
		}
		return put(ctx, client, args[1], remote, stdout, stderr)
	case "cp", "mv":
		if len(args) != 3 {
			usage(stderr)
			return 2
		}
		sourcePath, err := parseDevicePath(args[1])
		if err != nil {
			return report(stderr, err)
		}
		source, err := client.Documents.Resolve(ctx, sourcePath)
		if err != nil {
			return report(stderr, err)
		}
		if source.Type != dpwire.EntryDocument {
			return report(stderr, errors.New("source path is not a PDF"))
		}
		parentEntry, name, err := resolveDestination(ctx, client, args[2], source.Name)
		if err != nil {
			return report(stderr, err)
		}
		var entry dpwire.Entry
		if args[0] == "cp" {
			entry, err = client.Documents.Copy(ctx, source.ID, parentEntry.ID, name)
		} else {
			entry, err = client.Documents.Update(ctx, source.ID, parentEntry.ID, name)
		}
		if err != nil {
			return report(stderr, err)
		}
		return encode(stdout, presentEntry(entry))
	case "rm", "rmdir":
		if len(args) != 2 {
			usage(stderr)
			return 2
		}
		path, err := parseDevicePath(args[1])
		if err != nil {
			return report(stderr, err)
		}
		entry, err := client.Documents.Resolve(ctx, path)
		if err != nil {
			return report(stderr, err)
		}
		if args[0] == "rm" {
			if entry.Type != dpwire.EntryDocument {
				return report(stderr, errors.New("path is a folder; use rmdir"))
			}
			if err := client.Documents.Delete(ctx, entry.ID, entry.Revision); err != nil {
				return report(stderr, err)
			}
		} else {
			if path.String() == "Document" {
				return report(stderr, errors.New("device root cannot be deleted"))
			}
			if entry.Type != dpwire.EntryFolder {
				return report(stderr, errors.New("path is not a folder; use rm"))
			}
			if err := client.Folders.DeleteEmpty(ctx, entry.ID); err != nil {
				return report(stderr, err)
			}
		}
		return encode(stdout, map[string]string{"removed": devicePathString(path)})
	case "open":
		if len(args) < 2 || len(args) > 3 {
			usage(stderr)
			return 2
		}
		path, err := parseDevicePath(args[1])
		if err != nil {
			return report(stderr, err)
		}
		entry, err := client.Documents.Resolve(ctx, path)
		if err != nil {
			return report(stderr, err)
		}
		page := 0
		if len(args) == 3 {
			if _, err := fmt.Sscan(args[2], &page); err != nil || page < 1 {
				return report(stderr, errors.New("page must be a positive integer"))
			}
		}
		if err := client.Documents.Open(ctx, entry.ID, page); err != nil {
			return report(stderr, err)
		}
		return encode(stdout, map[string]any{"opened": devicePathString(path), "page": page})
	default:
		usage(stderr)
		return 2
	}
}

func profileManager(directory string) (*profiles.Manager, error) {
	if directory == "" {
		var err error
		directory, err = profiles.DefaultRoot()
		if err != nil {
			return nil, err
		}
	} else if !filepath.IsAbs(directory) {
		absolute, err := filepath.Abs(directory)
		if err != nil {
			return nil, err
		}
		directory = absolute
	}
	return profiles.New(directory)
}

func selectedProfile(manager *profiles.Manager, selection string) (dpwire.DeviceProfile, error) {
	if selection == "" {
		_, profile, err := manager.Current()
		if errors.Is(err, os.ErrNotExist) {
			return dpwire.DeviceProfile{}, errors.New("no default profile; run 'dp profile import-sony' or select -profile")
		}
		return profile, err
	}
	if filepath.IsAbs(selection) || strings.ContainsRune(selection, filepath.Separator) {
		return dpwire.LoadProfile(selection)
	}
	if _, err := os.Stat(selection); err == nil {
		return dpwire.LoadProfile(selection)
	}
	return manager.Load(selection)
}

func profileCommand(ctx context.Context, manager *profiles.Manager, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 2 && args[1] == "list" {
		items, err := manager.List()
		if err != nil {
			return report(stderr, err)
		}
		writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		for _, item := range items {
			marker := " "
			if item.Current {
				marker = "*"
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", marker, item.Name, item.Connection, item.Address)
		}
		if err := writer.Flush(); err != nil {
			return 1
		}
		return 0
	}
	if len(args) == 3 && args[1] == "use" {
		if err := manager.Use(args[2]); err != nil {
			return report(stderr, err)
		}
		fmt.Fprintln(stdout, args[2])
		return 0
	}
	if (len(args) == 2 || len(args) == 3) && args[1] == "show" {
		name := ""
		var profile dpwire.DeviceProfile
		var err error
		if len(args) == 3 {
			name = args[2]
			profile, err = manager.Load(name)
		} else {
			name, profile, err = manager.Current()
		}
		if err != nil {
			return report(stderr, err)
		}
		currentName, _, currentErr := manager.Current()
		return encode(stdout, profiles.Summary{Name: name, Address: profile.Address, Connection: profile.EffectiveConnection(), Current: currentErr == nil && currentName == name})
	}
	if len(args) == 6 && args[1] == "import-sony" {
		profile, err := manager.ImportSony(args[2], args[3], args[4], args[5])
		if err != nil {
			return report(stderr, err)
		}
		fmt.Fprintln(stdout, profile.Name)
		return 0
	}
	if len(args) == 4 && args[1] == "pair" {
		reader := bufio.NewReader(io.LimitReader(stdin, 1024))
		profile, err := manager.Pair(ctx, args[2], args[3], func(context.Context) (string, error) {
			fmt.Fprint(stderr, "Enter the PIN displayed on the device: ")
			value, readErr := reader.ReadString('\n')
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return "", readErr
			}
			return strings.TrimSpace(value), nil
		})
		if err != nil {
			return report(stderr, err)
		}
		fmt.Fprintln(stdout, profile.Name)
		return 0
	}
	usage(stderr)
	return 2
}

func put(ctx context.Context, client *dpwire.Client, local, remote string, stdout, stderr io.Writer) int {
	parentEntry, name, err := resolveDestination(ctx, client, remote, filepath.Base(local))
	if err != nil {
		return report(stderr, err)
	}
	file, err := os.Open(local)
	if err != nil {
		return report(stderr, err)
	}
	defer file.Close()
	entry, result, err := client.Documents.CreateAndUpload(ctx, parentEntry.ID, name, file)
	if err != nil {
		return report(stderr, err)
	}
	return encode(stdout, map[string]any{"entry": presentEntry(entry), "upload": result})
}

func splitRemoteTarget(value string) (dpwire.RemotePath, string, error) {
	path, err := parseDevicePath(value)
	if err != nil {
		return dpwire.RemotePath{}, "", err
	}
	index := strings.LastIndex(path.String(), "/")
	if index < 0 {
		return dpwire.RemotePath{}, "", errors.New("remote target must be below Document")
	}
	parent, err := dpwire.ParseRemotePath(path.String()[:index])
	if err != nil {
		return dpwire.RemotePath{}, "", err
	}
	return parent, path.String()[index+1:], nil
}

// parseDevicePath keeps the protocol's Document root out of the CLI.
func parseDevicePath(value string) (dpwire.RemotePath, error) {
	if value == "." {
		return dpwire.MustRemotePath("Document"), nil
	}
	if value == "Document" || strings.HasPrefix(value, "Document/") {
		return dpwire.RemotePath{}, errors.New("device paths must omit the internal Document/ prefix")
	}
	return dpwire.ParseRemotePath("Document/" + value)
}

func devicePathString(path dpwire.RemotePath) string {
	if path.String() == "Document" {
		return "."
	}
	return strings.TrimPrefix(path.String(), "Document/")
}

func resolveDestination(ctx context.Context, client *dpwire.Client, value, defaultName string) (dpwire.Entry, string, error) {
	path, err := parseDevicePath(value)
	if err != nil {
		return dpwire.Entry{}, "", err
	}
	destination, err := client.Documents.Resolve(ctx, path)
	if err == nil {
		if destination.Type != dpwire.EntryFolder {
			return dpwire.Entry{}, "", errors.New("destination already exists; overwrite is disabled")
		}
		return destination, defaultName, nil
	}
	var apiError *dpwire.APIError
	if !errors.As(err, &apiError) || apiError.Code != "40401" {
		return dpwire.Entry{}, "", err
	}
	parent, name, err := splitRemoteTarget(value)
	if err != nil {
		return dpwire.Entry{}, "", err
	}
	parentEntry, err := client.Documents.Resolve(ctx, parent)
	if err != nil {
		return dpwire.Entry{}, "", err
	}
	if parentEntry.Type != dpwire.EntryFolder {
		return dpwire.Entry{}, "", errors.New("destination parent is not a folder")
	}
	return parentEntry, name, nil
}

type entryOutput struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Path           string           `json:"path"`
	Type           dpwire.EntryType `json:"type"`
	Created        string           `json:"created,omitempty"`
	Modified       string           `json:"modified,omitempty"`
	MIMEType       string           `json:"mime_type,omitempty"`
	Size           string           `json:"size,omitempty"`
	DocumentType   string           `json:"document_type,omitempty"`
	Author         string           `json:"author,omitempty"`
	Title          string           `json:"title,omitempty"`
	TotalPages     string           `json:"total_pages,omitempty"`
	CurrentPage    string           `json:"current_page,omitempty"`
	ReadingDate    string           `json:"reading_date,omitempty"`
	ParentFolderID string           `json:"parent_folder_id,omitempty"`
	IsNew          string           `json:"is_new,omitempty"`
	DocumentSource string           `json:"document_source,omitempty"`
	ExternalID     string           `json:"external_id,omitempty"`
	FileHash       string           `json:"file_hash,omitempty"`
	Revision       string           `json:"revision,omitempty"`
}

func presentEntry(entry dpwire.Entry) entryOutput {
	return entryOutput{
		ID: entry.ID, Name: entry.Name, Path: devicePathString(entry.Path), Type: entry.Type,
		Created: entry.Created, Modified: entry.Modified, MIMEType: entry.MIMEType, Size: entry.Size,
		DocumentType: entry.DocumentType, Author: entry.Author, Title: entry.Title,
		TotalPages: entry.TotalPages, CurrentPage: entry.CurrentPage, ReadingDate: entry.ReadingDate,
		ParentFolderID: entry.ParentFolderID, IsNew: entry.IsNew, DocumentSource: entry.DocumentSource,
		ExternalID: entry.ExternalID, FileHash: entry.FileHash, Revision: entry.Revision,
	}
}

func parseListArguments(arguments []string) (long bool, target string, ok bool) {
	switch len(arguments) {
	case 0:
		return false, "", true
	case 1:
		if arguments[0] == "-l" {
			return true, "", true
		}
		if strings.HasPrefix(arguments[0], "-") {
			return false, "", false
		}
		return false, arguments[0], true
	case 2:
		if arguments[0] == "-l" {
			return true, arguments[1], true
		}
	}
	return false, "", false
}

func printEntries(output io.Writer, entries []dpwire.Entry, long bool) int {
	if !long {
		for _, entry := range entries {
			name := entry.Name
			if entry.Type == dpwire.EntryFolder {
				name += "/"
			}
			fmt.Fprintln(output, name)
		}
		return 0
	}
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	for _, entry := range entries {
		kind, size, modified, name := "-", entry.Size, entry.Modified, entry.Name
		if entry.Type == dpwire.EntryFolder {
			kind, size, name = "d", "-", name+"/"
		}
		if size == "" {
			size = "-"
		}
		if modified == "" {
			modified = "-"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", kind, size, modified, entry.ID, name)
	}
	if err := writer.Flush(); err != nil {
		return 1
	}
	return 0
}

func get(ctx context.Context, client *dpwire.Client, remote, local string, stdout, stderr io.Writer) int {
	path, err := parseDevicePath(remote)
	if err != nil {
		return report(stderr, err)
	}
	entry, err := client.Documents.Resolve(ctx, path)
	if err != nil {
		return report(stderr, err)
	}
	if entry.Type != dpwire.EntryDocument {
		return report(stderr, errors.New("remote path is not a document"))
	}
	if local == "" {
		local = entry.Name
	}
	file, err := os.OpenFile(local, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return report(stderr, err)
	}
	result, downloadErr := client.Documents.Download(ctx, entry.ID, file)
	closeErr := file.Close()
	if downloadErr != nil || closeErr != nil {
		_ = os.Remove(local)
		if downloadErr != nil {
			return report(stderr, downloadErr)
		}
		return report(stderr, closeErr)
	}
	return encode(stdout, map[string]any{"path": local, "bytes": result.Bytes, "etag": result.ETag})
}

func encode(output io.Writer, value any) int {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return 1
	}
	return 0
}

func report(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "dp:", err)
	return 1
}

func usage(output io.Writer) {
	fmt.Fprintln(output, "usage: dp [-profile NAME|FILE] COMMAND [ARG...]")
	fmt.Fprintln(output, "commands:")
	fmt.Fprintln(output, "  version                         print version")
	fmt.Fprintln(output, "  inspect-cert ADDRESS            inspect untrusted first-contact certificate")
	fmt.Fprintln(output, "  credentials find ROOT           list existing Sony credential pairs")
	fmt.Fprintln(output, "  profile import-sony NAME ADDRESS SHA256 CREDENTIAL_DIR")
	fmt.Fprintln(output, "  profile pair NAME DIRECT_ADDRESS  register a new direct client identity")
	fmt.Fprintln(output, "  profile list                    list configured devices")
	fmt.Fprintln(output, "  profile use NAME                select the default device")
	fmt.Fprintln(output, "  profile show [NAME]             show safe connection details")
	fmt.Fprintln(output, "  auth                            verify profile authentication")
	fmt.Fprintln(output, "  device                          show firmware, battery, and storage")
	fmt.Fprintln(output, "  ls [-l] [DEVICE_PATH]           list the root, a folder, or one PDF")
	fmt.Fprintln(output, "  stat DEVICE_PATH                show complete entry metadata")
	fmt.Fprintln(output, "  file DEVICE_PATH                alias for stat")
	fmt.Fprintln(output, "  get DEVICE_PATH [LOCAL_FILE]    download PDF without overwriting")
	fmt.Fprintln(output, "  mkdir DEVICE_PATH               create one folder")
	fmt.Fprintln(output, "  put LOCAL_PDF [DEVICE_PATH]     create and upload without overwriting")
	fmt.Fprintln(output, "  cp SOURCE_PATH DEST_PATH        copy a PDF within the device")
	fmt.Fprintln(output, "  mv SOURCE_PATH DEST_PATH        move or rename a PDF within the device")
	fmt.Fprintln(output, "  rm DEVICE_PATH                  remove one PDF with a revision guard")
	fmt.Fprintln(output, "  rmdir DEVICE_PATH               remove one empty folder only")
	fmt.Fprintln(output, "  open DEVICE_PATH [PAGE]         display a PDF on the device")
	fmt.Fprintln(output, "device paths are root-relative; use . for the root, for example Codex_dp/paper.pdf")
}
