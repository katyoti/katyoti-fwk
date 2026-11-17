// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
	"github.com/spf13/cobra"
)

// fakeSkillManager is an in-memory SkillManager used by command tests so they
// don't need a real loader, embed FS, or service container.
//
// Post-2026-05-24: the GetReference + RunScript RPC methods are gone
// from SkillManager. The fake catalog (newFakeCatalogFromSkills)
// provides L0/L1; this fake carries only the GetSkill / GetSkillRequires
// in-process accessors.
type fakeSkillManager struct {
	skills       []api.SkillInfo
	instructions map[string]string
}

func (f *fakeSkillManager) GetSkill(name string) (api.Skill, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakeSkillManager) GetSkillRequires(name string) []string {
	for _, s := range f.skills {
		if s.Name == name {
			return s.Requires
		}
	}
	return nil
}
func (f *fakeSkillManager) Reload(_ context.Context) error { return nil }

// fakeCatalog is an in-memory `mbapi.Catalog` used by command tests so
// the catalog-backed `skills list` path can resolve without a full
// manifest extension wiring. Built from a `fakeSkillManager`'s skill
// list so the two sources stay in lockstep.
type fakeCatalog struct {
	// source holds each skill's raw SKILL.md, served through OpenLayer the
	// way a real catalog serves the source layer.
	source map[string]string

	manifest *mbapi.Manifest
}

func (f *fakeCatalog) Get(_ context.Context, name string) (*mbapi.Manifest, error) {
	if f.manifest != nil && f.manifest.Provider == name {
		return f.manifest, nil
	}
	return nil, mbapi.ErrSidecarNotInCatalog
}

func (f *fakeCatalog) List(_ context.Context) ([]*mbapi.Manifest, error) {
	if f.manifest == nil {
		return nil, nil
	}
	return []*mbapi.Manifest{f.manifest}, nil
}

func (f *fakeCatalog) Names(_ context.Context) ([]string, error) {
	if f.manifest == nil {
		return nil, nil
	}
	return []string{f.manifest.Provider}, nil
}

func (f *fakeCatalog) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
	if f.manifest == nil {
		return nil, nil
	}
	var out []contribapi.Contribution
	for _, e := range f.manifest.Contributions {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeCatalog) GetBody(_ context.Context, uri string) ([]byte, error) {
	if f.manifest == nil {
		return nil, mbapi.ErrSidecarNotInCatalog
	}
	pu, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return nil, err
	}
	for _, e := range f.manifest.Contributions {
		prov := e.Provider
		if prov == "" {
			prov = f.manifest.Provider
		}
		if e.Type == pu.Type && e.ID == pu.ID && prov == pu.Provider {
			return e.Data(), nil
		}
	}
	return nil, mbapi.ErrSidecarNotInCatalog
}

// newFakeCatalogFromSkills projects a `fakeSkillManager`'s skill list
// into a single-manifest fake catalog under provider `"ktw-test"` (the
// same identity the test cobra root advertises).
func newFakeCatalogFromSkills(t *testing.T, fake *fakeSkillManager) *fakeCatalog {
	t.Helper()
	m := &mbapi.Manifest{Provider: "ktw-test"}
	source := map[string]string{}
	for _, s := range fake.skills {
		body := mbapi.SkillBody{Requires: s.Requires, Provides: s.Provides}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		// The instructions are the skill's SOURCE layer, not a body field —
		// so the fake serves the raw SKILL.md the way a real catalog does.
		md := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s",
			s.Name, s.Description, fake.instructions[s.Name])
		source[contribapi.Contribution{Provider: "ktw-test", Type: mbapi.ResourceTypeSkill, ID: s.Name}.ResolvedURI()] = md
		m.Contributions = append(m.Contributions, contribapi.Contribution{
			Provider: "ktw-test",
			Type:     mbapi.ResourceTypeSkill,
			ID:       s.Name,
			Layers: map[string]contribapi.LayerRef{
				contribapi.LayerData: contribapi.InlineData(b)[contribapi.LayerData],
				contribapi.LayerSource: {
					Hash: contribapi.HashContent([]byte(md)), Size: int64(len(md)),
					MediaType: "text/markdown",
				},
			},
		})
	}
	return &fakeCatalog{manifest: m, source: source}
}

// withFakeManager registers a fakeSkillManager AND a fakeCatalog into a
// fresh Application context so commands' `UseService[SkillManager]`
// (legacy runtime ops) and `UseService[Catalog]` (catalog-backed list)
// both resolve.
func withFakeManager(t *testing.T, fake *fakeSkillManager) context.Context {
	t.Helper()
	app := application.New(application.Config())
	application.PutService[api.SkillManager](app, "default", fake)
	application.PutService[mbapi.Catalog](app, "default", newFakeCatalogFromSkills(t, fake))
	return application.WithApplication(context.Background(), app)
}

func defaultFake() *fakeSkillManager {
	return &fakeSkillManager{
		skills: []api.SkillInfo{
			{Name: "doc-create", Description: "Create documents."},
			{Name: "leanix-impact", Description: "Impact analysis.", Requires: []string{"workspace"}},
		},
		instructions: map[string]string{
			"doc-create": "# doc-create\n\nFull instructions body.",
		},
	}
}

func runCmd(t *testing.T, ctx context.Context, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewSkillsCommand()
	// NewSkillsCommand returns the `skills` subtree alone; for the binary-name
	// detection in `list` to work, parent it under a fake binary root.
	binRoot := newBinaryRoot("ktw-test")
	binRoot.AddCommand(root)

	var outBuf, errBuf bytes.Buffer
	binRoot.SetOut(&outBuf)
	binRoot.SetErr(&errBuf)
	binRoot.SetArgs(append([]string{"skills"}, args...))

	err = binRoot.ExecuteContext(ctx)
	return outBuf.String(), errBuf.String(), err
}

// newBinaryRoot creates a Cobra root command mimicking a sidecar binary so
// that subcommand tests exercise the same parent-walk logic that the live
// binary uses.
func newBinaryRoot(name string) *cobra.Command {
	return &cobra.Command{Use: name}
}

func TestSkillsList_EmitsCLIPromptBlock(t *testing.T) {
	ctx := withFakeManager(t, defaultFake())
	stdout, _, err := runCmd(t, ctx, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	mustContain := []string{
		"<skills_system>",
		"## Available Skills",
		// Catalog-backed list always populates Provider — names render
		// as the full URI form `<provider>:/skill/<name>` (canonical
		// per the command-surface decision, locked 2026-05-24).
		"<name>ktw-test:/skill/doc-create</name>",
		"<name>ktw-test:/skill/leanix-impact</name>",
		// `get-reference` and `run-script` rows dropped along with
		// the corresponding SkillManager RPC surface; all resource
		// content (skills included) resolves through the resources
		// surface with the full `<provider>:/<type>/<id>` URI — the
		// Addressing block names `llm resources read` as the single
		// CLI-mode verb (one method per outcome).
		"`ktw-test llm resources read",
	}
	for _, frag := range mustContain {
		if !strings.Contains(stdout, frag) {
			t.Errorf("list output missing %q\n--- stdout ---\n%s", frag, stdout)
		}
	}
	// Must not advertise agent-mode tool names from the CLI surface.
	for _, frag := range []string{"get_skill_instructions(", "run_skill_script("} {
		if strings.Contains(stdout, frag) {
			t.Errorf("list output leaked agent-mode tool name %q", frag)
		}
	}
}

func TestSkillsGetInstructions_PrintsBody(t *testing.T) {
	ctx := withFakeManager(t, defaultFake())
	// Catalog-served — InstructionsFromCatalog requires the full URI form
	// (post-2026-05-24 normalization).
	stdout, _, err := runCmd(t, ctx, "get-instructions", "ktw-test:/skill/doc-create")
	if err != nil {
		t.Fatalf("get-instructions: %v", err)
	}
	if !strings.Contains(stdout, "Full instructions body.") {
		t.Errorf("expected instructions body, got %q", stdout)
	}
}

// TestSkillsGetReference + TestSkillsRunScript dropped 2026-05-24 —
// the CLI subverbs they exercised (`sc skills get-reference` /
// `sc skills run-script`) were retired along with the SkillManager RPC.
// Equivalent coverage now lives in:
//   - References: `sc resources get <provider>:/reference/<path>`
//     (tested in `internal/extensions/manifest/commands/resources_test.go`)
//   - Standalone scripts: `<provider>:scripts/<name>` slot tests in
//     `internal/extensions/scripts/`
//
// TestParseParams + mapEqual were helpers for the dropped run-script
// command — removed alongside it.

// Two-phase bootstrap protocol stubs (hub-boot-order ADR).
// OpenLayer serves the source layer — where a skill's instructions live.
func (f *fakeCatalog) OpenLayer(_ context.Context, uri, _ string) (io.ReadCloser, contribapi.LayerRef, error) {
	raw, ok := f.source[uri]
	if !ok {
		return nil, contribapi.LayerRef{}, mbapi.ErrSidecarNotInCatalog
	}
	return io.NopCloser(strings.NewReader(raw)), contribapi.LayerRef{
		Hash: contribapi.HashContent([]byte(raw)), Size: int64(len(raw)),
	}, nil
}

func (*fakeCatalog) SetSchema(_ mbapi.CatalogSchemaService) error { return nil }
func (*fakeCatalog) Reindex(_ context.Context, _ []string) error  { return nil }
func (*fakeCatalog) MarkReady(_ context.Context) error            { return nil }
func (*fakeCatalog) Ready() bool                                  { return true }
