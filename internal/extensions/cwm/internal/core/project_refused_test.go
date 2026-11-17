// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : a project whose type needs a solution the universe
// lacks is refused at OPENING, not at service creation - New succeeds (the
// sidecar starts and answers), and every read and write of the project answers
// an error wrapping api.ErrProjectRefused whose text starts exactly
// `project refused: ` followed by the reason, with no `invalid input:` between
// them (the GUI reads that lead, projectRefusal.ts). The right universe opens.
func TestAProjectNeedingAMissingSolutionIsRefusedAtOpening(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, api.ProjectFileName), []byte("id: demo\ntype: dossier-architecture\nname: Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	types := []api.ObjectType{{Name: "dossier-architecture", Label: "Dossier d'architecture", Solution: "archifactory"}}
	open := func(solution string) *Service {
		t.Helper()
		cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: root, UniverseID: "acme", UniverseSolution: solution,
			CatalogTypes: CatalogTypeSet{Objects: types}}
		svc, err := New(stdctx.Background(), cfg)
		if err != nil {
			t.Fatalf("New with solution %q: %v - the service must start, the refusal comes at opening", solution, err)
		}
		t.Cleanup(func() { _ = svc.Close() })
		return svc
	}
	ctx := stdctx.Background()
	svc := open("")

	refusedAs := func(verb string, err error) {
		t.Helper()
		if !errors.Is(err, api.ErrProjectRefused) {
			t.Errorf("%s: %v; want an error wrapping ErrProjectRefused", verb, err)
			return
		}
		msg := err.Error()
		if !strings.HasPrefix(msg, "project refused: this project is a Dossier d'architecture, which needs the solution archifactory") {
			t.Errorf("%s: %q; want it to start 'project refused: ' followed directly by the reason", verb, msg)
		}
		if strings.Contains(msg, "invalid input") || !strings.Contains(msg, "katyoti universe set-solution acme archifactory") {
			t.Errorf("%s: %q; want no 'invalid input', the verb named", verb, msg)
		}
	}
	_, err := svc.ObjectTree(ctx, api.ObjectTreeInput{})
	refusedAs("ObjectTree", err)
	_, err = svc.ObjectList(ctx, api.ObjectListInput{})
	refusedAs("ObjectList", err)
	_, err = svc.ObjectGet(ctx, api.ObjectGetInput{ID: "demo"})
	refusedAs("ObjectGet", err)
	_, err = svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{ID: "m.x", Type: "dossier-architecture", Name: "x"}})
	refusedAs("ObjectCreate", err)
	if errs := svc.RunBootstrapHooks(ctx, BootstrapOwn); len(errs) != 1 || !errors.Is(errs[0], api.ErrProjectRefused) {
		t.Errorf("RunBootstrapHooks: %v; want only the refusal", errs)
	}

	// : the project-level verbs answer the same refusal and write
	// nothing - the conversion (dry run and real, and its commit) and a
	// second init.
	before := treeOf(t, root)
	_, err = svc.Migrate(ctx, api.MigrateInput{DryRun: true})
	refusedAs("Migrate dry run", err)
	_, err = svc.Migrate(ctx, api.MigrateInput{})
	refusedAs("Migrate", err)
	_, err = svc.MigrateCommit(ctx, api.MigrateCommitInput{})
	refusedAs("MigrateCommit", err)
	_, err = svc.ProjectInit(ctx, api.ProjectInitInput{Name: "again"})
	refusedAs("ProjectInit", err)
	if after := treeOf(t, root); after != before {
		t.Errorf("a refused project's verbs wrote in it:\nbefore %s\nafter  %s", before, after)
	}

	// : the known projects are the way out of a refused project -
	// ProjectsKnown answers, the served entry names the refusal, another
	// project of the projects folder is listed without it, nothing written.
	folder := t.TempDir()
	other := filepath.Join(folder, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, api.ProjectFileName), []byte("id: other\ntype: dossier-architecture\nname: Other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.Config.ProjectsFolder = folder
	beforeOther := treeOf(t, folder)
	known, err := svc.ProjectsKnown(ctx, api.ProjectsKnownInput{})
	if err != nil {
		t.Fatalf("ProjectsKnown on a refused project: %v; want the projects listed", err)
	}
	if len(known.Projects) != 2 {
		t.Fatalf("ProjectsKnown: %+v; want the served project and the other", known.Projects)
	}
	if p := known.Projects[0]; !p.Served || p.ID != "demo" ||
		!strings.HasPrefix(p.Refused, "project refused: this project is a Dossier d'architecture, which needs the solution archifactory") {
		t.Errorf("served entry %+v; want demo served, Refused naming the refusal", p)
	}
	if p := known.Projects[1]; p.Served || p.ID != "other" || p.Refused != "" {
		t.Errorf("other entry %+v; want other listed, not served, no refusal", p)
	}
	if filtered, err := svc.ProjectsKnown(ctx, api.ProjectsKnownInput{Capability: "dossier"}); err != nil || len(filtered.Projects) != 0 {
		t.Errorf("ProjectsKnown by a capability no declared type carries: %+v, %v; want none and no error", filtered.Projects, err)
	}
	if treeOf(t, root) != before || treeOf(t, folder) != beforeOther {
		t.Errorf("ProjectsKnown wrote in a project")
	}

	// The universe of the right solution opens the project.
	ok := open("archifactory@2026.10")
	if _, err := ok.ObjectTree(ctx, api.ObjectTreeInput{}); errors.Is(err, api.ErrProjectRefused) {
		t.Errorf("in an ArchiFactory universe: %v", err)
	}
}

// treeOf is every file under root with its content, one line each - two calls
// compare equal only when nothing was written, added or removed.
func treeOf(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		b.WriteString(filepath.ToSlash(rel) + "=" + string(data) + "\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// : a project whose data is newer than the binary is
// refused at OPENING, not at service creation - New succeeds (ktw-agents
// starts and answers ktsp/ping), and every read, write and conversion of the
// project answers an error wrapping api.ErrProjectRefused, `project refused: `
// followed directly by the reason naming the data's version, the binary's and
// the release to install. Newer data wins over a missing solution: this
// binary's type checks may not even read it. ProjectsKnown keeps answering,
// the served entry naming the refusal.
func TestAProjectOfNewerDataIsRefusedAtOpening(t *testing.T) {
	root := t.TempDir()
	newer := dataVersionOf(migrationSteps) + 1
	body := fmt.Sprintf("id: demo\ntype: dossier-architecture\nname: Demo\ncapabilities:\n  project:\n    data_version: %d\n", newer)
	if err := os.WriteFile(filepath.Join(root, api.ProjectFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// The type needs a solution the universe lacks too: the newer data wins.
	types := []api.ObjectType{{Name: "dossier-architecture", Label: "Dossier d'architecture", Solution: "archifactory"}}
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: root, UniverseID: "acme",
		CatalogTypes: CatalogTypeSet{Objects: types}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New on data at version %d: %v - the service must start, the refusal comes at opening", newer, err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()

	lead := fmt.Sprintf("project refused: the project's data is at version %d and this binary knows up to version %d", newer, newer-1)
	refusedAs := func(verb string, err error) {
		t.Helper()
		if !errors.Is(err, api.ErrProjectRefused) {
			t.Errorf("%s: %v; want an error wrapping ErrProjectRefused", verb, err)
			return
		}
		msg := err.Error()
		if !strings.HasPrefix(msg, lead) || strings.Contains(msg, "invalid input") || !strings.Contains(msg, "install a newer KaTyoti release") {
			t.Errorf("%s: %q; want it to start %q, name the release to install, no 'invalid input'", verb, msg, lead)
		}
	}
	before := treeOf(t, root)
	_, err = svc.ObjectTree(ctx, api.ObjectTreeInput{})
	refusedAs("ObjectTree", err)
	_, err = svc.ObjectGet(ctx, api.ObjectGetInput{ID: "demo"})
	refusedAs("ObjectGet", err)
	_, err = svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: api.Object{ID: "m.x", Type: "dossier-architecture", Name: "x"}})
	refusedAs("ObjectCreate", err)
	_, err = svc.Migrate(ctx, api.MigrateInput{DryRun: true})
	refusedAs("Migrate dry run", err)
	_, err = svc.Migrate(ctx, api.MigrateInput{})
	refusedAs("Migrate", err)
	if errs := svc.RunBootstrapHooks(ctx, BootstrapOwn); len(errs) != 1 || !errors.Is(errs[0], api.ErrProjectRefused) {
		t.Errorf("RunBootstrapHooks: %v; want only the refusal", errs)
	}
	if after := treeOf(t, root); after != before {
		t.Errorf("a refused project's verbs wrote in it:\nbefore %s\nafter  %s", before, after)
	}

	svc.Config.ProjectsFolder = t.TempDir()
	known, err := svc.ProjectsKnown(ctx, api.ProjectsKnownInput{})
	if err != nil || len(known.Projects) != 1 {
		t.Fatalf("ProjectsKnown on a refused project: %+v, %v; want the served project listed", known.Projects, err)
	}
	if p := known.Projects[0]; !p.Served || !strings.HasPrefix(p.Refused, lead) || p.Conversion != api.ConversionNewer {
		t.Errorf("served entry %+v; want served, Refused naming the newer data, conversion newer", p)
	}
}
