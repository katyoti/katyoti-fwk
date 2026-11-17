// SPDX-License-Identifier: Apache-2.0

// Package core implements the GoToolService — thin os/exec wrappers
// around the standard Go toolchain. Each method shells out to a real
// command, captures stdout/stderr, parses structured output where the
// tool supports JSON, and returns a typed result.
//
// Design constraints (per modules/code-gocode/tools.md §2):
//
//   - Pure pass-through to the toolchain — no business logic. Item
//     writes + status updates belong to GocodeService verbs.
//   - Failed commands still return a populated result with ExitCode
//     != 0 instead of an error. Tools "failing cleanly" (vet finding
//     issues, tests failing) is data, not an exception. Genuine
//     execution failures (binary not found, working dir missing)
//     return the Go error.
//   - Tools that need parsing (go test -json, gofmt -l, …) parse
//     their output into the typed slice fields. Tools that don't
//     (go build, go mod tidy) just surface ExitCode + Stdout/Stderr.
package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// GoToolServiceImpl implements api.GoToolService. Methods shell out
// to the local Go toolchain binaries (go, gofmt, goimports,
// golangci-lint) under the package's `workDir` (defaults to the
// caller's CWD when Input.WorkDir is empty).
type GoToolServiceImpl struct {
	cfg *Config
}

// NewGoToolService is the factory consumed by appcore.PublishService.
func NewGoToolService(_ stdctx.Context, cfg *Config) (api.GoToolService, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	return &GoToolServiceImpl{cfg: cfg}, nil
}

// runTool executes `cmd args...` in `workDir` and returns the
// CommonToolResult. Genuine execution failures (binary not found)
// return a Go error; non-zero exit with captured output is data.
func runTool(ctx stdctx.Context, workDir, cmd string, args ...string) (api.CommonToolResult, error) {
	start := time.Now()
	c := exec.CommandContext(ctx, cmd, args...)
	if workDir != "" {
		c.Dir = workDir
	}
	var stdout, stderr strings.Builder
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	res := api.CommonToolResult{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMS: time.Since(start).Milliseconds(),
		ExitCode:   c.ProcessState.ExitCode(),
	}
	// `exec.ExitError` is normal — the tool ran but returned non-zero.
	// Other errors (path resolution, context cancellation) are real
	// failures the caller must handle.
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return res, fmt.Errorf("%s: %w", cmd, err)
		}
	}
	return res, nil
}

func (s *GoToolServiceImpl) GoBuild(ctx stdctx.Context, in api.GoBuildInput) (api.GoBuildOutput, error) {
	args := []string{"build"}
	if len(in.Tags) > 0 {
		args = append(args, "-tags="+strings.Join(in.Tags, ","))
	}
	if in.Race {
		args = append(args, "-race")
	}
	if len(in.Packages) == 0 {
		args = append(args, "./...")
	} else {
		args = append(args, in.Packages...)
	}
	res, err := runTool(ctx, in.WorkDir, "go", args...)
	return api.GoBuildOutput{CommonToolResult: res}, err
}

func (s *GoToolServiceImpl) GoTest(ctx stdctx.Context, in api.GoTestInput) (api.GoTestOutput, error) {
	args := []string{"test"}
	if in.Count == 0 {
		in.Count = 1 // bypass cache by default — verify wants fresh runs
	}
	args = append(args, "-count="+strconv.Itoa(in.Count))
	if in.Race {
		args = append(args, "-race")
	}
	if len(in.Tags) > 0 {
		args = append(args, "-tags="+strings.Join(in.Tags, ","))
	}
	if in.Cover {
		args = append(args, "-cover")
	}
	if in.RunRegex != "" {
		args = append(args, "-run", in.RunRegex)
	}
	if in.JSON {
		args = append(args, "-json")
	}
	if len(in.Packages) == 0 {
		args = append(args, "./...")
	} else {
		args = append(args, in.Packages...)
	}
	res, err := runTool(ctx, in.WorkDir, "go", args...)
	out := api.GoTestOutput{CommonToolResult: res}

	// Parse -json events when requested. Each line is a JSON object
	// with {Time, Action, Package, Test, Elapsed, Output}.
	if in.JSON && res.Stdout != "" {
		var ev struct {
			Action  string  `json:"Action"`
			Package string  `json:"Package"`
			Test    string  `json:"Test,omitempty"`
			Elapsed float64 `json:"Elapsed,omitempty"`
			Output  string  `json:"Output,omitempty"`
		}
		for _, line := range strings.Split(res.Stdout, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				continue
			}
			out.Results = append(out.Results, api.GoTestResult{
				Package: ev.Package,
				Test:    ev.Test,
				Action:  ev.Action,
				Elapsed: ev.Elapsed,
				Output:  ev.Output,
			})
			// Tally on test-end events (not package-level).
			if ev.Test == "" {
				continue
			}
			switch ev.Action {
			case "pass":
				out.Passed++
			case "fail":
				out.Failed++
			case "skip":
				out.Skipped++
			}
		}
	}
	return out, err
}

func (s *GoToolServiceImpl) GoVet(ctx stdctx.Context, in api.GoVetInput) (api.GoVetOutput, error) {
	args := []string{"vet"}
	if len(in.Packages) == 0 {
		args = append(args, "./...")
	} else {
		args = append(args, in.Packages...)
	}
	res, err := runTool(ctx, in.WorkDir, "go", args...)
	out := api.GoVetOutput{CommonToolResult: res}
	// `go vet` emits findings to stderr as `file:line:col: message`.
	for _, line := range strings.Split(res.Stderr, "\n") {
		issue, ok := parseColonFileLine(line)
		if ok {
			out.Issues = append(out.Issues, issue)
		}
	}
	return out, err
}

func (s *GoToolServiceImpl) GolangciLint(ctx stdctx.Context, in api.GolangciLintInput) (api.GolangciLintOutput, error) {
	args := []string{"run", "--out-format=json"}
	if len(in.Enable) > 0 {
		args = append(args, "--enable", strings.Join(in.Enable, ","))
	}
	if len(in.Disable) > 0 {
		args = append(args, "--disable", strings.Join(in.Disable, ","))
	}
	if in.OnlyNewIssues {
		args = append(args, "--new")
	}
	if len(in.Packages) > 0 {
		args = append(args, in.Packages...)
	}
	res, err := runTool(ctx, in.WorkDir, "golangci-lint", args...)
	out := api.GolangciLintOutput{CommonToolResult: res}
	if res.Stdout != "" {
		var parsed struct {
			Issues []struct {
				Linter   string `json:"FromLinter"`
				Severity string `json:"Severity,omitempty"`
				Text     string `json:"Text"`
				Position struct {
					Filename string `json:"Filename"`
					Line     int    `json:"Line"`
					Column   int    `json:"Column"`
				} `json:"Pos"`
			} `json:"Issues"`
		}
		if jsonErr := json.Unmarshal([]byte(res.Stdout), &parsed); jsonErr == nil {
			for _, i := range parsed.Issues {
				out.Issues = append(out.Issues, api.GolangciLintIssue{
					Linter:   i.Linter,
					Severity: i.Severity,
					File:     i.Position.Filename,
					Line:     i.Position.Line,
					Column:   i.Position.Column,
					Message:  i.Text,
				})
			}
		}
	}
	return out, err
}

func (s *GoToolServiceImpl) GoAPIDiff(ctx stdctx.Context, in api.GoAPIDiffInput) (api.GoAPIDiffOutput, error) {
	// v1: shell out to `apidiff` if installed. Without it, return a
	// non-zero exit with a clear hint so the caller (architect/reviewer)
	// knows the tool isn't on PATH rather than getting an empty diff.
	if in.OldRef == "" {
		return api.GoAPIDiffOutput{
			CommonToolResult: api.CommonToolResult{
				ExitCode: 2,
				Stderr:   "go.APIDiff: OldRef is required",
			},
		}, nil
	}
	newRef := in.NewRef
	if newRef == "" {
		newRef = "HEAD"
	}
	args := []string{in.OldRef, newRef}
	if len(in.Packages) > 0 {
		args = append(args, in.Packages...)
	}
	res, err := runTool(ctx, in.WorkDir, "apidiff", args...)
	out := api.GoAPIDiffOutput{CommonToolResult: res}
	// apidiff output is line-oriented:
	//   - "+ added.Symbol" / "- removed.Symbol" / "! breaking: Symbol"
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+ "):
			out.Added = append(out.Added, strings.TrimPrefix(line, "+ "))
		case strings.HasPrefix(line, "- "):
			out.Removed = append(out.Removed, strings.TrimPrefix(line, "- "))
		case strings.HasPrefix(line, "! "):
			out.Breaking = append(out.Breaking, strings.TrimPrefix(line, "! "))
		}
	}
	return out, err
}

func (s *GoToolServiceImpl) GoPkgInventory(ctx stdctx.Context, in api.GoPkgInventoryInput) (api.GoPkgInventoryOutput, error) {
	args := []string{"list", "-json"}
	if len(in.Packages) == 0 {
		args = append(args, "./...")
	} else {
		args = append(args, in.Packages...)
	}
	res, err := runTool(ctx, in.WorkDir, "go", args...)
	out := api.GoPkgInventoryOutput{CommonToolResult: res}

	// `go list -json` emits a stream of JSON objects (one per
	// package), concatenated but each is a complete JSON document.
	// Decode using json.Decoder.
	dec := json.NewDecoder(strings.NewReader(res.Stdout))
	for {
		var pkg struct {
			ImportPath string   `json:"ImportPath"`
			Name       string   `json:"Name"`
			Exports    []string `json:"Exports,omitempty"`
		}
		if jsonErr := dec.Decode(&pkg); jsonErr != nil {
			break
		}
		bucket := bucketForImportPath(pkg.ImportPath)
		out.Packages = append(out.Packages, api.PkgInventoryEntry{
			ImportPath:  pkg.ImportPath,
			Bucket:      bucket,
			IsCmd:       pkg.Name == "main",
			PublicCount: len(pkg.Exports),
		})
	}
	return out, err
}

// bucketForImportPath classifies an import path into the 3-package
// convention (public / internal / local / other) by inspecting path
// segments. Per modules/code-gocode/concepts.md §2 a module's structure is:
//
//	<module>/public/...    → "public"
//	<module>/internal/...  → "internal"
//	<module>/local/...     → "local"
func bucketForImportPath(p string) string {
	switch {
	case strings.Contains(p, "/public/") || strings.HasSuffix(p, "/public"):
		return "public"
	case strings.Contains(p, "/internal/") || strings.HasSuffix(p, "/internal"):
		return "internal"
	case strings.Contains(p, "/local/") || strings.HasSuffix(p, "/local"):
		return "local"
	default:
		return "other"
	}
}

func (s *GoToolServiceImpl) GoDepCheck(ctx stdctx.Context, in api.GoDepCheckInput) (api.GoDepCheckOutput, error) {
	// `go list -json -deps ./...` gives the full import graph. We
	// walk each package + its Imports and flag violations of the
	// 3-package convention:
	//   - public/ imports internal/ or local/ → violation
	//   - module A's internal/ imported from module B → violation
	args := []string{"list", "-json", "-deps"}
	if in.ModulePath != "" {
		args = append(args, in.ModulePath+"/...")
	} else {
		args = append(args, "./...")
	}
	res, err := runTool(ctx, in.WorkDir, "go", args...)
	out := api.GoDepCheckOutput{CommonToolResult: res}

	dec := json.NewDecoder(strings.NewReader(res.Stdout))
	for {
		var pkg struct {
			ImportPath string   `json:"ImportPath"`
			Imports    []string `json:"Imports,omitempty"`
		}
		if jsonErr := dec.Decode(&pkg); jsonErr != nil {
			break
		}
		importer := pkg.ImportPath
		importerBucket := bucketForImportPath(importer)
		if importerBucket != "public" {
			// Only public packages have import restrictions worth
			// flagging in v1; internal/ may import anything within
			// the module per modules/code-gocode/concepts.md §2.
			continue
		}
		for _, imp := range pkg.Imports {
			impBucket := bucketForImportPath(imp)
			if impBucket == "internal" || impBucket == "local" {
				out.Violations = append(out.Violations, api.DepViolation{
					Importer: importer,
					Imported: imp,
					Reason:   "public package imports " + impBucket + "/ — violates §5.2 boundary",
				})
			}
		}
	}
	return out, err
}

func (s *GoToolServiceImpl) Gofmt(ctx stdctx.Context, in api.GofmtInput) (api.GofmtOutput, error) {
	args := []string{}
	if in.Write {
		args = append(args, "-w")
	} else {
		args = append(args, "-l") // list files that would be formatted
	}
	paths := in.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}
	args = append(args, paths...)
	res, err := runTool(ctx, in.WorkDir, "gofmt", args...)
	out := api.GofmtOutput{CommonToolResult: res}
	// -l (check) prints one filename per line; -w also prints them.
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out.FormattedFiles = append(out.FormattedFiles, line)
		}
	}
	return out, err
}

func (s *GoToolServiceImpl) Goimports(ctx stdctx.Context, in api.GoimportsInput) (api.GoimportsOutput, error) {
	args := []string{}
	if in.Write {
		args = append(args, "-w")
	} else {
		args = append(args, "-l")
	}
	paths := in.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}
	args = append(args, paths...)
	res, err := runTool(ctx, in.WorkDir, "goimports", args...)
	out := api.GoimportsOutput{CommonToolResult: res}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out.FormattedFiles = append(out.FormattedFiles, line)
		}
	}
	return out, err
}

func (s *GoToolServiceImpl) GoModTidy(ctx stdctx.Context, in api.GoModTidyInput) (api.GoModTidyOutput, error) {
	args := []string{"mod", "tidy"}
	if in.Diff {
		args = append(args, "-diff")
	}
	res, err := runTool(ctx, in.WorkDir, "go", args...)
	out := api.GoModTidyOutput{CommonToolResult: res}
	if in.Diff {
		for _, line := range strings.Split(res.Stdout, "\n") {
			line = strings.TrimRight(line, "\r\n")
			if line != "" {
				out.Changes = append(out.Changes, line)
			}
		}
	}
	return out, err
}

// parseColonFileLine parses `file:line:col: message` (or
// `file:line: message`) lines from go vet output. Returns false when
// the line doesn't match.
func parseColonFileLine(line string) (api.GoVetIssue, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return api.GoVetIssue{}, false
	}
	// Strip leading `# package` headers go vet emits before issues.
	if strings.HasPrefix(line, "#") {
		return api.GoVetIssue{}, false
	}
	// First field is path:line:col:
	parts := strings.SplitN(line, ":", 4)
	if len(parts) < 3 {
		return api.GoVetIssue{}, false
	}
	lineNum, err := strconv.Atoi(parts[1])
	if err != nil {
		return api.GoVetIssue{}, false
	}
	issue := api.GoVetIssue{
		File: filepath.ToSlash(parts[0]),
		Line: lineNum,
	}
	if len(parts) == 4 {
		col, colErr := strconv.Atoi(parts[2])
		if colErr == nil {
			issue.Column = col
			issue.Message = strings.TrimSpace(parts[3])
		} else {
			issue.Message = strings.TrimSpace(parts[2] + ":" + parts[3])
		}
	} else {
		issue.Message = strings.TrimSpace(parts[2])
	}
	return issue, true
}
