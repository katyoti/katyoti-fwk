// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : project groups are retired - no pack declares the project-group
// type any more - yet a repository written before data version 41 whose root
// is still a group opens through the legacy reading until cwm migrate joins
// it: its root reads as a project, valid, the conversion said waiting; its
// embedded children's modules stay out of its graph until the join; and no
// new object of the retired type is made.
func TestAProjectGroupNotYetJoinedStillOpens(t *testing.T) {
	cfg, _ := realCorpusConfig(t)
	if _, declared := typeNamed(cfg, "project-group"); declared {
		t.Fatal("a pack still declares the project-group type")
	}
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The data version before the step that retires project groups.
	retire := slices.IndexFunc(migrationSteps, func(s MigrationStep) bool { return strings.HasPrefix(s.Title, "project groups are retired") })
	if retire < 0 {
		t.Fatal("no step retires project groups")
	}
	write(api.ProjectFileName, "id: demos\ntype: project-group\nname: Demos\ncapabilities:\n  project:\n    data_version: "+
		strconv.Itoa(dataVersionOf(migrationSteps[:retire]))+"\n  project-group:\n    projects_dir: projects\n")
	write("projects/paie/"+api.ProjectFileName, "id: paie\ntype: dossier-architecture\nname: Dossier paie\n")
	write("projects/paie/modules/etude-paie/objects/etude-paie.cwm.yaml", "id: etude-paie\ntype: etude-existant\nname: Étude paie\n")

	c := *cfg
	c.ProjectRoot = root
	svc, err := New(stdctx.Background(), &c)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	scan, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true, Validate: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, inv := range scan.Invalid {
		t.Errorf("%s: %s", inv.ID, strings.Join(inv.Problems, " || "))
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "demos"}); err != nil {
		t.Fatalf("the group's root does not open: %v", err)
	}
	if ok, _ := api.HasCapability(ctx, svc, "demos", api.CapProject); !ok {
		t.Error("the group's root is read as no project")
	}
	if len(svc.conversionFindings()) == 0 {
		t.Error("the conversion is not said waiting")
	}
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: "etude-paie"}); err == nil {
		t.Error("a child's module was read before the join")
	}
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Object: api.Object{ID: "other", Type: "project-group", Name: "Other"}}); err == nil {
		t.Error("a new object of the retired project-group type was made")
	}
}
