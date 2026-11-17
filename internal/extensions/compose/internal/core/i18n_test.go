// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// `ktw-compose i18n <folder>`: the report names each missing text with its
// source; --scaffold writes the stubs beside the entry; --missing keeps only
// the entries that miss something. On an executable it reads the manifest,
// and refuses to scaffold.
func TestI18n_FolderAndExecutable(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "embed", "cwm", "object-types")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("note.yaml", "name: note\nlabel: Note\ndescription: A short note.\n")
	write("done.yaml", "name: done\nlabel: Done\n")
	write("done.i18n.yaml", "fr:\n  label: Fait\n")

	s := &Service{repoRoot: root}
	out, err := s.I18n(context.Background(), api.I18nInput{Target: root, Missing: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Lang != "fr" || out.Entries != 2 || out.Status != "incomplete" || len(out.Report) != 1 {
		t.Fatalf("report = %+v", out)
	}
	if e := out.Report[0]; !strings.HasSuffix(filepath.ToSlash(e.Entry), "object-types/note.yaml") || len(e.Missing) != 2 || e.Missing[0].Source == "" {
		t.Fatalf("note = %+v", e)
	}

	out, err = s.I18n(context.Background(), api.I18nInput{Target: root, Scaffold: true})
	if err != nil || len(out.Scaffolded) != 1 {
		t.Fatalf("scaffold = %+v, %v", out, err)
	}
	stub, _ := os.ReadFile(filepath.Join(dir, "note.i18n.yaml"))
	if !strings.Contains(string(stub), "label: ~ # Note") {
		t.Errorf("stub file:\n%s", stub)
	}

	ot := contribapi.Contribution{Provider: "ktw-x", Type: "object-type", ID: "note"}
	ot.SetData([]byte(`{"name":"note","label":"Note"}`))
	s.exeOverride = fakeExes{manifests: map[string]*mbapi.Manifest{"ktw-x": {Provider: "ktw-x", Contributions: []contribapi.Contribution{ot}}}}
	out, err = s.I18n(context.Background(), api.I18nInput{Target: "ktw-x"})
	if err != nil || out.Missing != 1 || out.Report[0].Missing[0].Path != "label" {
		t.Fatalf("exe report = %+v, %v", out, err)
	}
	if _, err := s.I18n(context.Background(), api.I18nInput{Target: "ktw-x", Scaffold: true}); err == nil {
		t.Error("scaffolding an executable was accepted")
	}
}
