// SPDX-License-Identifier: Apache-2.0

// Package packages is code-gocode's derived store of Go package facts: a JSON
// file per package under a cache directory, recomputed when the package's
// files change (their size or modification time, looked at no more than once
// per TTL), and the analyses recorded on each package with the basis they were
// made on. It parses the files itself (go/parser), never running `go list`.
package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	reviewsapi "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// DefaultTTL is how long a package's file listing is trusted before it is
// looked at again.
const DefaultTTL = 30 * time.Second

// Store answers api.PackageFactsService over a repository root.
type Store struct {
	root  string // the repository root
	dir   string // where the facts are kept
	ttl   time.Duration
	now   func() time.Time
	mu    sync.Mutex
	cache map[string]*entry
	gomod string // the module path of go.mod
}

// entry is a package as the store keeps it.
type entry struct {
	Facts     api.PackageFacts `json:"facts"`
	Stamp     string           `json:"stamp"` // the files' names, sizes and times
	CheckedAt time.Time        `json:"checked_at"`
}

// New is a store over root keeping its facts in dir.
func New(root, dir string) *Store {
	return &Store{root: root, dir: dir, ttl: DefaultTTL, now: time.Now, cache: map[string]*entry{}}
}

// WithClock sets the store's clock and TTL (tests).
func (s *Store) WithClock(now func() time.Time, ttl time.Duration) *Store {
	s.now, s.ttl = now, ttl
	return s
}

func clean(path string) string {
	return strings.Trim(strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(path)), "./"), "/")
}

// PackageFacts implements api.PackageFactsService.
func (s *Store) PackageFacts(_ context.Context, in api.PackageFactsInput) (api.PackageFactsOutput, error) {
	path := clean(in.Path)
	if path == "" {
		return api.PackageFactsOutput{}, fmt.Errorf("%w: path required", api.ErrInvalidInput)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.load(path)
	if err != nil {
		return api.PackageFactsOutput{}, err
	}
	f := e.Facts
	f.Analyses = s.withStaleness(path, f.Analyses)
	return api.PackageFactsOutput{Facts: f}, nil
}

// PackageList implements api.PackageFactsService: the packages the module
// owns (its code_packages, the longest declared path winning).
func (s *Store) PackageList(_ context.Context, in api.PackageListInput) (api.PackageListOutput, error) {
	if in.Module == "" {
		return api.PackageListOutput{}, fmt.Errorf("%w: module required", api.ErrInvalidInput)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	owners := s.owners()
	var out api.PackageListOutput
	for _, pkg := range s.goPackages() {
		if owner(pkg, owners) != in.Module {
			continue
		}
		e, err := s.load(pkg)
		if err != nil {
			return out, err
		}
		f := e.Facts
		f.Analyses = s.withStaleness(pkg, f.Analyses)
		out.Packages = append(out.Packages, f)
	}
	return out, nil
}

// PackageRecord implements api.PackageFactsService: the analysis replaces the
// package's previous one of the same kind, with today's basis.
func (s *Store) PackageRecord(_ context.Context, in api.PackageRecordInput) (api.PackageRecordOutput, error) {
	path := clean(in.Path)
	if path == "" || in.Kind == "" || in.Verdict == "" {
		return api.PackageRecordOutput{}, fmt.Errorf("%w: path, kind and verdict required", api.ErrInvalidInput)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.load(path)
	if err != nil {
		return api.PackageRecordOutput{}, err
	}
	basis := e.Facts.Basis
	if in.Deep {
		if basis, err = s.deepBasis(path, map[string]bool{}); err != nil {
			return api.PackageRecordOutput{}, err
		}
	}
	a := api.PackageAnalysis{Kind: in.Kind, At: s.now().UTC().Format(time.RFC3339), Verdict: in.Verdict,
		Detail: in.Detail, Deep: in.Deep, Basis: basis}
	kept := e.Facts.Analyses[:0]
	for _, x := range e.Facts.Analyses {
		if x.Kind != in.Kind {
			kept = append(kept, x)
		}
	}
	e.Facts.Analyses = append(kept, a)
	sort.Slice(e.Facts.Analyses, func(i, j int) bool { return e.Facts.Analyses[i].Kind < e.Facts.Analyses[j].Kind })
	if err := s.save(path, e); err != nil {
		return api.PackageRecordOutput{}, err
	}
	return api.PackageRecordOutput{Analysis: a}, nil
}

// withStaleness marks each analysis whose basis moved.
func (s *Store) withStaleness(path string, as []api.PackageAnalysis) []api.PackageAnalysis {
	out := make([]api.PackageAnalysis, len(as))
	for i, a := range as {
		now := ""
		if e, err := s.load(path); err == nil {
			now = e.Facts.Basis
		}
		if a.Deep {
			now, _ = s.deepBasis(path, map[string]bool{})
		}
		a.Stale = now != a.Basis
		out[i] = a
	}
	return out
}

// deepBasis is the package's basis folded with its repository imports' deep
// bases (Go import graphs have no cycles; seen guards a malformed tree).
func (s *Store) deepBasis(path string, seen map[string]bool) (string, error) {
	if seen[path] {
		return "", nil
	}
	seen[path] = true
	e, err := s.load(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(e.Facts.Basis))
	for _, imp := range e.Facts.Imports {
		b, err := s.deepBasis(imp, seen)
		if err != nil {
			continue // an import outside the tree reads as nothing
		}
		h.Write([]byte(imp + "=" + b + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// load is the package's entry: kept when its files did not change, else
// recomputed and saved.
func (s *Store) load(path string) (*entry, error) {
	e := s.cache[path]
	if e == nil {
		e = s.read(path)
	}
	if e != nil && s.now().Sub(e.CheckedAt) < s.ttl {
		return e, nil
	}
	stamp, files, err := s.stamp(path)
	if err != nil {
		return nil, err
	}
	if e != nil && e.Stamp == stamp {
		e.CheckedAt = s.now()
		s.cache[path] = e
		return e, nil
	}
	facts, err := s.compute(path, files)
	if err != nil {
		return nil, err
	}
	if e != nil {
		facts.Analyses = e.Facts.Analyses // analyses outlive a change; their basis says if they still hold
	}
	ne := &entry{Facts: facts, Stamp: stamp, CheckedAt: s.now()}
	if err := s.save(path, ne); err != nil {
		return nil, err
	}
	return ne, nil
}

// stamp is the package's .go files with their sizes and modification times.
func (s *Store) stamp(path string) (string, []string, error) {
	dir := filepath.Join(s.root, filepath.FromSlash(path))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, fmt.Errorf("%w: package %s: %v", api.ErrInvalidInput, path, err)
	}
	var b strings.Builder
	var files []string
	for _, d := range ents {
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			continue
		}
		info, err := d.Info()
		if err != nil {
			return "", nil, err
		}
		files = append(files, d.Name())
		fmt.Fprintf(&b, "%s:%d:%d\n", d.Name(), info.Size(), info.ModTime().UnixNano())
	}
	if len(files) == 0 {
		return "", nil, fmt.Errorf("%w: %s holds no Go file", api.ErrInvalidInput, path)
	}
	sort.Strings(files)
	return b.String(), files, nil
}

// compute reads the package's files: their hashes and basis, its repository
// imports, its package comment, its owner and kind.
func (s *Store) compute(path string, files []string) (api.PackageFacts, error) {
	f := api.PackageFacts{Path: path, Kind: api.PackageKind(path), Owner: owner(path, s.owners())}
	basis := sha256.New()
	imports := map[string]bool{}
	fset := token.NewFileSet()
	modPath := s.modulePath()
	for _, name := range files {
		full := filepath.Join(s.root, filepath.FromSlash(path), name)
		data, err := os.ReadFile(full)
		if err != nil {
			return f, err
		}
		sum := sha256.Sum256(data)
		test := strings.HasSuffix(name, "_test.go")
		f.Files = append(f.Files, api.PackageFile{Name: name, Test: test, Hash: hex.EncodeToString(sum[:8])})
		basis.Write([]byte(name + "\x00"))
		basis.Write(data)
		if test {
			continue
		}
		ast, err := parser.ParseFile(fset, full, data, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			continue // a file that does not parse still counts in the basis
		}
		if f.Description == "" && ast.Doc != nil {
			f.Description = firstSentence(ast.Doc.Text())
		}
		for _, imp := range ast.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if modPath != "" && strings.HasPrefix(p, modPath+"/") {
				imports[strings.TrimPrefix(p, modPath+"/")] = true
			}
		}
	}
	f.Basis = hex.EncodeToString(basis.Sum(nil))[:16]
	for p := range imports {
		f.Imports = append(f.Imports, p)
	}
	sort.Strings(f.Imports)
	return f, nil
}

func firstSentence(doc string) string {
	doc = strings.Join(strings.Fields(doc), " ")
	if i := strings.Index(doc, ". "); i > 0 {
		return doc[:i+1]
	}
	return doc
}

// modulePath is the module path go.mod declares.
func (s *Store) modulePath() string {
	if s.gomod != "" {
		return s.gomod
	}
	data, err := os.ReadFile(filepath.Join(s.root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if p, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			s.gomod = strings.TrimSpace(p)
			break
		}
	}
	return s.gomod
}

// owners maps each declared code package path to the module declaring it.
func (s *Store) owners() map[string]string {
	files, _ := filepath.Glob(filepath.Join(s.root, "modules", "*", "objects", "*.cwm.yaml"))
	out := map[string]string{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var obj struct {
			ID           string `json:"id" yaml:"id"`
			Capabilities struct {
				Code struct {
					CodePackages []string `json:"code_packages" yaml:"code_packages"`
				} `json:"code" yaml:"code"`
			} `json:"capabilities" yaml:"capabilities"`
		}
		if config.DecodeBytes(data, marshal.YAML, &obj) != nil {
			continue
		}
		for _, p := range obj.Capabilities.Code.CodePackages {
			if p = strings.TrimSuffix(clean(p), "/..."); p != "" {
				out[p] = obj.ID
			}
		}
	}
	return out
}

// ChangeOwners implements api.PackageFactsService: each changed .go file goes
// to the module owning its package (the longest code_packages path); a file
// no module owns is left out. Owners in name order, their paths as given.
func (s *Store) ChangeOwners(_ context.Context, in reviewsapi.ChangeOwnersInput) (reviewsapi.ChangeOwnersOutput, error) {
	s.mu.Lock()
	owners := s.owners()
	s.mu.Unlock()
	byOwner := map[string][]string{}
	for _, p := range in.Paths {
		p = clean(p)
		if !strings.HasSuffix(p, ".go") {
			continue
		}
		if m := owner(clean(path.Dir(p)), owners); m != "" && !slices.Contains(byOwner[m], p) {
			byOwner[m] = append(byOwner[m], p)
		}
	}
	out := reviewsapi.ChangeOwnersOutput{Owners: []reviewsapi.ChangeOwner{}}
	for _, m := range slices.Sorted(maps.Keys(byOwner)) {
		out.Owners = append(out.Owners, reviewsapi.ChangeOwner{Object: m, Paths: byOwner[m]})
	}
	return out, nil
}

// owner is the module owning pkg: the longest declared path holding it.
func owner(pkg string, owners map[string]string) string {
	best := ""
	for p := range owners {
		if (pkg == p || strings.HasPrefix(pkg, p+"/")) && len(p) > len(best) {
			best = p
		}
	}
	return owners[best]
}

// goPackages are the directories under root holding a .go file, as `go list`
// would see them (dot- and underscore-prefixed, testdata and node_modules
// skipped).
func (s *Store) goPackages() []string {
	seen := map[string]bool{}
	_ = filepath.WalkDir(s.root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		n := d.Name()
		if d.IsDir() {
			if p != s.root && (strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") || n == "testdata" || n == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
			if rel, err := filepath.Rel(s.root, filepath.Dir(p)); err == nil && rel != "." {
				seen[filepath.ToSlash(rel)] = true
			}
		}
		return nil
	})
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// file is where a package's entry is kept.
func (s *Store) file(path string) string {
	sum := sha256.Sum256([]byte(path))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:8])+".json")
}

func (s *Store) read(path string) *entry {
	data, err := os.ReadFile(s.file(path))
	if err != nil {
		return nil
	}
	var e entry
	if json.Unmarshal(data, &e) != nil || e.Facts.Path != path {
		return nil
	}
	e.CheckedAt = time.Time{} // a restart looks at the files again
	return &e
}

func (s *Store) save(path string, e *entry) error {
	s.cache[path] = e
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.file(path), data, 0o644)
}
