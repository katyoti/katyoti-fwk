// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore/frozenhost"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// Marketplace-authoring methods — the publisher verbs, over the shared
// marketplace/authoring engine. Exposed as ComposeService methods so they are
// KTSP-callable, generate the flat CLI verbs, and are object-type-addressable
// actions (uniform with create-universe / create-sidecar).

// wsPath resolves a user-supplied path: absolute paths pass through; a relative
// path resolves against the calling universe's WORKSPACE (the hub injects
// KTW_WORKSPACE into the spawned sidecar, so Env().WorkspaceDir() is that
// universe's workspace) — NOT the sidecar process cwd; on a command line that
// names no universe, against the current folder. Empty stays empty.
//
// Leading/trailing whitespace is trimmed: a stray space from a config form (esp.
// a TRAILING one) is silently swallowed by Windows when a file is opened, but
// survives into `path + ".minisig"` — producing `…yaml .minisig` that the
// consumer (looking for `…yaml.minisig`) can never find.
func wsPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	// A command run from a shell naming no universe (`ktw-compose keygen
	// --folder keys`): the workspace is only the fallback one, and the person
	// means their current folder.
	if cenv.PreflightInfo().Source == "fallback" {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
	}
	if ws := application.Env().WorkspaceDir(); ws != "" {
		return filepath.Join(ws, p)
	}
	return p
}

func (s *Service) BuildMarketplace(_ context.Context, in api.BuildMarketplaceInput) (api.BuildMarketplaceOutput, error) {
	r, err := authoring.New().Build(wsPath(in.Source), in.Name, in.BaseURL, wsPath(in.Out))
	if err != nil {
		return api.BuildMarketplaceOutput{}, err
	}
	return api.BuildMarketplaceOutput{CatalogPath: r.CatalogPath, ItemCount: r.ItemCount, Skipped: r.Skipped}, nil
}

// ChannelMarketplace puts a published version on a channel.
func (s *Service) ChannelMarketplace(_ context.Context, in api.MarketplaceChannelInput) (api.MarketplaceChannelOutput, error) {
	r, err := authoring.New().MarkChannel(wsPath(in.Out), in.Version, in.Channel, in.SecretKeyPath)
	return api.MarketplaceChannelOutput{Items: r.Items, Warning: r.Warning}, err
}

// PromoteCandidate promotes a candidate to its official version on stable,
// its own artifacts - nothing rebuilt.
func (s *Service) PromoteCandidate(_ context.Context, in api.MarketplacePromoteInput) (api.MarketplaceChannelOutput, error) {
	r, err := authoring.New().Promote(wsPath(in.Out), in.From, in.To, in.SecretKeyPath)
	return api.MarketplaceChannelOutput{Items: r.Items, Warning: r.Warning}, err
}

func (s *Service) ReleaseMarketplace(_ context.Context, in api.ReleaseMarketplaceInput) (api.ReleaseMarketplaceOutput, error) {
	r, err := authoring.New().Release(wsPath(in.Source), in.Name, in.BaseURL, wsPath(in.Out), in.Keep)
	if err != nil {
		return api.ReleaseMarketplaceOutput{}, err
	}
	return api.ReleaseMarketplaceOutput{
		CatalogPath: r.CatalogPath,
		ItemCount:   r.ItemCount,
		Published:   r.Published,
		AssetCount:  r.AssetCount,
		Skipped:     r.Skipped,
		Pruned:      r.Pruned,
	}, nil
}

func (s *Service) SnapshotUniverse(_ context.Context, in api.SnapshotUniverseInput) (api.SnapshotUniverseOutput, error) {
	sd := authoring.SolutionSeed{}
	if in.Project != "" {
		sd.Project = wsPath(in.Project)
		if in.Tasks != "" {
			sd.Tasks = wsPath(in.Tasks)
		}
		sd.Name = in.SeedName
	}
	home, err := universeHome(in.Workspace)
	if err != nil {
		return api.SnapshotUniverseOutput{}, err
	}
	spec := authoring.SolutionSpec{ConfigSets: commaList(in.ConfigSets), Vaults: commaList(in.Vaults), Docs: wsPath(in.Docs), WhatsNew: wsPath(in.WhatsNew)}
	if spec.Others, err = otherUniverses(in); err != nil {
		return api.SnapshotUniverseOutput{}, err
	}
	r, err := authoring.New().AddSolution(home, in.Version, in.Name, catalogOf(wsPath(in.Marketplace)), secretKey(in.KeyFolder, in.KeyPrefix), sd, spec)
	if err != nil {
		return api.SnapshotUniverseOutput{}, err
	}
	members := make([]string, 0, len(r.Members))
	for _, m := range r.Members {
		v := m.Version
		if v == "" {
			v = "(unpinned)"
		}
		members = append(members, m.Name+"@"+v)
	}
	return api.SnapshotUniverseOutput{Name: r.Name, Version: r.Version, Members: members, Unpinned: r.Unpinned, Seed: r.Seed, SeedFiles: r.SeedFiles, Infra: r.Infra, Documents: r.Documents,
		Universes: r.Universes, OtherSeeds: r.OtherSeeds}, nil
}

// otherUniverses are the solution's other universes a snapshot names
// (--other <name>=<universe>), each with its seed (--other-project
// <name>=<folder>, its tasks the universe's own) and its config-sets
// (--other-config-set <name>=<config-set>@<version>). A seed or config-set
// for a universe no --other names is refused.
func otherUniverses(in api.SnapshotUniverseInput) ([]authoring.OtherUniverse, error) {
	var others []authoring.OtherUniverse
	at := map[string]int{}
	for _, o := range commaList(in.Others) {
		name, universe, err := namedValue("other", o)
		if err != nil {
			return nil, err
		}
		if _, dup := at[name]; dup {
			return nil, fmt.Errorf("--other %s is named twice", name)
		}
		home, err := universeHome(universe)
		if err != nil {
			return nil, fmt.Errorf("--other %s: %w", name, err)
		}
		at[name] = len(others)
		others = append(others, authoring.OtherUniverse{Name: name, Home: home})
	}
	of := func(flag, name string) (*authoring.OtherUniverse, error) {
		i, ok := at[name]
		if !ok {
			return nil, fmt.Errorf("--%s %s: no --other names the universe %s", flag, name, name)
		}
		return &others[i], nil
	}
	for _, p := range commaList(in.OtherProjects) {
		name, folder, err := namedValue("other-project", p)
		if err != nil {
			return nil, err
		}
		o, err := of("other-project", name)
		if err != nil {
			return nil, err
		}
		o.Seed.Project = wsPath(folder)
		if tasks := filepath.Join(application.EnvOf(o.Home).HomeDir(), "tasks"); isDir(tasks) {
			o.Seed.Tasks = tasks
		}
	}
	for _, c := range commaList(in.OtherConfigSets) {
		name, ref, err := namedValue("other-config-set", c)
		if err != nil {
			return nil, err
		}
		o, err := of("other-config-set", name)
		if err != nil {
			return nil, err
		}
		o.ConfigSets = append(o.ConfigSets, ref)
	}
	return others, nil
}

// namedValue splits a flag's <name>=<value>.
func namedValue(flag, v string) (string, string, error) {
	name, value, ok := strings.Cut(v, "=")
	name, value = strings.TrimSpace(name), strings.TrimSpace(value)
	if !ok || name == "" || value == "" {
		return "", "", fmt.Errorf("--%s must be <name>=<value>, got %q", flag, v)
	}
	return name, value, nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// commaList is a repeatable or comma-separated flag's values.
func commaList(in []string) []string {
	var out []string
	for _, v := range in {
		for _, one := range strings.Split(v, ",") {
			if one = strings.TrimSpace(one); one != "" {
				out = append(out, one)
			}
		}
	}
	return out
}

// BuildVault publishes a sealed keys file as a vault.
func (s *Service) BuildVault(_ context.Context, in api.BuildVaultInput) (api.BuildVaultOutput, error) {
	r, err := authoring.New().BuildVault(wsPath(in.File), in.Name, in.Version, wsPath(in.Out), in.Keep, secretKey(in.KeyFolder, in.KeyPrefix))
	if err != nil {
		return api.BuildVaultOutput{}, err
	}
	return api.BuildVaultOutput{Warning: r.Warning, CatalogPath: r.CatalogPath, Name: r.Name, Version: r.Version, AssetPath: r.AssetPath, Sha256: r.Sha256}, nil
}

// universeHome resolves a universe named by the user: a registered name is its
// home, anything else a folder (wsPath).
func universeHome(arg string) (string, error) {
	t := strings.TrimSpace(arg)
	if t != "" && !filepath.IsAbs(t) && !strings.ContainsAny(t, `/\`) {
		if regPath, err := orchcatalog.DefaultRegistryPath(); err == nil {
			if reg, err := orchcatalog.LoadRegistry(regPath); err == nil {
				if e, err := reg.Find(t); err == nil && e.HomePath != "" {
					return e.HomePath, nil
				}
			}
		}
	}
	home := wsPath(t)
	if st, err := os.Stat(home); err != nil || !st.IsDir() {
		return "", fmt.Errorf("universe %q is neither a registered universe nor a folder", arg)
	}
	return home, nil
}

// InitMarketplace makes a folder a marketplace to publish into.
func (s *Service) InitMarketplace(_ context.Context, in api.InitMarketplaceInput) (api.InitMarketplaceOutput, error) {
	r, err := authoring.New().InitMarketplace(wsPath(in.Folder), in.Name, wsPath(in.KeyFolder), strings.TrimSpace(in.KeyPrefix))
	if err != nil {
		return api.InitMarketplaceOutput{}, err
	}
	return api.InitMarketplaceOutput{Catalog: r.Catalog, KeyCreated: r.KeyCreated, PubKeyLine: r.PubKeyLine}, nil
}

// BuildInfraChannel publishes the control plane of a built tree into the
// marketplace's infra channel.
func (s *Service) BuildInfraChannel(_ context.Context, in api.BuildInfraChannelInput) (api.BuildInfraChannelOutput, error) {
	r, err := authoring.New().BuildInfraChannel(wsPath(in.Dist), wsPath(in.Output), strings.TrimSpace(in.Version), secretKey(in.KeyFolder, in.KeyPrefix), in.Demo)
	if err != nil {
		return api.BuildInfraChannelOutput{}, err
	}
	return api.BuildInfraChannelOutput{Channel: r.Channel, Version: r.Version, Previous: r.Previous, Platforms: r.Platforms, Pruned: r.Pruned}, nil
}

func (s *Service) Keygen(_ context.Context, in api.KeygenInput) (api.KeygenOutput, error) {
	prefix := strings.TrimSpace(in.Prefix)
	if prefix == "" {
		return api.KeygenOutput{}, fmt.Errorf("prefix (key filename prefix) is required")
	}
	pubFile, secFile, pubLine, err := authoring.New().Keygen(in.Comment)
	if err != nil {
		return api.KeygenOutput{}, err
	}
	// Keys land in Folder (relative → workspace, absolute as-is; created if
	// given) as <folder>/<prefix>.key(.pub).
	folder := wsPath(in.Folder)
	if folder != "" {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return api.KeygenOutput{}, fmt.Errorf("create key folder %q: %w", folder, err)
		}
	}
	base := filepath.Join(folder, prefix)
	secPath, pubPath := base+".key", base+".key.pub"
	if _, err := os.Stat(secPath); err == nil {
		return api.KeygenOutput{}, fmt.Errorf("%s already exists — refusing to overwrite a secret key", secPath)
	}
	if err := os.WriteFile(secPath, []byte(secFile), 0o600); err != nil {
		return api.KeygenOutput{}, err
	}
	if err := os.WriteFile(pubPath, []byte(pubFile), 0o644); err != nil {
		return api.KeygenOutput{}, err
	}
	return api.KeygenOutput{SecretKeyPath: secPath, PublicKeyPath: pubPath, PublicKeyLine: pubLine}, nil
}

func (s *Service) SignMarketplace(_ context.Context, in api.SignMarketplaceInput) (api.SignMarketplaceOutput, error) {
	keyPrefix := strings.TrimSpace(in.KeyPrefix)
	if keyPrefix == "" {
		return api.SignMarketplaceOutput{}, fmt.Errorf("key-prefix is required (the signing key is <key-folder>/<key-prefix>.key)")
	}
	// The secret key is <folder>/<prefix>.key — joined the same way keygen wrote
	// it (filepath.Join handles an empty folder cleanly). Relative → workspace.
	key := filepath.Join(wsPath(in.KeyFolder), keyPrefix) + ".key"
	sig, err := authoring.New().Sign(catalogOf(wsPath(in.Catalog)), key, in.Comment)
	if err != nil {
		return api.SignMarketplaceOutput{}, err
	}
	return api.SignMarketplaceOutput{SignaturePath: sig}, nil
}

// FreezeHost bundles a content folder into a single self-contained exe by
// appending it to a copy of the ktw-host binary (freeze-to-exe).
func (s *Service) FreezeHost(ctx context.Context, in api.FreezeHostInput) (api.FreezeHostOutput, error) {
	folder := wsPath(in.Folder)
	if _, err := os.Stat(filepath.Join(folder, "ktw-host.yaml")); err != nil {
		return api.FreezeHostOutput{}, fmt.Errorf("%s has no ktw-host.yaml — not a content folder", folder)
	}
	host := wsPath(in.Host)
	if host == "" {
		host = defaultHostPath()
	}
	hostBytes, err := os.ReadFile(host)
	if err != nil {
		return api.FreezeHostOutput{}, fmt.Errorf("read ktw-host binary %q (pass --host): %w", host, err)
	}
	if frozenhost.IsFrozen(hostBytes) {
		return api.FreezeHostOutput{}, fmt.Errorf("%s is ALREADY a frozen host — embed into a plain ktw-host", host)
	}
	// Sealing ships the content too, so it gets the same check as packaging.
	// The freeze path is not a way around it.
	if err := refusePackIfInvalid(ctx, host, folder); err != nil {
		return api.FreezeHostOutput{}, err
	}
	// Sealing is a distribution event: the folder stops being something its
	// author edits and becomes something that ships. Assert origin BEFORE
	// packing, so the assertions are inside what gets sealed.
	if _, err := authoring.SignFolder(host, folder, secretKey(in.KeyFolder, in.KeyPrefix), in.Publisher); err != nil {
		return api.FreezeHostOutput{}, fmt.Errorf("sign %s: %w", folder, err)
	}
	frozen, err := frozenhost.Pack(hostBytes, folder)
	if err != nil {
		return api.FreezeHostOutput{}, err
	}
	out := wsPath(in.Out)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return api.FreezeHostOutput{}, err
	}
	if err := os.WriteFile(out, frozen, 0o755); err != nil {
		return api.FreezeHostOutput{}, err
	}
	return api.FreezeHostOutput{ExePath: out, Bytes: len(frozen)}, nil
}

// defaultHostPath finds the ktw-host binary beside the running exe (dev bin/ or
// a bundle folder), else falls back to the bare name (PATH).
func defaultHostPath() string {
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "ktw-host"+suffix)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return "ktw-host" + suffix
}

func (s *Service) PublishMarketplace(_ context.Context, in api.PublishMarketplaceInput) (api.PublishMarketplaceOutput, error) {
	pubKey := ""
	if kp := strings.TrimSpace(in.KeyPrefix); kp != "" {
		pubKey = filepath.Join(wsPath(in.KeyFolder), kp) + ".key.pub"
	}
	sum, err := authoring.New().Publish(wsPath(in.Output), pubKey)
	if err != nil {
		return api.PublishMarketplaceOutput{}, err
	}
	return api.PublishMarketplaceOutput{Summary: sum}, nil
}

// RemoveVersion withdraws one published <item>@<version> from a marketplace and
// re-signs the catalog. Re-signing is mandatory (KeyPrefix required): a catalog
// edited without a fresh signature stops verifying for every consumer pinned to
// the key.
func (s *Service) RemoveVersion(_ context.Context, in api.RemoveVersionInput) (api.RemoveVersionOutput, error) {
	item, version, ok := strings.Cut(strings.TrimSpace(in.Ref), "@")
	if !ok || strings.TrimSpace(item) == "" || strings.TrimSpace(version) == "" {
		return api.RemoveVersionOutput{}, fmt.Errorf("ref must be <item>@<version>, got %q", in.Ref)
	}
	keyPrefix := strings.TrimSpace(in.KeyPrefix)
	if keyPrefix == "" {
		return api.RemoveVersionOutput{}, fmt.Errorf("key-prefix is required — the catalog must be re-signed after a removal")
	}
	key := filepath.Join(wsPath(in.KeyFolder), keyPrefix) + ".key"

	r, err := authoring.New().RemoveVersion(wsPath(in.Out), item, version, key, in.Comment, in.Force)
	if err != nil {
		return api.RemoveVersionOutput{}, err
	}
	return api.RemoveVersionOutput{
		CatalogPath:   r.CatalogPath,
		Item:          r.Item,
		Version:       r.Version,
		RemovedAssets: r.RemovedAssets,
		ItemRemoved:   r.ItemRemoved,
		SignaturePath: r.SignaturePath,
	}, nil
}

// BuildContent publishes a folder as a `kind: content` catalog item. Identity
// is read from the folder's ktw-host.yaml (see authoring.BuildContent).
func (s *Service) BuildContent(ctx context.Context, in api.BuildContentInput) (api.BuildContentOutput, error) {
	// Check BEFORE packaging: a pack that does not match its own declared
	// schemas must not become a distributable artifact. This is the one path
	// with no compiler in the loop, so it is the one that most needs it.
	if err := refusePackIfInvalid(ctx, defaultHostPath(), wsPath(in.Folder)); err != nil {
		return api.BuildContentOutput{}, err
	}
	// The host binary is what can answer "what does this folder publish?" —
	// packaging cannot read a files-only pack's contributions off disk, because
	// it does not have any until a host derives them.
	r, err := authoring.New().BuildContent(
		wsPath(in.Folder), in.Version, wsPath(in.Out), in.Keep,
		secretKey(in.KeyFolder, in.KeyPrefix), defaultHostPath(), in.Publisher)
	if err != nil {
		return api.BuildContentOutput{}, err
	}
	return api.BuildContentOutput{
		Warning:     r.Warning,
		CatalogPath: r.CatalogPath,
		Name:        r.Name,
		Version:     r.Version,
		AssetPath:   r.AssetPath,
		Sha256:      r.Sha256,
		Pruned:      r.Pruned,
		BulkLayers:  r.BulkLayers,
	}, nil
}

// BuildSeed packages a project folder (and a universe's tasks) into a
// `kind: seed` catalog version (see authoring.BuildSeed).
func (s *Service) BuildSeed(_ context.Context, in api.BuildSeedInput) (api.BuildSeedOutput, error) {
	var req []mktapi.PinnedRef
	for _, r := range in.Requires {
		for _, one := range strings.Split(r, ",") {
			if one = strings.TrimSpace(one); one == "" {
				continue
			}
			name, version, _ := strings.Cut(one, "@")
			req = append(req, mktapi.PinnedRef{Name: name, Version: version})
		}
	}
	tasks := ""
	if in.Tasks != "" {
		tasks = wsPath(in.Tasks)
	}
	r, err := authoring.New().BuildSeed(wsPath(in.Project), tasks, in.Name, in.Version, in.DisplayName, in.Description,
		req, wsPath(in.Out), in.Keep, secretKey(in.KeyFolder, in.KeyPrefix), in.Demo)
	if err != nil {
		return api.BuildSeedOutput{}, err
	}
	return api.BuildSeedOutput{Warning: r.Warning, CatalogPath: r.CatalogPath, Name: r.Name, Version: r.Version,
		AssetPath: r.AssetPath, Sha256: r.Sha256, Files: r.Files, Pruned: r.Pruned}, nil
}

// BuildConfigSet packages a config-set bundle into a `kind: config` catalog
// version. Validation is publisher-side on purpose (see authoring.BuildConfigSet).
func (s *Service) BuildConfigSet(_ context.Context, in api.BuildConfigSetInput) (api.BuildConfigSetOutput, error) {
	r, err := authoring.New().BuildConfigSet(
		wsPath(in.Folder), in.Name, in.Version, in.DisplayName, in.Description, wsPath(in.Out), in.Keep, secretKey(in.KeyFolder, in.KeyPrefix))
	if err != nil {
		return api.BuildConfigSetOutput{}, err
	}
	return api.BuildConfigSetOutput{
		Warning:     r.Warning,
		CatalogPath: r.CatalogPath,
		Name:        r.Name,
		Version:     r.Version,
		AssetPath:   r.AssetPath,
		Sha256:      r.Sha256,
		Keys:        r.Keys,
		Templates:   r.Templates,
		Pruned:      r.Pruned,
	}, nil
}

// secretKey resolves <key-folder>/<key-prefix>.key, the same way keygen wrote
// it. Empty prefix means "no key was supplied" — the caller then warns rather
// than silently leaving a signed catalog stale.
// catalogOf is a marketplace's catalog: a folder names its
// katyoti-marketplace.yaml, a file is taken as the catalog itself.
func catalogOf(p string) string {
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return filepath.Join(p, "katyoti-marketplace.yaml")
	}
	return p
}

func secretKey(folder, prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	return filepath.Join(wsPath(folder), prefix) + ".key"
}
