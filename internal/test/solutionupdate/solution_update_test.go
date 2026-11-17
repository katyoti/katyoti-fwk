// SPDX-License-Identifier: Apache-2.0

//go:build solution_update

// Package solutionupdate proves `katyoti solution update` end to end
// (serving), closed in a folder of its own like
// fresh-install: a marketplace authored with the publisher verbs offers the
// solution demo in three versions -
//
//	v1  its ktw-agents knows data version 1 (built from before the first
//	    migration step),
//	v2  its ktw-agents carries the steps to the data version this tree writes (5),
//	v3  its ktw-agents carries one more step that always fails (this tree,
//	    patched in a copy),
//
// the installer puts v1 in a world of its own, the update moves it to v2 (the
// project's data migrated, its objects kept, a backup taken), and an update to
// v3 fails at the migration and rolls back to v2 with the data unchanged.
// Nothing is written outside the world.
//
// `make solution-update`; never part of `go test ./...` (minutes: it builds the
// binaries).
package solutionupdate

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// beforeFirstStep is the last commit whose ktw-agents knows data version 1:
// the parent of commit, which shipped the first step.
const beforeFirstStep = "309285616^"

const module = "github.com/katyoti/katyoti-fwk"

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func TestASolutionUpdatesAndRollsBack(t *testing.T) {
	work := t.TempDir()
	if root := os.Getenv("KTW_TEST_ROOT"); root != "" {
		work = filepath.Join(root, "solution-update")
		_ = os.RemoveAll(work)
	}
	plat := runtime.GOOS + "-" + runtime.GOARCH
	repo := repoRoot(t)
	tools := filepath.Join(work, "tools")

	// 1. The builds: the control plane from this tree, and ktw-agents three
	//    times - v1 from before the first step, v2 from this tree, v3 from a
	//    copy of this tree with a step that fails.
	dist := func(v string) string { return filepath.Join(work, "dist-"+v) }
	for _, name := range []string{"katyoti", "ktw-hub", "ktw-univers"} {
		build(t, repo, name, "v1.0.0", filepath.Join(dist("v1"), plat))
	}
	run(t, repo, nil, "go", "build", "-o", filepath.Join(tools, exe("ktw-compose")), "./app/ktw-compose")
	compose := filepath.Join(tools, exe("ktw-compose"))

	old := extract(t, repo, beforeFirstStep, filepath.Join(work, "src-v1"))
	build(t, old, "ktw-agents", "v1.0.0", filepath.Join(dist("v1"), plat))
	build(t, repo, "ktw-agents", "v1.1.0", filepath.Join(dist("v2"), plat))
	broken := extract(t, repo, "HEAD", filepath.Join(work, "src-v3"))
	failingStep(t, broken)
	build(t, broken, "ktw-agents", "v1.2.0", filepath.Join(dist("v3"), plat))

	// 2. The marketplace: the three releases, and the solution demo at each,
	//    its member the very bytes released (pinned by content), its project a
	//    CWM project with objects.
	market := filepath.Join(work, "market")
	keys := filepath.Join(work, "keys")
	key := []string{"--key-folder", keys, "--key-prefix", "demo"}
	run(t, work, nil, compose, "init-marketplace", market, "--name", "demo-market", "--key-folder", keys, "--key-prefix", "demo")
	project := filepath.Join(work, "demo-project")
	write(t, filepath.Join(project, "project.cwm.yaml"), "id: demo\ntype: project\nname: Démo\n")
	write(t, filepath.Join(project, "modules", "demo", "objects", "demo.cwm.yaml"), "id: demo\ntype: module\nname: Démo\n")
	write(t, filepath.Join(project, "modules", "demo", "features", "F-001.cwm.yaml"), "id: demo.F-001\ntype: feature\nname: Une fonctionnalité\nparent: demo\n")
	for _, v := range []string{"v1", "v2", "v3"} {
		release(t, work, compose, dist(v), market)
		uni := filepath.Join(work, "demo-src-"+v)
		write(t, filepath.Join(uni, ".katyoti", "config", "Univers.yaml"), "name: demo-src\ndisplay_name: Démo\nsidecars:\n  - name: ktw-agents\n    transport: nats\n    binary: "+
			filepath.ToSlash(filepath.Join(dist(v), plat, exe("ktw-agents")))+"\n    args: [sc, ktsp, nats]\n")
		run(t, work, nil, compose, append([]string{"snapshot-universe", uni, "--version", v, "--marketplace", market, "--name", "demo", "--project", project}, key...)...)
	}
	run(t, work, nil, compose, "sign-marketplace", market, "--key-folder", keys, "--key-prefix", "demo")
	run(t, work, nil, compose, append([]string{"build-infra-channel", dist("v1"), "--output", market, "--version", "v1", "--demo", "demo@v1"}, key...)...)
	run(t, work, nil, compose, append([]string{"publish-marketplace", market}, key...)...)

	// 3. A world of its own, and the installer run as a person runs it.
	world := filepath.Join(work, "machine")
	home, install := filepath.Join(world, "home"), filepath.Join(world, "install")
	_ = os.MkdirAll(home, 0o755)
	env := []string{"KTW_INSTALLATION_DIR=" + install, "USERPROFILE=" + home, "HOME=" + home,
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"), "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"KTW_SHORTCUT_DIR=" + filepath.Join(world, "shortcuts"), "KTW_SETUP_ARGS=--no-gui"}
	hostReg := hostRegistry()
	before := modTime(hostReg)
	if runtime.GOOS == "windows" {
		run(t, work, env, "cmd", "/c", filepath.Join(market, "infra", "install-on-windows.cmd"))
	} else {
		run(t, work, env, "sh", filepath.Join(market, "infra", "install.sh"))
	}
	katyoti := filepath.Join(install, "root", "control-plane", exe("katyoti"))
	universe := filepath.Join(install, "root", "universes", "demo")
	// The universe's hub is started detached by the update: stopped at the
	// end, in the world it runs in.
	t.Cleanup(func() { _, _ = runErr(work, env, katyoti, "universe", "stop", "demo") })
	installedProject := filepath.Join(home, "Documents", "Katyoti", "install")
	feature := filepath.Join(installedProject, "modules", "demo", "features", "F-001.cwm.yaml")
	if _, err := os.Stat(feature); err != nil {
		t.Fatalf("the installed project should hold the solution's objects: %v", err)
	}
	if v := dataVersion(t, installedProject); v != "" {
		t.Fatalf("v1's project should carry no data version (version 1), got %q", v)
	}
	if !pins(t, universe, "v1.0.0") {
		t.Fatalf("the installed universe should pin ktw-agents v1.0.0:\n%s", readFile(t, univers(universe)))
	}

	// 4. The update to v2: the data migrated, the objects kept, a backup.
	out := run(t, work, env, katyoti, "solution", "update", "--universe", "demo", "--to", "v2")
	if !strings.Contains(out, "demo is at v2") {
		t.Fatalf("the update to v2 should end at v2:\n%s", out)
	}
	if !pins(t, universe, "v1.1.0") || !strings.Contains(readFile(t, univers(universe)), "solution: demo@v2") {
		t.Fatalf("after the update the universe should pin ktw-agents v1.1.0 and record demo@v2:\n%s", readFile(t, univers(universe)))
	}
	if v := dataVersion(t, installedProject); v != "5" {
		t.Fatalf("the project's data should be migrated to version 5, data_version = %q\n%s", v, out)
	}
	if _, err := os.Stat(feature); err != nil {
		t.Fatalf("the update lost an object: %v", err)
	}
	if backups, _ := filepath.Glob(filepath.Join(install, "data", "katyoti", "universes", "demo", "backups", "update-*")); len(backups) == 0 {
		t.Fatalf("the update should back the project's data up under the universe's data folder:\n%s", out)
	}
	afterV2 := tree(t, filepath.Join(installedProject, "modules"))
	projectV2 := readFile(t, filepath.Join(installedProject, "project.cwm.yaml"))

	// 5. The update to v3 fails at its migration and rolls back to v2.
	out, err := runErr(work, env, katyoti, "solution", "update", "--universe", "demo", "--to", "v3")
	if err == nil {
		t.Fatalf("the update to v3 should fail (its migration fails):\n%s", out)
	}
	if !strings.Contains(out, "rolled back to v2") {
		t.Fatalf("the failed update should roll back to v2:\n%s", out)
	}
	if !pins(t, universe, "v1.1.0") || !strings.Contains(readFile(t, univers(universe)), "solution: demo@v2") {
		t.Fatalf("after the rollback the universe should pin v1.1.0 and record demo@v2 again:\n%s", readFile(t, univers(universe)))
	}
	if got := tree(t, filepath.Join(installedProject, "modules")); !sameTree(afterV2, got) || readFile(t, filepath.Join(installedProject, "project.cwm.yaml")) != projectV2 {
		t.Fatal("the rollback did not leave the project's data as it was at v2")
	}

	// 6. Nothing written outside the world.
	if after := modTime(hostReg); !after.Equal(before) {
		t.Fatalf("the installation this ran from had its registry %s rewritten", hostReg)
	}
}

// release publishes the built tree dist into market and checks it published
// its ktw-agents. Release asks each binary to describe itself; a binary built a
// moment ago can be held by the antivirus's scan and answer nothing, and the
// item is then skipped - so it is asked again, a few times.
func release(t *testing.T, work, compose, dist, market string) {
	t.Helper()
	var out string
	for attempt := 0; attempt < 6; attempt++ {
		out = run(t, work, nil, compose, "release-marketplace", dist, "--output", market, "--name", "demo-market", "--out", "json")
		if strings.Contains(out, `"published": 1`) || strings.Contains(out, `"published": 2`) {
			return
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("release-marketplace %s never published its ktw-agents:\n%s", dist, out)
}

// build builds app from the tree src into dir, stamped with version.
func build(t *testing.T, src, app, version, dir string) {
	t.Helper()
	run(t, src, nil, "go", "build", "-ldflags", "-X "+module+"/app/"+app+"/cmd.version="+version,
		"-o", filepath.Join(dir, exe(app)), "./app/"+app)
}

// extract writes the tree of repo at commit into dst, from git.
func extract(t *testing.T, repo, commit, dst string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "archive", "--format=tar", commit)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("git archive %s: %v %s", commit, err, stderr.String())
	}
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		p := filepath.Join(dst, filepath.FromSlash(h.Name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

// failingStep adds, in the copy at src, a migration step that always fails.
func failingStep(t *testing.T, src string) {
	t.Helper()
	p := filepath.Join(src, "internal", "extensions", "cwm", "internal", "core", "migrate_failing_step.go")
	write(t, p, `package core

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A step that always fails: the solution-update test's broken release.
func init() {
	migrationSteps = append(migrationSteps, MigrationStep{Title: "a step that fails (solution-update test)",
		Object: func(*api.Object) (bool, error) { return false, fmt.Errorf("forced migration failure") }})
}
`)
}

func univers(home string) string { return filepath.Join(home, ".katyoti", "config", "Univers.yaml") }

// pins says whether the universe at home runs ktw-agents at version.
func pins(t *testing.T, home, version string) bool {
	t.Helper()
	return strings.Contains(filepath.ToSlash(readFile(t, univers(home))), "/ktw-agents/"+version+"/")
}

// dataVersion is the project's recorded data version ("" when none, or no
// project object yet: CWM mints it the first time it opens the project).
func dataVersion(t *testing.T, project string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(project, "project.cwm.yaml"))
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "data_version:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(root, p)
			out[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	return out
}

func sameTree(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// hostRegistry is the universe registry of the installation running the test.
func hostRegistry() string {
	root := os.Getenv("KTW_KATYOTI_ROOT")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, ".config", "katyoti")
	}
	return filepath.Join(root, "universes.yaml")
}

func modTime(p string) time.Time {
	st, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

func run(t *testing.T, dir string, env []string, prog string, args ...string) string {
	t.Helper()
	out, err := runErr(dir, env, prog, args...)
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", filepath.Base(prog), args, err, out)
	}
	return out
}

func runErr(dir string, env []string, prog string, args ...string) (string, error) {
	cmd := exec.Command(prog, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(cleanEnv(), env...)
	}
	b, err := cmd.CombinedOutput()
	return string(b), err
}

// cleanEnv is this process's environment without an installation's locations.
func cleanEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "KTW_KATYOTI_ROOT", "KTW_DATA", "KTW_CACHE", "XDG_DATA_HOME", "KTW_HOME", "KTW_WORKSPACE", "KTW_UNIVERSE", "KTW_INFRA_CHANNEL",
			"KTW_INSTALLATION", "KTW_INSTALLATION_DIR", "KTW_INSTANCES", "KTW_SHORTCUT_DIR", "USERPROFILE", "HOME", "LOCALAPPDATA", "APPDATA",
			"KTW_HUB_BINARY", "KTW_NATS_URL", "KTW_DOTENV_KEYS":
			continue
		}
		out = append(out, kv)
	}
	return out
}

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	wd, _ := os.Getwd()
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			t.Fatal("no go.mod above the test")
		}
	}
}
