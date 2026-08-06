package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryCatalogsAreCurrent(t *testing.T) {
	root := filepath.Join("..", "..")
	if err := run(root, false); err != nil {
		t.Fatal(err)
	}
}

func TestDerivedCatalogCounts(t *testing.T) {
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, referencePath))
	if err != nil {
		t.Fatal(err)
	}
	var spec swagger
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	operations, errorsCatalog := derive(spec, "test")
	if err := applyImplementation(filepath.Join(root, "spec", "compat", "implementation.json"), &operations); err != nil {
		t.Fatal(err)
	}
	if got, want := len(operations.Operations), 113; got != want {
		t.Fatalf("operation count = %d, want %d", got, want)
	}
	if got, want := len(errorsCatalog.Errors), 32; got != want {
		t.Fatalf("error count = %d, want %d", got, want)
	}
	statuses := make(map[string]string)
	for _, operation := range operations.Operations {
		statuses[operation.ID] = operation.Status
	}
	if statuses["GET /documents2"] != "device-verified" {
		t.Fatalf("GET /documents2 status = %q", statuses["GET /documents2"])
	}
}
