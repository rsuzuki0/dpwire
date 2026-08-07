// Command arch-check enforces the repository's one-way dependency rules.
package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const module = "github.com/rsuzuki0/digitalpaper"

type violation struct{ file, imported, reason string }

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	violations, err := check(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "arch-check:", err)
		os.Exit(1)
	}
	for _, violation := range violations {
		fmt.Fprintf(os.Stderr, "%s imports %s: %s\n", violation.file, violation.imported, violation.reason)
	}
	if len(violations) > 0 {
		os.Exit(1)
	}
}

func check(root string) ([]violation, error) {
	var violations []violation
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "artifacts" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil || !strings.HasPrefix(imported, module) {
				continue
			}
			if reason := forbidden(rel, imported); reason != "" {
				violations = append(violations, violation{rel, imported, reason})
			}
		}
		return nil
	})
	sort.Slice(violations, func(i, j int) bool {
		return violations[i].file+violations[i].imported < violations[j].file+violations[j].imported
	})
	return violations, err
}

func forbidden(rel, imported string) string {
	path := filepath.ToSlash(rel)
	// External integration tests may depend on dptest; production packages may not.
	if strings.HasSuffix(path, "_test.go") {
		return ""
	}
	suffix := strings.TrimPrefix(imported, module)
	importsAny := func(prefixes ...string) bool {
		for _, prefix := range prefixes {
			if suffix == prefix || strings.HasPrefix(suffix, prefix+"/") {
				return true
			}
		}
		return false
	}

	// Public library packages must never depend on workflows, renderers, CLI,
	// simulator helpers, or commands.
	public := !strings.Contains(path, "/") || strings.HasPrefix(path, "credentials/") || strings.HasPrefix(path, "discovery/") || strings.HasPrefix(path, "profiles/")
	if public && importsAny("/workflow", "/render", "/internal/cli", "/cmd", "/dptest") {
		return "public library cannot depend on workflow, rendering, CLI, command, or simulator layers"
	}
	if strings.HasPrefix(path, "internal/wire/") && importsAny("/workflow", "/render", "/internal/cli", "/cmd", "/dptest") {
		return "wire layer cannot depend on consumers or test simulator"
	}
	if strings.HasPrefix(path, "render/") && importsAny("/workflow", "/internal/wire", "/internal/cli", "/cmd", "/dptest") {
		return "renderer adapters must remain independent from protocol and CLI layers"
	}
	if strings.HasPrefix(path, "workflow/") && importsAny("/internal/wire", "/internal/cli", "/cmd", "/dptest") {
		return "workflow must use the public library rather than wire or command internals"
	}
	return ""
}
