// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/app/ktw-sync/cmd"
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
)

// TestToolRecipes: `ktw-sync describe --out json`, run in process on the
// extensions the binary ships, carries each program it needs — required or
// optional as declared — with its install recipes on the platforms declared,
// windows, darwin and linux for every required one, and the declaration
// validates.
func TestToolRecipes(t *testing.T) {
	census.Isolate(t)
	app := application.New(application.Config(), cmd.Extensions(embeddedFS)...)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	t.Cleanup(func() { appcore.CloseApplicationScope(application.WithApplication(context.Background(), app)) })

	tools := describeTools(t, app.Context())
	all := publish.Platforms
	want := map[string]program{
		"rclone": {required: true, platforms: all},
	}
	assertToolRecipes(t, tools, want)
}

// describeTools runs `describe --out json` in process and returns its programs.
func describeTools(t *testing.T, ctx context.Context) []publish.ToolRequirement {
	t.Helper()
	c := appcore.NewDescribeCommand(appcore.AppConfig{AppName: "ktw-sync", Version: "test"})
	c.SetContext(ctx)
	c.SetArgs([]string{"--out", "json"})
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// Drain the pipe while describe writes: its JSON outgrows the pipe buffer.
	var buf bytes.Buffer
	drained := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, r); close(drained) }()
	stdout := os.Stdout
	os.Stdout = w
	runErr := c.Execute()
	os.Stdout = stdout
	_ = w.Close()
	<-drained
	if runErr != nil {
		t.Fatalf("describe: %v", runErr)
	}
	var d appcore.SidecarDescription
	if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
		t.Fatalf("describe --out json is not a description: %v\n%s", err, buf.String())
	}
	return d.Tools
}

// program is what a described program must carry: required or optional, and
// the platforms it has an install recipe for.
type program struct {
	required  bool
	platforms []string
}

// assertToolRecipes checks the described programs against want: the same set,
// the same required flags, a recipe on exactly the expected platforms (all of
// them for a required program), the prose fallback kept, and a declaration an
// installer engine can run.
func assertToolRecipes(t *testing.T, tools []publish.ToolRequirement, want map[string]program) {
	t.Helper()
	got := map[string]publish.ToolRequirement{}
	for _, tr := range tools {
		got[tr.Name] = tr
	}
	if len(got) != len(want) {
		t.Errorf("describe carries %d programs, want %d: %+v", len(got), len(want), tools)
	}
	for name, w := range want {
		tr, ok := got[name]
		if !ok {
			t.Errorf("describe carries no program %q", name)
			continue
		}
		if tr.Required != w.required {
			t.Errorf("%s: required=%v, want %v", name, tr.Required, w.required)
		}
		if tr.InstallSkill == "" {
			t.Errorf("%s: no install skill, the prose fallback", name)
		}
		def := publish.ToolDef{Name: tr.Name, Recipes: tr.Recipes}
		if err := def.Validate(); err != nil {
			t.Errorf("%s: declaration does not validate: %v", name, err)
		}
		if w.required && len(w.platforms) != len(publish.Platforms) {
			t.Fatalf("%s: a required program needs a recipe on every platform; the expectation lists %v", name, w.platforms)
		}
		for _, p := range publish.Platforms {
			expected := false
			for _, e := range w.platforms {
				expected = expected || e == p
			}
			if has := len(def.RecipesFor(p)) > 0; has != expected {
				t.Errorf("%s: install recipe for %s = %v, want %v", name, p, has, expected)
			}
		}
	}
}
