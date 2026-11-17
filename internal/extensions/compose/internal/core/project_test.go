// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	pcapi "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
)

// fakeCWM is one project's CWM in memory: its objects by module.local id,
// its module files by module/path.
type fakeCWM struct {
	root    string
	objects map[string]cwmapi.Object
	files   map[string][]byte
}

func newFakeCWM(root string) *fakeCWM {
	return &fakeCWM{root: root, objects: map[string]cwmapi.Object{}, files: map[string][]byte{}}
}

func moduleOf(id string) string { m, _, _ := strings.Cut(id, "."); return m }

func (f *fakeCWM) ObjectGet(_ context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	o, ok := f.objects[in.Module+"."+in.ID]
	if !ok {
		return cwmapi.ObjectGetOutput{}, fmt.Errorf("%s.%s: %w", in.Module, in.ID, cwmapi.ErrNotFound)
	}
	return cwmapi.ObjectGetOutput{Object: o}, nil
}

func (f *fakeCWM) ObjectCreate(_ context.Context, in cwmapi.ObjectCreateInput) (cwmapi.ObjectCreateOutput, error) {
	id := in.Module + "." + in.Object.ID
	if _, ok := f.objects[id]; ok {
		return cwmapi.ObjectCreateOutput{}, cwmapi.ErrAlreadyExists
	}
	o := in.Object
	o.ID = id
	f.objects[id] = o
	return cwmapi.ObjectCreateOutput{Object: o}, nil
}

func (f *fakeCWM) ObjectUpdate(_ context.Context, in cwmapi.ObjectUpdateInput) (cwmapi.ObjectUpdateOutput, error) {
	f.objects[in.Module+"."+strings.TrimPrefix(in.Object.ID, in.Module+".")] = in.Object
	return cwmapi.ObjectUpdateOutput{}, nil
}

func (f *fakeCWM) ObjectDelete(_ context.Context, in cwmapi.ObjectDeleteInput) (cwmapi.ObjectDeleteOutput, error) {
	delete(f.objects, in.Module+"."+in.ID)
	return cwmapi.ObjectDeleteOutput{}, nil
}

func (f *fakeCWM) ObjectResolve(context.Context, cwmapi.ObjectResolveInput) (cwmapi.ObjectResolveOutput, error) {
	return cwmapi.ObjectResolveOutput{ProjectRoot: f.root}, nil
}

func (f *fakeCWM) ObjectFileRead(_ context.Context, in cwmapi.ObjectFileReadInput) (cwmapi.ObjectFileReadOutput, error) {
	b, ok := f.files[moduleOf(in.ID)+"/"+in.Path]
	if !ok {
		return cwmapi.ObjectFileReadOutput{}, cwmapi.ErrNotFound
	}
	return cwmapi.ObjectFileReadOutput{Content: b}, nil
}

func (f *fakeCWM) ObjectFileWrite(_ context.Context, in cwmapi.ObjectFileWriteInput) (cwmapi.ObjectFileWriteOutput, error) {
	f.files[moduleOf(in.ID)+"/"+in.Path] = in.Content
	return cwmapi.ObjectFileWriteOutput{}, nil
}

func (f *fakeCWM) ObjectFileDelete(_ context.Context, in cwmapi.ObjectFileDeleteInput) (cwmapi.ObjectFileDeleteOutput, error) {
	delete(f.files, moduleOf(in.ID)+"/"+in.Path)
	return cwmapi.ObjectFileDeleteOutput{}, nil
}

// fakeEntries is the universe's entries, each layer's content by name.
type fakeEntries map[string]map[string]string

func (f fakeEntries) Entry(_ context.Context, uri string) (contribapi.Contribution, error) {
	layers, ok := f[uri]
	if !ok {
		return contribapi.Contribution{}, fmt.Errorf("no entry %s", uri)
	}
	u, _ := contribapi.ParseResourceURI(uri)
	c := contribapi.Contribution{Provider: u.Provider, Type: u.Type, ID: u.ID, URI: uri, Layers: map[string]contribapi.LayerRef{}}
	for name := range layers {
		c.Layers[name] = contribapi.LayerRef{MediaType: "application/yaml"}
	}
	return c, nil
}

func (f fakeEntries) Layer(_ context.Context, uri, layer string) ([]byte, string, error) {
	return []byte(f[uri][layer]), "application/yaml", nil
}

func (f *fakeCWM) objectFiles() pcapi.ObjectFiles { return f }

// noteKinds declares the made-up kind `note`, read from embed/note/.
func noteKinds() *contribapi.Kinds {
	decl := contribapi.Contribution{Provider: "system", Type: contribapi.ResourceTypeContributionType, ID: "note"}
	decl.SetData(json.RawMessage(`{"name":"note","layers":{"data":{"mediaType":"application/json"}}}`))
	k := contribapi.KindsFrom([]contribapi.Contribution{decl})
	return &k
}

func writeFile(t *testing.T, p, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// create-skill and create-task --target project, with no sidecar: the first
// ensures sys-contributions (a system module operating contributions), both
// write into the folder projectfile names, and the folder read as the
// project's member lists the skill under the project's token.
func TestProjectTarget_CreateSkillAndTask(t *testing.T) {
	root := filepath.Join(t.TempDir(), "acme-project")
	cwm := newFakeCWM(root)
	s, _ := fixtureService(t)
	s.cwmOverride = cwm
	ctx := context.Background()

	out, err := s.CreateSkill(ctx, api.CreateSkillInput{Sidecar: "hello-skill", Target: "project"})
	if err != nil {
		t.Fatal(err)
	}
	mod, ok := cwm.objects["sys-contributions.sys-contributions"]
	if !ok || cwmapi.SystemModuleOperates(mod) != "contributions" {
		t.Fatalf("sys-contributions = %+v (%v); want a system module operating contributions", mod, ok)
	}
	folder := filepath.Join(root, "modules", "sys-contributions")
	if want := filepath.Join(folder, "skills", "hello-skill.md"); out.Path != want {
		t.Fatalf("path = %s; want %s", out.Path, want)
	}
	task, err := s.CreateTask(ctx, api.CreateTaskInput{Sidecar: "hello-task", Target: "project"})
	if err != nil || task.Path != filepath.Join(folder, "tasks", "hello-task.md") {
		t.Fatalf("task = %+v, %v", task, err)
	}
	if _, err := s.CreateSkill(ctx, api.CreateSkillInput{Sidecar: "ktw-x", Name: "other", Target: "project"}); err == nil {
		t.Error("--target project with a sidecar was accepted")
	}

	kinds, err := builtin.Kinds()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range manifestcontrib.PackProviders("acme-project", loader.NewFSLoader(folder), ".", kinds) {
		entries, _ := p.Enumerate(ctx)
		for _, e := range entries {
			if e.Provider == "acme-project" && e.ID == "hello-skill" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("the project's folder member does not list hello-skill under the project token")
	}
}

// create-override --uri: an override object in sys-contributions carrying
// only the layers named, each a file seeded from the entry's layer, of the
// shape EntryOfObject reads back as an override of that URI; an entry of the
// project itself is refused.
func TestCreateOverride_ProjectObject(t *testing.T) {
	cwm := newFakeCWM(filepath.Join(t.TempDir(), "acme-project"))
	s := &Service{cwmOverride: cwm, entriesOverride: fakeEntries{
		"ktw-example:/note/hello":  {"data": "title: Hello\n", "translations": "fr:\n  title: Bonjour\n"},
		"acme-project:/note/local": {"data": "title: Local\n"},
	}}
	ctx := context.Background()
	out, err := s.CreateOverride(ctx, api.CreateOverrideInput{URI: "ktw-example:/note/hello", Layers: []string{"data"}, Target: "project"})
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := cwm.objects[out.Object]
	if !ok || out.Object != "sys-contributions."+pcapi.OverrideObjectID("ktw-example", "note", "hello") {
		t.Fatalf("object %s not created (%v)", out.Object, cwm.objects)
	}
	if len(out.Layers) != 1 || cwm.files["sys-contributions/overrides/ktw-example/note/hello/data.yaml"] == nil {
		t.Fatalf("layers = %v, files = %v; want the data layer only, its file written", out.Layers, cwm.files)
	}
	e, err := pcapi.EntryOfObject(ctx, obj, "acme-project", cwm.objectFiles())
	if err != nil || e.Overrides != "ktw-example:/note/hello" {
		t.Fatalf("read back = %+v, %v", e, err)
	}
	if _, err := s.CreateOverride(ctx, api.CreateOverrideInput{URI: "ktw-example:/note/hello"}); err == nil {
		t.Error("a second override of the same entry was accepted")
	}
	if _, err := s.CreateOverride(ctx, api.CreateOverrideInput{URI: "acme-project:/note/local"}); err == nil {
		t.Error("an override of the project's own entry was accepted")
	}
}

// i18n --scaffold --into project on a sidecar: per entry missing texts, an
// override object carrying only its translations layer, the languages the
// entry already has kept and a ~ stub per missing text added.
func TestI18n_ScaffoldIntoProject(t *testing.T) {
	cwm := newFakeCWM(filepath.Join(t.TempDir(), "acme-project"))
	ot := contribapi.Contribution{Provider: "ktw-x", Type: "object-type", ID: "note"}
	ot.SetData([]byte(`{"name":"note","label":"Note"}`))
	if ot.Layers == nil {
		ot.Layers = map[string]contribapi.LayerRef{}
	}
	ot.Layers[contribapi.LayerTranslations] = contribapi.LayerRef{MediaType: "application/yaml", Inline: []byte("de:\n  label: Notiz\n")}
	s := &Service{cwmOverride: cwm, exeOverride: fakeExes{manifests: map[string]*mbapi.Manifest{"ktw-x": {Provider: "ktw-x", Contributions: []contribapi.Contribution{ot}}}}}
	out, err := s.I18n(context.Background(), api.I18nInput{Target: "ktw-x", Scaffold: true, Into: "project"})
	if err != nil {
		t.Fatal(err)
	}
	want := "sys-contributions." + pcapi.OverrideObjectID("ktw-x", "object-type", "note")
	if len(out.Objects) != 1 || out.Objects[0] != want {
		t.Fatalf("objects = %v; want %s", out.Objects, want)
	}
	c, _, _ := pcapi.ContributesOf(cwm.objects[want])
	if len(c.Layers) != 1 || c.Layers[contribapi.LayerTranslations] == "" {
		t.Fatalf("layers = %v; want the translations layer only", c.Layers)
	}
	file := string(cwm.files["sys-contributions/"+strings.TrimPrefix(c.Layers[contribapi.LayerTranslations], "files/")])
	if !strings.Contains(file, "label: Notiz") || !strings.Contains(file, "label: ~ # Note") {
		t.Fatalf("translations layer:\n%s\nwant de kept and the fr stub", file)
	}
	if _, err := s.I18n(context.Background(), api.I18nInput{Target: "ktw-x", Scaffold: true, Into: "elsewhere"}); err == nil {
		t.Error("an unknown --into was accepted")
	}
}

// promote --object: each layer of a project override object is written into
// the file of app/<sidecar>/embed/ it is read from (a translations layer the
// pack lacks goes beside the entry), then the object and its files are gone;
// a layer read from part of a file is refused, naming it, nothing written.
func TestPromote_OverrideObjectIntoSource(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example\n")
	writeFile(t, filepath.Join(repo, "app", "ktw-example", "embed.go"), "//go:embed embed/note\n")
	writeFile(t, filepath.Join(repo, "app", "ktw-example", "embed", "note", "hello.yaml"), "title: Hello\n")
	cwm := newFakeCWM(filepath.Join(t.TempDir(), "acme-project"))
	s := &Service{repoRoot: repo, cwmOverride: cwm, kindsOverride: noteKinds(), entriesOverride: fakeEntries{
		"ktw-example:/note/hello": {"source": "title: Hello\n", "data": `{"title":"Hello"}`},
	}}
	ctx := context.Background()
	o, err := s.CreateOverride(ctx, api.CreateOverrideInput{URI: "ktw-example:/note/hello", Layers: []string{"data"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(ctx, api.PromoteInput{Object: o.Object}); err == nil || !strings.Contains(err.Error(), "layer data is read from part of embed/note/hello.yaml") {
		t.Fatalf("promote of a derived layer = %v; want refused naming it", err)
	}
	delete(cwm.objects, o.Object)
	delete(cwm.files, "sys-contributions/overrides/ktw-example/note/hello/data.yaml")
	o, err = s.CreateOverride(ctx, api.CreateOverrideInput{URI: "ktw-example:/note/hello", Layers: []string{"source"}})
	if err != nil {
		t.Fatal(err)
	}
	cwm.files["sys-contributions/overrides/ktw-example/note/hello/source.yaml"] = []byte("title: Hello again\n")
	// The translations layer, added to the object as i18n --into project does.
	obj := cwm.objects[o.Object]
	obj.Capabilities[pcapi.CapContributes]["layers"].(map[string]any)["translations"] = "files/overrides/ktw-example/note/hello/translations.yaml"
	cwm.files["sys-contributions/overrides/ktw-example/note/hello/translations.yaml"] = []byte("fr:\n  title: Bonjour\n")

	out, err := s.Promote(ctx, api.PromoteInput{Object: o.Object})
	if err != nil {
		t.Fatal(err)
	}
	embed := filepath.Join(repo, "app", "ktw-example", "embed", "note")
	if b, _ := os.ReadFile(filepath.Join(embed, "hello.yaml")); string(b) != "title: Hello again\n" {
		t.Fatalf("hello.yaml = %q; want the override's data", b)
	}
	if b, _ := os.ReadFile(filepath.Join(embed, "hello.i18n.yaml")); string(b) != "fr:\n  title: Bonjour\n" {
		t.Fatalf("hello.i18n.yaml = %q; want the override's translations", b)
	}
	if len(out.Written) != 2 || out.Removed != o.Object {
		t.Fatalf("out = %+v", out)
	}
	if _, ok := cwm.objects[o.Object]; ok || len(cwm.files) != 0 {
		t.Fatalf("object or files left: %v %v", cwm.objects[o.Object], cwm.files)
	}
	if _, err := s.Promote(ctx, api.PromoteInput{Object: o.Object}); err == nil {
		t.Error("promoting a removed object was accepted")
	}
}

// i18n --scaffold --into project, the translations layer above the inline
// band: its languages are read through the universe and kept in the
// override, never dropped.
func TestI18n_ScaffoldIntoProjectKeepsFileBackedLanguages(t *testing.T) {
	cwm := newFakeCWM(filepath.Join(t.TempDir(), "acme-project"))
	ot := contribapi.Contribution{Provider: "ktw-x", Type: "object-type", ID: "note"}
	ot.SetData([]byte(`{"name":"note","label":"Note"}`))
	if ot.Layers == nil {
		ot.Layers = map[string]contribapi.LayerRef{}
	}
	ot.Layers[contribapi.LayerTranslations] = contribapi.LayerRef{MediaType: "application/yaml", Size: 5000}
	uri := ot.ResolvedURI()
	s := &Service{cwmOverride: cwm,
		exeOverride:     fakeExes{manifests: map[string]*mbapi.Manifest{"ktw-x": {Provider: "ktw-x", Contributions: []contribapi.Contribution{ot}}}},
		entriesOverride: fakeEntries{uri: {contribapi.LayerTranslations: "de:\n  label: Notiz\n"}}}
	out, err := s.I18n(context.Background(), api.I18nInput{Target: "ktw-x", Scaffold: true, Into: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Objects) != 1 {
		t.Fatalf("objects = %v; want one override", out.Objects)
	}
	c, _, _ := pcapi.ContributesOf(cwm.objects[out.Objects[0]])
	file := string(cwm.files["sys-contributions/"+strings.TrimPrefix(c.Layers[contribapi.LayerTranslations], "files/")])
	if !strings.Contains(file, "label: Notiz") {
		t.Fatalf("translations layer:\n%s\nwant the file-backed de kept", file)
	}
}

// failingFileCWM fails ObjectFileWrite for the paths ending with fail.
type failingFileCWM struct {
	*fakeCWM
	fail string
}

func (f failingFileCWM) ObjectFileWrite(ctx context.Context, in cwmapi.ObjectFileWriteInput) (cwmapi.ObjectFileWriteOutput, error) {
	if strings.HasSuffix(in.Path, f.fail) {
		return cwmapi.ObjectFileWriteOutput{}, fmt.Errorf("disk full")
	}
	return f.fakeCWM.ObjectFileWrite(ctx, in)
}

// create-override whose second layer file fails to write: the file written
// before it and the override object are taken back - no object is left
// naming a missing file.
func TestCreateOverride_FailedFileLeavesNoObject(t *testing.T) {
	cwm := newFakeCWM(filepath.Join(t.TempDir(), "acme-project"))
	s := &Service{cwmOverride: failingFileCWM{fakeCWM: cwm, fail: "translations.yaml"}, entriesOverride: fakeEntries{
		"ktw-example:/note/hello":  {"data": "title: Hello\n", "translations": "fr:\n  title: Bonjour\n"},
		"acme-project:/note/local": {"data": "title: Local\n"},
	}}
	_, err := s.CreateOverride(context.Background(), api.CreateOverrideInput{URI: "ktw-example:/note/hello", Layers: []string{"data", "translations"}, Target: "project"})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("create-override with a failing file = %v; want the write error", err)
	}
	for id := range cwm.objects {
		if strings.HasPrefix(id, pcapi.ModuleID+".") && id != pcapi.ModuleID+"."+pcapi.ModuleID {
			if _, has, _ := pcapi.ContributesOf(cwm.objects[id]); has {
				t.Errorf("override object %s left after a failed file write", id)
			}
		}
	}
	for f := range cwm.files {
		if strings.Contains(f, "overrides/") {
			t.Errorf("layer file %s left after a failed file write", f)
		}
	}
}

// promote --object whose second file fails to write: the file written before
// it is restored to its previous content, the file it created is removed,
// and the override object and its files stay.
func TestPromote_FailedWriteRestoresSource(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example\n")
	writeFile(t, filepath.Join(repo, "app", "ktw-example", "embed.go"), "//go:embed embed/note\n")
	writeFile(t, filepath.Join(repo, "app", "ktw-example", "embed", "note", "hello.yaml"), "title: Hello\n")
	cwm := newFakeCWM(filepath.Join(t.TempDir(), "acme-project"))
	s := &Service{repoRoot: repo, cwmOverride: cwm, kindsOverride: noteKinds(), entriesOverride: fakeEntries{
		"ktw-example:/note/hello": {"source": "title: Hello\n"},
	}}
	ctx := context.Background()
	o, err := s.CreateOverride(ctx, api.CreateOverrideInput{URI: "ktw-example:/note/hello", Layers: []string{"source"}})
	if err != nil {
		t.Fatal(err)
	}
	cwm.files["sys-contributions/overrides/ktw-example/note/hello/source.yaml"] = []byte("title: Hello again\n")
	obj := cwm.objects[o.Object]
	obj.Capabilities[pcapi.CapContributes]["layers"].(map[string]any)["translations"] = "files/overrides/ktw-example/note/hello/translations.yaml"
	cwm.files["sys-contributions/overrides/ktw-example/note/hello/translations.yaml"] = []byte("fr:\n  title: Bonjour\n")
	embed := filepath.Join(repo, "app", "ktw-example", "embed", "note")
	// Layers are written in name order: source (hello.yaml) then
	// translations (hello.i18n.yaml, a new file) - the second fails after
	// creating its file.
	keep := promoteWriteFile
	t.Cleanup(func() { promoteWriteFile = keep })
	promoteWriteFile = func(name string, data []byte, perm os.FileMode) error {
		if strings.HasSuffix(name, "hello.i18n.yaml") {
			_ = os.WriteFile(name, []byte("half"), perm)
			return fmt.Errorf("disk full")
		}
		return os.WriteFile(name, data, perm)
	}
	if _, err := s.Promote(ctx, api.PromoteInput{Object: o.Object}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("promote with a failing write = %v; want the write error", err)
	}
	if b, _ := os.ReadFile(filepath.Join(embed, "hello.yaml")); string(b) != "title: Hello\n" {
		t.Errorf("hello.yaml = %q; want its previous content restored", b)
	}
	if _, err := os.Stat(filepath.Join(embed, "hello.i18n.yaml")); !os.IsNotExist(err) {
		t.Errorf("hello.i18n.yaml left after a failed promote (%v)", err)
	}
	if _, ok := cwm.objects[o.Object]; !ok || len(cwm.files) != 2 {
		t.Fatalf("override object or its files gone after a failed promote: %v %v", cwm.objects[o.Object], cwm.files)
	}
}
