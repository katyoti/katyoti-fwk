// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	reviewsapi "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// PackageFactsService answers the facts of the repository's Go packages from
// a derived store (the cache dir): computed facts — files, imports, basis —
// recomputed only when a package's files change, human facts read where they
// live — the owner from the modules' code_packages, the kind from the path,
// the description from the package comment — and the analyses recorded on a
// package, each kept with the basis it was made on and reported stale once
// that basis moves. It never runs `go list`: the files are parsed directly.
type PackageFactsService interface {
	// PackageFacts is one package's facts, refreshed when its files changed.
	PackageFacts(ctx context.Context, in PackageFactsInput) (PackageFactsOutput, error)
	// PackageList is the facts of every package a module owns.
	PackageList(ctx context.Context, in PackageListInput) (PackageListOutput, error)
	// PackageRecord records an analysis of a package with its current basis.
	PackageRecord(ctx context.Context, in PackageRecordInput) (PackageRecordOutput, error)
	// ChangeOwners is the modules owning the Go files a change touched, each
	// with its own: the owners action of the go-checks review type, so an
	// automatic review checks every module whose Go code changed.
	ChangeOwners(ctx context.Context, in reviewsapi.ChangeOwnersInput) (reviewsapi.ChangeOwnersOutput, error)
}

// PackageFacts are one Go package's facts.
type PackageFacts struct {
	Path        string            `json:"path"                  jsonschema:"description=Repository-relative path"`
	Owner       string            `json:"owner,omitempty"       jsonschema:"description=The module whose code_packages holds it (the longest path)"`
	Kind        string            `json:"kind"                  jsonschema:"description=Its role read from the path (PackageKind)"`
	Description string            `json:"description,omitempty" jsonschema:"description=The package comment's first sentence"`
	Files       []PackageFile     `json:"files"                 jsonschema:"description=Its .go files with the hash of their content"`
	Imports     []string          `json:"imports,omitempty"     jsonschema:"description=The repository packages it imports"`
	Basis       string            `json:"basis"                 jsonschema:"description=The hash of its files' content"`
	Analyses    []PackageAnalysis `json:"analyses,omitempty"    jsonschema:"description=The analyses recorded on it"`
}

// PackageFile is one .go file of a package.
type PackageFile struct {
	Name string `json:"name"`
	Test bool   `json:"test,omitempty"`
	Hash string `json:"hash"`
}

// PackageAnalysis is an analysis recorded on a package: what it was, when,
// what it found, and the basis it was made on — the package's own, or, for a
// deep analysis (a test run), its own and its repository imports' together.
type PackageAnalysis struct {
	Kind    string `json:"kind"              jsonschema:"description=What was analysed (tests or vet or go-checks or another)"`
	At      string `json:"at"                jsonschema:"description=When"`
	Verdict string `json:"verdict"           jsonschema:"description=What it found (pass or fail or a rating)"`
	Detail  string `json:"detail,omitempty"  jsonschema:"description=A line on what it found"`
	Deep    bool   `json:"deep,omitempty"    jsonschema:"description=Its basis folds the package's repository imports (a test run)"`
	Basis   string `json:"basis"             jsonschema:"description=The basis it was made on"`
	Stale   bool   `json:"stale,omitempty"   jsonschema:"description=The basis moved since: a file or (deep) an import changed"`
}

// PackageFactsInput names a package.
type PackageFactsInput struct {
	Path string `json:"path" jsonschema:"description=Repository-relative path of the package; required"`
}

// PackageFactsOutput is its facts.
type PackageFactsOutput struct {
	Facts PackageFacts `json:"facts"`
}

// PackageListInput names a module.
type PackageListInput struct {
	Module string `json:"module" jsonschema:"description=The module whose packages to list; required"`
}

// PackageListOutput is the facts of its packages, by path.
type PackageListOutput struct {
	Packages []PackageFacts `json:"packages"`
}

// PackageRecordInput is an analysis to record.
type PackageRecordInput struct {
	Path    string `json:"path"             jsonschema:"description=The package; required"`
	Kind    string `json:"kind"             jsonschema:"description=What was analysed; required"`
	Verdict string `json:"verdict"          jsonschema:"description=What it found; required"`
	Detail  string `json:"detail,omitempty" jsonschema:"description=A line on what it found"`
	Deep    bool   `json:"deep,omitempty"   jsonschema:"description=Its basis folds the package's repository imports (a test run)"`
}

// PackageRecordOutput is the analysis as recorded.
type PackageRecordOutput struct {
	Analysis PackageAnalysis `json:"analysis"`
}

// PackageFactsServiceDef describes the `package-facts` slot.
var PackageFactsServiceDef = meta.ServiceDef{
	Name:    "package-facts",
	Short:   "The facts of the repository's Go packages: owner, kind, description, files, imports and the analyses recorded on them",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"PackageFacts":  {Short: "One package's facts, refreshed when its files changed", ReadOnly: true},
		"PackageList":   {Short: "The facts of every package a module owns", ReadOnly: true},
		"PackageRecord": {Short: "Record an analysis of a package with its current basis"},
		"ChangeOwners":  {Short: "The modules owning the Go files a change touched - each with its own", ReadOnly: true},
	},
}
