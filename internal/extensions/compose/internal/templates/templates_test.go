// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// templatesDir resolves the canonical templates shipped in the repo.
// Tests run from this package's directory; the repo root is 4 up.
func templatesDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "app", "ktw-compose", "embed", "templates")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("canonical templates dir not found: %v", err)
	}
	return dir
}

func TestRender_Strict(t *testing.T) {
	out, err := Render("hello {{name}}!", map[string]string{"name": "world"})
	if err != nil || out != "hello world!" {
		t.Fatalf("got %q, %v", out, err)
	}
	if _, err := Render("hello {{name}} {{name}} {{other}}", map[string]string{"other": "x"}); err == nil ||
		!strings.Contains(err.Error(), "name") {
		t.Fatalf("expected unresolved-placeholder error naming 'name', got %v", err)
	}
	// Unused vars are fine.
	if _, err := Render("static", map[string]string{"unused": "y"}); err != nil {
		t.Fatalf("unused var must not error: %v", err)
	}
}

// TestEveryTemplateRenders walks every shipped .tmpl and renders it with
// a sample var map — a template that grows a new placeholder without the
// sample map knowing it fails here (drift guard, audit F5).
func TestEveryTemplateRenders(t *testing.T) {
	sample := map[string]string{
		"name":             "ktw-zztest",
		"includes":         "embed/skills",
		"short":            "test sidecar",
		"long":             "test sidecar long description",
		"ktsp_description": "test",
		"options":          "",
		"description":      "A test artifact",
		"hashtags":         "test",
		"category":         "guide",
		"advertised":       "false",
		"body":             "Body text.",
		"type":             "agent",
		"config":           "  plugins:\n    - workspace",
		"bind_mode":        "global",
		"types":            "'*'",
		"skills":           "",
		"title":            "A reference",
	}
	dir := templatesDir(t)
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return err
		}
		count++
		raw, rErr := os.ReadFile(path)
		if rErr != nil {
			return rErr
		}
		if _, rErr := Render(string(raw), sample); rErr != nil {
			t.Errorf("%s: %v", path, rErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count < 9 {
		t.Fatalf("expected >=9 templates, found %d", count)
	}
}

// TestRenderedSidecarBuilds renders the sidecar templates into a
// throwaway app/ folder INSIDE the module (imports resolve only there)
// and `go build`s it. The drift guard with teeth: a template that no
// longer matches the live sidecarkit API fails compilation here.
func TestRenderedSidecarBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: renders + builds a sidecar")
	}
	dir := templatesDir(t)
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(dir)))) // repo root
	// Underscore-prefixed on purpose: `./...` patterns and the corpus walkers
	// skip `_` dirs, so this throwaway never races a test that walks app/.
	name := "_ktw-zztmpltest"
	target := filepath.Join(root, "app", name)
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(target)

	fsys := os.DirFS(dir)
	vars := map[string]string{
		"name":             name,
		"includes":         "embed/skills",
		"short":            "template integration test sidecar",
		"long":             "Throwaway sidecar rendered by the templates integration test.",
		"ktsp_description": "template test",
		"options":          "",
	}
	for tmpl, out := range map[string]string{
		"sidecar/main.go.tmpl":          filepath.Join(target, "main.go"),
		"sidecar/embed.go.tmpl":         filepath.Join(target, "embed.go"),
		"sidecar/cmd/root.go.tmpl":      filepath.Join(target, "cmd", "root.go"),
		"sidecar/cmd/bootstrap.go.tmpl": filepath.Join(target, "cmd", "bootstrap.go"),
	} {
		if err := RenderFSToFile(fsys, tmpl, vars, out); err != nil {
			t.Fatalf("render %s: %v", tmpl, err)
		}
	}
	// The embed directive needs the folder to exist with one file.
	skillsDir := filepath.Join(target, "embed", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skill := "---\nname: zz-noop\ndescription: integration-test noop skill. Load never.\nadvertised: false\n---\n\nNoop.\n"
	if err := os.WriteFile(filepath.Join(skillsDir, "zz-noop.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "build", "./app/"+name)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rendered sidecar does not build:\n%s", out)
	}
}
