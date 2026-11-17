// SPDX-License-Identifier: Apache-2.0

package seed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A project packs its objects, documents and files, and the universe its
// tasks; the project's attachment, repository and packages are left out.
func TestASeedCarriesWhatAPersonAuthored(t *testing.T) {
	base := t.TempDir()
	proj, tasks, dst := filepath.Join(base, "project"), filepath.Join(base, "tasks"), filepath.Join(base, "seed")
	for _, rel := range []string{"README.md", "modules/todo/objects/todo.cwm.yaml", "modules/todo/concepts.md", "src/index.ts", "data/todo.json",
		".katyoti/config/workspace.yaml", ".git/HEAD", "node_modules/x/index.js", "modules/todo/node_modules/y.js"} {
		write(t, proj, rel, "x")
	}
	write(t, tasks, "welcome.md", "---\nname: welcome\n---\n")
	n, err := Stage(proj, tasks, dst)
	if err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Errorf("staged %d files, want 6 (5 of the project, 1 task)", n)
	}
	for rel, want := range map[string]bool{
		"seed.yaml": true, "project/README.md": true, "project/modules/todo/objects/todo.cwm.yaml": true,
		"project/data/todo.json": true, "universe/tasks/welcome.md": true,
		"project/.katyoti": false, "project/.git": false, "project/node_modules": false, "project/modules/todo/node_modules": false,
	} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); (err == nil) != want {
			t.Errorf("%s in the seed: %v, want %v", rel, err == nil, want)
		}
	}
	if err := Check(dst); err != nil {
		t.Errorf("a staged seed fails its own check: %v", err)
	}
}

// A secret anywhere in the project refuses the seed: it is never packed.
func TestASeedNeverPacksASecret(t *testing.T) {
	for _, rel := range []string{".env", "modules/x/.env.local", "src/.ENV"} {
		base := t.TempDir()
		proj := filepath.Join(base, "project")
		write(t, proj, "README.md", "x")
		write(t, proj, rel, "KEY=secret")
		if _, err := Stage(proj, "", filepath.Join(base, "seed")); err == nil || !strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: %v, want refused as a secret", rel, err)
		}
	}
}

// A published seed that carries what no seed may — a secret, a left-out
// folder, something outside its parts — or has no marker is refused at
// install.
func TestAPublishedSeedIsCheckedAtInstall(t *testing.T) {
	for name, rel := range map[string]string{
		"secret":       "project/.env",
		"attachment":   "project/.katyoti/config/workspace.yaml",
		"repository":   "project/.git/HEAD",
		"outside":      "universe/config/LLMProfiles.yaml",
		"at the root":  "notes.txt",
		"tasks secret": "universe/tasks/.env",
	} {
		root := t.TempDir()
		write(t, root, Marker, markerBody)
		write(t, root, "project/README.md", "x")
		write(t, root, rel, "x")
		if err := Check(root); err == nil {
			t.Errorf("%s (%s): accepted, want refused", name, rel)
		}
	}
	root := t.TempDir()
	write(t, root, "project/README.md", "x")
	if err := Check(root); err == nil {
		t.Error("a seed without its marker: accepted, want refused")
	}
}

// A seed lays its project into an empty folder; a folder that already holds
// files is kept as it is, and a task already in the universe is kept.
func TestASeedNeverOverwritesAProject(t *testing.T) {
	root := t.TempDir()
	write(t, root, Marker, markerBody)
	write(t, root, "project/README.md", "from the seed")
	write(t, root, "universe/tasks/welcome.md", "from the seed")
	write(t, root, "universe/tasks/tour.md", "from the seed")

	fresh, tasks := filepath.Join(t.TempDir(), "demo-project"), t.TempDir()
	write(t, tasks, "welcome.md", "the person's")
	a, err := Apply(root, fresh, tasks)
	if err != nil || a.ProjectKept || a.Tasks != 1 {
		t.Fatalf("into a fresh folder: %+v, %v; want the project laid out and one task written", a, err)
	}
	if b, _ := os.ReadFile(filepath.Join(fresh, "README.md")); string(b) != "from the seed" {
		t.Errorf("the project was not laid out: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(tasks, "welcome.md")); string(b) != "the person's" {
		t.Errorf("a task already there was overwritten: %q", b)
	}

	mine := t.TempDir()
	write(t, mine, "README.md", "the person's")
	a, err = Apply(root, mine, tasks)
	if err != nil || !a.ProjectKept {
		t.Fatalf("into a folder with files: %+v, %v; want it kept", a, err)
	}
	if b, _ := os.ReadFile(filepath.Join(mine, "README.md")); string(b) != "the person's" {
		t.Errorf("the person's project was overwritten: %q", b)
	}
}

// Applying a seed checks it first: one that carries a secret lays nothing out.
func TestASeedIsCheckedBeforeItIsLaidOut(t *testing.T) {
	root := t.TempDir()
	write(t, root, Marker, markerBody)
	write(t, root, "project/README.md", "x")
	write(t, root, "project/.env", "KEY=secret")
	proj := filepath.Join(t.TempDir(), "p")
	if _, err := Apply(root, proj, t.TempDir()); err == nil {
		t.Fatal("a seed holding a secret was applied")
	}
	if _, err := os.Stat(proj); err == nil {
		t.Error("a refused seed still laid its project out")
	}
}

// A laid-out project gets its own identity: the seed's project object keeps
// everything but its id (the folder's name now) and its uuid (fresh); two
// projects applied from one seed never share a uuid. An object of another
// shape is removed (CWM mints a new one when it opens the project).
func TestAppliedProjectsGetTheirOwnIdentity(t *testing.T) {
	src := "# the project object\nid: demo\ntype: project\nname: The demo\ncapabilities:\n  project:\n    uuid: 01999999-aaaa-7bbb-8ccc-dddddddddddd\n    modules_dir: modules\n"
	uuids := map[string]bool{}
	for _, folder := range []string{"alpha-project", "beta-project"} {
		dir := filepath.Join(t.TempDir(), folder)
		write(t, dir, ProjectObject, src)
		id, err := Remint(dir)
		if err != nil || id != folder {
			t.Fatalf("Remint = %q, %v; want %q", id, err, folder)
		}
		got := readString(t, filepath.Join(dir, ProjectObject))
		for _, want := range []string{"id: " + folder + "\n", "name: The demo", "modules_dir: modules", "# the project object"} {
			if !strings.Contains(got, want) {
				t.Errorf("after remint, %q missing:\n%s", want, got)
			}
		}
		if strings.Contains(got, "01999999-aaaa") {
			t.Errorf("the source's uuid survived:\n%s", got)
		}
		uuids[got[strings.Index(got, "uuid: ")+6:][:36]] = true
	}
	if len(uuids) != 2 {
		t.Errorf("two projects from one seed share a uuid")
	}
	odd := t.TempDir()
	write(t, odd, ProjectObject, "id: demo\ntype: project\n")
	if id, err := Remint(odd); err != nil || id != "" {
		t.Fatalf("an object without a uuid = %q, %v; want removed", id, err)
	}
	if _, err := os.Stat(filepath.Join(odd, ProjectObject)); err == nil {
		t.Error("an object of another shape was kept with the source's identity")
	}
	if id, err := Remint(t.TempDir()); err != nil || id != "" {
		t.Errorf("no project object = %q, %v; want nothing", id, err)
	}
}

func readString(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A project published before CWM ever opened it has no project object, only
// the seed of one in its store (its type, its title): the seed carries that
// seed - the store itself still left out - and laying it out answers it for
// the new project's store, so the install gets the declared type, not the
// default. A project object the project has wins: it is
// re-minted, and no seed of one travels beside it.
func TestASeedCarriesTheProjectObjectsSeed(t *testing.T) {
	base := t.TempDir()
	proj, dst := filepath.Join(base, "project"), filepath.Join(base, "seed")
	write(t, proj, "modules/archi/objects/archi.cwm.yaml", "id: archi\ntype: documentation\n")
	write(t, proj, ".katyoti/project-seed.yaml", "title: ArchiFactory\ntype: dossier-architecture\n")
	if _, err := Stage(proj, "", dst); err != nil {
		t.Fatal(err)
	}
	carried, err := WithProjectSeed(dst, filepath.Join(proj, ".katyoti", "project-seed.yaml"))
	if err != nil || !carried {
		t.Fatalf("WithProjectSeed = %v, %v; want carried", carried, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "project", ".katyoti")); err == nil {
		t.Error("the project's store was packed")
	}
	if err := Check(dst); err != nil {
		t.Fatalf("a seed carrying its project object's seed fails its check: %v", err)
	}
	into := filepath.Join(base, "installed")
	a, err := Apply(dst, into, filepath.Join(base, "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	if a.ProjectSeed == "" || !strings.Contains(readString(t, a.ProjectSeed), "type: dossier-architecture") {
		t.Fatalf("Apply answers the project object's seed %q; want the dossier-architecture one", a.ProjectSeed)
	}
	if _, err := os.Stat(filepath.Join(into, ProjectSeedPart)); err == nil {
		t.Error("the project object's seed was laid out in the project's folder, not left for its store")
	}

	// A project with its project object: no seed of one travels.
	withObject, dst2 := filepath.Join(base, "with-object"), filepath.Join(base, "seed2")
	write(t, withObject, ProjectObject, "id: p\ntype: dossier-architecture\ncapabilities:\n  project:\n    uuid: 01999999-aaaa-7bbb-8ccc-dddddddddddd\n")
	write(t, withObject, ".katyoti/project-seed.yaml", "type: software-project\n")
	if _, err := Stage(withObject, "", dst2); err != nil {
		t.Fatal(err)
	}
	if carried, err := WithProjectSeed(dst2, filepath.Join(withObject, ".katyoti", "project-seed.yaml")); err != nil || carried {
		t.Errorf("WithProjectSeed beside a project object = %v, %v; want none carried", carried, err)
	}
	// No seed of one in the store: nothing to carry.
	if carried, err := WithProjectSeed(dst2, filepath.Join(base, "none.yaml")); err != nil || carried {
		t.Errorf("WithProjectSeed with no file = %v, %v; want none", carried, err)
	}
	// A folder by that name is no seed of a project object.
	odd := filepath.Join(base, "odd")
	if _, err := Stage(proj, "", odd); err != nil {
		t.Fatal(err)
	}
	write(t, odd, ProjectSeedPart+"/x.yaml", "x")
	if err := Check(odd); err == nil {
		t.Error("a folder named after the project object's seed passed the check")
	}
}
