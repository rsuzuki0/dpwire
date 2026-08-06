// Command spec-check validates immutable reference hashes and deterministic
// compatibility catalogs.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const referencePath = "spec/references/polaris-0.6.0.swagger.json"

type provenance struct {
	References []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"references"`
}

type swagger struct {
	Swagger string                          `json:"swagger"`
	Info    struct{ Version, Title string } `json:"info"`
	Paths   map[string]map[string]operation `json:"paths"`
}

type operation struct {
	Tags        []string            `json:"tags"`
	Description string              `json:"description"`
	Responses   map[string]response `json:"responses"`
}

type response struct {
	Description string `json:"description"`
}

type operationCatalog struct {
	SchemaVersion int              `json:"schema_version"`
	Source        string           `json:"source"`
	SourceSHA256  string           `json:"source_sha256"`
	Operations    []operationEntry `json:"operations"`
}

type operationEntry struct {
	ID         string `json:"id"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Tag        string `json:"tag"`
	Status     string `json:"status"`
	Deprecated bool   `json:"deprecated"`
}

type errorCatalog struct {
	SchemaVersion int          `json:"schema_version"`
	Source        string       `json:"source"`
	Errors        []errorEntry `json:"errors"`
}

type errorEntry struct {
	Code         string   `json:"code"`
	HTTPStatuses []string `json:"http_statuses"`
	Operations   []string `json:"operations"`
}

type implementationCatalog struct {
	SchemaVersion int               `json:"schema_version"`
	Operations    map[string]string `json:"operations"`
}

var methods = map[string]bool{
	"get": true, "post": true, "put": true, "delete": true, "patch": true,
}

var errorCode = regexp.MustCompile(`\[\s*([0-9]{5})\s*\]`)

func main() {
	update := flag.Bool("update", false, "rewrite deterministic derived catalogs")
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if err := run(*root, *update); err != nil {
		fmt.Fprintln(os.Stderr, "spec-check:", err)
		os.Exit(1)
	}
}

func run(root string, update bool) error {
	referenceFile := filepath.Join(root, referencePath)
	raw, err := os.ReadFile(referenceFile)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	if err := checkProvenance(filepath.Join(root, "spec/references/provenance.json"), digest); err != nil {
		return err
	}
	var spec swagger
	if err := json.Unmarshal(raw, &spec); err != nil {
		return fmt.Errorf("decode reference: %w", err)
	}
	if spec.Swagger != "2.0" || spec.Info.Version != "0.6.0" {
		return fmt.Errorf("unexpected reference identity: swagger=%q version=%q", spec.Swagger, spec.Info.Version)
	}
	operations, errorsCatalog := derive(spec, digest)
	if err := applyImplementation(filepath.Join(root, "spec/compat/implementation.json"), &operations); err != nil {
		return err
	}
	checks := []struct {
		path  string
		value any
	}{
		{"spec/compat/operations.json", operations},
		{"spec/compat/error_codes.json", errorsCatalog},
	}
	for _, check := range checks {
		path := filepath.Join(root, check.path)
		encoded, err := json.MarshalIndent(check.value, "", "  ")
		if err != nil {
			return err
		}
		encoded = append(encoded, '\n')
		if update {
			if err := os.WriteFile(path, encoded, 0o644); err != nil {
				return err
			}
			continue
		}
		current, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w (run with -update)", check.path, err)
		}
		if !bytes.Equal(current, encoded) {
			return fmt.Errorf("%s is stale (run with -update)", check.path)
		}
	}
	return checkModels(filepath.Join(root, "spec/compat/models.json"))
}

func applyImplementation(path string, catalog *operationCatalog) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var implementation implementationCatalog
	if err := json.Unmarshal(raw, &implementation); err != nil {
		return err
	}
	if implementation.SchemaVersion != 1 {
		return errors.New("implementation catalog has unsupported schema")
	}
	seen := make(map[string]bool)
	for index := range catalog.Operations {
		entry := &catalog.Operations[index]
		status, ok := implementation.Operations[entry.ID]
		if !ok {
			continue
		}
		switch status {
		case "documented", "implemented", "emulated", "device-verified", "deprecated", "experimental", "unsupported":
			entry.Status = status
		default:
			return fmt.Errorf("invalid operation status %q for %s", status, entry.ID)
		}
		seen[entry.ID] = true
	}
	for id := range implementation.Operations {
		if !seen[id] {
			return fmt.Errorf("implementation status references unknown operation %s", id)
		}
	}
	return nil
}

func checkProvenance(path, digest string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var data provenance
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	for _, reference := range data.References {
		if reference.Path == referencePath {
			if !strings.EqualFold(reference.SHA256, digest) {
				return fmt.Errorf("reference checksum mismatch: provenance=%s actual=%s", reference.SHA256, digest)
			}
			return nil
		}
	}
	return errors.New("reference missing from provenance")
}

func derive(spec swagger, digest string) (operationCatalog, errorCatalog) {
	operations := operationCatalog{SchemaVersion: 1, Source: referencePath, SourceSHA256: digest}
	type accumulator struct {
		statuses   map[string]bool
		operations map[string]bool
	}
	errorMap := make(map[string]*accumulator)
	for path, pathItem := range spec.Paths {
		for method, op := range pathItem {
			method = strings.ToLower(method)
			if !methods[method] {
				continue
			}
			id := strings.ToUpper(method) + " " + path
			tag := ""
			if len(op.Tags) > 0 {
				tag = op.Tags[0]
			}
			operations.Operations = append(operations.Operations, operationEntry{
				ID: id, Method: strings.ToUpper(method), Path: path, Tag: tag,
				Status: "documented", Deprecated: strings.Contains(strings.ToLower(op.Description), "deprecated"),
			})
			for status, response := range op.Responses {
				matches := errorCode.FindAllStringSubmatch(response.Description, -1)
				for _, match := range matches {
					item := errorMap[match[1]]
					if item == nil {
						item = &accumulator{statuses: make(map[string]bool), operations: make(map[string]bool)}
						errorMap[match[1]] = item
					}
					item.statuses[status] = true
					item.operations[id] = true
				}
			}
		}
	}
	sort.Slice(operations.Operations, func(i, j int) bool { return operations.Operations[i].ID < operations.Operations[j].ID })
	errorsCatalog := errorCatalog{SchemaVersion: 1, Source: referencePath}
	for code, item := range errorMap {
		errorsCatalog.Errors = append(errorsCatalog.Errors, errorEntry{
			Code: code, HTTPStatuses: sortedKeys(item.statuses), Operations: sortedKeys(item.operations),
		})
	}
	sort.Slice(errorsCatalog.Errors, func(i, j int) bool { return errorsCatalog.Errors[i].Code < errorsCatalog.Errors[j].Code })
	return operations, errorsCatalog
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func checkModels(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var data struct {
		SchemaVersion int `json:"schema_version"`
		Models        []struct {
			Models       []string          `json:"models"`
			Capabilities map[string]string `json:"capabilities"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if data.SchemaVersion != 1 || len(data.Models) == 0 {
		return errors.New("model catalog is empty or has an unsupported schema")
	}
	for _, model := range data.Models {
		if len(model.Models) == 0 || len(model.Capabilities) == 0 {
			return errors.New("model entry is missing identifiers or capabilities")
		}
	}
	return nil
}
