// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
)

// A build checks everything before it writes anything: a definition whose key,
// build, sources, vault and case are all wrong is refused with each problem
// named, and its output folder is never created.
func TestABuildThatFailsItsChecksWritesNothing(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	if err := os.MkdirAll(filepath.Join(dist, "linux-amd64"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("OTHER=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "base"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "base", "keys.yaml"), []byte("apiVersion: 1\nkeys:\n  - name: TEAM_TOKEN\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	def := `name: team
output: out
key: {folder: keys, prefix: team}
platforms: [windows-amd64]
config_sets: [{path: base}, {path: missing-set}]
vaults: [{name: v, env_file: .env, keys: base/keys.yaml, password_env: KTW_TEST_UNSET_PASSWORD}]
solutions: [{case: no-such-case, config_sets: [base]}]
`
	if err := os.WriteFile(filepath.Join(dir, "marketplace.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KTW_TEST_UNSET_PASSWORD", "")
	_, err := New(nil).BuildFromDefinition(context.Background(), api.BuildFromDefinitionInput{
		Definition: filepath.Join(dir, "marketplace.yaml"), Version: "v1", Dist: dist,
	})
	if err == nil {
		t.Fatal("a definition failing its checks was published")
	}
	for _, want := range []string{"no signing key", "has no windows-amd64", "holds linux-amd64", "missing-set",
		"KTW_TEST_UNSET_PASSWORD, which is not set", "has a value for none", "solution no-such-case"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should name %q:\n%v", want, err)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dir, "out")); statErr == nil {
		t.Fatal("a refused build wrote its output folder")
	}
}

// A sidecar of the build that cannot describe itself, or a platform without its
// control plane, refuses the publish before anything is written: published anyway, it would be missing from the catalog
// and a solution pinning it would fail half-published.
func TestABuildWithABrokenSidecarIsRefusedBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	host := runtime.GOOS + "-" + runtime.GOARCH
	dist := filepath.Join(dir, "dist")
	bin := filepath.Join(dist, host)
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	broken := "ktw-broken"
	if runtime.GOOS == "windows" {
		broken += ".exe"
	}
	if err := os.WriteFile(filepath.Join(bin, broken), []byte("not a program"), 0o755); err != nil {
		t.Fatal(err)
	}
	pub, sec, _, err := authoring.New().Keygen("test")
	if err != nil {
		t.Fatal(err)
	}
	keys := filepath.Join(dir, "keys")
	if err := os.MkdirAll(keys, 0o755); err != nil {
		t.Fatal(err)
	}
	for dst, body := range map[string]string{"team.key.pub": pub, "team.key": sec} {
		if err := os.WriteFile(filepath.Join(keys, dst), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	def := "name: team\noutput: out\nkey: {folder: keys, prefix: team}\nplatforms: [" + host + "]\n"
	if err := os.WriteFile(filepath.Join(dir, "marketplace.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = New(nil).BuildFromDefinition(context.Background(), api.BuildFromDefinitionInput{
		Definition: filepath.Join(dir, "marketplace.yaml"), Version: "v1", Dist: dist,
	})
	for _, want := range []string{broken + " cannot be published", "has no control plane"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should name %q: %v", want, err)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dir, "out")); statErr == nil {
		t.Fatal("a refused build wrote its output folder")
	}
}

// The cut's first check, before the build: a version the marketplace already
// published, or one below its last, is refused; a higher one names the last.
// A start over (--reinit) compares nothing, its tree being wiped.
func TestACutRefusesAVersionPublishedOrLowerBeforeTheBuild(t *testing.T) {
	dir := t.TempDir()
	pub, sec, _, err := authoring.New().Keygen("test")
	if err != nil {
		t.Fatal(err)
	}
	keys := filepath.Join(dir, "keys")
	out := filepath.Join(dir, "out")
	for _, d := range []string{keys, out} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for dst, body := range map[string]string{"team.key.pub": pub, "team.key": sec} {
		if err := os.WriteFile(filepath.Join(keys, dst), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalog := "apiVersion: 1\nmarketplace: {name: team}\nitems:\n  - name: ktw-agents\n    versions:\n      - version: \"1.005\"\n      - version: \"1.004\"\n"
	if err := os.WriteFile(filepath.Join(out, "katyoti-marketplace.yaml"), []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}
	def := "name: team\noutput: out\nkey: {folder: keys, prefix: team}\nplatforms: [windows-amd64]\n"
	if err := os.WriteFile(filepath.Join(dir, "marketplace.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	cut := func(version string, reinit bool) (api.BuildFromDefinitionOutput, error) {
		return New(nil).BuildFromDefinition(context.Background(), api.BuildFromDefinitionInput{
			Definition: filepath.Join(dir, "marketplace.yaml"), Version: version,
			DryRun: true, BeforeBuild: true, Reinit: reinit,
		})
	}
	for version, refusal := range map[string]string{"1.004": "already published", "1.003": "lower than 1.005", "v1.005": "the same as 1.005"} {
		if _, err := cut(version, false); err == nil || !strings.Contains(err.Error(), refusal) {
			t.Errorf("cut %s: want a refusal saying %q, got %v", version, refusal, err)
		}
	}
	res, err := cut("1.006", false)
	if err != nil || res.LastPublished != "1.005" {
		t.Fatalf("cut 1.006: last=%q err=%v", res.LastPublished, err)
	}
	if _, err := cut("1.004", true); err != nil {
		t.Fatalf("a start over refused a version of the tree it wipes: %v", err)
	}
}
