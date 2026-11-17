// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A PROJECT GROUP IS JOINED INTO ONE PROJECT: on
// a fixture shaped like a demo project group - a project-group root holding
// programme modules and system modules, embedded dossier children under
// projects/ each with its study modules, one child holding system modules of
// its own - `cwm migrate` moves every child module flat into the root's
// modules folder, joins the system modules file by file, removes the child
// project objects and projects/, points a parent naming a child project at
// the root, drops the project-group capability and runs the steps from the
// lowest data version; every id stays and resolves. A dry run writes
// nothing; a module name two children hold is reported and refused, writing
// nothing.
func TestAProjectGroupIsJoinedIntoOneProject(t *testing.T) {
	types := []api.ObjectType{
		{Name: "project-group", Capabilities: []string{api.CapProject, legacyCapProjectGroup}},
		{Name: "dossier-architecture", Capabilities: []string{api.CapProject, "dossier"}},
		{Name: "note"},
	}
	svc, root := projectFixture(t, types)
	ctx := stdctx.Background()
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
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil
	}
	version := dataVersionOf(svc.steps)
	write(api.ProjectFileName, "id: demo\ntype: project-group\ncapabilities:\n  project:\n    uuid: 00000000-0000-7000-8000-0000000000c2\n    data_version: "+strconv.Itoa(version)+"\n  project-group: {}\n")
	write("modules/presentation/objects/presentation.cwm.yaml", "id: presentation\ntype: documentation\nname: Presentation\n")
	write("modules/sys-journal/objects/sys-journal.cwm.yaml", "id: sys-journal\ntype: system-module\nname: root journal\n")
	write("modules/sys-journal/days/2026-10-01.jsonl", "{}\n")
	write("modules/sys-inbox/objects/inbox.cwm.yaml", "id: sys-inbox.inbox\ntype: note\nname: inbox\n")
	child := func(dir, id string, v int) {
		write("projects/"+dir+"/"+api.ProjectFileName, "id: "+id+"\ntype: dossier-architecture\nname: "+id+"\ncapabilities:\n  dossier:\n    project_code: SP4321\n  project:\n    uuid: 01a16c17-a322-707a-8b5f-"+strconv.Itoa(100000+v)+"aaaaaa\n    data_version: "+strconv.Itoa(v)+"\n")
	}
	child("erp-acme-dossier", "erp-acme-dossier", version-2)
	write("projects/erp-acme-dossier/modules/erp-acme/objects/erp-acme.cwm.yaml", "id: erp-acme\ntype: etude-existant\nname: ERP Acme\nedges:\n  - kind: attaches\n    to: modules/erp-acme/confluence/as-is/index.md\n  - kind: relates-to\n    to: stellar.N-001\n")
	write("projects/erp-acme-dossier/modules/erp-acme/confluence/as-is/index.md", "# AS-IS\n")
	child("stellar-dossier", "stellar-dossier", version)
	write("projects/stellar-dossier/modules/stellar/objects/stellar.cwm.yaml", "id: stellar\ntype: etude-existant\nname: STELLAR\nparent: stellar-dossier\n")
	write("projects/stellar-dossier/modules/stellar/notes/N-001.cwm.yaml", "id: stellar.N-001\ntype: note\nname: a note\nparent: stellar\n")
	write("projects/stellar-dossier/modules/sys-journal/objects/sys-journal.cwm.yaml", "id: sys-journal\ntype: system-module\nname: stellar journal\n")
	write("projects/stellar-dossier/modules/sys-journal/days/2026-11-09.jsonl", "{\"n\":1}\n")
	write("projects/stellar-dossier/modules/sys-inbox/objects/inbox.cwm.yaml", "id: sys-inbox.inbox\ntype: note\nname: inbox\n")
	write("projects/stellar-dossier/.katyoti/planification/queue.db", "local")
	if err := os.MkdirAll(filepath.Join(root, "projects/erp-acme-dossier/.katyoti"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}

	// The dry run says what the join does and writes nothing.
	dry, err := svc.Migrate(ctx, api.MigrateInput{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	m := dry.GroupMerge
	if m == nil || m.Group != "demo" || m.Type != "dossier-architecture" || m.DataVersion != version-2 || len(m.Children) != 2 || len(m.Moved) != 2 || len(m.Joined) != 2 || len(m.Collisions) != 0 {
		t.Fatalf("the dry run: %+v", m)
	}
	if m.Before != 7 || m.After != 5 { // root 3 + erp 1 + stellar 3; then 3 + erp-acme + stellar
		t.Fatalf("modules before %d, after %d", m.Before, m.After)
	}
	if len(m.Retargeted) != 1 || m.Retargeted[0] != "projects/stellar-dossier/modules/stellar/objects/stellar.cwm.yaml" {
		t.Fatalf("retargeted: %v", m.Retargeted)
	}
	if len(m.KeptAside) != 1 || m.KeptAside[0].From != "projects/stellar-dossier/.katyoti" {
		t.Fatalf("kept aside: %+v", m.KeptAside)
	}
	if len(m.Dropped) != 2 || m.Dropped[1].Capabilities["dossier"]["project_code"] != "SP4321" {
		t.Fatalf("dropped: %+v", m.Dropped)
	}
	if !exists("projects/stellar-dossier/modules/stellar") || exists("modules/stellar") {
		t.Fatal("the dry run wrote")
	}

	// The join, then the steps.
	out, err := svc.Migrate(ctx, api.MigrateInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.From != version-2 || out.To != version {
		t.Fatalf("migrated %d to %d", out.From, out.To)
	}
	if exists("projects") {
		t.Fatal("projects/ is left")
	}
	for _, rel := range []string{"modules/erp-acme/confluence/as-is/index.md", "modules/stellar/notes/N-001.cwm.yaml", "modules/sys-journal/days/2026-11-09.jsonl", "modules/sys-journal/days/2026-10-01.jsonl", ".katyoti/joined-projects/stellar-dossier/planification/queue.db"} {
		if !exists(rel) {
			t.Fatalf("%s missing after the join", rel)
		}
	}
	project, err := ReadObjectFile(filepath.Join(root, api.ProjectFileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, still := project.Capability(legacyCapProjectGroup); still || project.Type != "dossier-architecture" {
		t.Fatalf("the project after: %+v", project)
	}
	if g, _ := project.Capability(api.CapProject); g["data_version"] != version && g["data_version"] != float64(version) && g["data_version"] != uint64(version) {
		t.Fatalf("data version after: %v", g["data_version"])
	}
	journal, _ := ReadObjectFile(filepath.Join(root, "modules/sys-journal/objects/sys-journal.cwm.yaml"))
	if journal.Name != "root journal" {
		t.Fatalf("the root's system module object was not kept: %+v", journal)
	}
	for _, want := range []struct{ module, id, parent string }{{"stellar", "stellar", "demo"}, {"stellar", "stellar.N-001", "stellar"}, {"erp-acme", "erp-acme", ""}} {
		got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: want.module, ID: want.id})
		if err != nil {
			t.Fatalf("%s does not resolve after the join: %v", want.id, err)
		}
		if got.Object.Parent != want.parent {
			t.Fatalf("%s parent %q, want %q", want.id, got.Object.Parent, want.parent)
		}
	}

	// A module name two children hold: reported, refused, nothing written.
	svc2, root2 := projectFixture(t, types)
	root = root2
	write(api.ProjectFileName, "id: demo\ntype: project-group\ncapabilities:\n  project:\n    uuid: 00000000-0000-7000-8000-0000000000c2\n    data_version: "+strconv.Itoa(version)+"\n  project-group: {}\n")
	child("erp-acme-dossier", "erp-acme-dossier", version)
	write("projects/erp-acme-dossier/modules/etude-erp-acme/objects/etude-erp-acme.cwm.yaml", "id: etude-erp-acme\ntype: etude-existant\n")
	child("stellar-dossier", "stellar-dossier", version)
	write("projects/stellar-dossier/modules/etude-erp-acme/objects/etude-erp-acme.cwm.yaml", "id: etude-erp-acme\ntype: etude-existant\nparent: stellar-dossier\nedges:\n  - kind: attaches\n    to: modules/etude-erp-acme/confluence/index.md\n")
	write("projects/stellar-dossier/modules/etude-erp-acme/objects/doc-index.cwm.yaml", "id: etude-erp-acme.doc-index\ntype: note\nparent: etude-erp-acme\n")
	write("projects/stellar-dossier/modules/etude-erp-acme/confluence/index.md", "# Code SP etude-erp-acme\n")
	write("projects/stellar-dossier/modules/stellar/objects/stellar.cwm.yaml", "id: stellar\ntype: etude-existant\nparent: stellar-dossier\nedges:\n  - kind: relates-to\n    to: etude-erp-acme.doc-index\n")
	write("projects/erp-acme-dossier/modules/erp-acme/objects/erp-acme.cwm.yaml", "id: erp-acme\ntype: etude-existant\nedges:\n  - kind: relates-to\n    to: etude-erp-acme\n  - kind: relates-to\n    to: stellar-dossier:etude-erp-acme.doc-index\n")
	dry, err = svc2.Migrate(ctx, api.MigrateInput{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if c := dry.GroupMerge.Collisions; len(c) != 1 || c[0].Module != "etude-erp-acme" || len(c[0].Paths) != 2 {
		t.Fatalf("the collision: %+v", c)
	}
	if _, err := svc2.Migrate(ctx, api.MigrateInput{}); !errors.Is(err, api.ErrConflict) || !strings.Contains(err.Error(), "etude-erp-acme") {
		t.Fatalf("a colliding join: %v", err)
	}
	if !exists("projects/stellar-dossier/modules/etude-erp-acme/objects/etude-erp-acme.cwm.yaml") || exists("modules/etude-erp-acme") {
		t.Fatal("a refused join wrote")
	}
	if o, _ := ReadObjectFile(filepath.Join(root, "projects/stellar-dossier/modules/etude-erp-acme/objects/etude-erp-acme.cwm.yaml")); o.Parent != "stellar-dossier" {
		t.Fatal("a refused join retargeted")
	}

	// Both kept: the stellar copy renamed as the group joins - its folder,
	// its ids, every parent and edge its own project named them by; the
	// other copy and the text of its documents left as they are.
	if _, err := svc2.Migrate(ctx, api.MigrateInput{DryRun: true, GroupRenames: map[string]string{"projects/stellar-dossier/modules/nope": "x"}}); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("a rename naming no child module: %v", err)
	}
	renames := map[string]string{"projects/stellar-dossier/modules/etude-erp-acme": "etude-erp-acme-stellar"}
	dry, err = svc2.Migrate(ctx, api.MigrateInput{DryRun: true, GroupRenames: renames})
	if err != nil {
		t.Fatal(err)
	}
	if m := dry.GroupMerge; len(m.Collisions) != 0 || m.After != 4 || exists("modules/etude-erp-acme-stellar") {
		t.Fatalf("the renaming dry run: %+v", m)
	}
	// The join's writes are CWM's own: the index rebuilt after it announces
	// no catch-up (no card of objects changed outside the verbs).
	if _, err := svc2.ObjectList(ctx, api.ObjectListInput{}); err != nil { // the catch-up's baseline
		t.Fatal(err)
	}
	var announced []catchUp
	svc2.index().catch.on = func(c catchUp) { announced = append(announced, c) }
	if _, err := svc2.MigrateCommit(ctx, api.MigrateCommitInput{GroupRenames: renames}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc2.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	if len(announced) != 0 {
		t.Fatalf("the join was announced as an outside change: %+v", announced)
	}
	if exists("projects") || !exists("modules/etude-erp-acme-stellar/objects/etude-erp-acme-stellar.cwm.yaml") || !exists("modules/etude-erp-acme/objects/etude-erp-acme.cwm.yaml") {
		t.Fatal("the renamed join's folders")
	}
	read := func(rel string) api.Object {
		t.Helper()
		o, err := ReadObjectFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	if o := read("modules/etude-erp-acme-stellar/objects/etude-erp-acme-stellar.cwm.yaml"); o.ID != "etude-erp-acme-stellar" || o.Parent != "demo" || o.Edges[0].To != "modules/etude-erp-acme-stellar/confluence/index.md" {
		t.Fatalf("the renamed module's object: %+v", o)
	}
	if o := read("modules/etude-erp-acme-stellar/objects/doc-index.cwm.yaml"); o.ID != "etude-erp-acme-stellar.doc-index" || o.Parent != "etude-erp-acme-stellar" {
		t.Fatalf("the renamed module's document: %+v", o)
	}
	if o := read("modules/stellar/objects/stellar.cwm.yaml"); o.Edges[0].To != "etude-erp-acme-stellar.doc-index" {
		t.Fatalf("its own project's edge: %+v", o)
	}
	if o := read("modules/erp-acme/objects/erp-acme.cwm.yaml"); o.Edges[0].To != "etude-erp-acme" || o.Edges[1].To != "etude-erp-acme-stellar.doc-index" {
		t.Fatalf("the other project's edges: %+v", o)
	}
	if o := read("modules/etude-erp-acme/objects/etude-erp-acme.cwm.yaml"); o.ID != "etude-erp-acme" {
		t.Fatalf("the other copy: %+v", o)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "modules/etude-erp-acme-stellar/confluence/index.md")); string(b) != "# Code SP etude-erp-acme\n" {
		t.Fatalf("a document's text was rewritten: %q", b)
	}
	for _, id := range []string{"etude-erp-acme-stellar", "etude-erp-acme-stellar.doc-index", "etude-erp-acme"} {
		mod, _, _ := strings.Cut(id, ".")
		if _, err := svc2.ObjectGet(ctx, api.ObjectGetInput{Module: mod, ID: id}); err != nil {
			t.Fatalf("%s does not resolve after the renamed join: %v", id, err)
		}
	}
}
