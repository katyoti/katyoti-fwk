// SPDX-License-Identifier: Apache-2.0

// Package keysfile is the one format of a sealed keys file — what `katyoti keys
// export` writes, `keys import` and a solution's vaults open, and a marketplace
// publish seals from an .env: each key with the scope it was declared in, so an
// import writes it back into the same .env, sealed with a password
// (internal/core/sealed).
package keysfile

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/sealed"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configset"
)

// Format names what a sealed keys file holds once opened.
const Format = "katyoti-keys/1"

// Bundle is the plain content of a keys file.
type Bundle struct {
	Format string  `yaml:"format"`
	Keys   []Entry `yaml:"keys"`
}

// Entry is one key: its name, the scope it is written into (universe or
// global), its description, its value.
type Entry struct {
	Name        string `yaml:"name"`
	Scope       string `yaml:"scope"`
	Description string `yaml:"description,omitempty"`
	Value       string `yaml:"value"`
}

// Seal turns keys into a file only password opens.
func Seal(keys []Entry, password string) ([]byte, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("nothing to seal: no key has a value")
	}
	plain, err := marshal.Marshal(Bundle{Format: Format, Keys: keys}, marshal.YAML)
	if err != nil {
		return nil, err
	}
	return sealed.Seal(plain, password)
}

// Open opens a keys file with its password and checks what it holds.
func Open(file []byte, password string) (Bundle, error) {
	var b Bundle
	plain, err := sealed.Open(file, password)
	if err != nil {
		return b, err
	}
	// Decoded verbatim, not through config.DecodeBytes: that expands ${...}
	// placeholders, and a key's value is data that must come back unchanged.
	if err := marshal.Unmarshal(plain, marshal.YAML, &b); err != nil {
		return b, fmt.Errorf("the keys file does not parse: %w", err)
	}
	if b.Format != Format {
		return b, fmt.Errorf("not a keys file (format %q, expected %q)", b.Format, Format)
	}
	for _, e := range b.Keys {
		if e.Name == "" || (e.Scope != configset.ScopeUniverse && e.Scope != configset.ScopeGlobal) {
			return b, fmt.Errorf("the keys file holds an entry without a name or with scope %q", e.Scope)
		}
		if err := configset.CheckKeyName(e.Name); err != nil {
			return b, fmt.Errorf("the keys file holds an entry it may not import: %w", err)
		}
		if err := configset.CheckKeyValue(e.Name, e.Value); err != nil {
			return b, fmt.Errorf("the keys file holds an entry it may not import: %w", err)
		}
	}
	return b, nil
}

// FromEnvFile takes, from an .env file, the value of every key a keys.yaml
// declares — each with its declared scope and description — and names the
// declared keys the file has no value for. The file is read, never printed.
func FromEnvFile(envFile, keysYAML string) ([]Entry, []string, error) {
	rk, err := configset.LoadFile(keysYAML)
	if err != nil {
		return nil, nil, err
	}
	values, err := configset.ReadEnv(envFile)
	if err != nil {
		return nil, nil, err
	}
	var keys []Entry
	var missing []string
	for _, k := range rk.Keys {
		v := values[k.Name]
		if v == "" {
			missing = append(missing, k.Name)
			continue
		}
		keys = append(keys, Entry{Name: k.Name, Scope: k.EffectiveScope(), Description: k.Description, Value: v})
	}
	return keys, missing, nil
}
