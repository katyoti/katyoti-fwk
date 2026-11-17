// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// DefinitionFile is the conventional name of a marketplace's definition, at
// the root of its source repository.
const DefinitionFile = "marketplace.yaml"

// Definition is a marketplace.yaml: everything a marketplace publishes, in one
// file kept with its sources — what `ktw-compose marketplace build` publishes
// at a version from the binaries `make` built. Paths are relative to the file;
// ${{env:VAR}} reads a variable. It never holds a secret: a vault names the
// variable its password is read from.
type Definition struct {
	// Name is what subscribers call the marketplace.
	Name string `yaml:"name"`
	// Output is the published tree; Mirrors receive a copy of it.
	Output  string   `yaml:"output"`
	Mirrors []string `yaml:"mirrors,omitempty"`
	// Key signs everything: <folder>/<prefix>.key (never committed) and its
	// .key.pub.
	Key DefinitionKey `yaml:"key"`
	// Keep bounds the versions kept per item (0: 3).
	Keep int `yaml:"keep,omitempty"`
	// BaseURL is set only when the assets are hosted apart from the catalog.
	BaseURL string `yaml:"base_url,omitempty"`
	// Platforms the build must hold (<os>-<arch>); every sidecar built for
	// them is published.
	Platforms []string `yaml:"platforms"`
	// ConfigSets, Seeds, Vaults and Solutions are published at the version.
	ConfigSets []DefinitionConfigSet `yaml:"config_sets,omitempty"`
	Seeds      []DefinitionSeed      `yaml:"seeds,omitempty"`
	Vaults     []DefinitionVault     `yaml:"vaults,omitempty"`
	Solutions  []DefinitionSolution  `yaml:"solutions,omitempty"`
	// Demo is the solution a fresh install opens on (one of Solutions).
	Demo string `yaml:"demo,omitempty"`
	// ScenarioRoot is where the solutions' scenario cases run (default
	// $KTW_TEST_ROOT, else ~/katyoti-tests).
	ScenarioRoot string `yaml:"scenario_root,omitempty"`
	// SecretScan tunes the scan of every artefact before anything is
	// published: a secret refuses every release; the deny-list only where
	// the definition names one.
	SecretScan DefinitionSecretScan `yaml:"secret_scan,omitempty"`

	path string
}

// DefinitionSecretScan names the lists the artefact scan reads (the format of
// tools/secret-scan's -deny and -allow): Deny, terms no artefact may hold;
// Allow, the library samples shaped like a credential that pass, by path and
// kind.
type DefinitionSecretScan struct {
	Deny  string `yaml:"deny,omitempty"`
	Allow string `yaml:"allow,omitempty"`
}

// DefinitionKey is the signing key: <Folder>/<Prefix>.key.
type DefinitionKey struct {
	Folder string `yaml:"folder"`
	Prefix string `yaml:"prefix"`
}

// DefinitionConfigSet is a config-set folder (named after it unless Name).
type DefinitionConfigSet struct {
	Path        string `yaml:"path"`
	Name        string `yaml:"name,omitempty"`
	DisplayName string `yaml:"display_name,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// DefinitionSeed is a project folder published as a seed.
type DefinitionSeed struct {
	Path        string `yaml:"path"`
	Name        string `yaml:"name,omitempty"`
	Tasks       string `yaml:"tasks,omitempty"`
	DisplayName string `yaml:"display_name,omitempty"`
	Description string `yaml:"description,omitempty"`
	// Demo: its content is demo data (a « Démo » badge in the Marketplace).
	Demo bool `yaml:"demo,omitempty"`
}

// DefinitionVault is a vault sealed at publish from an .env: the values of the
// keys a keys.yaml declares, sealed with the password read from PasswordEnv.
type DefinitionVault struct {
	Name        string `yaml:"name"`
	EnvFile     string `yaml:"env_file"`
	Keys        string `yaml:"keys"`
	PasswordEnv string `yaml:"password_env"`
}

// DefinitionSolution is a scenario case published as a solution.
type DefinitionSolution struct {
	// Case is the case's folder; Name the solution's (default: the case's).
	Case       string   `yaml:"case"`
	Name       string   `yaml:"name,omitempty"`
	Checkpoint string   `yaml:"checkpoint,omitempty"`
	ConfigSets []string `yaml:"config_sets,omitempty"`
	Vaults     []string `yaml:"vaults,omitempty"`
	// Docs is its documents folder (<lang>/*.md; default the case's docs/).
	Docs string `yaml:"docs,omitempty"`
	// LetterCovers are the change logs its written « Quoi de neuf » covers
	// beside the build's --whats-new (the KaTyoti version it ships), each
	// relative to the definition's folder.
	LetterCovers []string `yaml:"letter_covers,omitempty"`
	// OtherUniverses give the case's other universes (its other_universes,
	// published beside its own, the main one) their config-sets, by name.
	OtherUniverses []DefinitionOtherUniverse `yaml:"other_universes,omitempty"`
}

// DefinitionOtherUniverse is one of a solution case's other universes: its
// name in the case, the config-sets (published by this marketplace) it
// applies, in order.
type DefinitionOtherUniverse struct {
	Name       string   `yaml:"name"`
	ConfigSets []string `yaml:"config_sets,omitempty"`
}

// Path is the definition file; Dir its folder.
func (d *Definition) Path() string { return d.path }
func (d *Definition) Dir() string  { return filepath.Dir(d.path) }

// SecretKey and PublicKey are the signing key's files.
func (d *Definition) SecretKey() string {
	return filepath.Join(d.Key.Folder, d.Key.Prefix+".key")
}
func (d *Definition) PublicKey() string {
	return filepath.Join(d.Key.Folder, d.Key.Prefix+".key.pub")
}

// ItemName is an item's name: given, else its folder's.
func (c DefinitionConfigSet) ItemName() string { return nameOr(c.Name, c.Path) }
func (s DefinitionSeed) ItemName() string      { return nameOr(s.Name, s.Path) }
func (s DefinitionSolution) ItemName() string  { return nameOr(s.Name, s.Case) }

func nameOr(name, path string) string {
	if name != "" {
		return name
	}
	return filepath.Base(path)
}

// LoadDefinition reads a marketplace definition (a file, or the marketplace.yaml
// of a folder): variables expanded, paths made absolute, every reference
// checked.
func LoadDefinition(path string) (*Definition, error) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		path = filepath.Join(path, DefinitionFile)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	if left := unresolved(withoutComments(string(data))); left != "" {
		return nil, fmt.Errorf("%s: %s is not set", abs, left)
	}
	var d Definition
	if err := config.DecodeBytes(data, marshal.YAML, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", abs, err)
	}
	d.path = abs
	d.resolve()
	if err := d.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", abs, err)
	}
	return &d, nil
}

// withoutComments drops the comment lines (a comment may show ${{env:VAR}}).
func withoutComments(s string) string {
	var b strings.Builder
	for _, l := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// unresolved names the first ${{env:VAR}} whose variable is not set.
func unresolved(s string) string {
	for {
		i := strings.Index(s, "${{env:")
		if i < 0 {
			return ""
		}
		j := strings.Index(s[i:], "}}")
		if j < 0 {
			return ""
		}
		name := s[i+len("${{env:") : i+j]
		if _, ok := os.LookupEnv(name); !ok {
			return "the variable " + name
		}
		s = s[i+j:]
	}
}

func (d *Definition) resolve() {
	abs := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return filepath.Clean(p)
		}
		return filepath.Join(d.Dir(), p)
	}
	d.Output = abs(d.Output)
	for i := range d.Mirrors {
		d.Mirrors[i] = abs(d.Mirrors[i])
	}
	d.Key.Folder = abs(d.Key.Folder)
	if d.ScenarioRoot != "" {
		d.ScenarioRoot = abs(d.ScenarioRoot)
	}
	if d.SecretScan.Deny != "" {
		d.SecretScan.Deny = abs(d.SecretScan.Deny)
	}
	if d.SecretScan.Allow != "" {
		d.SecretScan.Allow = abs(d.SecretScan.Allow)
	}
	if d.Keep <= 0 {
		d.Keep = 3
	}
	for i := range d.ConfigSets {
		d.ConfigSets[i].Path = abs(d.ConfigSets[i].Path)
	}
	for i := range d.Seeds {
		d.Seeds[i].Path = abs(d.Seeds[i].Path)
		if d.Seeds[i].Tasks != "" {
			d.Seeds[i].Tasks = abs(d.Seeds[i].Tasks)
		}
	}
	for i := range d.Vaults {
		d.Vaults[i].EnvFile = abs(d.Vaults[i].EnvFile)
		d.Vaults[i].Keys = abs(d.Vaults[i].Keys)
	}
	for i := range d.Solutions {
		d.Solutions[i].Case = abs(d.Solutions[i].Case)
		if d.Solutions[i].Docs != "" {
			d.Solutions[i].Docs = abs(d.Solutions[i].Docs)
		}
	}
}

func (d *Definition) check() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("a marketplace has a name")
	}
	if d.Output == "" || d.Output == "." {
		return fmt.Errorf("no output: where is it published")
	}
	if d.Key.Folder == "" || d.Key.Prefix == "" {
		return fmt.Errorf("no key: key: {folder, prefix}")
	}
	if len(d.Platforms) == 0 {
		return fmt.Errorf("no platforms: which builds it publishes (e.g. windows-amd64)")
	}
	seen := map[string]bool{}
	unique := func(kind, name string) error {
		if seen[kind+"/"+name] {
			return fmt.Errorf("two %ss named %q", kind, name)
		}
		seen[kind+"/"+name] = true
		return nil
	}
	for _, c := range d.ConfigSets {
		if err := unique("config-set", c.ItemName()); err != nil {
			return err
		}
	}
	for _, s := range d.Seeds {
		if err := unique("seed", s.ItemName()); err != nil {
			return err
		}
	}
	for _, v := range d.Vaults {
		if v.Name == "" || v.PasswordEnv == "" || v.EnvFile == "" || v.Keys == "" {
			return fmt.Errorf("vault %q: name, env_file, keys and password_env are all needed", v.Name)
		}
		if err := unique("vault", v.Name); err != nil {
			return err
		}
	}
	for _, s := range d.Solutions {
		if err := unique("solution", s.ItemName()); err != nil {
			return err
		}
		for _, c := range s.ConfigSets {
			if !seen["config-set/"+c] {
				return fmt.Errorf("solution %s pins the config-set %q, which this marketplace does not publish", s.ItemName(), c)
			}
		}
		for _, v := range s.Vaults {
			if !seen["vault/"+v] {
				return fmt.Errorf("solution %s pins the vault %q, which this marketplace does not publish", s.ItemName(), v)
			}
		}
		others := map[string]bool{}
		for _, o := range s.OtherUniverses {
			if strings.TrimSpace(o.Name) == "" || others[o.Name] {
				return fmt.Errorf("solution %s: each of its other_universes is named, once", s.ItemName())
			}
			others[o.Name] = true
			for _, c := range o.ConfigSets {
				if !seen["config-set/"+c] {
					return fmt.Errorf("solution %s: its universe %s pins the config-set %q, which this marketplace does not publish", s.ItemName(), o.Name, c)
				}
			}
		}
	}
	if d.Demo != "" && !seen["solution/"+d.Demo] {
		return fmt.Errorf("the demo %q is not one of the solutions", d.Demo)
	}
	return nil
}
