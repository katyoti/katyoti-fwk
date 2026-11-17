// SPDX-License-Identifier: Apache-2.0

package configset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bundle builds a config-set folder: manifest text + template files.
func bundle(t *testing.T, manifest string, templates map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if templates != nil {
		td := filepath.Join(dir, TemplateDir)
		if err := os.MkdirAll(td, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range templates {
			if err := os.WriteFile(filepath.Join(td, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

const goodManifest = `apiVersion: 1
keys:
  - name: LEANIX_API_TOKEN
    description: LeanIX API token
    secret: true
    scope: universe
  - name: CONFLUENCE_BASE_URL
    description: Confluence base URL
    scope: global
`

func TestLoad_DefaultsAndParsing(t *testing.T) {
	dir := bundle(t, goodManifest, map[string]string{"LeanIX.yaml": "token: ${{env:LEANIX_API_TOKEN}}"})
	rk, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if rk.APIVersion != 1 || len(rk.Keys) != 2 {
		t.Fatalf("unexpected manifest: %+v", rk)
	}
	if got := rk.Keys[0].EffectiveScope(); got != ScopeUniverse {
		t.Errorf("explicit universe scope = %q", got)
	}
	if got := rk.Keys[1].EffectiveScope(); got != ScopeGlobal {
		t.Errorf("global scope = %q", got)
	}
	// An omitted scope defaults to universe rather than erroring.
	k := Key{Name: "X"}
	if got := k.EffectiveScope(); got != ScopeUniverse {
		t.Errorf("omitted scope should default to universe, got %q", got)
	}
}

func TestLoad_MissingManifestIsAnError(t *testing.T) {
	dir := bundle(t, "", map[string]string{"a.yaml": "x: 1"})
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), ManifestFile) {
		t.Fatalf("a bundle without the manifest must be rejected, got: %v", err)
	}
}

func TestLoad_RejectsDuplicateAndUnnamedAndBadScope(t *testing.T) {
	cases := map[string]string{
		"duplicate": "apiVersion: 1\nkeys:\n  - name: A\n  - name: A\n",
		"unnamed":   "apiVersion: 1\nkeys:\n  - description: no name\n",
		"bad scope": "apiVersion: 1\nkeys:\n  - name: A\n    scope: galaxy\n",
	}
	for label, m := range cases {
		dir := bundle(t, m, map[string]string{"a.yaml": "x: 1"})
		if _, err := Load(dir); err == nil {
			t.Errorf("%s manifest should be rejected", label)
		}
	}
}

// THE rule that earns the whole validation step: an undeclared ${{env:VAR}} means the
// consumer would be asked for a value nobody declared — or silently get an empty
// one. It must fail at PUBLISH time.
func TestValidate_RejectsUndeclaredPlaceholder(t *testing.T) {
	dir := bundle(t, goodManifest, map[string]string{
		"LeanIX.yaml":    "token: ${{env:LEANIX_API_TOKEN}}",
		"Dynatrace.yaml": "url: ${{env:DYNATRACE_URL}}", // never declared
	})
	_, err := Validate(dir)
	if err == nil {
		t.Fatal("an undeclared ${{env:VAR}} must fail validation")
	}
	if !strings.Contains(err.Error(), "DYNATRACE_URL") {
		t.Errorf("the error must name the offending key, got: %v", err)
	}
}

func TestValidate_AcceptsFullyDeclaredBundle(t *testing.T) {
	dir := bundle(t, goodManifest, map[string]string{
		"LeanIX.yaml":     "token: ${{env:LEANIX_API_TOKEN}}",
		"Confluence.yaml": "base: ${{env:CONFLUENCE_BASE_URL}}\nnote: no placeholders here",
	})
	res, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(res.Keys) != 2 || len(res.Templates) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Templates[0] != "Confluence.yaml" {
		t.Errorf("templates should be sorted, got %v", res.Templates)
	}
}

func TestValidate_RejectsMissingOrEmptyTemplateDir(t *testing.T) {
	noDir := bundle(t, goodManifest, nil)
	if _, err := Validate(noDir); err == nil {
		t.Error("a bundle with no config/ dir must be rejected")
	}
	empty := bundle(t, goodManifest, map[string]string{})
	if _, err := Validate(empty); err == nil {
		t.Error("a bundle with an empty config/ dir must be rejected")
	}
}

func TestValidate_RejectsNonConfigTemplateName(t *testing.T) {
	dir := bundle(t, goodManifest, map[string]string{"README.md": "not a config file"})
	if _, err := Validate(dir); err == nil {
		t.Error("templates must look like universe config files (.yaml/.yml)")
	}
}

// The shell-style ${VAR} is refused outright. Templates are copied into the
// universe verbatim and the runtime expands only ${{env:VAR}}, so a ${VAR}
// template validated, applied, collected its key — and handed the sidecar the
// literal string. The message must name the form to write instead.
func TestValidate_RefusesPlainPlaceholderForm(t *testing.T) {
	dir := bundle(t, goodManifest, map[string]string{"LeanIX.yaml": "token: ${LEANIX_API_TOKEN}"})
	_, err := Validate(dir)
	if err == nil {
		t.Fatal("a ${VAR} placeholder must fail validation")
	}
	if !strings.Contains(err.Error(), "${{env:VAR}}") || !strings.Contains(err.Error(), "LeanIX.yaml uses ${LEANIX_API_TOKEN}") {
		t.Fatalf("error must name the runtime form and the offending template: %v", err)
	}
}

// A key name is an environment variable name: keys.yaml refuses `*`, `X_*`,
// `A,B` - written into the global declaration they would be read as a
// pattern or a list - nor a KaTyoti variable (KTW_*):
// KTW_GLOBAL_KEYS with the value * would declare every global key.
func TestLoad_RejectsANameOutsideTheGrammar(t *testing.T) {
	for _, name := range []string{"'*'", "'X_*'", "'A,B'", "1A", "A-B",
		"KTW_GLOBAL_KEYS", "KTW_WORKSPACE", "KTW_UNIVERSE", "KTW_DOTENV_KEYS", "KTW_DATA"} {
		dir := bundle(t, "keys:\n  - name: "+name+"\n    scope: global\n", nil)
		if _, err := Load(dir); err == nil {
			t.Errorf("keys.yaml with key %s loaded", name)
		}
	}
	dir := bundle(t, "keys:\n  - name: _A1\n", nil)
	if _, err := Load(dir); err != nil {
		t.Fatalf("a valid name refused: %v", err)
	}
}
