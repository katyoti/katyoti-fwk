// SPDX-License-Identifier: Apache-2.0

// extension-layout inventories every package of every extension against the
// layout convention (modules/framework/extension-layout.md): the role each
// package plays, the name the convention gives that role, how many packages
// import it, and what a rename would cost.
//
//	go run ./tools/extension-layout            print the inventory
//	go run ./tools/extension-layout --write    write modules/framework/_generated/extension-layout.md
//	go run ./tools/extension-layout --check    exit 1 when the committed file differs from a fresh run
//
// The rows come from `go list -e -json ./...` — the import graph, the package
// clause, the doc comment — never from memory. Roles are read from the path;
// the target of a deviation is the convention's decision, recorded in
// `decided` below so that the table and the page cannot disagree.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const module = "github.com/katyoti/katyoti-fwk"

// units are the extension roots the inventory walks: every folder under
// internal/extensions/, plus the sidecar contract extension that lives in
// appcore (modules/contributions/packages.md lists it with the extensions).
const extensionsRoot = "internal/extensions"

var extraUnits = []string{"internal/appcore/sidecar"}

// roles are the directory names the convention reserves, one per role.
var roles = map[string]string{
	"api":      "contract",
	"local":    "wiring",
	"contrib":  "producer",
	"lint":     "lint",
	"commands": "commands",
	"remote":   "remote",
	"plugin":   "plugin",
	"testing":  "testing",
}

// decided maps a deviating package (module-relative path) to the target the
// convention gives it. A target of "=" keeps the path and documents why; a
// target holding " + " is a split. Every entry carries its reason.
var decided = map[string]decision{
	// a producer wired as `local`
	"internal/extensions/docs/local": {"local + contrib", "services.go is the wiring; stencil_extension.go is a provider (stencilSetProvider) and moves to contrib/"},
	// same role, another name
	"internal/extensions/skills/agentplugin":             {"plugin", "the skill-access agent plugin, wired by ktw-agents"},
	"internal/extensions/task/internal/taskplugin":       {"internal/plugin/task", "the `task` plugin factory (complete / delegate / task-status)"},
	"internal/extensions/task/internal/contextplugin":    {"internal/plugin/context", "the `context` plugin factory"},
	"internal/extensions/agent/local/assembly":           {"internal/assembly", "wiring code under local/ that only local/ imports: private"},
	"internal/extensions/agent/local/compaction":         {"internal/plugin/compaction", "an agent plugin only the extension wires"},
	"internal/extensions/agent/local/keepalive":          {"plugin/keepalive", "an agent plugin a binary wires (ktw-agents): public"},
	"internal/extensions/agent/internal/plugin":          {"=", "the agent's plugin machinery (manager, provider, resolver): the owner of the concept, not an instance of it"},
	"internal/extensions/agent/local/loopsafety":         {"internal/plugin/loopsafety", "an agent plugin only the extension wires"},
	"internal/extensions/agent/local/user_comms_adapter": {"internal/plugin/user_comms_adapter", "an agent plugin only the extension wires"},
	"internal/extensions/manifest/teststub":              {"testing", "an in-memory MessageBus for tests"},
	"internal/extensions/events/smoke":                   {"internal/testing", "a test-only package (one _test.go file, no importer)"},
	"internal/extensions/marketplace/e2etest":            {"internal/testing", "a build-tagged test harness (katyoti_e2e), no importer; the Makefile names its path"},
	"internal/extensions/confluence/internal/synctest":   {"internal/testing", "the mock Confluence API for the sync acceptance tests"},
	"internal/extensions/connmgr/internal/fakesidecar":   {"internal/testing/fakesidecar", "a fake sidecar binary for the connmgr integration tests"},
	"internal/extensions/confluence/client":              {"remote", "the bus-backed ConfluenceService client"},
	"internal/appcore/sidecar/client":                    {"remote", "the typed clients of the sidecar contract services"},
	"internal/extensions/methods/methodsservice":         {"local", "publishes MethodsService: it is the extension's wiring"},
	// a role name taken by a domain package
	"internal/extensions/confluence/internal/api": {"internal/restapi", "`api` is the contract's name; this is the Confluence REST client"},
	// Go files directly in internal/
	"internal/extensions/testharness/internal":   {"internal/core", "the implementation sits in internal/core"},
	"internal/extensions/displaybridge/internal": {"internal/core", "the implementation sits in internal/core"},
	"internal/appcore/sidecar/internal":          {"internal/core", "the implementation sits in internal/core"},
	// a root package beside sub-packages
	"internal/extensions/humangate": {"local", "extension.go, plugin_ext.go and task_strategy.go are wiring"},
	"internal/extensions/tools":     {"local", "decorators.go registers the built-in tools' display decorators: wiring"},
	"internal/extensions/schema":    {"internal/core", "two _test.go files over testdata/corpus; tests sit beside the package they test"},
	"internal/extensions/tui":       {"local + internal/core", "run.go/app.go wire the TUI; the Bubbletea program is the implementation"},
	"internal/appcore/sidecar":      {"local", "factory.go and the providers are the sidecar contract's wiring"},
}

type decision struct {
	target string
	reason string
}

type pkg struct {
	ImportPath   string
	Name         string
	Dir          string
	Doc          string
	Imports      []string
	TestImports  []string
	XTestImports []string
	GoFiles      []string
	TestGoFiles  []string
	XTestGoFiles []string
	Error        *struct{ Err string }
}

type row struct {
	Ext      string
	Path     string // module-relative
	Rel      string // inside the unit ("" = the root package)
	Clause   string
	Role     string
	Target   string
	In, Out  int // importers inside / outside the unit (prod + test)
	Refs     int // mentions in Makefile, rules.yaml, arch tests, scripts
	Cost     string
	Note     string
	unitPath string
	external []string
}

const (
	costLeave    = "leave and document"
	costRename   = "rename-only"
	costImporter = "rename + importer edits"
	costSplit    = "needs a split"
	costDecision = "needs a decision"
)

var costOrder = []string{costRename, costImporter, costSplit, costDecision, costLeave}

func main() {
	write, check := false, false
	for _, a := range os.Args[1:] {
		switch a {
		case "--write":
			write = true
		case "--check":
			check = true
		default:
			fmt.Fprintf(os.Stderr, "unknown flag %q\n", a)
			os.Exit(2)
		}
	}
	root, err := repoRoot()
	if err != nil {
		fail(err)
	}
	pkgs, err := listPackages(root)
	if err != nil {
		fail(err)
	}
	pkgs = append(pkgs, excludedPackages(root, pkgs)...)
	rows := inventory(root, pkgs)
	out := render(rows)
	target := filepath.Join(root, "modules", "framework", "_generated", "extension-layout.md")
	switch {
	case write:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			fail(err)
		}
		if err := os.WriteFile(target, []byte(out), 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("wrote %s\n", rel(root, target))
		printSummary(rows)
	case check:
		have, err := os.ReadFile(target)
		if err != nil {
			fail(fmt.Errorf("%s: %w (run with --write)", rel(root, target), err))
		}
		if !bytes.Equal(bytes.ReplaceAll(have, []byte("\r\n"), []byte("\n")), []byte(out)) {
			fmt.Fprintf(os.Stderr, "%s is stale: run `go run ./tools/extension-layout --write`\n", rel(root, target))
			os.Exit(1)
		}
		fmt.Printf("%s is current\n", rel(root, target))
	default:
		fmt.Print(out)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "extension-layout:", err)
	os.Exit(1)
}

func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(r)
}

func repoRoot() (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		return "", fmt.Errorf("go list -m: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func listPackages(root string) ([]pkg, error) {
	cmd := exec.Command("go", "list", "-e", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("go list: %v\n%s", err, ee.Stderr)
		}
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []pkg
	for dec.More() {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

var packageClause = regexp.MustCompile(`(?m)^package\s+(\w+)`)

// excludedPackages walks the units for directories holding Go files that
// `go list ./...` did not report — a package whose every file carries a build
// constraint (marketplace/e2etest, tagged katyoti_e2e) is still a package of
// its extension and belongs in the inventory.
func excludedPackages(root string, listed []pkg) []pkg {
	known := map[string]bool{}
	for _, p := range listed {
		known[p.ImportPath] = true
	}
	var extra []pkg
	units := append([]string{}, extraUnits...)
	entries, _ := os.ReadDir(filepath.Join(root, extensionsRoot))
	for _, e := range entries {
		if e.IsDir() {
			units = append(units, extensionsRoot+"/"+e.Name())
		}
	}
	for _, u := range units {
		filepath.WalkDir(filepath.Join(root, u), func(path string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if n := d.Name(); n == "testdata" || n == "embed" || strings.HasPrefix(n, ".") {
				return filepath.SkipDir
			}
			ip := module + "/" + rel(root, path)
			if known[ip] {
				return nil
			}
			files, _ := filepath.Glob(filepath.Join(path, "*.go"))
			if len(files) == 0 {
				return nil
			}
			data, _ := os.ReadFile(files[0])
			name := filepath.Base(path)
			if m := packageClause.FindSubmatch(data); m != nil {
				name = string(m[1])
			}
			var goFiles []string
			for _, f := range files {
				goFiles = append(goFiles, filepath.Base(f))
			}
			extra = append(extra, pkg{ImportPath: ip, Name: name, Dir: path, GoFiles: goFiles,
				Error: &struct{ Err string }{"build constraints exclude all Go files"}})
			return nil
		})
	}
	return extra
}

// unitOf returns the unit path (module-relative) a package belongs to, and the
// extension name, or "" when the package is outside every unit.
func unitOf(modRel string) (unit, ext string) {
	if strings.HasPrefix(modRel, extensionsRoot+"/") {
		r := strings.TrimPrefix(modRel, extensionsRoot+"/")
		ext = strings.SplitN(r, "/", 2)[0]
		return extensionsRoot + "/" + ext, ext
	}
	for _, u := range extraUnits {
		if modRel == u || strings.HasPrefix(modRel, u+"/") {
			return u, strings.TrimPrefix(u, "internal/")
		}
	}
	return "", ""
}

func inventory(root string, pkgs []pkg) []row {
	byPath := map[string]*pkg{}
	for i := range pkgs {
		byPath[pkgs[i].ImportPath] = &pkgs[i]
	}
	// importers: target -> list of importing packages (prod + test)
	importers := map[string][]string{}
	for _, p := range pkgs {
		seen := map[string]bool{}
		for _, list := range [][]string{p.Imports, p.TestImports, p.XTestImports} {
			for _, imp := range list {
				if seen[imp] || imp == p.ImportPath {
					continue
				}
				seen[imp] = true
				importers[imp] = append(importers[imp], p.ImportPath)
			}
		}
	}
	refsText := loadRefsText(root)
	unitMembers := map[string]int{}
	for _, p := range pkgs {
		if u, _ := unitOf(strings.TrimPrefix(p.ImportPath, module+"/")); u != "" {
			unitMembers[u]++
		}
	}

	var rows []row
	for _, p := range pkgs {
		modRel := strings.TrimPrefix(p.ImportPath, module+"/")
		unit, ext := unitOf(modRel)
		if unit == "" {
			continue
		}
		r := row{Ext: ext, Path: modRel, unitPath: unit, Clause: p.Name}
		if modRel != unit {
			r.Rel = strings.TrimPrefix(modRel, unit+"/")
		}
		for _, imp := range importers[p.ImportPath] {
			impRel := strings.TrimPrefix(imp, module+"/")
			if impRel == unit || strings.HasPrefix(impRel, unit+"/") {
				r.In++
			} else {
				r.Out++
				r.external = append(r.external, impRel)
			}
		}
		r.Refs = strings.Count(refsText, modRel+"\n") + strings.Count(refsText, modRel+" ") + strings.Count(refsText, modRel+"\"") + strings.Count(refsText, modRel+"/...")
		classify(&r, &p, unitMembers[unit] == 1, byPath)
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Ext != rows[j].Ext {
			return rows[i].Ext < rows[j].Ext
		}
		return rows[i].Rel < rows[j].Rel
	})
	return rows
}

// loadRefsText concatenates the files where a package path may be named
// outside Go imports: a rename must edit these too.
func loadRefsText(root string) string {
	var b strings.Builder
	files := []string{"Makefile", "tools/import-discipline/rules.yaml"}
	for _, glob := range []string{"internal/test/arch/*.go", "scripts/*"} {
		m, _ := filepath.Glob(filepath.Join(root, glob))
		for _, f := range m {
			files = append(files, rel(root, f))
		}
	}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			continue
		}
		b.Write(data)
		b.WriteString("\n")
	}
	return b.String()
}

var wireDecl = regexp.MustCompile(`meta\.ServiceDef\{|ServiceDef = meta\.ServiceDef|NewTool\[`)

func classify(r *row, p *pkg, flat bool, byPath map[string]*pkg) {
	segs := strings.Split(r.Rel, "/")
	first := segs[0]
	if r.Rel == "" {
		segs = nil
		first = ""
	}
	setTarget := func(role, target string) {
		r.Role = role
		r.Target = target
	}
	d, decidedHere := decided[r.Path]

	switch {
	case r.Rel == "" && flat:
		setTarget("flat", "=")
	case r.Rel == "":
		setTarget("root beside sub-packages", "")
	case first == "internal" && len(segs) == 1:
		setTarget("files in internal/", "")
	case first == "internal" && segs[1] == "core":
		setTarget("implementation", "=")
	case first == "internal" && roles[segs[1]] != "" && segs[1] != "api" && segs[1] != "local" && segs[1] != "contrib" && segs[1] != "commands" && segs[1] != "remote":
		// internal/plugin, internal/testing, internal/lint: a role kept private
		setTarget(roles[segs[1]]+" (private)", "=")
	case first == "internal" && roles[segs[1]] != "":
		setTarget("domain named like a role", "")
	case first == "internal":
		setTarget("domain (private)", "=")
	case first == "api" && len(segs) > 1:
		setTarget("contract (shared types)", "=")
		if reaches(p.ImportPath, module+"/internal/appcore", byPath, map[string]bool{}) {
			r.Note = "closure reaches internal/appcore: not light"
		} else {
			r.Note = "light (closure clear of internal/appcore)"
		}
	case first == "local" && len(segs) > 1:
		setTarget("sub-package of local/", "")
	case roles[first] != "" && len(segs) == 1:
		setTarget(roles[first], "=")
	case roles[first] != "":
		setTarget(roles[first]+" ("+strings.Join(segs[1:], "/")+")", "=")
	case r.Ext == "tools" && len(segs) == 1:
		setTarget("tool (family member)", "=")
	default:
		// a domain package outside internal/
		if r.Out > 0 {
			setTarget("domain (public)", "=")
			ext := uniq(r.external)
			r.Note = fmt.Sprintf("imported outside the extension by %s", strings.Join(ext, ", "))
		} else {
			setTarget("domain (no external importer)", "internal/"+r.Rel)
		}
	}
	if decidedHere {
		r.Target = d.target
		if r.Note != "" {
			r.Note = d.reason + "; " + r.Note
		} else {
			r.Note = d.reason
		}
	}
	if r.Target == "" {
		r.Target = "?"
		r.Note = strings.TrimPrefix(r.Note+"; no decision recorded", "; ")
	}
	if p.Doc == "" && (r.Target == "=" && strings.HasPrefix(r.Role, "domain (public)")) {
		r.Note += "; package doc comment missing (the reason it is public is undocumented)"
	}
	if p.Error != nil {
		r.Note = strings.TrimPrefix(r.Note+"; build-constrained ("+firstLine(p.Error.Err)+")", "; ")
	}

	// cost
	switch {
	case r.Target == "=":
		r.Cost = costLeave
	case strings.Contains(r.Target, " + "):
		r.Cost = costSplit
	case r.Target == "?":
		r.Cost = costDecision
	case r.In+r.Out > 25:
		r.Cost = costDecision
		r.Note = strings.TrimPrefix(r.Note+"; more than 25 importers", "; ")
	case wireDecl.MatchString(readGoFiles(p)):
		r.Cost = costDecision
		r.Note = strings.TrimPrefix(r.Note+"; declares wire names (ServiceDef / NewTool): confirm none derives from the path before moving", "; ")
	case r.Out > 0 || r.Refs > 0:
		r.Cost = costImporter
	default:
		r.Cost = costRename
	}
	if r.Refs > 0 && r.Cost != costLeave {
		r.Note = strings.TrimPrefix(r.Note+fmt.Sprintf("; path named %d× in Makefile / rules.yaml / arch tests / scripts", r.Refs), "; ")
	}
}

func readGoFiles(p *pkg) string {
	var b strings.Builder
	for _, f := range p.GoFiles {
		data, err := os.ReadFile(filepath.Join(p.Dir, f))
		if err == nil {
			b.Write(data)
		}
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// reaches reports whether target is in the production import closure of from.
func reaches(from, target string, byPath map[string]*pkg, seen map[string]bool) bool {
	if seen[from] {
		return false
	}
	seen[from] = true
	p := byPath[from]
	if p == nil {
		return false
	}
	for _, imp := range p.Imports {
		if imp == target || strings.HasPrefix(imp, target+"/") {
			return true
		}
		if strings.HasPrefix(imp, module+"/") && reaches(imp, target, byPath, seen) {
			return true
		}
	}
	return false
}

func render(rows []row) string {
	var b strings.Builder
	b.WriteString("# Extension layout — the inventory\n\n")
	b.WriteString("Generated by `go run ./tools/extension-layout --write`; `--check` refuses a stale copy. Do not edit.\n\n")
	b.WriteString("Every package under `internal/extensions/*` and `internal/appcore/sidecar`, against the convention in [../extension-layout.md](../extension-layout.md): its role read from the path, the name the convention gives that role (`=` keeps the path), the importers inside and outside its extension (production and test packages, from `go list`), and the cost of the move.\n\n")

	exts := map[string]bool{}
	costs := map[string]int{}
	rolesCount := map[string]int{}
	for _, r := range rows {
		exts[r.Ext] = true
		costs[r.Cost]++
		rolesCount[r.Role]++
	}
	fmt.Fprintf(&b, "## Summary\n\n%d packages in %d extensions.\n\n", len(rows), len(exts))
	b.WriteString("| Cost | Packages |\n|---|---|\n")
	for _, c := range costOrder {
		fmt.Fprintf(&b, "| %s | %d |\n", c, costs[c])
	}
	b.WriteString("\n| Role | Packages |\n|---|---|\n")
	var roleNames []string
	for r := range rolesCount {
		roleNames = append(roleNames, r)
	}
	sort.Strings(roleNames)
	for _, r := range roleNames {
		fmt.Fprintf(&b, "| %s | %d |\n", r, rolesCount[r])
	}

	b.WriteString("\n## Packages\n\n")
	b.WriteString("| Extension | Package | Clause | Role | Target | In | Out | Cost | Note |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		clause := ""
		if r.Clause != filepath.Base(r.Path) && r.Clause != "main" {
			clause = "`" + r.Clause + "`"
		}
		fmt.Fprintf(&b, "| %s | `%s` | %s | %s | %s | %d | %d | %s | %s |\n",
			r.Ext, strings.TrimPrefix(r.Path, extensionsRoot+"/"), clause, r.Role, target(r), r.In, r.Out, r.Cost, esc(r.Note))
	}

	b.WriteString("\n## Renames by cost\n\n")
	for _, c := range costOrder {
		if c == costLeave {
			continue
		}
		var list []row
		for _, r := range rows {
			if r.Cost == c {
				list = append(list, r)
			}
		}
		if len(list) == 0 {
			continue
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].In+list[i].Out != list[j].In+list[j].Out {
				return list[i].In+list[i].Out < list[j].In+list[j].Out
			}
			return list[i].Path < list[j].Path
		})
		fmt.Fprintf(&b, "### %s (%d)\n\n", c, len(list))
		for _, r := range list {
			fmt.Fprintf(&b, "- `%s` → %s — %d importer(s) inside, %d outside", strings.TrimPrefix(r.Path, extensionsRoot+"/"), target(r), r.In, r.Out)
			if r.Note != "" {
				fmt.Fprintf(&b, "; %s", r.Note)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Package clause differs from the directory\n\n")
	b.WriteString("The convention names the package after its directory; an importer that needs another identifier aliases at the import site.\n\n")
	for _, r := range rows {
		if r.Clause != filepath.Base(r.Path) && r.Clause != "main" {
			fmt.Fprintf(&b, "- `%s` is `package %s` — %d importer(s)\n", strings.TrimPrefix(r.Path, extensionsRoot+"/"), r.Clause, r.In+r.Out)
		}
	}

	b.WriteString("\n## Deviations the guard allows\n\n")
	b.WriteString("Directory names directly under an extension that are not a role, `internal`, `testdata` or `embed` (the `tools` family excepted), and root packages beside sub-packages. `internal/test/arch/extension_layout_test.go` carries the same list; a new one fails there.\n\n")
	dev := map[string][]string{}
	rootBeside := []string{}
	internalFiles := []string{}
	for _, r := range rows {
		if !strings.HasPrefix(r.Path, extensionsRoot+"/") {
			continue
		}
		switch {
		case r.Role == "root beside sub-packages":
			rootBeside = append(rootBeside, r.Ext)
		case r.Role == "files in internal/":
			internalFiles = append(internalFiles, r.Ext)
		}
		segs := strings.Split(r.Rel, "/")
		if r.Rel == "" || segs[0] == "internal" || roles[segs[0]] != "" || r.Ext == "tools" {
			continue
		}
		if !contains(dev[r.Ext], segs[0]) {
			dev[r.Ext] = append(dev[r.Ext], segs[0])
		}
	}
	var devExts []string
	for e := range dev {
		devExts = append(devExts, e)
	}
	sort.Strings(devExts)
	n := 0
	for _, e := range devExts {
		sort.Strings(dev[e])
		n += len(dev[e])
		fmt.Fprintf(&b, "- `%s`: %s\n", e, strings.Join(quoteAll(dev[e]), ", "))
	}
	fmt.Fprintf(&b, "\n%d deviating directory names in %d extensions; root package beside sub-packages: %s; Go files directly in `internal/`: %s.\n",
		n, len(devExts), strings.Join(quoteAll(uniq(rootBeside)), ", "), strings.Join(quoteAll(uniq(internalFiles)), ", "))
	return b.String()
}

func target(r row) string {
	if r.Target == "=" {
		return "="
	}
	return "`" + r.Target + "`"
}

func esc(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func quoteAll(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = "`" + s + "`"
	}
	return out
}

func printSummary(rows []row) {
	costs := map[string]int{}
	for _, r := range rows {
		costs[r.Cost]++
	}
	fmt.Printf("%d packages\n", len(rows))
	for _, c := range costOrder {
		fmt.Printf("  %-26s %d\n", c, costs[c])
	}
}
