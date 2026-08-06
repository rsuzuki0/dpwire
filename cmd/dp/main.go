package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/rsuzuki0/digitalpaper"
	"github.com/rsuzuki0/digitalpaper/credentials"
)

const version = "0.1.0-p1"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("dp", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "", "device profile JSON file")
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
		certificate, err := digitalpaper.InspectUntrustedCertificate(context.Background(), args[1])
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
	if *profilePath == "" {
		fmt.Fprintln(stderr, "dp: -profile is required")
		return 2
	}
	profile, err := digitalpaper.LoadProfile(*profilePath)
	if err != nil {
		return report(stderr, err)
	}
	client, err := digitalpaper.NewClient(profile)
	if err != nil {
		return report(stderr, err)
	}
	ctx := context.Background()
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
			Firmware digitalpaper.FirmwareStatus `json:"firmware"`
			Battery  digitalpaper.BatteryStatus  `json:"battery"`
			Storage  digitalpaper.StorageStatus  `json:"storage"`
		}{firmware, battery, storage})
	case "ls":
		if len(args) > 2 {
			usage(stderr)
			return 2
		}
		var entries []digitalpaper.Entry
		if len(args) == 2 {
			entries, err = client.Folders.List(ctx, args[1], digitalpaper.ListOptions{})
		} else {
			entries, err = client.Documents.List(ctx, digitalpaper.ListOptions{})
		}
		if err != nil {
			return report(stderr, err)
		}
		return encode(stdout, entries)
	case "stat":
		if len(args) != 2 {
			usage(stderr)
			return 2
		}
		path, err := digitalpaper.ParseRemotePath(args[1])
		if err != nil {
			return report(stderr, err)
		}
		entry, err := client.Documents.Resolve(ctx, path)
		if err != nil {
			return report(stderr, err)
		}
		return encode(stdout, entry)
	case "get":
		if len(args) != 3 {
			usage(stderr)
			return 2
		}
		return get(ctx, client, args[1], args[2], stdout, stderr)
	default:
		usage(stderr)
		return 2
	}
}

func get(ctx context.Context, client *digitalpaper.Client, remote, local string, stdout, stderr io.Writer) int {
	path, err := digitalpaper.ParseRemotePath(remote)
	if err != nil {
		return report(stderr, err)
	}
	entry, err := client.Documents.Resolve(ctx, path)
	if err != nil {
		return report(stderr, err)
	}
	if entry.Type != digitalpaper.EntryDocument {
		return report(stderr, errors.New("remote path is not a document"))
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
	fmt.Fprintln(output, "usage: dp [-profile FILE] COMMAND [ARG...]")
	fmt.Fprintln(output, "commands:")
	fmt.Fprintln(output, "  version                         print version")
	fmt.Fprintln(output, "  inspect-cert ADDRESS            inspect untrusted first-contact certificate")
	fmt.Fprintln(output, "  credentials find ROOT           list existing Sony credential pairs")
	fmt.Fprintln(output, "  auth                            verify profile authentication")
	fmt.Fprintln(output, "  device                          show firmware, battery, and storage")
	fmt.Fprintln(output, "  ls [FOLDER_ID]                  list documents or direct folder entries")
	fmt.Fprintln(output, "  stat REMOTE_PATH                resolve entry metadata")
	fmt.Fprintln(output, "  get REMOTE_PATH LOCAL_FILE      download PDF without overwriting")
}
