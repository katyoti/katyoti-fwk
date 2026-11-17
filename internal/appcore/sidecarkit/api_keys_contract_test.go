// SPDX-License-Identifier: Apache-2.0

package sidecarkit_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// The fixture's keys: one required, one optional, neither set.
var (
	contractKeyRequired = publish.APIKeyDef{EnvName: "KTW_FIXTURE_CONTRACT_REQUIRED", Label: "Required key", Required: true}
	contractKeyOptional = publish.APIKeyDef{EnvName: "KTW_FIXTURE_CONTRACT_OPTIONAL", Label: "Optional key"}
)

// keySeverities maps each declared key to the severity its check item has when
// the key is unset: error for a required key, warn for an optional one.
func declaredSeverities(keys []publish.APIKeyRequirement) map[string]string {
	out := map[string]string{}
	for _, k := range keys {
		if k.Required {
			out[k.EnvName] = "error"
		} else {
			out[k.EnvName] = "warn"
		}
	}
	return out
}

// checkedSeverities maps each env item of a config check to its severity.
func checkedSeverities(rep api.ConfigCheckReport) map[string]string {
	out := map[string]string{}
	for _, it := range rep.Items {
		if it.Category == "env" {
			out[it.Name] = it.Severity
		}
	}
	return out
}

// sameKeys compares the check's keys and the declared ones both ways: every
// key the check asks for is declared, every declared key is checked, with the
// severity its declaration gives.
func sameKeys(t *testing.T, label string, checked, declared map[string]string) {
	t.Helper()
	var names []string
	for n := range checked {
		names = append(names, n)
	}
	for n := range declared {
		if _, ok := checked[n]; !ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		c, inCheck := checked[n]
		d, inDecl := declared[n]
		switch {
		case !inDecl:
			t.Errorf("%s: the config check asks for %s, which the sidecar does not declare", label, n)
		case !inCheck:
			t.Errorf("%s: %s is declared but the config check does not ask for it", label, n)
		case c != d:
			t.Errorf("%s: %s unset is %q in the config check, its declaration says %q", label, n, c, d)
		}
	}
	if len(declared) == 0 {
		t.Errorf("%s: the fixture declares no key", label)
	}
}

// Every key a sidecar's config check asks for is a key the sidecar declares
// (publish.APIKeyDef through WithAPIKeys: describe's apiKeys, the catalog's
// requires), and every declared key is checked — so a key the check reports
// missing is always one the common save path can write.
func TestAPIKeys_TheConfigCheckAsksExactlyTheDeclaredKeys(t *testing.T) {
	t.Setenv("KTW_WORKSPACE", t.TempDir())
	t.Setenv("KTW_DATA", t.TempDir())
	t.Setenv("KTW_CACHE", t.TempDir())
	t.Setenv(contractKeyRequired.EnvName, "")
	t.Setenv(contractKeyOptional.EnvName, "")
	app := application.New(application.Config(), sidecarkit.NewSidecar(
		appcore.AppConfig{AppName: "ktw-fixture", Version: "1.0.0", EmbeddedFS: fstest.MapFS{}},
		sidecarkit.WithDomain(publish.PublishSimpleService[recipeFixtureSvc](
			func(context.Context) (recipeFixtureSvc, error) { return recipeFixtureImpl{}, nil },
			publish.WithAPIKeys(contractKeyRequired, contractKeyOptional),
		)),
		sidecarkit.WithoutPack(), sidecarkit.WithoutKTSP(), sidecarkit.WithoutHTTPServer(),
	)...)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	t.Cleanup(func() { appcore.CloseApplicationScope(application.WithApplication(context.Background(), app)) })
	ctx := app.Context()

	d := describeJSON(t, ctx)
	svc, err := application.UseService[api.SidecarConfigService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := svc.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sameKeys(t, "coded sidecar", checkedSeverities(rep), declaredSeverities(d.APIKeys))
}

// The same contract for a folder member: CheckFolderConfig asks exactly the
// keys FolderConfig.Requirements declares (what the catalog copies).
func TestAPIKeys_AFolderMembersCheckAsksExactlyItsDeclaredKeys(t *testing.T) {
	t.Setenv(contractKeyRequired.EnvName, "")
	t.Setenv(contractKeyOptional.EnvName, "")
	c := sidecar.FolderConfig{APIKeys: []sidecar.FolderAPIKey{
		{EnvName: contractKeyRequired.EnvName, Label: contractKeyRequired.Label, Required: true},
		{EnvName: contractKeyOptional.EnvName, Label: contractKeyOptional.Label},
	}}
	rep, err := sidecar.CheckFolderConfig(context.Background(), t.TempDir(), "fixture-folder", c)
	if err != nil {
		t.Fatal(err)
	}
	_, keys, _ := c.Requirements()
	sameKeys(t, "folder member", checkedSeverities(rep), declaredSeverities(keys))
}

// envItemLiteral is a config check item of the env category built in code.
var envItemLiteral = regexp.MustCompile(`Category:\s*"env"`)

// An env check item is made in one place, from a declared key
// (default_config.go envCheckItem): no other code builds one, so no check can
// ask for a key the sidecar has not declared.
func TestAPIKeys_AnEnvCheckItemIsMadeOnlyFromADeclaredKey(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repository root not found at %s: %v", root, err)
	}
	want := filepath.Join("internal", "appcore", "sidecar", "internal", "core", "default_config.go")
	var found []string
	err = filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if n := e.Name(); path != root && (strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") || n == "node_modules" || n == "vendor" || n == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if envItemLiteral.Match(raw) {
			rel, _ := filepath.Rel(root, path)
			found = append(found, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0] != want {
		t.Errorf("env check items are built in %v; want only in %s (envCheckItem, from a declared key)", found, want)
	}
}
