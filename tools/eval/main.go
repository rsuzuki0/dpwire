// Command eval runs the reproducible project quality gates and writes a
// machine-readable report.
package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type result struct {
	Name       string `json:"name"`
	Command    string `json:"command"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Output     string `json:"output,omitempty"`
}

type report struct {
	SchemaVersion int       `json:"schema_version"`
	Mode          string    `json:"mode"`
	Started       time.Time `json:"started"`
	Finished      time.Time `json:"finished"`
	GoVersion     string    `json:"go_version"`
	OS            string    `json:"os"`
	Architecture  string    `json:"architecture"`
	Commit        string    `json:"commit"`
	Results       []result  `json:"results"`
	Passed        bool      `json:"passed"`
}

type step struct {
	name    string
	command string
	args    []string
	env     []string
	clean   func(string) error
}

type testSuites struct {
	XMLName  xml.Name  `xml:"testsuites"`
	Tests    int       `xml:"tests,attr"`
	Failures int       `xml:"failures,attr"`
	Suite    testSuite `xml:"testsuite"`
}

type testSuite struct {
	Name     string     `xml:"name,attr"`
	Tests    int        `xml:"tests,attr"`
	Failures int        `xml:"failures,attr"`
	Cases    []testCase `xml:"testcase"`
}

type testCase struct {
	Name      string   `xml:"name,attr"`
	Time      string   `xml:"time,attr"`
	Failure   *failure `xml:"failure,omitempty"`
	SystemOut string   `xml:"system-out,omitempty"`
}

type failure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

func main() {
	mode := flag.String("mode", "developer", "developer, ci, nightly, device, or release")
	flag.Parse()
	if !validMode(*mode) {
		fmt.Fprintln(os.Stderr, "eval: invalid mode", *mode)
		os.Exit(2)
	}
	if err := os.MkdirAll("artifacts/eval/build", 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}

	started := time.Now().UTC()
	results := execute(steps(*mode))
	report := report{
		SchemaVersion: 1, Mode: *mode, Started: started, Finished: time.Now().UTC(),
		GoVersion: runtime.Version(), OS: runtime.GOOS, Architecture: runtime.GOARCH,
		Commit: commit(), Results: results, Passed: passed(results),
	}
	if err := writeReports(report); err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
	for _, item := range results {
		fmt.Printf("%-24s %s\n", item.nameForPrint(), item.Status)
	}
	if !report.Passed {
		os.Exit(1)
	}
}

func validMode(mode string) bool {
	switch mode {
	case "developer", "ci", "nightly", "device", "release":
		return true
	default:
		return false
	}
}

func steps(mode string) []step {
	items := []step{
		{name: "format", command: "gofmt", args: append([]string{"-l"}, goFiles()...), clean: requireEmpty},
		{name: "vet", command: "go", args: []string{"vet", "./..."}},
		{name: "architecture", command: "go", args: []string{"run", "./tools/arch-check"}},
		{name: "spec", command: "go", args: []string{"run", "./tools/spec-check"}},
		{name: "unit-and-protocol", command: "go", args: []string{"test", "-coverprofile=artifacts/eval/coverage.out", "./..."}},
		{name: "coverage-summary", command: "go", args: []string{"tool", "cover", "-func=artifacts/eval/coverage.out"}, clean: requireCoverage(60)},
		{name: "cli-e2e", command: "go", args: []string{"run", "./cmd/dp", "version"}},
	}
	if mode != "developer" {
		items = append(items, step{name: "race", command: "go", args: []string{"test", "-race", "./..."}})
		for _, target := range []struct{ os, arch string }{{"darwin", "arm64"}, {"darwin", "amd64"}, {"linux", "arm64"}, {"linux", "amd64"}} {
			for _, command := range []string{"dp", "dp-sim"} {
				name := fmt.Sprintf("build-%s-%s-%s", command, target.os, target.arch)
				output := filepath.Join("artifacts", "eval", "build", name)
				items = append(items, step{
					name: name, command: "go", args: []string{"build", "-trimpath", "-o", output, "./cmd/" + command},
					env: []string{"CGO_ENABLED=0", "GOOS=" + target.os, "GOARCH=" + target.arch},
				})
			}
		}
	}
	if mode == "nightly" {
		items = append(items, step{name: "fuzz-aeskw", command: "go", args: []string{"test", "-run=^$", "-fuzz=FuzzWrapRoundTrip", "-fuzztime=30s", "./internal/crypto/aeskw"}})
	}
	if mode == "release" {
		items = append(items, step{name: "release-reproducibility", command: "go", args: []string{"run", "./tools/release", "-version", "0.0.0-eval", "-allow-dirty", "-check-reproducible"}})
	}
	return items
}

func goFiles() []string {
	var files []string
	_ = filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "artifacts", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	return files
}

func execute(steps []step) []result {
	results := make([]result, 0, len(steps))
	for _, item := range steps {
		started := time.Now()
		cmd := exec.Command(item.command, item.args...)
		cmd.Env = append(os.Environ(), item.env...)
		output, err := cmd.CombinedOutput()
		if err == nil && item.clean != nil {
			err = item.clean(string(output))
		}
		status := "passed"
		if err != nil {
			status = "failed"
			output = append(output, []byte("\n"+err.Error())...)
		}
		results = append(results, result{
			Name: item.name, Command: strings.Join(append([]string{item.command}, item.args...), " "),
			Status: status, DurationMS: time.Since(started).Milliseconds(), Output: trimOutput(output),
		})
	}
	return results
}

func requireEmpty(output string) error {
	if strings.TrimSpace(output) != "" {
		return fmt.Errorf("files need formatting: %s", strings.TrimSpace(output))
	}
	return nil
}

func requireCoverage(minimum float64) func(string) error {
	return func(output string) error {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		if len(lines) == 0 {
			return fmt.Errorf("coverage summary is empty")
		}
		fields := strings.Fields(lines[len(lines)-1])
		if len(fields) == 0 {
			return fmt.Errorf("coverage total is missing")
		}
		value, err := strconv.ParseFloat(strings.TrimSuffix(fields[len(fields)-1], "%"), 64)
		if err != nil {
			return fmt.Errorf("parse coverage: %w", err)
		}
		if value < minimum {
			return fmt.Errorf("coverage %.1f%% is below %.1f%%", value, minimum)
		}
		return nil
	}
}

func trimOutput(output []byte) string {
	const limit = 32 << 10
	output = bytes.TrimSpace(output)
	if len(output) > limit {
		output = append(output[:limit], []byte("\n... output truncated")...)
	}
	return string(output)
}

func commit() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "uncommitted"
	}
	return strings.TrimSpace(string(output))
}

func passed(results []result) bool {
	for _, item := range results {
		if item.Status != "passed" {
			return false
		}
	}
	return true
}

func writeReports(value report) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile("artifacts/eval/report.json", append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	suite := testSuite{Name: "digitalpaper-eval", Tests: len(value.Results)}
	for _, item := range value.Results {
		caseItem := testCase{Name: item.Name, Time: fmt.Sprintf("%.3f", float64(item.DurationMS)/1000), SystemOut: item.Output}
		if item.Status != "passed" {
			caseItem.Failure = &failure{Message: "quality gate failed", Text: item.Output}
			suite.Failures++
		}
		suite.Cases = append(suite.Cases, caseItem)
	}
	document := testSuites{Tests: suite.Tests, Failures: suite.Failures, Suite: suite}
	xmlData, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	xmlData = append([]byte(xml.Header), xmlData...)
	xmlData = append(xmlData, '\n')
	return os.WriteFile("artifacts/eval/junit.xml", xmlData, 0o644)
}

func (r result) nameForPrint() string { return r.Name }
