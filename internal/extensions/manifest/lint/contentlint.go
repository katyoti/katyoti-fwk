// SPDX-License-Identifier: Apache-2.0

// Package contentlint checks what a skill or task BODY refers to — relative
// links, resource URIs, tool URIs — against what a universe of manifests
// actually publishes, and runs each kind's format lint on the way.
//
// It is neutral: it knows the URI grammar and the manifest shape, and asks
// the owning packs (skills/lint, task/lint) what a well-formed file is. Only
// ktw-compose and test code import it — a sidecar's census lints the
// sidecar's own content at build without linking one extra package into
// the executable, and ktw-compose lints any compiled sidecar, or a whole
// folder of them, from their manifests (ms-contribution-ext e10).
//
// Two scopes. SELF checks a binary against itself: relative links must
// resolve to something the binary publishes, references to its own provider
// must exist, and references to OTHER providers are left alone — one binary
// cannot know what another publishes, and pretending otherwise would make
// every sidecar's build depend on every other's. UNIVERSE checks a set of
// binaries against each other: every provider named must exist and every
// reference must resolve.
//
// The folder is `lint`, the role the extension layout reserves for every
// extension's linter; the package name says which lint this is, beside
// skills/lint and task/lint, and its importers name it so.
package contentlint

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/el"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	guidelint "github.com/katyoti/katyoti-fwk/internal/extensions/guide/lint"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
	skilllint "github.com/katyoti/katyoti-fwk/internal/extensions/skills/lint"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib/deckkind"
	decklint "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/lint"
	tasklint "github.com/katyoti/katyoti-fwk/internal/extensions/task/lint"
)

// Scope selects what a reference is checked against.
type Scope int

const (
	// ScopeSelf: one binary against itself; foreign providers are not judged.
	ScopeSelf Scope = iota
	// ScopeUniverse: every provider must exist in the universe.
	ScopeUniverse
)

// RefKind classifies a reference found in a body.
type RefKind string

const (
	RefLink     RefKind = "link"     // a relative markdown link
	RefResource RefKind = "resource" // <provider>:/<type>/<id>
	RefTool     RefKind = "tool"     // <provider>:<slot>/<method>
)

// Ref is one reference found in a body.
type Ref struct {
	Kind RefKind
	Text string
}

var (
	reFence = regexp.MustCompile("(?s)```.*?```")
	reSpan  = regexp.MustCompile("`[^`\n]*`")
	reLink  = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	reRes   = regexp.MustCompile(`\b([a-z][a-z0-9-]*):/([a-z][a-z0-9-]*)/([A-Za-z0-9._/-]+)`)
	reTool  = regexp.MustCompile(`\b([a-z][a-z0-9-]*):([a-z][a-z0-9-]*)/([a-z][a-z0-9-]*(?:/[a-z0-9-]+)*)\b`)
)

// ExtractRefs finds every reference in a body. A relative link is one that
// is not absolute, not a URL, not an anchor and not a template (`{…}`,
// `<…>`, `…`) — those are prose, not references. A wildcard tool URI
// (`ktw-example:example/*`) is a selector, not a reference, and is skipped.
func ExtractRefs(body string) []Ref {
	// Fenced code is never prose. Inline spans are stripped for LINK syntax
	// only (a Go expression like UseService[X](ctx) reads as a link) — a URI
	// quoted in a span is exactly how skills cite resources and tools.
	prose := withoutFences(body)
	linkText := withoutSpans(prose)
	seen := map[string]bool{}
	var out []Ref
	add := func(k RefKind, t string) {
		key := string(k) + "|" + t
		if !seen[key] {
			seen[key] = true
			out = append(out, Ref{Kind: k, Text: t})
		}
	}
	for _, m := range reLink.FindAllStringSubmatch(linkText, -1) {
		t := m[1]
		if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") || strings.HasPrefix(t, "mailto:") || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.ContainsAny(t, "{}<>…") || strings.Contains(t, ":/") || strings.Contains(t, ":") {
			continue // template, or a URI handled below
		}
		t = strings.SplitN(t, "#", 2)[0]
		if !strings.ContainsAny(t, "/.") {
			continue // `[text](url)`, `[x](ctx)`: a placeholder word, not a path
		}
		add(RefLink, t)
	}
	for _, idx := range reRes.FindAllStringSubmatchIndex(prose, -1) {
		whole, prov, typ, id := prose[idx[0]:idx[1]], prose[idx[2]:idx[3]], prose[idx[4]:idx[5]], prose[idx[6]:idx[7]]
		if strings.ContainsAny(id, "{}<>…*") || templateContinues(prose, idx[1]) {
			continue // `ktw-x:/skill/doc-<type>` names a family, not a resource
		}
		_ = whole
		add(RefResource, prov+":/"+typ+"/"+strings.TrimRight(id, "."))
	}
	for _, idx := range reTool.FindAllStringSubmatchIndex(prose, -1) {
		if strings.Contains(prose[idx[0]:idx[1]], "*") || templateContinues(prose, idx[1]) {
			continue
		}
		add(RefTool, prose[idx[2]:idx[3]]+":"+prose[idx[4]:idx[5]]+"/"+prose[idx[6]:idx[7]])
	}
	return out
}

// Universe is what a set of manifests publishes, indexed for the checks.
type Universe struct {
	providers map[string]bool
	resources map[string]bool            // resolved URI
	byProv    map[string][]string        // provider → resource URIs (for link suffix matches)
	slots     map[string]bool            // provider|slot
	methods   map[string]map[string]bool // provider|slot → method names (nil when the manifest carries no method list)
}

// NewUniverse indexes manifests.
func NewUniverse(ms []*mbapi.Manifest) *Universe {
	u := &Universe{providers: map[string]bool{}, resources: map[string]bool{}, byProv: map[string][]string{}, slots: map[string]bool{}, methods: map[string]map[string]bool{}}
	for _, m := range ms {
		if m == nil {
			continue
		}
		u.providers[m.Provider] = true
		for _, c := range m.Contributions {
			uri := c.ResolvedURI()
			u.resources[uri] = true
			p := c.Provider
			if p == "" {
				p = m.Provider
			}
			u.providers[p] = true
			u.byProv[p] = append(u.byProv[p], uri)
		}
		typeMethods := map[string]map[string]bool{}
		for _, st := range m.SlotTypes {
			set := map[string]bool{}
			for _, meth := range st.Methods {
				set[meth.Name] = true
			}
			typeMethods[st.Type] = set
		}
		for _, s := range m.Slots {
			key := m.Provider + "|" + s.Name
			u.slots[key] = true
			if set, ok := typeMethods[s.Type]; ok && len(set) > 0 {
				u.methods[key] = set
			}
		}
	}
	return u
}

// HasProvider reports whether any manifest publishes under p.
func (u *Universe) HasProvider(p string) bool { return u.providers[p] }

// HasResource reports whether uri, or a folder under it, is published.
func (u *Universe) HasResource(uri string) bool {
	if u.resources[uri] {
		return true
	}
	prefix := strings.TrimSuffix(uri, "/") + "/"
	for r := range u.resources {
		if strings.HasPrefix(r, prefix) {
			return true
		}
	}
	return false
}

// HasSlot reports whether provider publishes a slot of that name.
func (u *Universe) HasSlot(provider, slot string) bool { return u.slots[provider+"|"+slot] }

// HasMethod reports (known, ok): known is false when the manifest carries no
// method list for the slot, in which case nothing can be said.
func (u *Universe) HasMethod(provider, slot, method string) (known, ok bool) {
	set := u.methods[provider+"|"+slot]
	if set == nil {
		return false, false
	}
	return true, set[method] || set[kebabToPascal(method)]
}

// LinkResolves reports whether a relative link from an entry of provider p
// names something p publishes: a resource whose id ends with the link path.
func (u *Universe) LinkResolves(p, rel string) bool {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	candidates := []string{rel, path.Base(rel)}
	// A sibling skill or task file links as `cwm-search.md`; its entry is
	// `<p>:/skill/cwm-search` — the id carries no extension.
	if ext := path.Ext(rel); ext == ".md" {
		candidates = append(candidates, strings.TrimSuffix(rel, ext), strings.TrimSuffix(path.Base(rel), ext))
	}
	for _, uri := range u.byProv[p] {
		for _, c := range candidates {
			if strings.HasSuffix(uri, "/"+c) {
				return true
			}
			// A link to a FOLDER (`diagrams/mermaid/`, `verbs/`) names the
			// entries published under it; it resolves when one exists.
			if strings.Contains(uri, "/"+c+"/") {
				return true
			}
		}
	}
	return false
}

// unknownProviderSeverity: a provider nothing publishes is an error when the
// token is shaped like one of ours (`ktw-*`, `system`, `katyoti`) — that is a
// typo or a renamed sidecar — and a warning otherwise, because `a:b/c` is also
// an OData filter (`m:m/email`), a Go embed pattern (`all:embed/x`) or a
// predicate (`exists:app/x`): worth a look, not a broken universe.
func unknownProviderSeverity(p string) verify.LintSeverity {
	if strings.HasPrefix(p, "ktw-") || p == "system" || p == "katyoti" {
		return verify.SeverityError
	}
	return verify.SeverityWarn
}

// assetDirs are the folders a skill addresses by relative path FROM the
// skill (`references/x.md`): its own files, served by the skill loader,
// never catalog entries. A manifest cannot see them, so a link into one is
// accepted in manifest mode and checked on disk in file mode.
var assetDirs = map[string]bool{"references": true, "scripts": true, "assets": true}

func isAssetLink(rel string) bool {
	first, _, _ := strings.Cut(strings.TrimPrefix(rel, "./"), "/")
	return assetDirs[first]
}

// CheckRefs judges every reference of an entry published by `self`.
func CheckRefs(self string, refs []Ref, u *Universe, scope Scope) []verify.LintFinding {
	return checkRefs(self, refs, u, scope, nil)
}

// CheckRefsOnDisk is CheckRefs for a FILE: a relative link is resolved
// against the file's folder (`exists`), so a skill's own assets are checked
// too; URIs are judged against `u` when one is given, else left alone.
func CheckRefsOnDisk(self string, refs []Ref, u *Universe, scope Scope, exists func(rel string) bool) []verify.LintFinding {
	return checkRefs(self, refs, u, scope, exists)
}

func checkRefs(self string, refs []Ref, u *Universe, scope Scope, exists func(rel string) bool) []verify.LintFinding {
	var out []verify.LintFinding
	for _, r := range refs {
		switch r.Kind {
		case RefLink:
			if exists != nil {
				if !exists(r.Text) {
					out = append(out, verify.LintFinding{Rule: "dead-link", Severity: verify.SeverityError,
						Message: fmt.Sprintf("relative link %q names no file next to this one", r.Text)})
				}
				continue
			}
			if isAssetLink(r.Text) {
				continue
			}
			if !u.LinkResolves(self, r.Text) {
				out = append(out, verify.LintFinding{Rule: "dead-link", Severity: verify.SeverityError,
					Message: fmt.Sprintf("relative link %q names nothing this provider publishes", r.Text)})
			}
		case RefResource:
			if u == nil {
				continue
			}
			p := r.Text[:strings.Index(r.Text, ":")]
			if !u.HasProvider(p) {
				if scope == ScopeUniverse || p == self {
					out = append(out, verify.LintFinding{Rule: "unknown-provider", Severity: unknownProviderSeverity(p),
						Message: fmt.Sprintf("%q names provider %q, which nothing publishes", r.Text, p)})
				}
				continue
			}
			if scope == ScopeSelf && p != self {
				continue
			}
			if !u.HasResource(r.Text) {
				out = append(out, verify.LintFinding{Rule: "unknown-resource", Severity: verify.SeverityError,
					Message: fmt.Sprintf("%q is not a published resource of %s", r.Text, p)})
			}
		case RefTool:
			if u == nil {
				continue
			}
			p := r.Text[:strings.Index(r.Text, ":")]
			rest := r.Text[len(p)+1:]
			slot, method, _ := strings.Cut(rest, "/")
			if !u.HasProvider(p) {
				if scope == ScopeUniverse || p == self {
					out = append(out, verify.LintFinding{Rule: "unknown-provider", Severity: unknownProviderSeverity(p),
						Message: fmt.Sprintf("%q names provider %q, which nothing publishes", r.Text, p)})
				}
				continue
			}
			if scope == ScopeSelf && p != self {
				continue
			}
			if slot == "tasks" || slot == "scripts" {
				continue // action slots, dispatched by the ActionService, not KTSP slots
			}
			if !u.HasSlot(p, slot) {
				out = append(out, verify.LintFinding{Rule: "unknown-slot", Severity: verify.SeverityError,
					Message: fmt.Sprintf("%q names slot %q, which %s does not publish", r.Text, slot, p)})
				continue
			}
			if known, ok := u.HasMethod(p, slot, method); known && !ok {
				out = append(out, verify.LintFinding{Rule: "unknown-method", Severity: verify.SeverityError,
					Message: fmt.Sprintf("%q names method %q, which slot %s/%s does not publish", r.Text, method, p, slot)})
			}
		}
	}
	return out
}

// Finding is one lint finding on one entry.
// Finding is verify.ResourceFinding: the type is owned by the neutral pack so
// api packages can name it without importing a lint package.
type Finding = verify.ResourceFinding

// Opener returns the bytes of one layer of one entry — the census answers
// from the providers' enumeration, ktw-compose from `resources/open-layer`.
type Opener func(uri, layer string) ([]byte, error)

// LintManifest lints every skill, task and guide recipe of `target` — format by kind, then
// every reference — against `u`, at `scope`. Entries whose source cannot be
// opened are a finding, never a skip: a skill without readable source is a
// skill nobody can read.
func LintManifest(target *mbapi.Manifest, open Opener, u *Universe, scope Scope) []Finding {
	var out []Finding
	if target == nil {
		return nil
	}
	kinds := contribapi.KindsFrom(target.Contributions)
	sys, _ := builtin.Kinds()
	for _, c := range target.Contributions {
		_, formatted := formatChecks[c.Type]
		if !formatted && !markdownEntry(c) {
			continue
		}
		uri := c.ResolvedURI()
		src, err := open(uri, contribapi.LayerSource)
		if err != nil || len(src) == 0 {
			if !formatted || !sourceRequired(kinds, sys, c.Type) {
				continue // a reference's links are checked when its bytes are at hand; an entry with a format check and a required source is a defect without it
			}
			out = append(out, Finding{URI: uri, LintFinding: verify.LintFinding{Rule: "source-unreadable", Severity: verify.SeverityError,
				Message: fmt.Sprintf("source layer cannot be opened: %v", err)}})
			continue
		}
		self := c.Provider
		if self == "" {
			self = target.Provider
		}
		for _, f := range LintBytes(c.Type, src, self, u, scope).Findings {
			out = append(out, Finding{URI: uri, LintFinding: f})
		}
	}
	out = append(out, lintManifestTranslations(target, open)...)
	out = append(out, lintManifestExpressions(target)...)
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// lintManifestTranslations lints the translations of every entry whose kind
// declares translatable texts, or which carries a translations layer — one
// lint for every kind (LintTranslations).
func lintManifestTranslations(target *mbapi.Manifest, open Opener) []Finding {
	kinds := contribapi.KindsFrom(target.Contributions)
	sys, _ := builtin.Kinds()
	packLangs := PackSourceLangs(target)
	var out []Finding
	for _, c := range target.Contributions {
		paths := kinds.Translatable(c.Type)
		if _, declared := kinds.Get(c.Type); !declared {
			paths = sys.Translatable(c.Type)
		}
		ref, has := c.Layers[contribapi.LayerTranslations]
		if len(paths) == 0 && !has {
			continue
		}
		uri := c.ResolvedURI()
		var tr []byte
		if has {
			tr = ref.Inline
			if tr == nil {
				b, err := open(uri, contribapi.LayerTranslations)
				if err != nil {
					out = append(out, Finding{URI: uri, LintFinding: verify.LintFinding{Rule: "translations-unreadable", Severity: verify.SeverityError,
						Message: fmt.Sprintf("translations layer cannot be opened: %v", err)}})
					continue
				}
				tr = b
			}
		}
		for _, f := range LintTranslations(paths, TranslatableData(c), tr, packLangs[c.Provider]) {
			out = append(out, Finding{URI: uri, LintFinding: f})
		}
	}
	return out
}

// lintManifestExpressions compiles every expression an entry's kind declares
// (KindDeclaration.Expressions) against the object vocabulary: a called name
// stands for an expression function (a capability of any member may declare
// it), any other name the vocabulary does not know is an error - the pane
// would never show what the expression guards.
func lintManifestExpressions(target *mbapi.Manifest) []Finding {
	kinds := contribapi.KindsFrom(target.Contributions)
	sys, _ := builtin.Kinds()
	var out []Finding
	for _, c := range target.Contributions {
		paths := kinds.Expressions(c.Type)
		if _, declared := kinds.Get(c.Type); !declared {
			paths = sys.Expressions(c.Type)
		}
		if len(paths) == 0 {
			continue
		}
		var data map[string]any
		if json.Unmarshal(TranslatableData(c), &data) != nil {
			continue
		}
		for _, p := range paths {
			expr, _ := dotted(data, p).(string)
			if strings.TrimSpace(expr) == "" {
				continue
			}
			if err := CheckObjectExpression(expr); err != nil {
				out = append(out, Finding{URI: c.ResolvedURI(), LintFinding: verify.LintFinding{Rule: "expression-unknown-name", Severity: verify.SeverityError,
					Message: fmt.Sprintf("%s: %v", p, err)}})
			}
		}
	}
	return out
}

// CheckObjectExpression compiles an el predicate over one object against the
// object vocabulary, each name it calls standing for an expression function:
// nil when every other name it reads is the vocabulary's.
func CheckObjectExpression(expr string) error {
	calls, err := calledNames(expr)
	if err != nil {
		return err
	}
	_, err = el.NewEnvCache(el.WithFunctions(el.ObjectVocabulary(), calls)).Compile(expr)
	return err
}

// calledNames are the names an expression calls as functions.
func calledNames(expr string) ([]string, error) {
	tree, err := parser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("el parse %q: %w", expr, err)
	}
	var out []string
	ast.Find(tree.Node, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallNode); ok {
			if id, ok := c.Callee.(*ast.IdentifierNode); ok {
				out = append(out, id.Value)
			}
		}
		return false
	})
	return out, nil
}

// dotted is the value at a dotted path of a decoded data layer - nil when a
// segment is missing.
func dotted(v any, path string) any {
	for _, seg := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[seg]
	}
	return v
}

// Errors keeps the findings that block.
func Errors(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Severity == verify.SeverityError {
			out = append(out, f)
		}
	}
	return out
}

func kebabToPascal(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// templateContinues reports whether the text right after a match opens a
// placeholder (`<type>`, `{name}`), which makes the match a pattern in prose
// rather than an address.
func templateContinues(body string, end int) bool {
	return end < len(body) && (body[end] == '<' || body[end] == '{')
}

// withoutFences blanks fenced code blocks; withoutSpans blanks inline code
// spans (for link syntax only — see ExtractRefs).
func withoutFences(body string) string { return reFence.ReplaceAllString(body, " ") }
func withoutSpans(body string) string  { return reSpan.ReplaceAllString(body, " ") }

// LintBytes lints one artifact's source by kind: the kind's format contract
// (skill, task, guide-recipe; a persona's config keys — a reference has none yet), then every reference
// in the body against `u` at `scope`. The one body every entry point shares.
func LintBytes(kind string, src []byte, self string, u *Universe, scope Scope) verify.LintResult {
	findings := formatFindings(kind, src)
	findings = append(findings, CheckRefs(self, ExtractRefs(string(src)), u, scope)...)
	return verify.Result(findings...)
}

// formatChecks is the format contract of each kind that has one: the check
// its source layer obeys, owned by the kind's domain (skill, task, guide
// recipe, persona, deck theme, slide group). A table, so the lint reads a kind's check by its name and
// branches on no kind: a kind with no entry has no format check.
var formatChecks = map[string]func([]byte) []verify.LintFinding{
	mbapi.ResourceTypeSkill:   func(b []byte) []verify.LintFinding { return skilllint.LintSkillFormatBytes(b).Findings },
	mbapi.ResourceTypeTask:    func(b []byte) []verify.LintFinding { return tasklint.LintTaskFormatBytes(b).Findings },
	guideapi.Kind:             func(b []byte) []verify.LintFinding { return guidelint.LintRecipeBytes(b).Findings },
	mbapi.ResourceTypePersona: lintPersonaConfigBytes,
	deckkind.KindDeckTheme:    func(b []byte) []verify.LintFinding { return decklint.LintThemeBytes(b).Findings },
	deckkind.KindSlideGroup:   func(b []byte) []verify.LintFinding { return decklint.LintSlideGroupBytes(b).Findings },
}

// formatFindings runs the format check of `kind` over src, none when the kind
// has no format check.
func formatFindings(kind string, src []byte) []verify.LintFinding {
	if check, ok := formatChecks[kind]; ok {
		return append([]verify.LintFinding(nil), check(src)...)
	}
	return nil
}

// sourceRequired reports whether `kind` declares its source layer required -
// read from the manifest's own declarations, else the system set's.
func sourceRequired(kinds, sys contribapi.Kinds, kind string) bool {
	d, ok := kinds.Get(kind)
	if !ok {
		d, ok = sys.Get(kind)
	}
	return ok && d.Layers[contribapi.LayerSource].Required
}

// markdownEntry reports whether an entry's source layer is markdown — a
// reference, a persona — so its links and URIs are worth judging.
func markdownEntry(c contribapi.Contribution) bool {
	ref, ok := c.SourceRef()
	return ok && strings.HasPrefix(ref.MediaType, "text/markdown")
}

// LintFile lints ONE file by kind ("skill", "task", or "" = by its folder:
// `skills/` → skill, `tasks/` → task, else markdown), resolving relative links
// on disk next to it and URIs against `u` when a universe is given.
func LintFile(pathOnDisk string, kind string, u *Universe) verify.LintResult {
	data, err := os.ReadFile(pathOnDisk)
	if err != nil {
		return verify.ErrorResult("unreadable", "cannot read "+pathOnDisk)
	}
	if kind == "" {
		kind = kindOfPath(pathOnDisk)
	}
	findings := formatFindings(kind, data)
	// The translations file beside it, by its kind's declared texts.
	if sys, err := builtin.Kinds(); err == nil && kind != "" {
		if paths := sys.Translatable(kind); len(paths) > 0 && !strings.HasSuffix(pathOnDisk, ".md") {
			tr, err := os.ReadFile(strings.TrimSuffix(pathOnDisk, filepath.Ext(pathOnDisk)) + manifestcontrib.TranslationsSuffix)
			if err != nil {
				tr = nil
			}
			findings = append(findings, LintTranslationsFiles(paths, data, tr, "")...)
		}
	}
	dir := filepath.Dir(pathOnDisk)
	root := packRoot(dir)
	exists := func(rel string) bool {
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(target); err != nil {
			return false
		}
		// The file exists on THIS disk, but a served skill cannot follow a
		// link out of its pack: `../../../../modules/x.md` is a repo path,
		// dead for every consumer that reads the skill from a catalog.
		if rel, err := filepath.Rel(root, target); err != nil || strings.HasPrefix(rel, "..") {
			return false
		}
		return true
	}
	scope := ScopeSelf
	if u != nil {
		scope = ScopeUniverse
	}
	findings = append(findings, CheckRefsOnDisk("", ExtractRefs(string(data)), u, scope, exists)...)
	return verify.Result(findings...)
}

func kindOfPath(p string) string {
	p = filepath.ToSlash(p)
	switch {
	case strings.Contains(p, "/skills/"):
		return mbapi.ResourceTypeSkill
	case strings.Contains(p, "/tasks/"):
		return mbapi.ResourceTypeTask
	case strings.Contains(p, "/embed/guide/") && strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".i18n.yaml"):
		return guideapi.Kind
	case strings.Contains(p, "/"+decklint.ThemesFolder+"/") && strings.HasSuffix(p, ".yaml"):
		return deckkind.KindDeckTheme
	case strings.Contains(p, "/"+decklint.GroupsFolder+"/") && strings.HasSuffix(p, ".yaml"):
		return deckkind.KindSlideGroup
	}
	return ""
}

// packRoot is the folder a served artifact can reach: the `embed` ancestor
// of the file when there is one (a sidecar's pack), else the file's folder.
func packRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	parts := strings.Split(filepath.ToSlash(abs), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "embed" {
			return filepath.FromSlash(strings.Join(parts[:i+1], "/"))
		}
	}
	return abs
}
