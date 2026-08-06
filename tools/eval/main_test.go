package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidModes(t *testing.T) {
	for _, mode := range []string{"developer", "ci", "nightly", "device", "release"} {
		if !validMode(mode) {
			t.Errorf("validMode(%q) = false", mode)
		}
	}
	if validMode("unknown") {
		t.Error("validMode(unknown) = true")
	}
}

func TestStepTiers(t *testing.T) {
	developer := steps("developer")
	ci := steps("ci")
	nightly := steps("nightly")
	if len(developer) >= len(ci) || len(ci) >= len(nightly) {
		t.Fatalf("step counts developer=%d ci=%d nightly=%d", len(developer), len(ci), len(nightly))
	}
}

func TestExecuteAndReports(t *testing.T) {
	results := execute([]step{{name: "go-version", command: "go", args: []string{"version"}}})
	if !passed(results) || len(results) != 1 {
		t.Fatalf("results = %#v", results)
	}
	directory := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.MkdirAll(filepath.Join("artifacts", "eval"), 0o755); err != nil {
		t.Fatal(err)
	}
	value := report{SchemaVersion: 1, Mode: "test", Results: results, Passed: true}
	if err := writeReports(value); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.json", "junit.xml"} {
		if _, err := os.Stat(filepath.Join("artifacts", "eval", name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCoverageParser(t *testing.T) {
	check := requireCoverage(60)
	if err := check("total:\t(statements)\t61.2%\n"); err != nil {
		t.Fatal(err)
	}
	if err := check("total:\t(statements)\t59.9%\n"); err == nil {
		t.Fatal("coverage below threshold passed")
	}
}
