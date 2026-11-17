// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE PROJECTS A UNIVERSE KNOWS (« Mes dossiers »): the one
// it serves first, then each project of the installation's projects folder by
// name, each with its type; a folder without a project object is not one; a
// capability keeps the projects whose type carries it.
func TestProjectsKnownListsTheServedThenTheProjectsFolderByCapability(t *testing.T) {
	types := []api.ObjectType{
		{Name: "software-project", Capabilities: []string{api.CapProject}},
		{Name: "dossier-architecture", Capabilities: []string{api.CapProject, "dossier"}},
	}
	svc, served := projectFixture(t, types)
	folder := t.TempDir()
	svc.Config.ProjectsFolder = folder
	writeProject := func(dir, id, typ, name string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "id: " + id + "\ntype: " + typ + "\nname: " + name + "\n"
		if err := os.WriteFile(filepath.Join(dir, api.ProjectFileName), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeProject(served, "katyoti", "software-project", "KaTyoti")
	writeProject(filepath.Join(folder, "erp-acme"), "erp-acme", "dossier-architecture", "ERP Acme")
	writeProject(filepath.Join(folder, "cosmos"), "cosmos", "dossier-architecture", "Cosmos")
	if err := os.MkdirAll(filepath.Join(folder, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := stdctx.Background()

	all, err := svc.ProjectsKnown(ctx, api.ProjectsKnownInput{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range all.Projects {
		got = append(got, p.ID+":"+p.Type)
	}
	if want := []string{"katyoti:software-project", "cosmos:dossier-architecture", "erp-acme:dossier-architecture"}; !equalStrings(got, want) {
		t.Fatalf("known %v; want %v", got, want)
	}
	if !all.Projects[0].Served || all.Projects[1].Served || all.Projects[0].Name != "KaTyoti" {
		t.Errorf("the served one first and only it served: %+v", all.Projects)
	}

	dossiers, err := svc.ProjectsKnown(ctx, api.ProjectsKnownInput{Capability: "dossier"})
	if err != nil {
		t.Fatal(err)
	}
	if len(dossiers.Projects) != 2 || dossiers.Projects[0].ID != "cosmos" || dossiers.Projects[1].Dir != filepath.Join(folder, "erp-acme") {
		t.Errorf("the dossiers: %+v; want cosmos then erp-acme", dossiers.Projects)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
