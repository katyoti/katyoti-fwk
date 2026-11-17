// SPDX-License-Identifier: Apache-2.0

// Package api defines the code-methodology contract layer: the
// CodeService (code analysis, module discovery, open decisions and the
// charter/manifest projections), the language-neutral domain model the
// language packs share, and the LanguagePack interface every
// ktw-<lang>code peer implements.
package api

import (
	"context"
	"errors"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// ErrInvalidInput — request payload failed validation.
var ErrInvalidInput = errors.New("code: invalid input")

// CodeService is the language-neutral service of the code methodology:
// the analysis read-side, module discovery (dispatched to the registered
// LanguagePack, see language_pack.go), open decisions and the
// charter/manifest projections. The module, use-case, verify, lint,
// promote and tasktest verbs belong to the language pack
// (GocodeService for Go).
type CodeService interface {
	// ── analysis ─────────────────────────────────────────────────────

	// AnalyzeFolder returns a high-level overview of a code folder.
	AnalyzeFolder(ctx context.Context, input AnalyzeFolderInput) (AnalyzeFolderOutput, error)

	// ExtractStructure extracts types, functions, interfaces from Go files.
	ExtractStructure(ctx context.Context, input ExtractStructureInput) (ExtractStructureOutput, error)

	// ExtractDependencies maps imports and call relationships.
	ExtractDependencies(ctx context.Context, input ExtractDependenciesInput) (ExtractDependenciesOutput, error)

	// ── module discovery ─────────────────────────────────────────────
	ModuleDiscover(ctx context.Context, in ModuleDiscoverInput) (ModuleDiscoverOutput, error)

	// ── od.* ─────────────────────────────────────────────────────────
	ODOpen(ctx context.Context, in ODOpenInput) (ODOpenOutput, error)
	ODResolve(ctx context.Context, in ODResolveInput) (ODResolveOutput, error)

	// ── projections ──────────────────────────────────────────────────
	CharterPublish(ctx context.Context, in CharterPublishInput) (CharterPublishOutput, error)
	ManifestWrite(ctx context.Context, in ManifestWriteInput) (ManifestWriteOutput, error)
}

// CodeServiceDef is the KTSP service definition for auto-discovery.
var CodeServiceDef = meta.ServiceDef{
	Name:  "code-service",
	Short: "Code analysis and structure extraction",
}

// --- Input/Output types ---

type AnalyzeFolderInput struct {
	Path       string   `json:"path" jsonschema:"Root folder to analyze"`
	Extensions []string `json:"extensions,omitempty" jsonschema:"File extensions to include (e.g. .go .ts). Empty = all"`
	MaxDepth   int      `json:"max_depth,omitempty" jsonschema:"Max directory depth. 0 = unlimited"`
}

type AnalyzeFolderOutput struct {
	RootPath    string       `json:"root_path"`
	TotalFiles  int          `json:"total_files"`
	TotalLines  int          `json:"total_lines"`
	Languages   []Language   `json:"languages"`
	Directories []DirSummary `json:"directories"`
}

type Language struct {
	Name      string `json:"name"`
	Extension string `json:"extension"`
	FileCount int    `json:"file_count"`
	LineCount int    `json:"line_count"`
}

type DirSummary struct {
	Path      string `json:"path"`
	FileCount int    `json:"file_count"`
	Purpose   string `json:"purpose,omitempty"` // inferred from name/content
}

type ExtractStructureInput struct {
	Path      string `json:"path" jsonschema:"File or folder to extract structure from"`
	Language  string `json:"language,omitempty" jsonschema:"Language hint (go, typescript, python). Auto-detected if empty"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"Recurse into subdirectories"`
}

type ExtractStructureOutput struct {
	Packages []PackageInfo `json:"packages"`
}

type PackageInfo struct {
	Name       string      `json:"name"`
	Path       string      `json:"path"`
	Doc        string      `json:"doc,omitempty"`
	Types      []TypeInfo  `json:"types,omitempty"`
	Functions  []FuncInfo  `json:"functions,omitempty"`
	Interfaces []IfaceInfo `json:"interfaces,omitempty"`
	Constants  []ConstInfo `json:"constants,omitempty"`
}

type TypeInfo struct {
	Name   string   `json:"name"`
	Doc    string   `json:"doc,omitempty"`
	Fields []string `json:"fields,omitempty"`
}

type FuncInfo struct {
	Name      string `json:"name"`
	Doc       string `json:"doc,omitempty"`
	Signature string `json:"signature"`
	Receiver  string `json:"receiver,omitempty"`
}

type IfaceInfo struct {
	Name    string   `json:"name"`
	Doc     string   `json:"doc,omitempty"`
	Methods []string `json:"methods"`
}

type ConstInfo struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

type ExtractDependenciesInput struct {
	Path string `json:"path" jsonschema:"Package or folder to analyze"`
}

type ExtractDependenciesOutput struct {
	Imports  []ImportInfo  `json:"imports"`
	Internal []DepRelation `json:"internal"` // internal package dependencies
}

type ImportInfo struct {
	Package string `json:"package"`
	Alias   string `json:"alias,omitempty"`
	Count   int    `json:"count"` // number of files importing this
}

type DepRelation struct {
	From string `json:"from"` // package path
	To   string `json:"to"`   // package path
}
