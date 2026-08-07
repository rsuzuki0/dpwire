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
		entries, err := walkDevice(ctx, client)
		if err != nil {
			return dpwire.Entry{}, err
		}
		matchPath := strings.Contains(pattern, "/")
		for _, entry := range entries {
			if expected != "" && entry.Type != expected {
				continue
			}
			value := foldGlobString(entry.Name)
			if matchPath {
				value = foldGlobString(devicePathString(entry.Path))
			}
			matched, matchErr := path.Match(pattern, value)
			if matchErr != nil {
				return dpwire.Entry{}, fmt.Errorf("invalid glob pattern: %w", matchErr)
			}
			if matched {
				candidates = append(candidates, entry)
			}
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

func walkDevice(ctx context.Context, client *dpwire.Client) ([]dpwire.Entry, error) {
	root, err := client.Documents.Resolve(ctx, dpwire.MustRemotePath("Document"))
	if err != nil {
		return nil, err
	}
	queue := []dpwire.Entry{root}
	visited := map[string]struct{}{root.ID: {}}
	entries := make([]dpwire.Entry, 0)
	for len(queue) > 0 {
		folder := queue[0]
		queue = queue[1:]
		children, listErr := client.Folders.List(ctx, folder.ID, dpwire.ListOptions{})
		if listErr != nil {
			return nil, listErr
		}
		if len(entries)+len(children) > maximumGlobEntries {
			return nil, fmt.Errorf("glob search exceeds the %d-object safety limit", maximumGlobEntries)
		}
		entries = append(entries, children...)
		for _, child := range children {
			if child.Type != dpwire.EntryFolder {
				continue
			}
			if _, exists := visited[child.ID]; exists {
				continue
			}
			visited[child.ID] = struct{}{}
			queue = append(queue, child)
		}
	}
	return entries, nil
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
