// SPDX-License-Identifier: Apache-2.0

// Package core implements ComposeService: universe verbs delegate to the
// in-process orchestrator service; sidecar scaffolding + artifact emission
// render the canonical templates (in-repo only, per Q-A1).
package core

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/compose/internal/templates"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// Artifact write targets (CreateSkillInput.Target etc.).
const (
	targetSource    = "source"    // in-repo app/<sidecar>/embed/… (needs rebuild)
	targetOverwrite = "overwrite" // workspace overlay (served live, no recompile)
	targetProject   = "project"   // the project's contributions folder (sys-contributions)
)

// artifactBase resolves the pack root an artifact is written under for
// `target` - the folder holding the kind folders (`<type>/<name>`) - plus the
// follow-up note:
//   - source    → app/<sidecar>/embed/ (repo-anchored; rebuild + go:embed note)
//   - overwrite → <HomeDir>/overwrites/<sidecar>/embed/ (live; promote note + gate warn)
//   - project   → the project's contributions folder (projectfile.Contributions),
//     its sys-contributions module ensured first; no sidecar
//
// embedToken is the go:embed dir (e.g. "embed/skills") used only for the source
// note's coverage check.
func (s *Service) artifactBase(ctx context.Context, target, sidecar, embedToken string) (base, note string, err error) {
	switch target {
	case "", targetSource:
		dir, derr := s.sidecarDir(sidecar)
		if derr != nil {
			return "", "", derr
		}
		return filepath.Join(dir, "embed"), joinNotes(sidecar, embedNote(dir, embedToken)), nil
	case targetOverwrite:
		home := s.home()
		return filepath.Join(cenv.OverwritesDir(home, sidecar), "embed"), overwriteNote(home, sidecar), nil
	case targetProject:
		if sidecar != "" {
			return "", "", fmt.Errorf("--target project takes no sidecar: the project's contributions folder is the destination (got %q)", sidecar)
		}
		return s.projectFolder(ctx)
	default:
		return "", "", fmt.Errorf("invalid target %q (want %q, %q or %q)", target, targetSource, targetOverwrite, targetProject)
	}
}

// projectArgs reads a Create* verb's positionals for --target project, which
// takes no sidecar: the one positional given lands in the sidecar slot and is
// the name.
func projectArgs(target string, sidecar, name *string) {
	if target == targetProject && *name == "" {
		*name, *sidecar = *sidecar, ""
	}
}

// overwriteNote tells the caller the artifact is served live from the overlay,
// warns if the universe gate is off (it won't be served), and points at promote.
func overwriteNote(home, sidecar string) string {
	notes := []string{"written to the workspace overlay — served live (no rebuild) once the universe sets allow_overwrites: true"}
	if !cenv.AllowOverwrites(home) {
		notes = append(notes, "WARNING: this universe has allow_overwrites disabled — the override is IGNORED until you set allow_overwrites: true in Univers.yaml")
	}
	notes = append(notes, fmt.Sprintf("promote when ready: ktw-compose promote %s <type>/<name>.md — copies into app/%s/embed/ and removes the overlay", sidecar, sidecar))
	return strings.Join(notes, "\n")
}

// ComposeServiceConfig is the (empty for now) service config.
type ComposeServiceConfig struct{}

// Service implements api.ComposeService.
type Service struct {
	// tmplFS is rooted AT the templates directory (embed/templates).
	tmplFS fs.FS
	// repoRoot overrides the working-directory repo root (tests).
	repoRoot string
	// homeOverride overrides Env().UniverseDir() for overlay-target resolution (tests).
	homeOverride string
	// orchOverride bypasses DI resolution of the orchestrator (tests).
	orchOverride orchapi.OrchestratorService
	// exeOverride answers manifest/source questions without a process (tests).
	exeOverride ExeReader
	// universeOverride is the aggregate `inspect --universe` reads (tests);
	// nil resolves the binary's universe catalog from ctx.
	universeOverride mbapi.CatalogReader
	// cwmOverride answers the project verbs' CWM calls (tests); nil resolves
	// CWMService from ctx (over the bus in ktw-compose).
	cwmOverride projectCWM
	// entriesOverride reads the entries an override is seeded from (tests).
	entriesOverride entryReader
	// kindsOverride are the kinds a sidecar's pack is read against (tests).
	kindsOverride *contribapi.Kinds
}

// home returns the .katyoti config root used for overlay-target writes:
// the test override when set, else the active universe dir.
func (s *Service) home() string {
	if s.homeOverride != "" {
		return s.homeOverride
	}
	return application.Env().UniverseDir()
}

// New constructs the service over the templates FS (rooted at the
// templates dir).
func New(tmplFS fs.FS) *Service { return &Service{tmplFS: tmplFS} }

var kebabRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// === Universe verbs — thin delegation to the orchestrator service ===

func (s *Service) orch(ctx context.Context) (orchapi.OrchestratorService, error) {
	if s.orchOverride != nil {
		return s.orchOverride, nil
	}
	svc, err := application.UseService[orchapi.OrchestratorService](ctx)
	if err != nil {
		return nil, fmt.Errorf("orchestrator service not available: %w", err)
	}
	return svc, nil
}

func (s *Service) CreateUniverse(ctx context.Context, in api.CreateUniverseInput) (orchapi.UniverseInfo, error) {
	o, err := s.orch(ctx)
	if err != nil {
		return orchapi.UniverseInfo{}, err
	}
	display := in.DisplayName
	if display == "" && in.Name != "" {
		display = strings.ToUpper(in.Name[:1]) + in.Name[1:]
	}
	return o.CreateUniverse(ctx, orchapi.CreateUniverseInput{
		Name:        in.Name,
		DisplayName: display,
		Icon:        in.Icon,
		Color:       in.Color,
		HomePath:    in.HomePath,
	})
}

func (s *Service) AddMember(ctx context.Context, in api.AddMemberInput) (orchapi.UniverseInfo, error) {
	o, err := s.orch(ctx)
	if err != nil {
		return orchapi.UniverseInfo{}, err
	}
	return o.AddSidecar(ctx, orchapi.AddSidecarInput{
		Universe: in.Universe,
		Sidecar:  orchapi.CreateSidecarSelection{Name: in.Sidecar, BinaryPath: in.Binary},
	})
}

func (s *Service) RemoveMember(ctx context.Context, in api.RemoveMemberInput) (orchapi.UniverseInfo, error) {
	o, err := s.orch(ctx)
	if err != nil {
		return orchapi.UniverseInfo{}, err
	}
	return o.RemoveSidecar(ctx, orchapi.RemoveSidecarInput{Universe: in.Universe, Sidecar: in.Sidecar})
}

// === Repo anchoring (Q-A1: in-repo only) ===

// root returns the repo root the file-emitting verbs anchor to: the
// current working directory, which MUST look like the ktw-agents repo
// (go.mod + app/). Out-of-repo emission is not supported in v1.
func (s *Service) root() (string, error) {
	root := s.repoRoot
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root = wd
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("in-repo only (Q-A1): %s has no go.mod — run from the ktw-agents repo root", root)
	}
	if st, err := os.Stat(filepath.Join(root, "app")); err != nil || !st.IsDir() {
		return "", fmt.Errorf("in-repo only (Q-A1): %s has no app/ directory — run from the ktw-agents repo root", root)
	}
	return root, nil
}

// sidecarDir validates the target sidecar exists and returns app/<name>.
func (s *Service) sidecarDir(name string) (string, error) {
	root, err := s.root()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "app", name)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("sidecar %q not found at %s", name, dir)
	}
	return dir, nil
}

// writeNew renders a template and writes it, refusing to overwrite.
func (s *Service) writeNew(tmpl string, vars map[string]string, outPath string) error {
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("%s already exists — compose never overwrites; edit it directly or pick another name", outPath)
	}
	return templates.RenderFSToFile(s.tmplFS, tmpl, vars, outPath)
}

// embedNote checks the sidecar's embed.go go:embed directive for the
// given dir token and returns a warning note when it is missing — the
// gotcha that silently hides new content from the binary.
func embedNote(sidecarDir, token string) string {
	raw, err := os.ReadFile(filepath.Join(sidecarDir, "embed.go"))
	if err != nil {
		return fmt.Sprintf("WARNING: could not read %s/embed.go — verify the go:embed directive includes %s", sidecarDir, token)
	}
	if !strings.Contains(string(raw), token) {
		return fmt.Sprintf("WARNING: %s/embed.go's //go:embed directive does NOT include %q — add it or the file ships invisible", sidecarDir, token)
	}
	return ""
}

func joinNotes(sidecar string, extra ...string) string {
	notes := []string{fmt.Sprintf("rebuild the sidecar to serve it: go build -o bin/%s.exe ./app/%s", sidecar, sidecar)}
	for _, e := range extra {
		if e != "" {
			notes = append(notes, e)
		}
	}
	return strings.Join(notes, "\n")
}

// === Sidecar scaffolding ===

// scaffoldIncludes maps the --include tokens to embed dir names.
var scaffoldIncludes = map[string]string{
	"skills":     "embed/skills",
	"tasks":      "embed/tasks",
	"personas":   "embed/personas",
	"references": "embed/references",
	"templates":  "embed/templates",
	"config":     "embed/config",
}

func (s *Service) CreateSidecar(ctx context.Context, in api.CreateSidecarInput) (api.CreateSidecarOutput, error) {
	if !kebabRe.MatchString(in.Name) {
		return api.CreateSidecarOutput{}, fmt.Errorf("invalid sidecar name %q: kebab-case required", in.Name)
	}
	root, err := s.root()
	if err != nil {
		return api.CreateSidecarOutput{}, err
	}
	target := filepath.Join(root, "app", in.Name)
	if _, err := os.Stat(target); err == nil {
		return api.CreateSidecarOutput{}, fmt.Errorf("app/%s already exists — compose never overwrites", in.Name)
	}

	include := in.Include
	if include == "" {
		include = "skills"
	}
	var dirs []string
	for _, tok := range strings.Split(include, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		dir, ok := scaffoldIncludes[tok]
		if !ok {
			keys := make([]string, 0, len(scaffoldIncludes))
			for k := range scaffoldIncludes {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return api.CreateSidecarOutput{}, fmt.Errorf("unknown include %q (recognized: %s)", tok, strings.Join(keys, ", "))
		}
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	short := in.Short
	if short == "" {
		short = fmt.Sprintf("%s — files-only sidecar", in.Name)
	}
	vars := map[string]string{
		"name":             in.Name,
		"includes":         strings.Join(dirs, " "),
		"short":            short,
		"long":             short + "\n\nScaffolded by ktw-compose (files-only: skills/tasks/references, no Go business logic).",
		"ktsp_description": short,
		"options":          "",
	}

	var files []string
	for tmpl, rel := range map[string]string{
		"sidecar/main.go.tmpl":          "main.go",
		"sidecar/embed.go.tmpl":         "embed.go",
		"sidecar/cmd/root.go.tmpl":      filepath.Join("cmd", "root.go"),
		"sidecar/cmd/bootstrap.go.tmpl": filepath.Join("cmd", "bootstrap.go"),
	} {
		out := filepath.Join(target, rel)
		if err := templates.RenderFSToFile(s.tmplFS, tmpl, vars, out); err != nil {
			return api.CreateSidecarOutput{}, err
		}
		files = append(files, out)
	}
	// Each included embed dir needs at least one embeddable file. The skills
	// dir gets the usage skill below instead — a README without frontmatter
	// there would fail the strict skill-corpus gate.
	for _, dir := range dirs {
		if dir == "embed/skills" {
			continue
		}
		readme := filepath.Join(target, filepath.FromSlash(dir), "README.md")
		content := fmt.Sprintf("# %s/%s\n\nContent dir prepared by `compose sidecar-new`.\n", in.Name, dir)
		if err := os.MkdirAll(filepath.Dir(readme), 0o755); err != nil {
			return api.CreateSidecarOutput{}, err
		}
		if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
			return api.CreateSidecarOutput{}, err
		}
		files = append(files, readme)
	}
	// A minimal usage skill when skills are included (the convention every
	// sidecar follows; advertised stays default true for usage).
	if strings.Contains(vars["includes"], "embed/skills") {
		usage := filepath.Join(target, "embed", "skills", "usage.md")
		uvars := map[string]string{
			"name":        "usage",
			"description": fmt.Sprintf("What %s is for and how to drive it. Load first when encountering this sidecar.", in.Name),
			"hashtags":    "usage",
			"category":    "guide",
			"advertised":  "true",
			"body":        fmt.Sprintf("# %s — usage\n\nDescribe the sidecar's purpose and its content here.", in.Name),
		}
		if err := s.writeNew("skill.md.tmpl", uvars, usage); err != nil {
			return api.CreateSidecarOutput{}, err
		}
		files = append(files, usage)
	}

	note, err := templates.RenderFS(s.tmplFS, "sidecar/makefile-note.txt.tmpl", map[string]string{"name": in.Name})
	if err != nil {
		return api.CreateSidecarOutput{}, err
	}
	sort.Strings(files)
	return api.CreateSidecarOutput{Path: target, Files: files, Note: note}, nil
}

// === Solution flow ===

// ApplyUniverse runs the headless composition: validate → (replace) →
// (scaffold + build the new sidecar) → create the universe → add every
// member. Fails fast at the first error; the Steps log records how far
// it got so the operator can resume by hand.
func (s *Service) ApplyUniverse(ctx context.Context, in api.ApplyUniverseInput) (api.ApplyUniverseOutput, error) {
	spec := in.Spec
	out := api.ApplyUniverseOutput{}
	step := func(format string, a ...any) {
		out.Steps = append(out.Steps, fmt.Sprintf(format, a...))
	}
	if spec.Name == "" {
		return out, fmt.Errorf("solution: spec.name is required")
	}
	if len(spec.Sidecars) == 0 && spec.NewSidecar == nil {
		return out, fmt.Errorf("solution: nothing to compose — select at least one existing sidecar or a new one")
	}
	if err := caseOwnsUniverse(spec.Name); err != nil {
		return out, err
	}
	o, err := s.orch(ctx)
	if err != nil {
		return out, err
	}

	// Idempotency: existing universe is an error unless Replace.
	if _, err := o.GetUniverse(ctx, orchapi.GetUniverseInput{Name: spec.Name}); err == nil {
		if !spec.Replace {
			return out, fmt.Errorf("universe %q already exists — pass replace to archive and re-create it", spec.Name)
		}
		if _, err := o.DeleteUniverse(ctx, orchapi.DeleteUniverseInput{Name: spec.Name}); err != nil {
			return out, fmt.Errorf("replace: archive existing universe: %w", err)
		}
		step("archived existing universe %q (replace)", spec.Name)
	}

	// Optional new files-only sidecar: scaffold + build.
	builtBinary := ""
	if ns := spec.NewSidecar; ns != nil {
		sc, err := s.CreateSidecar(ctx, api.CreateSidecarInput{Name: ns.Name, Include: ns.Include, Short: ns.Short})
		if err != nil {
			return out, fmt.Errorf("scaffold new sidecar: %w", err)
		}
		out.Scaffolded = &sc
		step("scaffolded app/%s (%d files)", ns.Name, len(sc.Files))
		if !spec.SkipBuild {
			root, err := s.root()
			if err != nil {
				return out, err
			}
			builtBinary = filepath.Join(root, "bin", ns.Name+exeSuffix())
			cmd := exec.CommandContext(ctx, "go", "build", "-o", builtBinary, "./app/"+ns.Name)
			cmd.Dir = root
			if outBytes, err := cmd.CombinedOutput(); err != nil {
				return out, fmt.Errorf("build scaffolded sidecar: %w; output: %s", err, outBytes)
			}
			out.BuiltBinary = builtBinary
			step("built %s", builtBinary)
		} else {
			step("skipped build of %s (skip_build)", ns.Name)
		}
	}

	// Create the universe (empty), then add every member.
	info, err := s.CreateUniverse(ctx, api.CreateUniverseInput{
		Name: spec.Name, DisplayName: spec.DisplayName,
		Icon: spec.Icon, Color: spec.Color, HomePath: spec.HomePath,
	})
	if err != nil {
		return out, fmt.Errorf("create universe: %w", err)
	}
	step("created universe %q", spec.Name)

	members := make([]orchapi.CreateSidecarSelection, 0, len(spec.Sidecars)+1)
	for _, m := range spec.Sidecars {
		members = append(members, orchapi.CreateSidecarSelection{Name: m.Name, BinaryPath: m.Binary})
	}
	if spec.NewSidecar != nil {
		members = append(members, orchapi.CreateSidecarSelection{Name: spec.NewSidecar.Name, BinaryPath: builtBinary})
	}
	for _, m := range members {
		info, err = o.AddSidecar(ctx, orchapi.AddSidecarInput{Universe: spec.Name, Sidecar: m})
		if err != nil {
			return out, fmt.Errorf("add sidecar %q: %w (universe created; completed steps: %s)",
				m.Name, err, strings.Join(out.Steps, "; "))
		}
		step("added sidecar %q", m.Name)
	}
	out.Universe = info
	return out, nil
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// === Artifact emission ===

func (s *Service) CreateSkill(ctx context.Context, in api.CreateSkillInput) (api.CreateItemOutput, error) {
	projectArgs(in.Target, &in.Sidecar, &in.Name)
	if !kebabRe.MatchString(in.Name) {
		return api.CreateItemOutput{}, fmt.Errorf("invalid skill name %q: kebab-case required", in.Name)
	}
	base, note, err := s.artifactBase(ctx, in.Target, in.Sidecar, "embed/skills")
	if err != nil {
		return api.CreateItemOutput{}, err
	}
	desc := in.Description
	if desc == "" {
		desc = fmt.Sprintf("TODO what %s does + when to load it (the L0 selection line).", in.Name)
	}
	category := in.Category
	if category == "" {
		category = "guide"
	}
	out := filepath.Join(base, "skills", in.Name+".md")
	vars := map[string]string{
		"name":        in.Name,
		"description": desc,
		"hashtags":    "",
		"category":    category,
		"advertised":  fmt.Sprintf("%t", in.Advertised),
		"body":        fmt.Sprintf("# %s\n\nTODO: second-person procedure with decisions. Cross-reference by bare URI.", in.Name),
	}
	if err := s.writeNew("skill.md.tmpl", vars, out); err != nil {
		return api.CreateItemOutput{}, err
	}
	return api.CreateItemOutput{Path: out, Note: note}, nil
}

func (s *Service) CreateTask(ctx context.Context, in api.CreateTaskInput) (api.CreateItemOutput, error) {
	projectArgs(in.Target, &in.Sidecar, &in.Name)
	if !kebabRe.MatchString(in.Name) {
		return api.CreateItemOutput{}, fmt.Errorf("invalid task name %q: kebab-case required", in.Name)
	}
	typ := in.Type
	if typ == "" {
		typ = "agent"
	}
	base, note, err := s.artifactBase(ctx, in.Target, in.Sidecar, "embed/tasks")
	if err != nil {
		return api.CreateItemOutput{}, err
	}
	desc := in.Description
	if desc == "" {
		desc = fmt.Sprintf("TODO one-line description of %s", in.Name)
	}
	out := filepath.Join(base, "tasks", in.Name+".md")
	vars := map[string]string{
		"name":        in.Name,
		"type":        typ,
		"description": desc,
		"config":      "  plugins:\n    - workspace",
		"body":        "TODO: the task body (for agent tasks this is the system prompt — drive to **complete**).",
	}
	if err := s.writeNew("task.md.tmpl", vars, out); err != nil {
		return api.CreateItemOutput{}, err
	}
	return api.CreateItemOutput{Path: out, Note: note}, nil
}

func (s *Service) CreatePersona(ctx context.Context, in api.CreatePersonaInput) (api.CreateItemOutput, error) {
	if !kebabRe.MatchString(in.Name) {
		return api.CreateItemOutput{}, fmt.Errorf("invalid persona name %q: kebab-case required", in.Name)
	}
	base, note, err := s.artifactBase(ctx, in.Target, in.Sidecar, "embed/personas")
	if err != nil {
		return api.CreateItemOutput{}, err
	}
	desc := in.Description
	if desc == "" {
		desc = fmt.Sprintf("TODO one-line description of the %s persona", in.Name)
	}
	out := filepath.Join(base, "personas", in.Name+".md")
	vars := map[string]string{
		"name":        in.Name,
		"description": desc,
		"bind_mode":   "global", // valid set: object | global | custom
		"types":       "'*'",
		"skills":      "",
		"config":      "  plugins:\n    - augmented\n    - cwm",
		"body":        fmt.Sprintf("TODO: the %s persona's system prompt (role + goal + numbered steps + complete).", in.Name),
	}
	if err := s.writeNew("persona.md.tmpl", vars, out); err != nil {
		return api.CreateItemOutput{}, err
	}
	return api.CreateItemOutput{Path: out, Note: note}, nil
}

func (s *Service) CreateReference(ctx context.Context, in api.CreateReferenceInput) (api.CreateItemOutput, error) {
	rel := filepath.ToSlash(filepath.Clean(in.Path))
	if rel == "" || strings.HasPrefix(rel, "..") || strings.HasPrefix(rel, "/") {
		return api.CreateItemOutput{}, fmt.Errorf("invalid reference path %q: must be relative under embed/references/", in.Path)
	}
	base, note, err := s.artifactBase(ctx, in.Target, in.Sidecar, "embed/references")
	if err != nil {
		return api.CreateItemOutput{}, err
	}
	title := in.Title
	if title == "" {
		b := filepath.Base(rel)
		title = strings.TrimSuffix(b, filepath.Ext(b))
	}
	out := filepath.Join(base, "references", filepath.FromSlash(rel))
	vars := map[string]string{
		"title": title,
		"body":  "TODO: passive lookup material (tables, schemas, catalogs) — no procedure, no decisions.",
	}
	if err := s.writeNew("reference.md.tmpl", vars, out); err != nil {
		return api.CreateItemOutput{}, err
	}
	return api.CreateItemOutput{Path: out, Note: note}, nil
}

// === Promote: overlay → source ===

// Promote copies a workspace-overlay artifact into the sidecar's in-repo embed
// tree, then DELETES the overlay copy. The overlay always wins, so leaving it
// after promotion would mask later source edits — removal makes the embedded
// (released) copy authoritative again. In-repo only (needs app/<sidecar>).
func (s *Service) Promote(ctx context.Context, in api.PromoteInput) (api.PromoteOutput, error) {
	if in.Object != "" {
		if in.Sidecar != "" || in.Path != "" {
			return api.PromoteOutput{}, fmt.Errorf("promote: --object names the override object alone - no sidecar or path")
		}
		return s.promoteObject(ctx, in.Object)
	}
	if in.Sidecar == "" {
		return api.PromoteOutput{}, fmt.Errorf("promote: give <sidecar> <path> (the universe overlay) or --object <override object>")
	}
	rel := filepath.ToSlash(filepath.Clean(in.Path))
	if rel == "" || rel == "." || strings.HasPrefix(rel, "..") || strings.HasPrefix(rel, "/") {
		return api.PromoteOutput{}, fmt.Errorf("invalid overlay path %q: must be relative under embed/ (e.g. skills/foo.md)", in.Path)
	}
	overlayFile := filepath.Join(cenv.OverwritesDir(s.home(), in.Sidecar), "embed", filepath.FromSlash(rel))
	data, err := os.ReadFile(overlayFile)
	if err != nil {
		return api.PromoteOutput{}, fmt.Errorf("no overlay artifact at %s: %w", overlayFile, err)
	}
	// sidecarDir anchors to the repo (go.mod + app/<sidecar>).
	dir, err := s.sidecarDir(in.Sidecar)
	if err != nil {
		return api.PromoteOutput{}, err
	}
	srcFile := filepath.Join(dir, "embed", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(srcFile), 0o755); err != nil {
		return api.PromoteOutput{}, fmt.Errorf("create source dir: %w", err)
	}
	if err := os.WriteFile(srcFile, data, 0o644); err != nil {
		return api.PromoteOutput{}, fmt.Errorf("write source %s: %w", srcFile, err)
	}
	if err := os.Remove(overlayFile); err != nil {
		return api.PromoteOutput{}, fmt.Errorf("written to source, but failed to remove overlay %s (remove it by hand — it would shadow the source): %w", overlayFile, err)
	}
	note := joinNotes(in.Sidecar,
		embedNote(dir, "embed/"+topSegment(rel)),
		"overlay removed — the embedded copy is authoritative again; open a PR for app/"+in.Sidecar+"/embed/"+rel)
	return api.PromoteOutput{Source: srcFile, Removed: overlayFile, Note: note}, nil
}

// topSegment returns the first path segment ("skills/foo.md" → "skills"), used
// to name the go:embed dir token for the coverage check.
func topSegment(rel string) string {
	if i := strings.IndexByte(rel, '/'); i >= 0 {
		return rel[:i]
	}
	return rel
}

// Compile-time assertion.
var _ api.ComposeService = (*Service)(nil)

// caseOwnsUniverse refuses, inside a scenario case's sandbox, a universe that
// is not the case's: a solution applied in a case creates and archives only
// scn-<case> and scn-<case>-<other> - never a universe named
// like the solution, which the person's installation may hold.
func caseOwnsUniverse(name string) error {
	box, ok := cenv.ScenarioSandbox()
	if !ok {
		return nil
	}
	own := "scn-" + filepath.Base(box)
	if name == own || strings.HasPrefix(name, own+"-") {
		return nil
	}
	return fmt.Errorf("universe %q is not the scenario case's (%s or %s-<name>): a solution applied inside a case creates and archives only the case's own universes", name, own, own)
}
