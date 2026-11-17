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
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/available"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configset"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/wire"
)

// ListConfigSets returns the `kind: config` items across enabled marketplaces —
// the same catalog walk SearchItems uses, filtered by kind so a user browsing
// config-sets is not shown runnable items.
func (s *service) ListConfigSets(ctx context.Context) ([]mktapi.ItemSummary, error) {
	cats, err := s.allCatalogs(ctx)
	if err != nil {
		return nil, err
	}
	out := []mktapi.ItemSummary{}
	for _, c := range cats {
		for _, it := range c.cat.Items {
			if it.Kind != mktapi.KindConfig {
				continue
			}
			out = append(out, mktapi.ItemSummary{
				Name:        it.Name,
				Kind:        it.Kind,
				DisplayName: it.DisplayName,
				Description: it.Description,
				Marketplace: c.marketplace,
				Latest:      latest(it),
			})
		}
	}
	return out, nil
}

// ShowConfigSet describes one config-set: its catalog versions always, and the
// keys it declares WHEN INSTALLED.
//
// The keys live inside the bundle, so they are only readable after an install
// unpacked it. Rather than silently fetching + unpacking on a read command, this
// reports Installed=false and leaves Keys empty — the caller can then say
// "install it to see its keys" instead of pretending the set declares none.
func (s *service) ShowConfigSet(ctx context.Context, in mktapi.ShowConfigSetInput) (mktapi.ShowConfigSetOutput, error) {
	var out mktapi.ShowConfigSetOutput

	mkt := ""
	if ins, err := s.installedItem(in.Name); err == nil {
		mkt = ins.Marketplace
	}
	it, resolvedMkt, _, err := s.findItem(ctx, in.Name, mkt)
	if err != nil {
		return out, err
	}
	if it.Kind != mktapi.KindConfig {
		return out, fmt.Errorf("%s is a %s item, not a config-set", in.Name, it.Kind)
	}
	out.Name = it.Name
	out.DisplayName = it.DisplayName
	out.Description = it.Description
	out.Marketplace = resolvedMkt
	for _, v := range it.Versions {
		out.Versions = append(out.Versions, v.Version)
	}

	ins, err := s.installedItem(in.Name)
	if err != nil {
		return out, nil // not installed — versions only
	}
	out.Installed = true
	out.Version = ins.Version

	folder := ins.FolderPath
	if folder == "" {
		if f, ferr := store.ConfigSetFolderPath(ins.Name, ins.Version); ferr == nil {
			folder = f
		}
	}
	rk, err := configset.Load(folder)
	if err != nil {
		return out, fmt.Errorf("%s@%s is recorded as installed but its bundle is unreadable: %w", ins.Name, ins.Version, err)
	}
	for _, k := range rk.Keys {
		out.Keys = append(out.Keys, mktapi.ConfigSetKey{
			Name:        k.Name,
			Description: k.Description,
			Secret:      k.Secret,
			Scope:       k.EffectiveScope(),
		})
	}
	return out, nil
}

// redacted is what a secret's current value is reported as. The caller is told
// the key HAS a value (so it can offer "keep current") without being handed the
// secret itself — this output crosses KTSP to a GUI.
const redacted = "********"

// ApplyConfigSet drops a config-set's templates into a universe and records the
// supplied key values into the scoped .env.
//
// It deliberately does NOT prompt. A service method has no TTY, and over KTSP
// there is no user at the far end — so it reports every aggregated key (with the
// current value as a default) plus which are still missing, and the caller (CLI
// prompt loop or GUI form) collects and calls again. The non-interactive path
// (--set K=V) is then the SAME code path, not a bypass.
//
// The aggregation is the point of the feature: the config-set's declared keys
// UNION whatever each wired sidecar's own config check still reports as needed,
// deduped by name with the manifest description winning. That is what turns
// "configure a universe" from a piecemeal hunt into one pass.
//
// An empty Name is the universe's OWN key set: the keys its members declare
// (each wired sidecar's config check), with no config-set — nothing is
// installed, no template copied, no member added, and the gate has nothing to
// hold; the values go through the same merge and write, so a key no member
// declares is still never written.
//
// Once keys are written, the running members that read one are restarted
func (s *service) ApplyConfigSet(ctx context.Context, in mktapi.ApplyConfigSetInput) (mktapi.ApplyConfigSetOutput, error) {
	out, err := s.applyConfigSet(ctx, in)
	if err != nil || in.DryRun {
		return out, err
	}
	out.Restarts = s.restartReaders(ctx, applyWrites(out))
	return out, nil
}

// applyConfigSet is ApplyConfigSet without the restart.
func (s *service) applyConfigSet(ctx context.Context, in mktapi.ApplyConfigSetInput) (mktapi.ApplyConfigSetOutput, error) {
	var out mktapi.ApplyConfigSetOutput

	// 1. Install if needed (idempotent — an already-placed version is not
	// refetched); the universe's own key set (no name) has nothing to install.
	ownKeys := strings.TrimSpace(in.Name) == ""
	rk, bundle := &configset.RequiredKeys{}, ""
	if !ownKeys {
		ins, err := s.Install(ctx, mktapi.ItemRef{Name: in.Name, Version: in.Version})
		if err != nil {
			return out, err
		}
		if ins.Kind != string(mktapi.KindConfig) {
			return out, fmt.Errorf("%s is a %s item, not a config-set", in.Name, ins.Kind)
		}
		out.Name, out.Version = ins.Name, ins.Version

		bundle = ins.FolderPath
		if bundle == "" {
			if f, ferr := store.ConfigSetFolderPath(ins.Name, ins.Version); ferr == nil {
				bundle = f
			}
		}
		if rk, err = configset.Load(bundle); err != nil {
			return out, err
		}
	}

	// 2. Resolve the target universe through the Env API — never compose these
	// paths by hand (the config dir lives under .katyoti/, which the Env owns).
	home := strings.TrimSpace(in.UniverseHome)
	if home == "" {
		home = application.Env().WorkspaceDir()
	}
	out.UniverseHome = home
	env := application.EnvOf(home)

	// 3. Current values (the defaults the caller shows) come from the scoped .env.
	uniEnvPath := filepath.Join(env.HomeDir(), ".env")
	globalEnvPath := filepath.Join(application.Env().GlobalHomeDir(), ".env")
	uniEnv, err := configset.ReadEnv(uniEnvPath)
	if err != nil {
		return out, err
	}
	globalEnv, err := configset.ReadEnv(globalEnvPath)
	if err != nil {
		return out, err
	}
	envFor := func(scope string) map[string]string {
		if scope == configset.ScopeGlobal {
			return globalEnv
		}
		return uniEnv
	}

	// 4. THE GATE — every non-optional key the config-set declares has a value
	// (given now, or already in its scope's .env) before anything is written.
	// A dry run reports them; a real apply is refused while one is missing.
	for _, k := range rk.Keys {
		if k.Optional {
			continue
		}
		if strings.TrimSpace(in.Values[k.Name]) == "" && strings.TrimSpace(envFor(k.EffectiveScope())[k.Name]) == "" {
			out.Blocking = append(out.Blocking, k.Name)
		}
	}
	if len(out.Blocking) > 0 && !in.DryRun {
		return out, fmt.Errorf("%s@%s needs %s before it is applied (nothing was written): give them (--set, --env-file, --keys) or mark them optional in its %s",
			out.Name, out.Version, strings.Join(out.Blocking, ", "), configset.ManifestFile)
	}

	// 4b. Every value given is one .env line: a line break would write lines
	// of its own (configset.CheckKeyValue). Refused before anything is copied
	// or written.
	for name, val := range in.Values {
		if err := configset.CheckKeyValue(name, val); err != nil {
			return out, fmt.Errorf("%s@%s: %w (nothing was written)", out.Name, out.Version, err)
		}
	}

	// 5. Copy templates (skipped when they exist, unless Force: clobbering a
	// config the user already tuned is not a side effect apply should have),
	// then MEMBERSHIP — before the keys are aggregated, so a sidecar wired by
	// this very call contributes its own required keys to the same answer.
	if !in.DryRun && !ownKeys {
		copied, skipped, err := copyTemplates(filepath.Join(bundle, configset.TemplateDir), env.ConfigDir(), in.Force)
		if err != nil {
			return out, err
		}
		out.TemplatesCopied, out.TemplatesSkipped = copied, skipped
		added, kept, err := s.ensureSolutionMembers(ctx, bundle, home)
		if err != nil {
			return out, err
		}
		out.MembersAdded, out.MembersKept = added, kept
	}

	// 6. Aggregate: manifest keys first (their descriptions win), then whatever
	// the universe's wired sidecars still report as needed.
	keys, order := map[string]*mktapi.ConfigSetKeyState{}, []string{}
	addKey := func(k mktapi.ConfigSetKeyState) {
		if _, seen := keys[k.Name]; seen {
			return // manifest wins; the first source recorded stays
		}
		keys[k.Name] = &k
		order = append(order, k.Name)
	}
	for _, k := range rk.Keys {
		addKey(mktapi.ConfigSetKeyState{
			Name: k.Name, Description: k.Description, Secret: k.Secret,
			Scope: k.EffectiveScope(), Source: "config-set", Optional: k.Optional,
		})
	}
	for _, k := range s.sidecarNeededKeys(ctx, home) {
		addKey(k)
	}

	// 6b. Write the supplied values into their scoped file (never in a dry run).
	uniVals, globalVals := map[string]string{}, map[string]string{}
	for name, val := range in.Values {
		st, known := keys[name]
		if !known {
			continue // never write a key nobody declared
		}
		if st.Scope == configset.ScopeGlobal {
			globalVals[name] = val
			globalEnv[name] = val
		} else {
			uniVals[name] = val
			uniEnv[name] = val
		}
	}
	if !in.DryRun {
		for _, w := range []struct {
			path string
			vals map[string]string
		}{{uniEnvPath, uniVals}, {globalEnvPath, globalVals}} {
			write := configset.WriteEnv
			if w.path == globalEnvPath {
				write = func(p string, v map[string]string) ([]string, error) { return configset.WriteGlobalEnv(p, v, nil) }
			}
			changed, werr := write(w.path, w.vals)
			if werr != nil {
				return out, werr
			}
			if len(changed) > 0 {
				out.EnvWritten = append(out.EnvWritten, w.path)
				out.KeysWritten = append(out.KeysWritten, changed...)
			}
		}
	}

	// 7. Report state: what is set (secrets redacted) and what is still missing.
	for _, name := range order {
		st := keys[name]
		if v, ok := envFor(st.Scope)[name]; ok && v != "" {
			st.Present = true
			st.Current = v
			if st.Secret {
				st.Current = redacted
			}
		} else {
			out.Missing = append(out.Missing, name)
		}
		out.Keys = append(out.Keys, *st)
	}
	sort.Strings(out.KeysWritten)
	return out, nil
}

// sidecarNeededKeys asks each of the universe's WIRED, marketplace-installed
// sidecars what its own config check still reports as missing, and turns those
// env/auth items into keys. Failures are skipped rather than fatal: a sidecar that
// cannot be checked must not block configuring the rest.
func (s *service) sidecarNeededKeys(ctx context.Context, universeHome string) []mktapi.ConfigSetKeyState {
	wired := wire.WiredNames(universeHome)
	if len(wired) == 0 {
		return nil
	}
	installed, err := available.List()
	if err != nil {
		return nil
	}
	var out []mktapi.ConfigSetKeyState
	for _, ins := range installed {
		if !wired[ins.Name] || ins.Source != "marketplace" {
			continue
		}
		// ASK THE SIDECAR ABOUT **THIS** UNIVERSE. An empty home makes the
		// check inherit the hub's own environment (runIn only sets
		// KTW_WORKSPACE when home != ""), so every sidecar reported what it
		// needed in the ADMIN universe — while `wired` above was correctly
		// scoped to the target. The form therefore listed the right SIDECARS
		// answering the wrong QUESTION: ktw-agents asked for ANTHROPIC_API_KEY
		// from the control plane's defaults, when this universe's freshly
		// copied LLMProviders.yaml uses ANTHROPIC_FOUNDRY_API_KEY and needs
		// nothing of the sort.
		rep, err := s.checkInstalled(ctx, universeHome, ins)
		if err != nil {
			continue
		}
		for _, item := range rep.Items {
			if item.Status == "ok" {
				continue
			}
			if item.Category != "env" && item.Category != "auth" {
				continue // config files + programs are not collectable key values
			}
			// ONLY WHAT THE SIDECAR SAYS IT REQUIRES.
			//
			// A sidecar declares every API key it CAN use, and reports each
			// unset one — `severity: error` when the key is required,
			// `severity: warn` when it merely supports it. Both were collected
			// here, so a universe wired to Anthropic Foundry was told it NEEDED
			// ANTHROPIC_API_KEY, OPENAI_API_KEY, SERP_API_KEY and four more it
			// will never call: alternatives the sidecar supports, badged as
			// requirements.
			//
			// Worse than noise: an unset key goes into `Missing`, so "all keys
			// set" could never be reached and a correctly configured universe
			// read as permanently half-done — which is exactly the signal this
			// form exists to give honestly.
			//
			// A genuinely REQUIRED key still gates the whole universe, which is
			// the documented behaviour (config-sets.md §6): a sidecar that
			// cannot run without a key still blocks, wherever that key was
			// declared.
			if item.Severity != "error" {
				continue
			}
			// A name a key set may not import (outside the key grammar, a
			// KTW_* variable) is never offered nor written: the sidecar's
			// report is not trusted to name a key.
			if configset.CheckKeyName(item.Name) != nil {
				continue
			}
			desc := item.Detail
			if desc == "" {
				desc = item.Hint
			}
			out = append(out, mktapi.ConfigSetKeyState{
				Name:        item.Name,
				Description: desc,
				Secret:      true, // a sidecar-reported credential is treated as secret
				Scope:       configset.ScopeUniverse,
				Source:      "sidecar:" + ins.Name,
			})
		}
	}
	return out
}

// copyTemplates copies every file from src into dst. An existing destination file
// is skipped (and reported) unless force — apply must not silently overwrite a
// config the user has already tuned.
func copyTemplates(src, dst string, force bool) (copied, skipped []string, err error) {
	entries, err := os.ReadDir(src)
	if os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("bundle has no %s/ directory at %s", configset.TemplateDir, src)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		target := filepath.Join(dst, e.Name())
		if _, serr := os.Stat(target); serr == nil && !force {
			skipped = append(skipped, e.Name())
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(src, e.Name()))
		if rerr != nil {
			return nil, nil, rerr
		}
		if werr := os.WriteFile(target, data, 0o644); werr != nil {
			return nil, nil, werr
		}
		copied = append(copied, e.Name())
	}
	sort.Strings(copied)
	sort.Strings(skipped)
	return copied, skipped, nil
}

// ensureSolutionMembers wires the config-set's declared sidecars into the
// universe, taking the LATEST version of any that is not already a member.
//
// AN ALREADY-WIRED MEMBER IS LEFT EXACTLY AS IT IS — not re-resolved, not
// re-versioned. A config-set says WHICH sidecars its config is for, not which
// build of them; silently moving a universe onto "latest" during an operation
// the user ran to change CONFIG would be a version change nobody asked for, and
// would undo a pin someone chose deliberately.
//
// This is also what lets SEVERAL config-sets target one universe: each
// contributes only what is missing, so membership ends up the UNION of their
// lists and the order they are applied in cannot change the result. Applying
// the same one twice is a no-op.
//
// A bundle with no members.yaml, or an empty list, wires nothing — the
// behaviour every config-set published before this had.
func (s *service) ensureSolutionMembers(ctx context.Context, bundle, home string) (added, kept []string, err error) {
	sol, err := configset.LoadMembers(bundle)
	if err != nil {
		return nil, nil, err
	}
	if len(sol.Sidecars) == 0 {
		return nil, nil, nil
	}
	wired := wire.WiredNames(home)
	for _, name := range sol.Sidecars {
		if wired[name] {
			kept = append(kept, name)
			continue
		}
		// Empty version = latest (pickVersion takes Versions[0], newest first).
		ins, ierr := s.Install(ctx, mktapi.ItemRef{Name: name})
		if ierr != nil {
			return nil, nil, fmt.Errorf("config-set member %q: install: %w", name, ierr)
		}
		if _, werr := wire.Into(ins, home); werr != nil {
			return nil, nil, fmt.Errorf("config-set member %q: wire into %s: %w", name, home, werr)
		}
		added = append(added, name+"@"+ins.Version)
	}
	return added, kept, nil
}
