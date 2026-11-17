// SPDX-License-Identifier: Apache-2.0

package discovery

// Inventory is the pure-data result of a discovery walk. Every slice
// is sorted (see sortInventory in discovery.go) so consecutive runs
// against unchanged source produce byte-identical output — that's
// the §13 LOCKED determinism contract.
type Inventory struct {
	Packages      []*PackageInfo
	PublicSymbols []*SymbolInfo
	TestFunctions []*TestFunctionInfo
	Imports       []*ImportInfo
	LayoutHints   *LayoutHints
}

// PackageInfo summarises one Go package directory.
type PackageInfo struct {
	Path          string // relative to scan root
	Name          string // Go package declaration name
	IsTest        bool   // _test.go-only package
	IsCmd         bool   // contains main()
	PublicExports int    // count of exported identifiers
}

// SymbolKind classifies a SymbolInfo entry.
type SymbolKind string

const (
	SymbolKindFunc      SymbolKind = "func"
	SymbolKindType      SymbolKind = "type"
	SymbolKindInterface SymbolKind = "interface"
	SymbolKindStruct    SymbolKind = "struct"
	SymbolKindVar       SymbolKind = "var"
	SymbolKindConst     SymbolKind = "const"
)

// SymbolInfo describes a single exported identifier — function,
// type, interface, struct, var or const.
type SymbolInfo struct {
	Name       string     // exported identifier
	Kind       SymbolKind // see SymbolKind*
	Package    string     // owning package directory (relative to scan root)
	Doc        string     // godoc body, trimmed
	Receiver   string     // for methods: receiver type (T or *T)
	Signature  string     // compact signature for funcs ("Foo(a int) error")
	IsExported bool       // always true in current output (filter applied)
}

// TestKind classifies a TestFunctionInfo entry.
type TestKind string

const (
	TestKindTest      TestKind = "test"
	TestKindBenchmark TestKind = "benchmark"
	TestKindExample   TestKind = "example"
	TestKindFuzz      TestKind = "fuzz"
)

// TestFunctionInfo describes one TestXxx / BenchmarkXxx /
// ExampleXxx / FuzzXxx function found in a _test.go file. The
// TestRef field follows the convention
// `<file-relative-to-root>::<func-name>` — that's the canonical
// form the tester persona quotes in UC DoD bodies.
type TestFunctionInfo struct {
	Name    string
	Package string
	File    string
	Kind    TestKind
	TestRef string
	Doc     string
}

// ImportInfo describes one edge in the import graph.
type ImportInfo struct {
	Importer string // owning package directory (relative to scan root)
	Imported string // imported package path (full, e.g. github.com/foo/bar)
	IsLocal  bool   // true when Imported starts with the host go.mod's module path
}

// LayoutHints holds heuristic detection of the 3-package convention
// (modules/code-gocode/concepts.md §2). Populated even when the scan finds no Go files,
// because the directory structure alone is informative.
type LayoutHints struct {
	HasPublicPkg   bool
	HasInternalPkg bool
	HasLocalPkg    bool
	HasAppCmd      bool   // cmd/main.go or root main.go
	AppFolder      string // app/<name>/ if the scan root sits under one
	SuggestedKind  string // "framework" | "library" | "sidecar" | "unclear"
}

// Options tweak the walk + filter behaviour. The zero value
// reflects the defaults the §13 LOCKED decision specifies:
// include tests; skip generated files; no file-size limit.
type Options struct {
	// IncludeTests scans _test.go files when true (default).
	IncludeTests bool
	// IncludeGenerated includes `*_gen.go` / `*.pb.go` and files
	// with the canonical "Code generated … DO NOT EDIT" header.
	// Default false — generated code is noise for retro-engineering.
	IncludeGenerated bool
	// MaxFileBytes skips files larger than this (defends against
	// gigantic generated tables). 0 = no limit.
	MaxFileBytes int64
}

// DefaultOptions returns the canonical defaults (per modules/code-gocode/concepts.md §3):
// include tests, skip generated, no size limit.
func DefaultOptions() Options {
	return Options{
		IncludeTests:     true,
		IncludeGenerated: false,
		MaxFileBytes:     0,
	}
}
