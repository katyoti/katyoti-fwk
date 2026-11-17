// SPDX-License-Identifier: Apache-2.0

//go:build katyoti_e2e

package e2etest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Two installations side by side: each katyoti finds its katyoti.yml above
// itself and works in its own roots — its universes, its settings — whatever
// the environment it inherits says; a program elsewhere enters one when
// KTW_INSTALLATION names it; both list themselves among the instances.
func TestInstallationsSideBySide(t *testing.T) {
	base := t.TempDir()
	stray := filepath.Join(base, "stray")
	instances := filepath.Join(base, "instances.yaml")
	install := func(name string) (string, string) {
		dir := filepath.Join(base, name)
		exe := filepath.Join(dir, "root", "control-plane", "katyoti"+exeSuffix())
		data, err := os.ReadFile(katyotiBin)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, exe, string(data))
		writeFile(t, filepath.Join(dir, "katyoti.yml"), "name: "+name+"\nroots:\n  config: root\n  data: data\n  cache: cache\n")
		return dir, exe
	}
	aDir, a := install("alpha-1.0")
	bDir, b := install("beta-2.0")

	// The environment a person's shell might carry: another installation's roots.
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !strings.HasPrefix(strings.ToUpper(k), "KTW_") && strings.ToUpper(k) != "XDG_DATA_HOME" {
			env = append(env, kv)
		}
	}
	env = append(env, "KTW_KATYOTI_ROOT="+stray, "KTW_DATA="+filepath.Join(stray, "data"), "KTW_CACHE="+filepath.Join(stray, "cache"),
		"KTW_INSTANCES="+instances)
	run := func(bin string, extra []string, args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = append(append([]string{}, env...), extra...)
		cmd.Dir = base
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", bin, args, err, out)
		}
		return string(out)
	}

	run(a, nil, "universe", "create", "uni-a")
	run(b, nil, "universe", "create", "uni-b")
	if l := run(a, nil, "universe", "list"); !strings.Contains(l, "uni-a") || strings.Contains(l, "uni-b") {
		t.Fatalf("installation a should see its universe only:\n%s", l)
	}
	if l := run(b, nil, "universe", "list"); !strings.Contains(l, "uni-b") || strings.Contains(l, "uni-a") {
		t.Fatalf("installation b should see its universe only:\n%s", l)
	}
	for _, p := range []string{filepath.Join(aDir, "root", "universes.yaml"), filepath.Join(bDir, "root", "universes.yaml")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("each installation keeps its registry in its own root: %v", err)
		}
	}
	if _, err := os.Stat(stray); err == nil {
		t.Fatal("an inherited root was written: the installation's file should win")
	}

	// Settings live in the installation's file.
	run(a, nil, "config", "set", "launch_mode", "cli")
	ya, _ := os.ReadFile(filepath.Join(aDir, "katyoti.yml"))
	yb, _ := os.ReadFile(filepath.Join(bDir, "katyoti.yml"))
	if !strings.Contains(string(ya), "launch_mode: cli") || strings.Contains(string(yb), "launch_mode") {
		t.Fatalf("a's setting should be in a's katyoti.yml only:\na:\n%s\nb:\n%s", ya, yb)
	}
	if !strings.Contains(string(ya), "name: alpha-1.0") {
		t.Fatalf("rewriting the file keeps the installation's name:\n%s", ya)
	}

	// A program outside both enters b when KTW_INSTALLATION names it.
	if l := run(katyotiBin, []string{"KTW_INSTALLATION=" + filepath.Join(bDir, "katyoti.yml")}, "universe", "list"); !strings.Contains(l, "uni-b") {
		t.Fatalf("KTW_INSTALLATION should enter installation b:\n%s", l)
	}

	// Each lists itself among the instances.
	if l := run(a, nil, "instance", "list"); !strings.Contains(l, aDir) || !strings.Contains(l, bDir) {
		t.Fatalf("both installations should be listed:\n%s", l)
	}
}

// In an installation, a project folder inside it (its data root) is refused
// before anything is written - an uninstall or a reinstall would take it - and
// the refusal names the projects folder; one in the projects folder attaches.
func TestAProjectLivesInTheProjectsFolder(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "gamma-1.0")
	exe := filepath.Join(dir, "root", "control-plane", "katyoti"+exeSuffix())
	data, err := os.ReadFile(katyotiBin)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, exe, string(data))
	projects := filepath.Join(base, "projects")
	writeFile(t, filepath.Join(dir, "katyoti.yml"), "name: gamma-1.0\nroots:\n  config: root\n  data: data\n  cache: cache\nsettings:\n  projects_folder: "+filepath.ToSlash(projects)+"\n")

	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !strings.HasPrefix(strings.ToUpper(k), "KTW_") {
			env = append(env, kv)
		}
	}
	env = append(env, "KTW_INSTANCES="+filepath.Join(base, "instances.yaml"), "USERPROFILE="+base, "HOME="+base)
	run := func(wantErr bool, args ...string) string {
		t.Helper()
		cmd := exec.Command(exe, args...)
		cmd.Env, cmd.Dir = env, base
		out, err := cmd.CombinedOutput()
		if (err != nil) != wantErr {
			t.Fatalf("katyoti %v: %v (want error %v)\n%s", args, err, wantErr, out)
		}
		return string(out)
	}

	run(false, "universe", "create", "uni-g")
	inside := filepath.Join(dir, "data", "uni-g-project")
	out := run(true, "project", "attach", inside, "--universe", "uni-g")
	if !strings.Contains(out, "inside") || !strings.Contains(out, projects) {
		t.Fatalf("a project inside the installation's data should be refused, naming the projects folder:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(inside, ".katyoti")); err == nil {
		t.Fatal("the refused project was written")
	}
	mine := filepath.Join(projects, "uni-g")
	run(false, "project", "attach", mine, "--universe", "uni-g")
	if _, err := os.Stat(filepath.Join(mine, ".katyoti", "project.yaml")); err != nil {
		t.Fatalf("a project in the projects folder should attach: %v", err)
	}
}
