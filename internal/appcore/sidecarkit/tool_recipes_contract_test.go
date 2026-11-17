// SPDX-License-Identifier: Apache-2.0

package sidecarkit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

type recipeFixtureSvc interface{ Ping(context.Context) error }
type recipeFixtureImpl struct{}

func (recipeFixtureImpl) Ping(context.Context) error { return nil }

// The fixture's programs: one required with a recipe per platform, one
// optional. Their commands are never on PATH, so the check reports them missing.
var (
	fixtureRequired = publish.ToolDef{
		Name:         "fixture-render",
		Commands:     []string{"ktw-fixture-render-absent"},
		InstallSkill: "ktw-fixture:/skill/install-render",
		VersionFlag:  "--version",
		Recipes: []publish.InstallRecipe{
			{Platform: publish.PlatformWindows, Kind: publish.RecipeWinget, Package: "Fixture.Render", MinVersion: "3.1"},
			{Platform: publish.PlatformWindows, Kind: publish.RecipeScoop, Package: "render"},
			{Platform: publish.PlatformDarwin, Kind: publish.RecipeBrew, Package: "render"},
			{Platform: publish.PlatformLinux, Kind: publish.RecipeDownload, URL: "https://example.org/render-3.1-linux.tar.gz", SHA256: strings.Repeat("ab", 32)},
			{Platform: publish.PlatformLinux, Kind: publish.RecipeScript, Script: "scripts/install-render.sh"},
		},
	}
	fixtureOptional = publish.ToolDef{
		Name:     "fixture-media",
		Commands: []string{"ktw-fixture-media-absent"},
		Optional: true,
		Recipes: []publish.InstallRecipe{
			{Platform: publish.PlatformWindows, Kind: publish.RecipePip, Package: "fixture-media>=1.0"},
			{Platform: publish.PlatformDarwin, Kind: publish.RecipeBun, Package: "fixture-media"},
			{Platform: publish.PlatformLinux, Kind: publish.RecipeApt, Package: "fixture-media"},
		},
	}
)

// recipeFixtureApp builds and runs a sidecar declaring tools, in process.
func recipeFixtureApp(t *testing.T, tools ...publish.ToolDef) (*application.Application, error) {
	t.Helper()
	t.Setenv("KTW_WORKSPACE", t.TempDir())
	t.Setenv("KTW_DATA", t.TempDir())
	t.Setenv("KTW_CACHE", t.TempDir())
	app := application.New(application.Config(), sidecarkit.NewSidecar(
		appcore.AppConfig{AppName: "ktw-fixture", Version: "1.0.0", EmbeddedFS: fstest.MapFS{}},
		sidecarkit.WithDomain(publish.PublishSimpleService[recipeFixtureSvc](
			func(context.Context) (recipeFixtureSvc, error) { return recipeFixtureImpl{}, nil },
			publish.WithTools(tools...),
		)),
		sidecarkit.WithoutPack(), sidecarkit.WithoutKTSP(), sidecarkit.WithoutHTTPServer(),
	)...)
	err := app.Run()
	if err == nil {
		t.Cleanup(func() { appcore.CloseApplicationScope(application.WithApplication(context.Background(), app)) })
	}
	return app, err
}

// describeJSON runs `describe --out json` in process and decodes what it prints.
func describeJSON(t *testing.T, ctx context.Context) appcore.SidecarDescription {
	t.Helper()
	cmd := appcore.NewDescribeCommand(appcore.AppConfig{AppName: "ktw-fixture", Version: "1.0.0"})
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--out", "json"})
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	runErr := cmd.Execute()
	os.Stdout = stdout
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	if runErr != nil {
		t.Fatalf("describe: %v", runErr)
	}
	var d appcore.SidecarDescription
	if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
		t.Fatalf("describe --out json is not a description: %v\n%s", err, buf.String())
	}
	return d
}

// A sidecar's program declarations — required or optional, and a structured
// install recipe per platform — read back from `describe --out json` and from
// the config check, so an installer engine plans them without knowing the
// sidecar.
func TestToolRecipes_ReadBackFromDescribeAndConfigCheck(t *testing.T) {
	app, err := recipeFixtureApp(t, fixtureRequired, fixtureOptional)
	if err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	ctx := app.Context()

	// describe --out json
	d := describeJSON(t, ctx)
	byName := map[string]publish.ToolRequirement{}
	for _, tr := range d.Tools {
		byName[tr.Name] = tr
	}
	for _, want := range []publish.ToolDef{fixtureRequired, fixtureOptional} {
		got, ok := byName[want.Name]
		if !ok {
			t.Fatalf("describe carries no program %q: %+v", want.Name, d.Tools)
		}
		if got.Required == want.Optional {
			t.Errorf("describe: %s required=%v, declared optional=%v", want.Name, got.Required, want.Optional)
		}
		if !reflect.DeepEqual(got.Recipes, want.Recipes) {
			t.Errorf("describe: %s recipes\n got %+v\nwant %+v", want.Name, got.Recipes, want.Recipes)
		}
		for _, p := range publish.Platforms {
			if len(want.RecipesFor(p)) == 0 {
				continue
			}
			found := false
			for _, r := range got.Recipes {
				found = found || r.Platform == p
			}
			if !found {
				t.Errorf("describe: %s has no recipe for %s", want.Name, p)
			}
		}
	}
	if got := byName[fixtureRequired.Name]; got.InstallSkill != fixtureRequired.InstallSkill || got.VersionFlag != "--version" {
		t.Errorf("describe: the prose fallback and the version check are lost: %+v", got)
	}

	// sc config check
	svc, err := application.UseService[api.SidecarConfigService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	report, err := svc.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deps := map[string]api.ToolDescriptor{}
	for _, td := range report.Deps {
		deps[td.Name] = td
	}
	items := map[string]api.CheckItem{}
	for _, it := range report.Items {
		if it.Category == "program" {
			items[it.Name] = it
		}
	}
	for _, want := range []publish.ToolDef{fixtureRequired, fixtureOptional} {
		td, ok := deps[want.Name]
		if !ok {
			t.Fatalf("config check carries no program %q", want.Name)
		}
		if td.Required == want.Optional {
			t.Errorf("config check: %s required=%v, declared optional=%v", want.Name, td.Required, want.Optional)
		}
		if !reflect.DeepEqual(td.Recipes, want.Recipes) {
			t.Errorf("config check: %s recipes\n got %+v\nwant %+v", want.Name, td.Recipes, want.Recipes)
		}
	}
	// A missing required program is an error; a missing optional one a warning.
	if it := items[fixtureRequired.Name]; it.Status != "missing" || it.Severity != "error" {
		t.Errorf("required program missing: status %q severity %q, want missing / error", it.Status, it.Severity)
	}
	if it := items[fixtureOptional.Name]; it.Status != "missing" || it.Severity != "warn" {
		t.Errorf("optional program missing: status %q severity %q, want missing / warn", it.Status, it.Severity)
	}

	// The check's JSON carries the recipe as the engine reads it.
	raw, _ := json.Marshal(deps[fixtureRequired.Name])
	for _, key := range []string{`"required":true`, `"platform":"linux"`, `"kind":"download"`, `"sha256":"abab`, `"script":"scripts/install-render.sh"`, `"min_version":"3.1"`} {
		if !bytes.Contains(raw, []byte(key)) {
			t.Errorf("config check JSON lacks %s: %s", key, raw)
		}
	}
}

// A declaration an installer engine could not run is refused at boot: the
// binary does not start.
func TestToolRecipes_InvalidDeclarationRefusedAtBoot(t *testing.T) {
	bad := fixtureOptional
	bad.Recipes = []publish.InstallRecipe{{Platform: publish.PlatformWindows, Kind: "chocolatey", Package: "fixture-media"}}
	_, err := recipeFixtureApp(t, fixtureRequired, bad)
	if err == nil || !strings.Contains(err.Error(), `unknown installer kind "chocolatey"`) || !strings.Contains(err.Error(), "fixture-media") {
		t.Fatalf("app.Run = %v, want the unknown installer kind refused naming the tool", err)
	}
}
