// SPDX-License-Identifier: Apache-2.0

package commands_test

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	codecmd "github.com/katyoti/katyoti-fwk/internal/extensions/code/commands"
	codelocal "github.com/katyoti/katyoti-fwk/internal/extensions/code/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/spf13/cobra"
)

// The module's objects, as files: a use case WITH a passing verdict (UC-001),
// one WITHOUT a verdict whose own fields still say `status_cache: pass`
// (UC-002 — the status must come from the run, never from the use case), and
// the run.
var fixture = map[string]string{
	"objects/m.cwm.yaml": `id: m
type: module
name: m
capabilities:
  module:
    kind: library
`,
	"use-cases/UC-001.cwm.yaml": `id: m.UC-001
type: use-case
name: It runs
description: The thing runs.
parent: m
capabilities:
  checkable:
    severity: core
body: Runs.
`,
	"use-cases/UC-001-AC1.cwm.yaml": `id: m.UC-001-AC1
type: acceptance-criterion
name: Run returns the output
parent: m.UC-001
fields:
  layer: L1
  test_ref: TestRuns
`,
	"use-cases/UC-002.cwm.yaml": `id: m.UC-002
type: use-case
name: It stops
description: The thing stops.
parent: m
fields:
  status_cache: pass
capabilities:
  checkable:
    severity: core
body: Stops.
`,
	"use-cases/UC-002-AC1.cwm.yaml": `id: m.UC-002-AC1
type: acceptance-criterion
name: Stop ends the run
parent: m.UC-002
fields:
  layer: L1
  test_ref: TestStops
`,
	"objects/test-run-20261003-120000.cwm.yaml": `id: m.test-run-20261003-120000
type: test-run
name: module verify L1 — 2026-10-03 12:00
parent: m
fields:
  module: m
  ran_at: "2026-10-03T12:00:00Z"
  layer_filter: L1
  totals: {pass: 1, fail: 0, fail_deferred: 0, no_test: 0, uc_total: 1}
  verdicts:
    UC-001:
      verdict: pass
      acs:
        UC-001-AC1: pass
`,
}

// newProject writes the fixture into a temp project and boots the code
// service whose publish is under test, on a CWM service that serves the
// project's object files (filesCWM) — what the real index is a view over.
func newProject(t *testing.T, extra map[string]string) (context.Context, string) {
	t.Helper()
	root := t.TempDir()
	moduleDir := filepath.Join(root, "modules", "m")
	files := map[string]string{}
	for rel, body := range fixture {
		files[rel] = body
	}
	for rel, body := range extra {
		files[rel] = body
	}
	for rel, body := range files {
		p := filepath.Join(moduleDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	app := application.New(
		config.NewConfigService(loader.NewMapLoader(nil)),
		func(init *application.ApplicationInit) {
			init.AddContribute("test", ".root", func(app *application.Application) error {
				application.PutService[appcore.CobraCommand](app, "root", appcore.CobraCommand{Cmd: &cobra.Command{Use: "ktw-test"}})
				application.PutService[cwmapi.CWMService](app, "default", &filesCWM{root: root})
				return nil
			})
		},
		codelocal.Extension(),
	)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	return application.WithApplication(context.Background(), app), moduleDir
}

// filesCWM answers the reads the publish makes, from the project's object
// files. Every other method is the embedded nil interface: the publish calling
// one is a contract change worth a failing test.
type filesCWM struct {
	cwmapi.CWMService
	root string
}

func (c *filesCWM) objects() (map[string]cwmapi.Object, error) {
	out := map[string]cwmapi.Object{}
	err := filepath.WalkDir(filepath.Join(c.root, "modules"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, cwmapi.ObjectFileSuffix) {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var o cwmapi.Object
		if err := marshal.Unmarshal(raw, marshal.YAML, &o); err != nil {
			return err
		}
		out[o.ID] = o
		return nil
	})
	return out, err
}

func (c *filesCWM) ObjectGet(_ context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	all, err := c.objects()
	if err != nil {
		return cwmapi.ObjectGetOutput{}, err
	}
	o, ok := all[in.ID]
	if !ok {
		return cwmapi.ObjectGetOutput{}, fmt.Errorf("%w: %s", cwmapi.ErrNotFound, in.ID)
	}
	return cwmapi.ObjectGetOutput{Object: o}, nil
}

// ObjectEffectiveCapabilities stands in for the shipped `module` type's
// bundle, which carries the `module` capability, plus the instance's groups.
func (c *filesCWM) ObjectEffectiveCapabilities(_ context.Context, in cwmapi.ObjectEffectiveCapabilitiesInput) (cwmapi.ObjectEffectiveCapabilitiesOutput, error) {
	all, err := c.objects()
	if err != nil {
		return cwmapi.ObjectEffectiveCapabilitiesOutput{}, err
	}
	o, ok := all[in.ID]
	if !ok {
		return cwmapi.ObjectEffectiveCapabilitiesOutput{}, fmt.Errorf("%w: %s", cwmapi.ErrNotFound, in.ID)
	}
	var out []cwmapi.CapabilityRef
	if o.Type == "module" {
		out = append(out, cwmapi.CapabilityRef{ID: "module", Source: "type"})
	}
	for id := range o.Capabilities {
		out = append(out, cwmapi.CapabilityRef{ID: id, Source: "object"})
	}
	return cwmapi.ObjectEffectiveCapabilitiesOutput{Capabilities: out}, nil
}

func (c *filesCWM) ObjectList(_ context.Context, in cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error) {
	all, err := c.objects()
	if err != nil {
		return cwmapi.ObjectListOutput{}, err
	}
	var out []cwmapi.Object
	for _, o := range all {
		if (in.Type == "" || o.Type == in.Type) && (in.Parent == "" || o.ParentID() == in.Parent) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return cwmapi.ObjectListOutput{Objects: out, Total: len(out)}, nil
}

func (c *filesCWM) ObjectTree(_ context.Context, in cwmapi.ObjectTreeInput) (cwmapi.ObjectTreeOutput, error) {
	all, err := c.objects()
	if err != nil {
		return cwmapi.ObjectTreeOutput{}, err
	}
	var build func(id string) []cwmapi.ObjectTreeNode
	build = func(id string) []cwmapi.ObjectTreeNode {
		var kids []cwmapi.Object
		for _, o := range all {
			if o.ParentID() == id {
				kids = append(kids, o)
			}
		}
		sort.Slice(kids, func(i, j int) bool { return kids[i].ID < kids[j].ID })
		var nodes []cwmapi.ObjectTreeNode
		for _, k := range kids {
			nodes = append(nodes, cwmapi.ObjectTreeNode{Object: k, Children: build(k.ID)})
		}
		return nodes
	}
	return cwmapi.ObjectTreeOutput{Roots: []cwmapi.ObjectTreeNode{{Object: all[in.Root], Children: build(in.Root)}}}, nil
}

// ObjectResolve answers the project root, as the real service does.
func (c *filesCWM) ObjectResolve(_ context.Context, in cwmapi.ObjectResolveInput) (cwmapi.ObjectResolveOutput, error) {
	return cwmapi.ObjectResolveOutput{ID: in.Ref, ProjectRoot: c.root}, nil
}

func publish(t *testing.T, ctx context.Context) {
	t.Helper()
	svc, err := application.UseService[api.CodeService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CharterPublish(ctx, api.CharterPublishInput{ModuleID: "m"}); err != nil {
		t.Fatalf("CharterPublish: %v", err)
	}
	if _, err := svc.ManifestWrite(ctx, api.ManifestWriteInput{ModuleID: "m"}); err != nil {
		t.Fatalf("ManifestWrite: %v", err)
	}
}

func project(t *testing.T, ctx context.Context, args ...string) string {
	t.Helper()
	cmd := codecmd.NewModuleCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"project", "m"}, args...))
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("module project %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func read(t *testing.T, moduleDir string) (charter, manifest []byte) {
	t.Helper()
	c, err := os.ReadFile(filepath.Join(moduleDir, "charter.md"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := os.ReadFile(filepath.Join(moduleDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	return c, m
}

// laterRun verdicts UC-002 as well: `module project` lints the use cases first
// and refuses a criterion no run ever verified.
var laterRun = map[string]string{
	"objects/test-run-20261003-130000.cwm.yaml": `id: m.test-run-20261003-130000
type: test-run
name: module verify L1 — 2026-10-03 13:00
parent: m
fields:
  module: m
  ran_at: "2026-10-03T13:00:00Z"
  layer_filter: L1
  uc_filter: UC-002
  totals: {pass: 0, fail: 1, fail_deferred: 0, no_test: 0, uc_total: 1}
  verdicts:
    UC-002:
      verdict: fail
      acs:
        UC-002-AC1: fail
`,
}

// One writer per file: the charter publish / manifest write render charter.md
// and manifest.json whole, and `module project` is a call to them. So the two
// orders give the same bytes, and a project right after a publish finds
// nothing stale. A splice re-introduced in `module project` breaks the
// project-first order (no section to splice into) or the byte equality.
func TestPublishAndProjectAreOneWriter(t *testing.T) {
	ctxA, dirA := newProject(t, laterRun)
	publish(t, ctxA)
	project(t, ctxA, "--apply")
	charterA, manifestA := read(t, dirA)
	if out := project(t, ctxA, "--check"); strings.Contains(out, "STALE") {
		t.Fatalf("publish then project left a stale projection:\n%s", out)
	}

	ctxB, dirB := newProject(t, laterRun)
	project(t, ctxB, "--apply")
	publish(t, ctxB)
	charterB, manifestB := read(t, dirB)

	if !bytes.Equal(charterA, charterB) {
		t.Errorf("charter.md differs by order:\n--- publish, project ---\n%s\n--- project, publish ---\n%s", charterA, charterB)
	}
	if !bytes.Equal(manifestA, manifestB) {
		t.Errorf("manifest.json differs by order:\n--- publish, project ---\n%s\n--- project, publish ---\n%s", manifestA, manifestB)
	}

	charter := string(charterA)
	if !strings.Contains(charter, "## Test plan") || !strings.Contains(charter, "`TestRuns` 🟢") {
		t.Errorf("the charter lacks its Test plan with UC-001's passing criterion:\n%s", charter)
	}
	if !strings.Contains(string(manifestA), `"coverage"`) {
		t.Errorf("manifest.json lacks its coverage block:\n%s", manifestA)
	}
}

// A use case's status in the charter is its latest verdict: UC-001 has a
// passing one and shows pass; UC-002 has none and shows no-test although its
// own fields say `status_cache: pass`.
func TestCharterStatusIsTheLatestVerdict(t *testing.T) {
	ctx, dir := newProject(t, nil)
	publish(t, ctx)
	charter, _ := read(t, dir)
	for _, want := range []string{
		"| `UC-001` | It runs | `core` | 🟢 pass |",
		"| `UC-002` | It stops | `core` | ⚪ no-test |",
		"| Passing | 1 |",
		"| No test | 1 |",
	} {
		if !strings.Contains(string(charter), want) {
			t.Errorf("charter lacks %q:\n%s", want, charter)
		}
	}
}

// manifest.json's source_paths are the module object's declared
// `code_packages`. A module that declares them projects them; one that does
// not is REFUSED by --apply rather than erasing the source_paths a written
// manifest lists — the packages `module verify` runs in.
func TestProjectNeverDropsSourcePaths(t *testing.T) {
	written := map[string]string{"manifest.json": `{"module": "m", "source_paths": ["internal/extensions/m/"]}`}
	for k, v := range laterRun {
		written[k] = v
	}

	ctx, dir := newProject(t, written)
	cmd := codecmd.NewModuleCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"project", "m", "--apply"})
	err := cmd.ExecuteContext(ctx)
	if err == nil || !strings.Contains(err.Error(), "code_packages") {
		t.Fatalf("project --apply on a module declaring no code_packages = %v, want refused naming code_packages", err)
	}
	if m, _ := os.ReadFile(filepath.Join(dir, "manifest.json")); !strings.Contains(string(m), "internal/extensions/m/") {
		t.Fatalf("the written manifest lost its source_paths:\n%s", m)
	}

	declared := map[string]string{"objects/m.cwm.yaml": `id: m
type: module
name: m
capabilities:
  code:
    kind: library
    code_packages: [internal/extensions/m/]
`}
	for k, v := range written {
		declared[k] = v
	}
	ctx, dir = newProject(t, declared)
	project(t, ctx, "--apply")
	if m, _ := os.ReadFile(filepath.Join(dir, "manifest.json")); !strings.Contains(string(m), `"source_paths"`) ||
		!strings.Contains(string(m), "internal/extensions/m/") {
		t.Fatalf("manifest.json does not list the declared code_packages as source_paths:\n%s", m)
	}
}
