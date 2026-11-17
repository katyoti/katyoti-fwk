// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	contentlint "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/lint"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

// The content lints over COMPILED sidecars. A sidecar is asked what it
// publishes through the two verbs every sidecar already has — `sc resources
// manifest` and `sc resources read` — so nothing is added to the sidecar and
// an executable built in another repository is as good as one built here.
// Each call runs in a throwaway workspace so the sidecar's own state is
// never touched.

const exeCallTimeout = 60 * time.Second

// ExeReader answers manifest and source questions for one executable. A seam
// so the lints are testable without a binary.
type ExeReader interface {
	Manifest(ctx context.Context, exe string) (*mbapi.Manifest, error)
	Source(ctx context.Context, exe, uri string) ([]byte, error)
}

type processReader struct{}

func (processReader) Manifest(ctx context.Context, exe string) (*mbapi.Manifest, error) {
	out, err := runExe(ctx, exe, "sc", "resources", "manifest", "--out", "json")
	if err != nil {
		return nil, err
	}
	var m mbapi.Manifest
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("%s: decode manifest: %w", filepath.Base(exe), err)
	}
	return &m, nil
}

func (processReader) Source(ctx context.Context, exe, uri string) ([]byte, error) {
	tmp, err := os.CreateTemp("", "ktw-lint-*.md")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if _, err := runExe(ctx, exe, "sc", "resources", "read", uri, "--into", tmp.Name()); err != nil {
		return nil, err
	}
	return os.ReadFile(tmp.Name())
}

func runExe(ctx context.Context, exe string, args ...string) ([]byte, error) {
	home, err := os.MkdirTemp("", "ktw-lint-ws-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(home)
	// A sidecar refuses to start in an uninitialised workspace; give it the
	// smallest one there is (the compiled-binaries harness does the same).
	cfg := filepath.Join(home, ".katyoti", "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(cfg, "Univers.yaml"), []byte("version: 1\nname: lint\ndisplay_name: Lint\nicon: L\ncolor: gray\nsidecars: []\n"), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(cfg, "Settings.yaml"), []byte("# lint workspace\n"), 0o644); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, exeCallTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(os.Environ(), "KTW_WORKSPACE="+home, "KTW_DATA="+home, "KTW_CACHE="+home)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w\n%s", filepath.Base(exe), strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// lintExes is the shared body: read every manifest, build the universe,
// lint each binary at the given scope.
func lintExes(ctx context.Context, rd ExeReader, exes []string, scope contentlint.Scope, extra ...func([]*mbapi.Manifest) []contentlint.Finding) (api.ContentLintOutput, error) {
	out := api.ContentLintOutput{Status: "pass"}
	manifests := make([]*mbapi.Manifest, 0, len(exes))
	byExe := map[string]*mbapi.Manifest{}
	var linted []string
	for _, exe := range exes {
		m, err := rd.Manifest(ctx, exe)
		if err != nil {
			// A binary in the folder that does not answer as a sidecar (a
			// helper, a GUI) is skipped BY NAME, never silently: the report
			// says what it could not read, and the lint still fails when it
			// read nothing at all.
			out.Skipped = append(out.Skipped, fmt.Sprintf("%s: %v", filepath.Base(exe), firstLine(err)))
			continue
		}
		manifests = append(manifests, m)
		byExe[exe] = m
		out.Binaries = append(out.Binaries, filepath.Base(exe))
		linted = append(linted, exe)
	}
	if len(linted) == 0 {
		return out, fmt.Errorf("no executable answered as a sidecar: %s", strings.Join(out.Skipped, "; "))
	}
	universe := contentlint.NewUniverse(manifests)
	if scope == contentlint.ScopeUniverse {
		// What no single sidecar can check about the KINDS: two members
		// declaring one kind (blind audit H4). Each binary's own census
		// refuses a duplicate within the binary; across binaries only a view
		// of all of them can see it. The registry resolves by origin (system
		// wins, then member order) and names every loser.
		out.Findings = append(out.Findings, kindFindingsAcross(manifests)...)
		// What CWM would find reading these members' declarations: a gate
		// entry reaching objects its provider does not own (narrowed), a
		// declaration it could not read or would read leniently. The same
		// answer as CWM's declaration.Findings, computed from the executables
		// (no universe here, so no grants).
		out.Findings = append(out.Findings, declarationFindingsAcross(manifests)...)
	}
	for _, check := range extra {
		out.Findings = append(out.Findings, check(manifests)...)
	}
	for _, exe := range linted {
		exe := exe
		open := func(uri, _ string) ([]byte, error) { return rd.Source(ctx, exe, uri) }
		u := universe
		if scope == contentlint.ScopeSelf {
			u = contentlint.NewUniverse([]*mbapi.Manifest{byExe[exe]})
		}
		out.Findings = append(out.Findings, contentlint.LintManifest(byExe[exe], open, u, scope)...)
	}
	sort.Slice(out.Findings, func(i, j int) bool { return out.Findings[i].String() < out.Findings[j].String() })
	if len(contentlint.Errors(out.Findings)) > 0 {
		out.Status = "fail"
	}
	return out, nil
}

// LintSidecar implements api.Service.
func (s *Service) LintSidecar(ctx context.Context, in api.LintSidecarInput) (api.ContentLintOutput, error) {
	if in.Exe == "" {
		return api.ContentLintOutput{}, fmt.Errorf("lint-sidecar: exe is required")
	}
	return lintExes(ctx, s.exeReader(), []string{in.Exe}, contentlint.ScopeSelf)
}

// LintUniverse implements api.Service.
func (s *Service) LintUniverse(ctx context.Context, in api.LintUniverseInput) (api.ContentLintOutput, error) {
	dir := in.BinDir
	if dir == "" {
		dir = "bin"
	}
	exes, err := sidecarExecutables(dir)
	if err != nil {
		return api.ContentLintOutput{}, err
	}
	if len(exes) == 0 {
		return api.ContentLintOutput{}, fmt.Errorf("lint-universe: %s holds no sidecar executables", dir)
	}
	var extra []func([]*mbapi.Manifest) []contentlint.Finding
	if in.Project != "" {
		extra = append(extra, func(ms []*mbapi.Manifest) []contentlint.Finding { return projectFindings(in.Project, ms) })
	}
	return lintExes(ctx, s.exeReader(), exes, contentlint.ScopeUniverse, extra...)
}

// projectFindings judges a project's contributions with the members that
// would serve them, as the hub does (projectfile.TokenConflict): its token
// is no member's name nor reserved, and its contributions folder holds no
// object file (`*.cwm.*`: the folder is read as a pack, which leaves one
// out). A project with no contributions folder contributes nothing.
func projectFindings(project string, manifests []*mbapi.Manifest) []contentlint.Finding {
	f, err := projectfile.Read(project)
	if err != nil {
		return []contentlint.Finding{{URI: project, LintFinding: verify.LintFinding{
			Rule: "project-settings", Severity: verify.SeverityError, Message: err.Error()}}}
	}
	dir := f.ContributionsDir(project)
	if dir == "" {
		return nil
	}
	var out []contentlint.Finding
	var members []string
	for _, m := range manifests {
		if m != nil {
			members = append(members, m.Provider)
		}
	}
	if why := projectfile.TokenConflict(f.Contributions.Provider, members); why != "" {
		out = append(out, contentlint.Finding{URI: f.Contributions.Provider + ":/", LintFinding: verify.LintFinding{
			Rule: "project-token", Severity: verify.SeverityError, Message: why}})
	}
	files, err := projectfile.CWMFiles(dir)
	if err != nil {
		out = append(out, contentlint.Finding{URI: dir, LintFinding: verify.LintFinding{
			Rule: "project-cwm-file", Severity: verify.SeverityError, Message: err.Error()}})
	}
	for _, rel := range files {
		out = append(out, contentlint.Finding{URI: filepath.ToSlash(filepath.Join(f.Contributions.Root, rel)), LintFinding: verify.LintFinding{
			Rule: "project-cwm-file", Severity: verify.SeverityError,
			Message: "an object file in the contributions folder: the folder is read as a pack and leaves it out; move it to a module"}})
	}
	return out
}

// kindFindingsAcross reports every kind declared by more than one member,
// as the universe's kind registry resolves it: the system declaration wins by
// origin, a member's redeclaration of it is an error; between members the
// first in the folder's order wins, one declaration two members carry alike
// is one kind (no finding), differing bodies are an error. One finding per losing declaration, located at its URI.
//
// Every binary carries the system set (`system:/contribution-type/*`), so
// the same system URI arrives once per manifest; KindsFrom counts one URI
// once, and only a distinct URI is a second declaration.
func kindFindingsAcross(manifests []*mbapi.Manifest) []contentlint.Finding {
	var all []contribapi.Contribution
	for _, m := range manifests {
		if m != nil {
			all = append(all, m.Contributions...)
		}
	}
	var out []contentlint.Finding
	for _, f := range contribapi.KindsFrom(all).Check() {
		if f.Rule != contribapi.KindRuleDuplicate && f.Rule != contribapi.KindRuleRedeclaresSystem {
			continue // an incomplete declaration is its own binary's census finding
		}
		sev := verify.SeverityError
		if f.IsWarning() {
			sev = verify.SeverityWarn
		}
		out = append(out, contentlint.Finding{URI: f.URI, LintFinding: verify.LintFinding{
			Rule: f.Rule, Severity: sev, Message: fmt.Sprintf("kind %q: %s", f.Kind, f.Message)}})
	}
	return out
}

// declarationFindingsAcross is cwmapi.DeclarationFindings over the members'
// manifests, as lint findings located at the declaration's URI (or, for a
// declaration found by name, at its provider).
func declarationFindingsAcross(manifests []*mbapi.Manifest) []contentlint.Finding {
	decl := cwmapi.DeclarationsFromManifests(manifests, nil)
	var out []contentlint.Finding
	for _, f := range cwmapi.DeclarationFindings(decl, nil) {
		sev := verify.SeverityError
		if f.Severity == cwmapi.FindingWarning {
			sev = verify.SeverityWarn
		}
		uri := f.URI
		if uri == "" {
			uri = contribapi.Contribution{Provider: f.Provider, Type: f.Kind, ID: f.Name}.ResolvedURI()
		}
		out = append(out, contentlint.Finding{URI: uri, LintFinding: verify.LintFinding{
			Rule: "declaration-" + f.Rule, Severity: sev, Message: f.Message}})
	}
	return out
}

func (s *Service) exeReader() ExeReader {
	if s.exeOverride != nil {
		return s.exeOverride
	}
	return processReader{}
}

// sidecarExecutables lists the ktw-* / katyoti executables of a folder —
// the same rule the compiled-binaries harness applies. ktw-gui and
// ktw-display are not sidecars.
func sidecarExecutables(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("lint-universe: read %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		// Exactly `<name>` or `<name>.exe`: a build's leftover (`ktw-x.exe~`,
		// `ktw-x.exe.old-…`) is a STALE copy and must never be the one judged.
		base, ok := executableBase(name)
		if e.IsDir() || !ok {
			continue
		}
		if base == "ktw-gui" || base == "ktw-display" || base == "ktw-hub" {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Strings(out)
	return out, nil
}

var reSidecarName = regexp.MustCompile(`^(ktw-[a-z0-9-]+|katyoti)$`)

// executableBase returns the sidecar name of a file in bin/ and whether the
// file is a sidecar executable at all.
func executableBase(name string) (string, bool) {
	base := strings.TrimSuffix(name, ".exe")
	return base, reSidecarName.MatchString(base)
}

func firstLine(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")
	return line
}
