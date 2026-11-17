// SPDX-License-Identifier: Apache-2.0

// Package configset owns the config-set BUNDLE format: the `keys.yaml`
// manifest and the publish-time validation of a bundle folder.
//
// It is deliberately a shared, neutral package rather than logic inside the
// publisher or the consumer, because BOTH read the manifest: `ktw-compose
// build-config-set` validates it before packaging, and `katyoti config-set
// show` / `apply` read the declared keys back out of an installed bundle.
// Duplicating the parser into each side would let the two drift on what a
// bundle means.
//
// Spec: modules/katyoti/config-sets.md §4, §5, §10.
package configset

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// ManifestFile is the key manifest every bundle MUST carry (§10) — the ONE
	// file that declares keys, for a config-set as for a keys export list.
	ManifestFile = "keys.yaml"
	// LegacyManifestFile is the manifest's former name: read, for bundles
	// published before the rename; never written, refused at publish.
	LegacyManifestFile = "required-keys.yaml"
	// TemplateDir holds the config templates copied into a universe on apply.
	TemplateDir = "config"

	// ScopeUniverse writes the key to the universe's own .env (the default);
	// ScopeGlobal writes it to the shared global .env.
	ScopeUniverse = "universe"
	ScopeGlobal   = "global"
)

// Key is one value a config-set needs collected before its templates resolve.
type Key struct {
	Name        string `yaml:"name"        json:"name"`
	Description string `yaml:"description" json:"description,omitempty"`
	// Secret only affects HOW the value is prompted (masked). Every collected
	// value lands in .env either way — never in a copied template (§10).
	Secret bool `yaml:"secret" json:"secret,omitempty"`
	// Scope decides WHICH .env receives it: "universe" (default) or "global".
	Scope string `yaml:"scope" json:"scope,omitempty"`
	// Optional keys may stay unset: every other declared key must have a value
	// before the config-set is applied.
	Optional bool `yaml:"optional" json:"optional,omitempty"`
}

// EffectiveScope resolves the default: an unset scope means universe.
func (k Key) EffectiveScope() string {
	if strings.TrimSpace(k.Scope) == "" {
		return ScopeUniverse
	}
	return k.Scope
}

// RequiredKeys is the parsed `keys.yaml`.
type RequiredKeys struct {
	APIVersion int   `yaml:"apiVersion" json:"apiVersion"`
	Keys       []Key `yaml:"keys"       json:"keys"`
}

// Names returns the declared key names, sorted — handy for messages and diffing.
func (r *RequiredKeys) Names() []string {
	out := make([]string, 0, len(r.Keys))
	for _, k := range r.Keys {
		out = append(out, k.Name)
	}
	sort.Strings(out)
	return out
}

// Load reads and validates `<folder>/keys.yaml` (or, for a bundle published
// before the rename, `required-keys.yaml`).
//
// Raw yaml.Unmarshal is correct here (and not a violation of the
// no-raw-yaml rule): this is a marketplace BUNDLE artifact read from an
// arbitrary publisher folder or an unpacked store path, not a katyoti config
// file resolved through ConfigService layering — there is no config dir, no
// variable expansion, and no envkey/default/required tag processing to apply.
// Expanding ${VAR} here would in fact defeat the point: the placeholders are
// the very thing being declared.
func Load(folder string) (*RequiredKeys, error) {
	path := filepath.Join(folder, ManifestFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		path = filepath.Join(folder, LegacyManifestFile)
		data, err = os.ReadFile(path)
	}
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%s: a config-set bundle MUST contain %s", folder, ManifestFile)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parse(path, data)
}

// LoadFile reads and validates one keys.yaml given by its path — a keys
// export list kept anywhere (the same file a config-set carries).
func LoadFile(path string) (*RequiredKeys, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parse(path, data)
}

func parse(path string, data []byte) (*RequiredKeys, error) {
	var rk RequiredKeys
	if err := yaml.Unmarshal(data, &rk); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if rk.APIVersion == 0 {
		rk.APIVersion = 1
	}
	seen := map[string]bool{}
	for i, k := range rk.Keys {
		if strings.TrimSpace(k.Name) == "" {
			return nil, fmt.Errorf("%s: keys[%d] has no name", ManifestFile, i)
		}
		if err := CheckKeyName(k.Name); err != nil {
			return nil, fmt.Errorf("%s: %w", ManifestFile, err)
		}
		if seen[k.Name] {
			return nil, fmt.Errorf("%s: key %q declared twice", ManifestFile, k.Name)
		}
		seen[k.Name] = true
		if s := k.EffectiveScope(); s != ScopeUniverse && s != ScopeGlobal {
			return nil, fmt.Errorf("%s: key %q has scope %q (want %q or %q)",
				ManifestFile, k.Name, k.Scope, ScopeUniverse, ScopeGlobal)
		}
	}
	return &rk, nil
}

// varRef matches a `${{env:VAR}}` placeholder — the ONE form the config
// pipeline expands at runtime (`variables.ExpandVars`, resolved from the
// environment, which cenv seeds from the universe's and the global `.env`).
var varRef = regexp.MustCompile(`\$\{\{env:([A-Za-z_][A-Za-z0-9_]*)\}\}`)

// plainRef matches the shell-style `${VAR}` form. It is REFUSED, not accepted
// as an alias: templates are copied into the universe verbatim and the runtime
// never expands `${VAR}`, so a template written that way validated, applied,
// collected its keys into `.env` — and then handed the sidecar the literal
// string "${LEANIX_API_TOKEN}". The demo shipped exactly that for a month
// without anyone reading the applied file back.
var plainRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ValidateResult reports what a bundle contains, for the publisher to echo.
type ValidateResult struct {
	Keys      []string // declared key names
	Templates []string // template filenames under config/
}

// Validate enforces the publish-time bundle contract (§10):
//
//   - `keys.yaml` is present and parses (the former `required-keys.yaml` is refused: rename it);
//   - there is at least one template under `config/`;
//   - template names look like universe config files (`.yaml`/`.yml`);
//   - placeholders use the runtime form `${{env:VAR}}` (the shell-style `${VAR}` is refused);
//   - EVERY `${{env:VAR}}` referenced by a template is a DECLARED key.
//
// The last rule is the point of the whole check: an undeclared placeholder means
// the consumer would be asked to supply a value nobody told them about — or,
// worse, would silently get an empty one. Failing at publish time is what keeps
// "one run collects every key" true on the consumer side.
func Validate(folder string) (ValidateResult, error) {
	var res ValidateResult

	// Published bundles carry the ONE manifest name: the former one is read
	// for what was already published, never written anew.
	if _, err := os.Stat(filepath.Join(folder, ManifestFile)); os.IsNotExist(err) {
		if _, lerr := os.Stat(filepath.Join(folder, LegacyManifestFile)); lerr == nil {
			return res, fmt.Errorf("%s: rename %s to %s — the one file that declares keys", folder, LegacyManifestFile, ManifestFile)
		}
	}
	if _, err := os.Stat(filepath.Join(folder, LegacyMembersFile)); err == nil {
		return res, fmt.Errorf("%s: rename %s to %s — the sidecars a config-set is for (a solution is what pins versions)", folder, LegacyMembersFile, MembersFile)
	}
	rk, err := Load(folder)
	if err != nil {
		return res, err
	}
	res.Keys = rk.Names()

	declared := map[string]bool{}
	for _, k := range rk.Keys {
		declared[k.Name] = true
	}

	tmplDir := filepath.Join(folder, TemplateDir)
	entries, err := os.ReadDir(tmplDir)
	if os.IsNotExist(err) {
		return res, fmt.Errorf("%s: no %s/ directory — a config-set ships the templates it configures", folder, TemplateDir)
	}
	if err != nil {
		return res, fmt.Errorf("read %s: %w", tmplDir, err)
	}

	var undeclared, wrongForm []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if ext := strings.ToLower(filepath.Ext(name)); ext != ".yaml" && ext != ".yml" {
			return res, fmt.Errorf("%s/%s: templates must be universe config files (.yaml/.yml)", TemplateDir, name)
		}
		res.Templates = append(res.Templates, name)

		data, err := os.ReadFile(filepath.Join(tmplDir, name))
		if err != nil {
			return res, fmt.Errorf("read %s/%s: %w", TemplateDir, name, err)
		}
		text := string(data)
		for _, m := range plainRef.FindAllStringSubmatch(text, -1) {
			wrongForm = append(wrongForm, fmt.Sprintf("%s uses ${%s}", name, m[1]))
		}
		for _, m := range varRef.FindAllStringSubmatch(text, -1) {
			if v := m[1]; !declared[v] {
				undeclared = append(undeclared, fmt.Sprintf("%s references ${{env:%s}}", name, v))
			}
		}
	}
	if len(res.Templates) == 0 {
		return res, fmt.Errorf("%s/%s: no templates found", folder, TemplateDir)
	}
	if len(wrongForm) > 0 {
		sort.Strings(wrongForm)
		return res, fmt.Errorf(
			"placeholder form — write ${{env:VAR}}, not ${VAR}: templates are copied verbatim and the runtime expands only the ${{env:…}} form, so ${VAR} reaches the sidecar as a literal string:\n  %s",
			strings.Join(wrongForm, "\n  "))
	}
	if len(undeclared) > 0 {
		sort.Strings(undeclared)
		return res, fmt.Errorf(
			"undeclared key(s) — every ${{env:VAR}} in a template must be declared in %s, or the consumer is asked to guess:\n  %s",
			ManifestFile, strings.Join(undeclared, "\n  "))
	}
	sort.Strings(res.Templates)
	return res, nil
}
