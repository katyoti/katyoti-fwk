// SPDX-License-Identifier: Apache-2.0

package api

import "context"

// GoToolService is the pure Go-toolchain wrapper surface. Auto-
// published under the `gotool.*` slot namespace by AutoKTSPContainer.
//
// Each method shells out to one Go toolchain command (go build,
// go test, gofmt, go vet, golangci-lint, …). The wrappers normalize
// args, capture output, and return structured results — no business
// logic, no item writes. Used by GocodeService orchestration verbs
// (ModuleVerify shells GoTest; ModuleLint shells GolangciLint, etc.)
// and exposable directly to agents that need a single Go command.
type GoToolService interface {
	GoBuild(ctx context.Context, in GoBuildInput) (GoBuildOutput, error)
	GoTest(ctx context.Context, in GoTestInput) (GoTestOutput, error)
	GoVet(ctx context.Context, in GoVetInput) (GoVetOutput, error)
	GolangciLint(ctx context.Context, in GolangciLintInput) (GolangciLintOutput, error)
	GoAPIDiff(ctx context.Context, in GoAPIDiffInput) (GoAPIDiffOutput, error)
	GoPkgInventory(ctx context.Context, in GoPkgInventoryInput) (GoPkgInventoryOutput, error)
	GoDepCheck(ctx context.Context, in GoDepCheckInput) (GoDepCheckOutput, error)
	Gofmt(ctx context.Context, in GofmtInput) (GofmtOutput, error)
	Goimports(ctx context.Context, in GoimportsInput) (GoimportsOutput, error)
	GoModTidy(ctx context.Context, in GoModTidyInput) (GoModTidyOutput, error)
}

// CommonToolResult fields shared by every Go-tool output. Embedded
// (not flattened) into each Output struct so JSON-Schema generation
// retains the structure.
type CommonToolResult struct {
	ExitCode   int    `json:"exit_code"                     jsonschema:"0 on success; non-zero on tool failure"`
	Stdout     string `json:"stdout,omitempty"              jsonschema:"Captured stdout (may be large)"`
	Stderr     string `json:"stderr,omitempty"              jsonschema:"Captured stderr"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// ============================================================================
// go.Build
// ============================================================================

type GoBuildInput struct {
	Packages []string `json:"packages,omitempty"  jsonschema:"Package patterns (e.g. ./... or ./internal/...)"`
	Tags     []string `json:"tags,omitempty"      jsonschema:"Build tags (-tags)"`
	Race     bool     `json:"race,omitempty"      jsonschema:"-race flag"`
	WorkDir  string   `json:"work_dir,omitempty"  jsonschema:"Working directory; default workspace root"`
}

type GoBuildOutput struct {
	CommonToolResult
}

// ============================================================================
// go.Test
// ============================================================================

type GoTestInput struct {
	Packages []string `json:"packages,omitempty"   jsonschema:"Package patterns"`
	RunRegex string   `json:"run_regex,omitempty"  jsonschema:"-run pattern for selective tests"`
	Count    int      `json:"count,omitempty"      jsonschema:"-count value; default 1 to bypass cache"`
	Race     bool     `json:"race,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Cover    bool     `json:"cover,omitempty"      jsonschema:"-cover (coverage output)"`
	JSON     bool     `json:"json,omitempty"       jsonschema:"-json (parseable output)"`
	WorkDir  string   `json:"work_dir,omitempty"`
}

type GoTestResult struct {
	Package string  `json:"package"`
	Test    string  `json:"test,omitempty"`
	Action  string  `json:"action"               jsonschema:"run | pass | fail | skip | output"`
	Elapsed float64 `json:"elapsed,omitempty"    jsonschema:"Seconds"`
	Output  string  `json:"output,omitempty"`
}

type GoTestOutput struct {
	CommonToolResult
	Results []GoTestResult `json:"results,omitempty"   jsonschema:"Parsed -json events; populated when Input.JSON=true"`
	Passed  int            `json:"passed,omitempty"`
	Failed  int            `json:"failed,omitempty"`
	Skipped int            `json:"skipped,omitempty"`
}

// ============================================================================
// go.Vet
// ============================================================================

type GoVetInput struct {
	Packages []string `json:"packages,omitempty"`
	WorkDir  string   `json:"work_dir,omitempty"`
}

type GoVetOutput struct {
	CommonToolResult
	Issues []GoVetIssue `json:"issues,omitempty"`
}

type GoVetIssue struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
}

// ============================================================================
// go.GolangciLint
// ============================================================================

type GolangciLintInput struct {
	Packages      []string `json:"packages,omitempty"`
	Enable        []string `json:"enable,omitempty"     jsonschema:"Linter names to pass via --enable flag"`
	Disable       []string `json:"disable,omitempty"`
	OnlyNewIssues bool     `json:"only_new_issues,omitempty"`
	WorkDir       string   `json:"work_dir,omitempty"`
}

type GolangciLintIssue struct {
	Linter   string `json:"linter"`
	Severity string `json:"severity,omitempty"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	Message  string `json:"message"`
}

type GolangciLintOutput struct {
	CommonToolResult
	Issues []GolangciLintIssue `json:"issues,omitempty"`
}

// ============================================================================
// go.APIDiff
// ============================================================================

type GoAPIDiffInput struct {
	OldRef   string   `json:"old_ref"             jsonschema:"Git ref or tag of the previous charter version"`
	NewRef   string   `json:"new_ref,omitempty"   jsonschema:"Defaults to HEAD"`
	Packages []string `json:"packages,omitempty"`
	WorkDir  string   `json:"work_dir,omitempty"`
}

type GoAPIDiffOutput struct {
	CommonToolResult
	Breaking []string `json:"breaking,omitempty"     jsonschema:"Breaking change descriptions"`
	Added    []string `json:"added,omitempty"`
	Removed  []string `json:"removed,omitempty"`
}

// ============================================================================
// go.PkgInventory — list public/internal/local packages of a module
// ============================================================================

type PkgInventoryEntry struct {
	ImportPath  string `json:"import_path"`
	Bucket      string `json:"bucket"                  jsonschema:"public | internal | local | other"`
	IsCmd       bool   `json:"is_cmd,omitempty"        jsonschema:"True if package main"`
	PublicCount int    `json:"public_count,omitempty"  jsonschema:"Count of exported identifiers"`
}

type GoPkgInventoryInput struct {
	WorkDir  string   `json:"work_dir,omitempty"`
	Packages []string `json:"packages,omitempty"`
}

type GoPkgInventoryOutput struct {
	CommonToolResult
	Packages []PkgInventoryEntry `json:"packages,omitempty"`
}

// ============================================================================
// go.DepCheck — verify imports respect public/internal boundaries
// ============================================================================

type DepViolation struct {
	Importer string `json:"importer"             jsonschema:"Package that violates"`
	Imported string `json:"imported"             jsonschema:"Package being imported"`
	Reason   string `json:"reason"               jsonschema:"E.g. 'public package imports internal/...'"`
}

type GoDepCheckInput struct {
	ModulePath string `json:"module_path,omitempty"   jsonschema:"Module repo-relative path; defaults to workspace root"`
	WorkDir    string `json:"work_dir,omitempty"`
}

type GoDepCheckOutput struct {
	CommonToolResult
	Violations []DepViolation `json:"violations,omitempty"`
}

// ============================================================================
// gofmt — Go source formatting check / apply
// ============================================================================

type GofmtInput struct {
	Paths   []string `json:"paths,omitempty"      jsonschema:"Files or directories"`
	Write   bool     `json:"write,omitempty"      jsonschema:"-w: apply formatting (default: check-only)"`
	WorkDir string   `json:"work_dir,omitempty"`
}

type GofmtOutput struct {
	CommonToolResult
	FormattedFiles []string `json:"formatted_files,omitempty" jsonschema:"Files that were reformatted (or would be)"`
}

// ============================================================================
// goimports — like gofmt + auto import management
// ============================================================================

type GoimportsInput struct {
	Paths   []string `json:"paths,omitempty"`
	Write   bool     `json:"write,omitempty"`
	WorkDir string   `json:"work_dir,omitempty"`
}

type GoimportsOutput struct {
	CommonToolResult
	FormattedFiles []string `json:"formatted_files,omitempty"`
}

// ============================================================================
// go.ModTidy — go mod tidy
// ============================================================================

type GoModTidyInput struct {
	WorkDir string `json:"work_dir,omitempty"`
	Diff    bool   `json:"diff,omitempty"           jsonschema:"-diff: show changes without writing"`
}

type GoModTidyOutput struct {
	CommonToolResult
	Changes []string `json:"changes,omitempty"   jsonschema:"Lines added/removed (when Input.Diff=true)"`
}
