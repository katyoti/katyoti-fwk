// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// overwritesFixture is a project at the baseline whose binary ships one step,
// the retirement of the overwrites trees, read against a made-up kind
// `note`; the project's tree of ktw-example overrides the note hello.
func overwritesFixture(t *testing.T) (*Service, string, string) {
	t.Helper()
	project := t.TempDir()
	// The universe declares the capability sys-contributions operates.
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: project}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "contributions"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	svc.steps = nil
	ctx := stdctx.Background()
	if _, err := svc.ProjectInit(ctx, api.ProjectInitInput{Name: "acme"}); err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	svc.steps = []MigrationStep{{Pack: "projectcontributions", Title: "the overwrites trees retire", Project: retireOverwritesTrees}}
	decl := contribapi.Contribution{Provider: "system", Type: contribapi.ResourceTypeContributionType, ID: "note"}
	decl.SetData(json.RawMessage(`{"name":"note","layers":{"data":{"mediaType":"application/json"}}}`))
	svc.contributionKinds = func() (contribapi.Kinds, error) { return contribapi.KindsFrom([]contribapi.Contribution{decl}), nil }
	svc.Config.BackupDir = filepath.Join(t.TempDir(), "backups")
	f, err := projectfile.Read(project)
	if err != nil {
		t.Fatal(err)
	}
	tree := f.OverwritesDir(project, "ktw-example")
	writeTreeFile(t, tree, "embed/note/hello.yaml", "title: Hello\ntext: overridden\n")
	writeTreeFile(t, tree, "embed/note/hello.i18n.yaml", "fr:\n  title: Bonjour\n")
	return svc, project, tree
}

func writeTreeFile(t *testing.T, tree, rel, text string) {
	t.Helper()
	p := filepath.Join(tree, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The migration converts a project's overwrites tree into an override object
// carrying `contributes`, its layers files of sys-contributions, reports the
// widening, keeps a copy of the tree in the backup folder and removes it;
// the dry run before says the same and writes nothing; a second run changes
// nothing.
func TestTheOverwritesTreeBecomesOverrideObjectsAndRetires(t *testing.T) {
	svc, project, tree := overwritesFixture(t)
	ctx := stdctx.Background()

	before := snapshot(t, project)
	dry, err := svc.Migrate(ctx, api.MigrateInput{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(dry.Steps) != 1 {
		t.Fatalf("dry run steps = %+v", dry.Steps)
	}
	lines := strings.Join(dry.Steps[0].Lines, "\n")
	for _, want := range []string{"overrides ktw-example:/note/hello", "become the whole layers", "removed, kept in"} {
		if !strings.Contains(lines, want) {
			t.Errorf("dry run lines lack %q:\n%s", want, lines)
		}
	}
	if len(dry.Steps[0].Changed) == 0 || len(dry.Steps[0].Removed) != 1 {
		t.Errorf("dry run changed %v, removed %v; want the layer files and the tree", dry.Steps[0].Changed, dry.Steps[0].Removed)
	}
	if after := snapshot(t, project); !mapsEqual(before, after) {
		t.Fatal("the dry run wrote to the project")
	}
	if _, err := os.Stat(svc.Config.BackupDir); !os.IsNotExist(err) {
		t.Fatal("the dry run made a backup")
	}

	if _, err := svc.Migrate(ctx, api.MigrateInput{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if f, _ := projectfile.Read(project); f.DataVersion != projectfile.BaselineDataVersion+1 {
		t.Fatalf("data version = %d, want %d", f.DataVersion, projectfile.BaselineDataVersion+1)
	}
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "sys-contributions", ID: "sys-contributions.override-ktw-example-note-hello"})
	if err != nil {
		t.Fatalf("the override object: %v", err)
	}
	c := got.Object.Capabilities["contributes"]
	if c["overrides"] != "ktw-example:/note/hello" || c["kind"] != "note" || c["id"] != "hello" {
		t.Fatalf("contributes = %v", c)
	}
	layers, _ := c["layers"].(map[string]any)
	if layers[contribapi.LayerData] == nil || layers[contribapi.LayerTranslations] == nil {
		t.Fatalf("layers = %v; want the data and the translations the tree served", layers)
	}
	for name, src := range layers {
		file, _ := strings.CutPrefix(src.(string), "files/")
		b, err := os.ReadFile(filepath.Join(project, "modules", "sys-contributions", filepath.FromSlash(file)))
		if err != nil || len(b) == 0 {
			t.Errorf("layer %s: file %s not written: %v", name, file, err)
		}
	}
	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Fatal("the tree is still there")
	}
	kept, _ := filepath.Glob(filepath.Join(svc.Config.BackupDir, "*", "ktw-example", "embed", "note", "hello.yaml"))
	if len(kept) != 1 {
		t.Fatalf("the backup of the tree = %v; want hello.yaml kept", kept)
	}

	again := snapshot(t, project)
	second, err := svc.Migrate(ctx, api.MigrateInput{})
	if err != nil || len(second.Steps) != 0 {
		t.Fatalf("second run = %+v, %v; want nothing to do", second, err)
	}
	if after := snapshot(t, project); !mapsEqual(again, after) {
		t.Fatal("a second migration changed the project")
	}
}

// A file of the tree that maps to no layer stops the migration, naming it:
// nothing is written, the tree is intact, the data version stays.
func TestAnUnmappedOverwritesFileStopsTheMigration(t *testing.T) {
	svc, project, tree := overwritesFixture(t)
	writeTreeFile(t, tree, "embed/other/readme.md", "# not an entry\n")
	ctx := stdctx.Background()
	before := snapshot(t, project)
	_, err := svc.Migrate(ctx, api.MigrateInput{})
	if err == nil || !strings.Contains(err.Error(), "ktw-example/embed/other/readme.md") {
		t.Fatalf("migrate = %v; want a refusal naming the unmapped file", err)
	}
	if after := snapshot(t, project); !mapsEqual(before, after) {
		t.Fatal("a refused migration wrote to the project")
	}
	if f, _ := projectfile.Read(project); f.DataVersion != projectfile.BaselineDataVersion {
		t.Fatalf("data version = %d after a refusal", f.DataVersion)
	}
}

// With no folder to keep a copy in, the tree is never removed.
func TestNoBackupFolderKeepsTheTree(t *testing.T) {
	svc, _, tree := overwritesFixture(t)
	svc.Config.BackupDir = ""
	if _, err := svc.Migrate(stdctx.Background(), api.MigrateInput{}); err == nil {
		t.Fatal("the migration removed a tree with no backup folder")
	}
	if _, err := os.Stat(filepath.Join(tree, "embed", "note", "hello.yaml")); err != nil {
		t.Fatalf("the tree is gone: %v", err)
	}
}
