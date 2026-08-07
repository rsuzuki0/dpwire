package main

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/rsuzuki0/dpwire"
	"github.com/rsuzuki0/dpwire/profiles"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const maximumGlobEntries = 10000

type selectorKind int

const (
	selectorPath selectorKind = iota
	selectorID
	selectorGlob
)

type objectSelector struct {
	kind  selectorKind
	value string
}

func parseObjectSelector(arguments []string) (objectSelector, []string, error) {
	if len(arguments) == 0 {
		return objectSelector{}, nil, errors.New("missing device object")
	}
	if arguments[0] == "--id" || arguments[0] == "--glob" {
		if len(arguments) < 2 || arguments[1] == "" {
			return objectSelector{}, nil, fmt.Errorf("%s requires a value", arguments[0])
		}
		kind := selectorID
		if arguments[0] == "--glob" {
			kind = selectorGlob
		}
		return objectSelector{kind: kind, value: arguments[1]}, arguments[2:], nil
	}
	if strings.HasPrefix(arguments[0], "-") {
		return objectSelector{}, nil, fmt.Errorf("unknown selector %q", arguments[0])
	}
	return objectSelector{kind: selectorPath, value: arguments[0]}, arguments[1:], nil
}

func resolveObject(ctx context.Context, client *dpwire.Client, store *profiles.ObjectReferenceStore, selector objectSelector, expected dpwire.EntryType) (dpwire.Entry, error) {
	var candidates []dpwire.Entry
	switch selector.kind {
	case selectorPath:
		remotePath, err := parseDevicePath(selector.value)
		if err != nil {
			return dpwire.Entry{}, err
		}
		entry, err := client.Documents.Resolve(ctx, remotePath)
		if err != nil {
			return dpwire.Entry{}, err
		}
		candidates = []dpwire.Entry{entry}
	case selectorID:
		if store == nil {
			return dpwire.Entry{}, errors.New("object reference store is unavailable")
		}
		references, err := store.Candidates(selector.value)
		if err != nil {
			return dpwire.Entry{}, err
		}
		for _, reference := range references {
			if expected != "" && reference.Type != expected {
				continue
			}
			entry, getErr := getObjectByReference(ctx, client, reference)
			if getErr != nil {
				if isNotFound(getErr) {
					continue
				}
				return dpwire.Entry{}, getErr
			}
			candidates = append(candidates, entry)
		}
	case selectorGlob:
		if store == nil {
			return dpwire.Entry{}, errors.New("object reference store is unavailable")
		}
		pattern := foldGlobString(selector.value)
		if _, err := path.Match(pattern, ""); err != nil {
			return dpwire.Entry{}, fmt.Errorf("invalid glob pattern: %w", err)
		}
		entries, err := globDevice(ctx, client, pattern)
		if err != nil {
			return dpwire.Entry{}, err
		}
		for _, entry := range entries {
			if expected != "" && entry.Type != expected {
				continue
			}
			candidates = append(candidates, entry)
		}
	default:
		return dpwire.Entry{}, errors.New("invalid object selector")
	}

	if expected != "" {
		if selector.kind == selectorPath && len(candidates) == 1 && candidates[0].Type != expected {
			return dpwire.Entry{}, fmt.Errorf("selected object is a %s; command requires a %s", candidates[0].Type, expected)
		}
		filtered := candidates[:0]
		for _, entry := range candidates {
			if entry.Type == expected {
				filtered = append(filtered, entry)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		kind := "device object"
		if expected != "" {
			kind = string(expected)
		}
		return dpwire.Entry{}, fmt.Errorf("no %s matches %s", kind, describeSelector(selector))
	}
	if len(candidates) > 1 {
		return dpwire.Entry{}, ambiguousObjectError(store, selector, candidates)
	}
	return candidates[0], nil
}

func foldGlobString(value string) string {
	return norm.NFC.String(cases.Fold().String(norm.NFC.String(value)))
}

func getObjectByReference(ctx context.Context, client *dpwire.Client, reference profiles.ObjectReference) (dpwire.Entry, error) {
	if reference.Type == dpwire.EntryFolder {
		return client.Folders.Get(ctx, reference.DeviceID)
	}
	return client.Documents.Get(ctx, reference.DeviceID)
}

func isNotFound(err error) bool {
	var apiError *dpwire.APIError
	return errors.As(err, &apiError) && strings.HasPrefix(apiError.Code, "404")
}

func globDevice(ctx context.Context, client *dpwire.Client, pattern string) ([]dpwire.Entry, error) {
	pattern = trimRootRelativePrefix(pattern)
	if pattern == "" {
		return nil, errors.New("glob pattern must name a device object")
	}
	segments := strings.Split(pattern, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return nil, fmt.Errorf("invalid glob path segment %q", segment)
		}
		if _, err := path.Match(segment, ""); err != nil {
			return nil, fmt.Errorf("invalid glob pattern: %w", err)
		}
	}
	if strings.EqualFold(segments[0], "Document") {
		return nil, errors.New("device globs must omit the internal Document/ prefix")
	}
	root, err := client.Documents.Resolve(ctx, dpwire.MustRemotePath("Document"))
	if err != nil {
		return nil, err
	}
	folders := []dpwire.Entry{root}
	visitedEntries := 0
	for index, segment := range segments {
		last := index == len(segments)-1
		var matches []dpwire.Entry
		var nextFolders []dpwire.Entry
		for _, folder := range folders {
			children, listErr := client.Folders.List(ctx, folder.ID, dpwire.ListOptions{})
			if listErr != nil {
				return nil, listErr
			}
			visitedEntries += len(children)
			if visitedEntries > maximumGlobEntries {
				return nil, fmt.Errorf("glob search exceeds the %d-object safety limit", maximumGlobEntries)
			}
			for _, child := range children {
				matched, matchErr := path.Match(segment, foldGlobString(child.Name))
				if matchErr != nil {
					return nil, fmt.Errorf("invalid glob pattern: %w", matchErr)
				}
				if !matched {
					continue
				}
				if last {
					matches = append(matches, child)
				} else if child.Type == dpwire.EntryFolder {
					nextFolders = append(nextFolders, child)
				}
			}
		}
		if last {
			return matches, nil
		}
		folders = nextFolders
		if len(folders) == 0 {
			return []dpwire.Entry{}, nil
		}
	}
	return []dpwire.Entry{}, nil
}

func ambiguousObjectError(store *profiles.ObjectReferenceStore, selector objectSelector, entries []dpwire.Entry) error {
	references, err := store.Assign(entries)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return devicePathString(entries[i].Path) < devicePathString(entries[j].Path) })
	var message strings.Builder
	fmt.Fprintf(&message, "multiple device objects match %s:\n", describeSelector(selector))
	for _, entry := range entries {
		reference := references[entry.ID]
		fmt.Fprintf(&message, "  %d  %s  %s\n", reference.Number, reference.Hex, devicePathString(entry.Path))
	}
	message.WriteString("use --id with one listed number or hexadecimal reference")
	return errors.New(message.String())
}

func describeSelector(selector objectSelector) string {
	switch selector.kind {
	case selectorID:
		return "ID " + selector.value
	case selectorGlob:
		return "glob " + fmt.Sprintf("%q", selector.value)
	default:
		return "path " + fmt.Sprintf("%q", selector.value)
	}
}
