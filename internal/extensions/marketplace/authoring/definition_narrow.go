// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"fmt"
	"slices"

	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// Narrowing is what a clean-machine proof changes in a published definition,
// and nothing else: where it is published and with which key, the platforms
// it builds, the values its vaults are sealed from and the password they are
// read with, where its cases run.
type Narrowing struct {
	Name         string
	Output       string
	KeyFolder    string
	KeyPrefix    string
	Platforms    []string
	EnvFile      string
	PasswordEnv  string
	ScenarioRoot string
}

// Narrow is the definition a proof builds from this published one: the
// solution a fresh install opens on (Demo) alone, with the config-sets and
// vaults it names, as published - so a proof and a release install the same
// thing - and the narrowing's output, key, platforms, vault values, password
// and case root. It is never mirrored and keeps one version. A definition
// with no demo, or whose demo is none of its solutions, is refused.
func (d *Definition) Narrow(n Narrowing) (*Definition, error) {
	if d.Demo == "" {
		return nil, fmt.Errorf("%s names no solution a fresh install opens on (demo)", d.path)
	}
	var sol *DefinitionSolution
	for i := range d.Solutions {
		if d.Solutions[i].ItemName() == d.Demo {
			sol = &d.Solutions[i]
		}
	}
	if sol == nil {
		return nil, fmt.Errorf("%s: its demo %q is none of its solutions", d.path, d.Demo)
	}
	names := slices.Clone(sol.ConfigSets)
	for _, o := range sol.OtherUniverses {
		names = append(names, o.ConfigSets...)
	}
	out := &Definition{
		Name: n.Name, Output: n.Output, Key: DefinitionKey{Folder: n.KeyFolder, Prefix: n.KeyPrefix},
		Keep: 1, Platforms: n.Platforms, Solutions: []DefinitionSolution{*sol}, Demo: d.Demo,
		ScenarioRoot: n.ScenarioRoot, path: d.path,
	}
	for _, c := range d.ConfigSets {
		if slices.Contains(names, c.ItemName()) {
			out.ConfigSets = append(out.ConfigSets, c)
		}
	}
	for _, v := range d.Vaults {
		if slices.Contains(sol.Vaults, v.Name) {
			v.EnvFile, v.PasswordEnv = n.EnvFile, n.PasswordEnv
			out.Vaults = append(out.Vaults, v)
		}
	}
	return out, nil
}

// Marshal is the definition as a marketplace.yaml, its paths as resolved.
func (d *Definition) Marshal() ([]byte, error) {
	return marshal.Marshal(d, marshal.YAML)
}
