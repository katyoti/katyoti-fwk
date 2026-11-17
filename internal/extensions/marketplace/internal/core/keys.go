// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configset"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/keysfile"
)

// A sealed keys file's format is keysfile's (one format: export, import,
// vaults, a marketplace's vaults sealed at publish).
type (
	keysBundle  = keysfile.Bundle
	bundleEntry = keysfile.Entry
)

// scopedEnv reads a universe's .env and the global .env — the only two places
// a key's value is taken from or written to. The process environment is never
// read: a key exported must be one the user set for katyoti, in its scope.
type scopedEnv struct {
	paths  map[string]string
	values map[string]map[string]string
}

func readScopedEnv(home string) (*scopedEnv, error) {
	se := &scopedEnv{
		paths: map[string]string{
			configset.ScopeUniverse: filepath.Join(application.EnvOf(home).HomeDir(), ".env"),
			configset.ScopeGlobal:   filepath.Join(application.Env().GlobalHomeDir(), ".env"),
		},
		values: map[string]map[string]string{},
	}
	for scope, p := range se.paths {
		v, err := configset.ReadEnv(p)
		if err != nil {
			return nil, err
		}
		se.values[scope] = v
	}
	return se, nil
}

// keysUniverse resolves the universe a keys call works on: a registered name or
// a folder, which must exist (keys are never the first thing a universe gets).
func keysUniverse(target string) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", fmt.Errorf("a universe is required (a registered name or a folder)")
	}
	home := universeTarget(target)
	if st, err := os.Stat(home); err != nil || !st.IsDir() {
		return "", fmt.Errorf("universe %q not found (%s)", target, home)
	}
	return home, nil
}

// keysManifest loads the ONE manifest an export follows: the keys.yaml of an
// installed config-set, or a keys.yaml file.
func (s *service) keysManifest(ctx context.Context, in mktapi.ExportKeysInput) (*configset.RequiredKeys, error) {
	switch {
	case in.ConfigSet != "" && in.Manifest != "":
		return nil, fmt.Errorf("give a config-set or a manifest, not both")
	case in.Manifest != "":
		return configset.LoadFile(in.Manifest)
	case in.ConfigSet != "":
		name, version, _ := strings.Cut(in.ConfigSet, "@")
		ins, err := s.Install(ctx, mktapi.ItemRef{Name: name, Version: version})
		if err != nil {
			return nil, err
		}
		if ins.Kind != string(mktapi.KindConfig) {
			return nil, fmt.Errorf("%s is a %s item, not a config-set", name, ins.Kind)
		}
		bundle := ins.FolderPath
		if bundle == "" {
			if f, ferr := store.ConfigSetFolderPath(ins.Name, ins.Version); ferr == nil {
				bundle = f
			}
		}
		return configset.Load(bundle)
	}
	return nil, fmt.Errorf("say which keys to export: a config-set (--config-set) or a %s (--manifest) — never a whole .env", configset.ManifestFile)
}

// ExportKeys seals the keys one manifest declares, with their scope.
func (s *service) ExportKeys(ctx context.Context, in mktapi.ExportKeysInput) (mktapi.ExportKeysOutput, error) {
	var out mktapi.ExportKeysOutput
	home, err := keysUniverse(in.UniverseHome)
	if err != nil {
		return out, err
	}
	rk, err := s.keysManifest(ctx, in)
	if err != nil {
		return out, err
	}
	se, err := readScopedEnv(home)
	if err != nil {
		return out, err
	}
	bundle := keysBundle{Format: keysfile.Format}
	var missing []string
	for _, k := range rk.Keys {
		scope := k.EffectiveScope()
		v := se.values[scope][k.Name]
		if strings.TrimSpace(v) == "" {
			missing = append(missing, k.Name)
			continue
		}
		bundle.Keys = append(bundle.Keys, bundleEntry{Name: k.Name, Scope: scope, Description: k.Description, Value: v})
		out.Keys = append(out.Keys, mktapi.KeyEntry{Name: k.Name, Scope: scope})
	}
	if len(missing) > 0 && !in.SkipMissing {
		return mktapi.ExportKeysOutput{}, fmt.Errorf("no value for %s in its scope's .env: set it, or leave it out (--skip-missing)", strings.Join(missing, ", "))
	}
	out.Skipped = missing
	if len(bundle.Keys) == 0 {
		return mktapi.ExportKeysOutput{}, fmt.Errorf("nothing to export: none of the declared keys has a value")
	}
	if in.Preview {
		return out, nil
	}
	out.File, err = keysfile.Seal(bundle.Keys, in.Password)
	if err != nil {
		return mktapi.ExportKeysOutput{}, err
	}
	return out, nil
}

// ImportKeys opens a keys file and writes each key into its scope's .env,
// then restarts the running members that read a key written.
func (s *service) ImportKeys(ctx context.Context, in mktapi.ImportKeysInput) (mktapi.ImportKeysOutput, error) {
	out, home, err := s.importKeys(ctx, in)
	if err != nil || in.DryRun {
		return out, err
	}
	out.Restarts = s.restartReaders(ctx, importWrites(home, out.Keys))
	return out, nil
}

// importKeys is ImportKeys without the restart, and the universe it wrote to.
func (s *service) importKeys(ctx context.Context, in mktapi.ImportKeysInput) (mktapi.ImportKeysOutput, string, error) {
	var out mktapi.ImportKeysOutput
	home, err := keysUniverse(in.UniverseHome)
	if err != nil {
		return out, "", err
	}
	bundle, err := openKeys(in.File, in.Password)
	if err != nil {
		return out, home, err
	}
	se, err := readScopedEnv(home)
	if err != nil {
		return out, home, err
	}
	writes := map[string]map[string]string{}
	for _, e := range bundle.Keys {
		cur := se.values[e.Scope][e.Name]
		entry := mktapi.KeyEntry{Name: e.Name, Scope: e.Scope}
		switch {
		case cur == e.Value:
			entry.State = "same"
		case cur == "":
			entry.State = "new"
		default:
			entry.State = "differs"
		}
		out.Keys = append(out.Keys, entry)
		if entry.State == "new" || (entry.State == "differs" && in.Overwrite) {
			if writes[e.Scope] == nil {
				writes[e.Scope] = map[string]string{}
			}
			writes[e.Scope][e.Name] = e.Value
		}
	}
	if in.DryRun {
		return out, home, nil
	}
	scopes := make([]string, 0, len(writes))
	for scope := range writes {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	// Every global key the import writes is declared global (WriteGlobalEnv),
	// and so is one already in the file with the key set's value: written
	// undeclared, no member sees it. A global key holding another value the
	// import kept (differs, no overwrite) stays undeclared: it may be another
	// account's value, written undeclared on purpose.
	var declare []string
	for i, e := range bundle.Keys {
		if e.Scope == configset.ScopeGlobal && out.Keys[i].State == "same" {
			declare = append(declare, e.Name)
		}
	}
	if len(declare) > 0 && writes[configset.ScopeGlobal] == nil {
		writes[configset.ScopeGlobal] = map[string]string{}
		scopes = append(scopes, configset.ScopeGlobal)
	}
	for _, scope := range scopes {
		var changed []string
		var err error
		if scope == configset.ScopeGlobal {
			changed, err = configset.WriteGlobalEnv(se.paths[scope], writes[scope], declare)
		} else {
			changed, err = configset.WriteEnv(se.paths[scope], writes[scope])
		}
		if err != nil {
			return out, home, err
		}
		out.Written = append(out.Written, changed...)
	}
	written := map[string]bool{}
	for _, n := range out.Written {
		written[n] = true
	}
	for i := range out.Keys {
		if written[out.Keys[i].Name] {
			out.Keys[i].State = "written"
		}
	}
	return out, home, nil
}

// openKeys opens a keys file (keysfile).
func openKeys(file []byte, password string) (keysBundle, error) { return keysfile.Open(file, password) }

// ImportVault installs a published vault and imports its keys.
func (s *service) ImportVault(ctx context.Context, in mktapi.ImportVaultInput) (mktapi.ImportKeysOutput, error) {
	name, version, _ := strings.Cut(in.Name, "@")
	inst, err := s.Install(ctx, mktapi.ItemRef{Name: name, Version: version})
	if err != nil {
		return mktapi.ImportKeysOutput{}, err
	}
	if inst.Kind != string(mktapi.KindVault) {
		return mktapi.ImportKeysOutput{}, fmt.Errorf("%s is a %s item, not a vault", name, inst.Kind)
	}
	file, err := os.ReadFile(inst.FolderPath)
	if err != nil {
		return mktapi.ImportKeysOutput{}, err
	}
	return s.ImportKeys(ctx, mktapi.ImportKeysInput{UniverseHome: in.UniverseHome, File: file, Password: in.Password, Overwrite: in.Overwrite, DryRun: in.DryRun})
}
