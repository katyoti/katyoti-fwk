// SPDX-License-Identifier: Apache-2.0

// Package core implements the SetupService engine over the registry, catalog,
// and available-items subsystems: marketplaces, install, update, configure
// and the universe and project verbs of the katyoti front door.
package core

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar"
	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/available"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/catalog"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configure"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/prereqs"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/registry"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/wire"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

type service struct {
	reg *registry.Registry
	// prereqs installs programs; nil is the system engine (prereqs.go). A
	// test sets a fake runner: nothing real is installed by a test.
	prereqs *prereqs.Engine
	// dialHub and keysOf are how the members that read a key saved are found
	// and restarted (restart_readers.go); nil is the real hub and config
	// check. A test sets fakes: no hub runs, no check is spawned.
	dialHub hubDialer
	keysOf  keysReader
}

// New builds the SetupService engine.
func New() (mktapi.SetupService, error) {
	reg, err := registry.New()
	if err != nil {
		return nil, err
	}
	return &service{reg: reg}, nil
}

// infraOps carries the control-plane MUTATION methods, and it is a DISTINCT
// TYPE from *service for a reason that is easy to miss.
//
// The KTSP container reflects over the CONCRETE value it is handed, not over the
// interface it was registered as. Leaving these methods on *service therefore
// published them on the wire no matter what SetupService declared — the interface
// split alone looked right and changed nothing. They must not exist on the type
// the hub serves.
//
// It embeds *service so the engine (channel resolution, store install, the
// available registry) stays in ONE place.
type infraOps struct{ *service }

// NewInfra builds the engine behind the control-plane MUTATION interface, which
// is registered in-process only — never over KTSP.
func NewInfra() (mktapi.InfraService, error) {
	reg, err := registry.New()
	if err != nil {
		return nil, err
	}
	return &infraOps{&service{reg: reg}}, nil
}

// ── Marketplaces (subscription) ──────────────────────────────────────────

func (s *service) ListMarketplaces(_ context.Context) ([]mktapi.Marketplace, error) {
	return s.reg.List()
}
func (s *service) AddMarketplace(_ context.Context, in mktapi.AddMarketplaceInput) (mktapi.AddMarketplaceOutput, error) {
	if err := s.reg.Add(in.Name, in.URL, in.PubKey, in.Trust); err != nil {
		return mktapi.AddMarketplaceOutput{}, err
	}
	return mktapi.AddMarketplaceOutput{Name: in.Name}, nil
}
func (s *service) RemoveMarketplace(_ context.Context, in mktapi.RemoveMarketplaceInput) (mktapi.RemoveMarketplaceOutput, error) {
	if err := s.reg.Remove(in.Name); err != nil {
		return mktapi.RemoveMarketplaceOutput{}, err
	}
	return mktapi.RemoveMarketplaceOutput{Name: in.Name}, nil
}
func (s *service) SetMarketplaceEnabled(_ context.Context, in mktapi.SetMarketplaceEnabledInput) (mktapi.SetMarketplaceEnabledOutput, error) {
	if err := s.reg.SetEnabled(in.Name, in.Enabled); err != nil {
		return mktapi.SetMarketplaceEnabledOutput{}, err
	}
	return mktapi.SetMarketplaceEnabledOutput{Name: in.Name, Enabled: in.Enabled}, nil
}

func (s *service) SetMarketplaceChannel(_ context.Context, in mktapi.SetMarketplaceChannelInput) (mktapi.SetMarketplaceChannelOutput, error) {
	if err := s.reg.SetChannel(in.Name, in.Channel); err != nil {
		return mktapi.SetMarketplaceChannelOutput{}, err
	}
	return mktapi.SetMarketplaceChannelOutput{Name: in.Name, Channel: in.Channel}, nil
}

func (s *service) RefreshCatalog(ctx context.Context, in mktapi.RefreshCatalogInput) (mktapi.Catalog, error) {
	mkts, err := s.reg.List()
	if err != nil {
		return mktapi.Catalog{}, err
	}
	if in.Name != "" {
		for _, m := range mkts {
			if m.Name == in.Name {
				return catalog.Fetch(ctx, m)
			}
		}
		return mktapi.Catalog{}, fmt.Errorf("marketplace %q not found", in.Name)
	}
	agg := mktapi.Catalog{APIVersion: 1, Marketplace: mktapi.CatalogMeta{Name: "(all)"}}
	for _, m := range mkts {
		if !m.Enabled {
			continue
		}
		cat, err := catalog.Fetch(ctx, m)
		if err != nil {
			continue
		}
		agg.Items = append(agg.Items, cat.Items...)
	}
	return agg, nil
}

// ── Catalog (read) ───────────────────────────────────────────────────────

type catalogWithSource struct {
	marketplace string
	url         string // the catalog URL — relative asset URLs resolve against its dir
	cat         mktapi.Catalog
}

// allCatalogs fetches every enabled marketplace's catalog, skipping failures.
func (s *service) allCatalogs(ctx context.Context) ([]catalogWithSource, error) {
	mkts, err := s.reg.List()
	if err != nil {
		return nil, err
	}
	var out []catalogWithSource
	for _, m := range mkts {
		if !m.Enabled {
			continue
		}
		cat, err := catalog.Fetch(ctx, m)
		if err != nil {
			continue
		}
		out = append(out, catalogWithSource{marketplace: m.Name, url: m.URL, cat: cat})
	}
	return out, nil
}

func (s *service) SearchItems(ctx context.Context, in mktapi.SearchItemsInput) ([]mktapi.ItemSummary, error) {
	cats, err := s.allCatalogs(ctx)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(in.Query))
	// NOT `var out []T`. A nil slice marshals to JSON `null`, and the generated
	// client declares this output as an array — so the GUI iterates it. The
	// empty case is the FIRST thing a new user meets (no marketplace
	// subscribed, nothing matching), and it is the one that broke.
	out := emptyItemSummaries()
	for _, c := range cats {
		for _, it := range c.cat.Items {
			if q != "" && !matches(it, q) {
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
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *service) GetItem(ctx context.Context, in mktapi.GetItemInput) (mktapi.Item, error) {
	it, _, _, err := s.findItem(ctx, in.Name, "")
	return it, err
}

// findItem returns an item + the marketplace name + the catalog URL that served
// it (the URL is the base for resolving the item's relative asset URLs).
//
// When marketplace != "", the lookup is qualified to that one marketplace — the
// install must come from the catalog the caller chose, never the first name
// match. When marketplace == "" and exactly one enabled marketplace serves the
// name, that one is used; when several do, the name is AMBIGUOUS and refused
// (naming the marketplaces to qualify with) rather than silently first-matched.
func (s *service) findItem(ctx context.Context, name, marketplace string) (mktapi.Item, string, string, error) {
	cats, err := s.allCatalogs(ctx)
	if err != nil {
		return mktapi.Item{}, "", "", err
	}
	var matches []catalogWithSource
	var hits []mktapi.Item
	for _, c := range cats {
		for _, it := range c.cat.Items {
			if it.Name != name {
				continue
			}
			if marketplace != "" && c.marketplace != marketplace {
				continue
			}
			matches = append(matches, c)
			hits = append(hits, it)
		}
	}
	switch {
	case len(hits) == 1:
		return hits[0], matches[0].marketplace, matches[0].url, nil
	case len(hits) > 1:
		return mktapi.Item{}, "", "", fmt.Errorf(
			"item %q is served by %d marketplaces (%s); qualify which one to install from",
			name, len(hits), strings.Join(marketplaceNames(matches), ", "))
	case marketplace != "":
		return mktapi.Item{}, "", "", fmt.Errorf("item %q not found in marketplace %q", name, marketplace)
	default:
		return mktapi.Item{}, "", "", fmt.Errorf("item %q not found in any enabled marketplace", name)
	}
}

func marketplaceNames(cs []catalogWithSource) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.marketplace
	}
	return out
}

// resolveAssetURL makes a (possibly relative) asset URL absolute by resolving it
// against the CATALOG's location — so a portable folder (assets beside the
// catalog) works copied/hosted anywhere without rewriting (and re-signing) the
// catalog. Absolute assets (http(s)://, file://, or an absolute path) pass
// through. A relative asset resolves against the catalog URL's directory:
// http(s) via RFC-3986 reference resolution; a local/file catalog via filepath.
func resolveAssetURL(catalogURL, assetURL string) string {
	if assetURL == "" {
		return assetURL
	}
	if strings.HasPrefix(assetURL, "http://") || strings.HasPrefix(assetURL, "https://") ||
		strings.HasPrefix(assetURL, "file://") || filepath.IsAbs(assetURL) {
		return assetURL
	}
	if strings.HasPrefix(catalogURL, "http://") || strings.HasPrefix(catalogURL, "https://") {
		base, err := url.Parse(catalogURL)
		if err != nil {
			return assetURL
		}
		ref, err := url.Parse(assetURL)
		if err != nil {
			return assetURL
		}
		return base.ResolveReference(ref).String()
	}
	// local / file catalog: resolve against the catalog file's directory.
	catPath := strings.TrimPrefix(catalogURL, "file://")
	return filepath.Join(filepath.Dir(catPath), filepath.FromSlash(assetURL))
}

func latest(it mktapi.Item) string {
	if len(it.Versions) == 0 {
		return ""
	}
	return it.Versions[0].Version
}

func matches(it mktapi.Item, q string) bool {
	if strings.Contains(strings.ToLower(it.Name), q) ||
		strings.Contains(strings.ToLower(it.DisplayName), q) ||
		strings.Contains(strings.ToLower(it.Description), q) {
		return true
	}
	for _, t := range it.Tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

// ── Available ────────────────────────────────────────────────────────────

func (s *service) ListInstalled(_ context.Context) ([]mktapi.Installed, error) {
	items, err := available.List()
	if err != nil {
		return nil, err
	}
	// An empty registry is the normal state of a fresh machine, and it must
	// still reach the client as `[]`. See emptyInstalled.
	// What a dev installation holds: the control-plane programs marked, each
	// item of its dev channel shown as that build.
	items = withDevBuilds(items)
	if items == nil {
		items = emptyInstalled()
	}
	// Annotate USED state across EVERY registered universe, and say which.
	// "the current universe" is admin when this is served over KTSP, so a caller
	// asking about the universe it has open was answered about a different one.
	byItem := wire.WiringByItem()
	pins := wire.PinsByItem()
	for i := range items {
		in := byItem[items[i].Name]
		items[i].WiredIn = in
		items[i].Wired = len(in) > 0
		items[i].Pins = pins[items[i].Name]
	}
	return items, nil
}

// ── Install / Update → the versioned store (ADR §14) ─────────────────────

func (s *service) Install(ctx context.Context, ref mktapi.ItemRef) (mktapi.Installed, error) {
	ctx, st := beginSteps(ctx, 1)
	st.begin("Installing " + itemLabel(ref.Name, ref.Version))
	inst, err := s.install(ctx, ref)
	if err == nil {
		st.end("Installed " + itemLabel(inst.Name, inst.Version))
	}
	return inst, err
}

// itemLabel is name@version, name alone for the latest.
func itemLabel(name, version string) string {
	if version == "" {
		return name
	}
	return name + "@" + version
}

func (s *service) install(ctx context.Context, ref mktapi.ItemRef) (mktapi.Installed, error) {
	it, mkt, catURL, err := s.findItem(ctx, ref.Name, ref.Marketplace)
	if err != nil {
		return mktapi.Installed{}, err
	}
	switch it.Kind {
	case mktapi.KindSidecar:
		return s.installSidecar(ctx, it, mkt, catURL, ref.Version)
	case mktapi.KindContent:
		return s.installContent(ctx, it, mkt, catURL, ref.Version)
	case mktapi.KindConfig:
		return s.installConfigSet(ctx, it, mkt, catURL, ref.Version)
	case mktapi.KindSeed:
		return s.installSeed(ctx, it, mkt, catURL, ref.Version)
	case mktapi.KindVault:
		return s.installVault(ctx, it, mkt, catURL, ref.Version)
	default:
		// A solution is not installed as an item into the store: it is
		// installed into a universe, which installs each member it pins.
		if it.Kind == mktapi.KindSolution {
			return mktapi.Installed{}, fmt.Errorf(
				"%s is a solution: install it into a universe with `katyoti solution install %s --universe <name>`", it.Name, it.Name)
		}
		return mktapi.Installed{}, fmt.Errorf("install: unknown kind %q (sidecar, content, config, seed, vault)", it.Kind)
	}
}

// installSidecar fetches + places a compiled binary in the versioned store.
func (s *service) installSidecar(ctx context.Context, it mktapi.Item, mkt, catURL, version string) (mktapi.Installed, error) {
	ver, iv, err := pickVersion(it, version)
	if err != nil {
		return mktapi.Installed{}, fmt.Errorf("marketplace %q: %w", mkt, err)
	}
	// Before any download: a sidecar writing a kind in a newer major than
	// this platform reads would have its entries of that kind refused. The
	// built-in reader knows every kind compiled in, a pack-declared one too.
	if err := RefuseNewerFormats(it.Name, ver, iv.Requires.Formats, builtin.Formats()); err != nil {
		return mktapi.Installed{}, err
	}
	asset, err := pickAsset(iv)
	if err != nil {
		return mktapi.Installed{}, fmt.Errorf("%s@%s: %w", it.Name, ver, err)
	}
	asset.URL = resolveAssetURL(catURL, asset.URL) // relative → resolved against the catalog
	if asset.Bundle != nil {
		// A copy: the catalog's own bundle stays as published.
		b := *asset.Bundle
		b.URL = resolveAssetURL(catURL, b.URL)
		asset.Bundle = &b
	}
	path, err := store.InstallSidecar(ctx, it.Name, ver, asset)
	if err != nil {
		return mktapi.Installed{}, err
	}
	inst := mktapi.Installed{
		Name:        it.Name,
		Kind:        string(it.Kind),
		Version:     ver,
		Source:      "marketplace",
		Marketplace: mkt,
		Sha256:      asset.Sha256,
		BinaryPath:  path,
	}
	if err := available.Record(inst); err != nil {
		return inst, err
	}
	return inst, nil
}

// installContent fetches + unpacks a files-only folder bundle into the content
// store; the recorded FolderPath is the `ktw-host --folder` wire target.
func (s *service) installContent(ctx context.Context, it mktapi.Item, mkt, catURL, version string) (mktapi.Installed, error) {
	ver, iv, err := pickVersion(it, version)
	if err != nil {
		return mktapi.Installed{}, fmt.Errorf("marketplace %q: %w", mkt, err)
	}
	asset, err := pickAsset(iv) // content asset is platform-agnostic (OS/Arch empty)
	if err != nil {
		return mktapi.Installed{}, fmt.Errorf("%s@%s: %w", it.Name, ver, err)
	}
	asset.URL = resolveAssetURL(catURL, asset.URL) // relative → resolved against the catalog
	folder, err := store.InstallContent(ctx, it.Name, ver, asset)
	if err != nil {
		return mktapi.Installed{}, err
	}
	inst := mktapi.Installed{
		Name:        it.Name,
		Kind:        string(it.Kind),
		Version:     ver,
		Source:      "marketplace",
		Marketplace: mkt,
		Sha256:      asset.Sha256,
		FolderPath:  folder,
	}
	if err := available.Record(inst); err != nil {
		return inst, err
	}
	// A content item runs on ktw-host: installed with it when a marketplace
	// offers it and none is installed, so its wiring never takes whatever
	// ktw-host the PATH happens to hold. None offered: the wiring falls back
	// as before (wire.HostBinaryPath).
	if it.Name != "ktw-host" {
		_ = s.ensureContentHost(ctx, mkt)
	}
	return inst, nil
}

// ensureContentHost installs ktw-host — from the content's marketplace, else
// any subscribed one — unless one installed from a marketplace is there (a
// dev binary found on PATH does not count: it is what wiring must not take).
func (s *service) ensureContentHost(ctx context.Context, mkt string) error {
	if items, err := available.List(); err == nil {
		for _, it := range items {
			if it.Name == "ktw-host" && it.Source == "marketplace" && it.BinaryPath != "" {
				if _, err := os.Stat(it.BinaryPath); err == nil {
					return nil
				}
			}
		}
	}
	// Only a compiled ktw-host: a catalog naming a content item ktw-host would
	// otherwise send this back into installContent.
	for _, m := range []string{mkt, ""} {
		if it, _, _, err := s.findItem(ctx, "ktw-host", m); err == nil && it.Kind == mktapi.KindSidecar {
			_, err := s.Install(ctx, mktapi.ItemRef{Name: "ktw-host", Marketplace: m})
			return err
		}
	}
	return fmt.Errorf("no marketplace offers a ktw-host sidecar")
}

// installConfigSet fetches + unpacks a config-set bundle into the config-set
// store. Unlike a sidecar or content item it is never wired or run — the recorded
// FolderPath is what `config-set apply` reads templates + keys.yaml from.
func (s *service) installConfigSet(ctx context.Context, it mktapi.Item, mkt, catURL, version string) (mktapi.Installed, error) {
	ver, iv, err := pickVersion(it, version)
	if err != nil {
		return mktapi.Installed{}, fmt.Errorf("marketplace %q: %w", mkt, err)
	}
	asset, err := pickAsset(iv) // one platform-agnostic bundle per version (OS/Arch empty)
	if err != nil {
		return mktapi.Installed{}, fmt.Errorf("%s@%s: %w", it.Name, ver, err)
	}
	asset.URL = resolveAssetURL(catURL, asset.URL)
	folder, err := store.InstallConfigSet(ctx, it.Name, ver, asset)
	if err != nil {
		return mktapi.Installed{}, err
	}
	inst := mktapi.Installed{
		Name:        it.Name,
		Kind:        string(it.Kind),
		Version:     ver,
		Source:      "marketplace",
		Marketplace: mkt,
		Sha256:      asset.Sha256,
		FolderPath:  folder,
	}
	if err := available.Record(inst); err != nil {
		return inst, err
	}
	return inst, nil
}

func (s *service) Uninstall(_ context.Context, in mktapi.UninstallInput) (mktapi.UninstallOutput, error) {
	name := in.Name
	// A version-qualified uninstall retires ONE store version and leaves the rest
	// (and the available record) alone.
	if v := strings.TrimSpace(in.Version); v != "" {
		return s.uninstallVersion(name, v)
	}
	// Guard: only marketplace-managed items are uninstallable. A discovered dev
	// binary (source=executable-dir) isn't in the store or the record — say so
	// instead of silently no-op'ing.
	ins, err := s.installedItem(name)
	if err != nil {
		return mktapi.UninstallOutput{}, err
	}
	if ins.Source != "marketplace" {
		return mktapi.UninstallOutput{}, fmt.Errorf("%q is a discovered %s binary (source: %s), not a marketplace install — nothing to uninstall", name, ins.Kind, ins.Source)
	}
	// Guard: don't strand a universe pointing at a deleted store path.
	//
	// Scoped to EVERY registered universe, not "the current" one. Universes live
	// centrally under the global root, so a CLI run stands in none of them and
	// over KTSP the current universe is always admin — the old `WiredNames("")`
	// guard therefore never fired. `katyoti uninstall <name>` deleted every store
	// version of an item other universes pinned, reported success, and left them
	// pointing at a path that no longer existed.
	if users := wire.UniversesWiring(name); len(users) > 0 {
		return mktapi.UninstallOutput{}, fmt.Errorf(
			"%q is wired into %s — unwire it first (`katyoti universe unwire %s --universe <name>`)",
			name, strings.Join(wire.RefNames(users), ", "), name)
	}
	// ktw-host runs every content member without being a member itself.
	if users := wire.UniversesHosting(name); len(users) > 0 {
		return mktapi.UninstallOutput{}, fmt.Errorf(
			"%q runs the content of %s — unwire their content items first", name, strings.Join(wire.RefNames(users), ", "))
	}
	if err := store.UninstallAll(name); err != nil {
		return mktapi.UninstallOutput{}, err
	}
	if err := available.RemoveInstalled(name); err != nil {
		return mktapi.UninstallOutput{}, err
	}
	return mktapi.UninstallOutput{Name: name}, nil
}

// uninstallVersion removes a SINGLE version from the store.
//
// It refuses to remove the version the available record currently points at.
// That version is the "current" one: the wired universe pin (when the item is
// wired) always tracks it, because SetVersion repoints the pin whenever the
// current version changes. So this one rule prevents BOTH a dangling install
// record and a universe left pinned at a deleted store path — while still
// letting old versions be reclaimed freely. To remove the current version, take
// the whole item (`uninstall <name>`) or switch first (`update <name> --to …`).
func (s *service) uninstallVersion(name, version string) (mktapi.UninstallOutput, error) {
	ins, err := s.installedItem(name)
	if err != nil {
		return mktapi.UninstallOutput{}, err
	}
	if ins.Source != "marketplace" {
		return mktapi.UninstallOutput{}, fmt.Errorf("%q is a discovered %s binary (source: %s), not a marketplace install — nothing to uninstall", name, ins.Kind, ins.Source)
	}
	if ins.Version == version {
		return mktapi.UninstallOutput{}, fmt.Errorf(
			"%s@%s is the current version — remove the whole item (`katyoti uninstall %s`) or switch first (`katyoti update %s --to <version>`)",
			name, version, name, name)
	}
	if !store.HasAnyVersion(name, version) {
		return mktapi.UninstallOutput{}, fmt.Errorf("%s@%s is not installed", name, version)
	}
	// A universe may pin an older version on purpose (one installed from a
	// solution): removing that version would leave it pointing at a deleted path.
	if users := wire.UniversesPinning(name, version); len(users) > 0 {
		return mktapi.UninstallOutput{}, fmt.Errorf(
			"%s@%s is what %s run — move each to another version first (`katyoti universe wire %s --universe <name>`), or keep it",
			name, version, strings.Join(wire.RefNames(users), ", "), name)
	}
	if err := store.UninstallVersion(name, version); err != nil {
		return mktapi.UninstallOutput{}, err
	}
	return mktapi.UninstallOutput{Name: name, Version: version}, nil
}

func (s *service) Unwire(_ context.Context, in mktapi.UnwireInput) (mktapi.UnwireOutput, error) {
	home, _, err := wire.OutOf(in.Name, in.UniverseHome)
	return mktapi.UnwireOutput{Home: home}, err
}

// Project attach/detach + universe create/list are owned by the registry-backed UniverseService
// (orchestrator) — the `universe` command resolves it directly. They were
// removed from SetupService (universe-service-consolidation).

func (s *service) Outdated(ctx context.Context) ([]mktapi.Update, error) {
	installed, err := available.List()
	if err != nil {
		return nil, err
	}
	// An item the installation runs from its dev channel follows that channel
	// - make dev moves it - so a marketplace's newer version is not "behind"
	// for it, even with a store copy kept.
	dev := devBuilds()
	var out []mktapi.Update
	for _, ins := range installed {
		if ins.Source != "marketplace" {
			continue // only marketplace-installed items have a catalog to diff against
		}
		if _, ok := dev[ins.Name]; ok {
			continue
		}
		// CONFIG-SETS ARE NOT UPDATABLE IN PLACE, so reporting them here would
		// invite an update that does nothing where it matters.
		//
		// A config-set reaches a universe through ApplyConfigSet, which drops
		// templates and collects keys. Nothing records WHICH universe applied
		// WHICH config-set, so there is no set of places to re-apply to. An
		// "update" therefore lands a new version in the store and leaves every
		// universe on the old templates — reporting success for a no-op, which
		// is worse than reporting nothing at all.
		//
		// `config-set apply` is the real propagation path; `config-set show`
		// reports available versions. See exclude-config-sets-from-update.
		// Seeds and vaults likewise: a seed is applied to a universe, a vault
		// imported into it — a newer one in the store changes nothing there.
		if !updatableInPlace(ins.Kind) {
			continue
		}
		it, _, _, err := s.findItem(ctx, ins.Name, ins.Marketplace)
		if err != nil {
			continue
		}
		if l := latest(it); l != "" && l != ins.Version {
			out = append(out, mktapi.Update{Name: ins.Name, Installed: ins.Version, Latest: l, Marketplace: ins.Marketplace})
		}
	}
	return out, nil
}

// updatableInPlace says whether a newer version of an installed item of this
// kind reaches what uses it by moving the store: a sidecar or content item
// (its universes adopt it), never a config-set, a seed or a vault (applied or
// imported into a universe once).
func updatableInPlace(kind string) bool {
	switch mktapi.ItemKind(kind) {
	case mktapi.KindConfig, mktapi.KindSeed, mktapi.KindVault:
		return false
	}
	return true
}

func (s *service) Update(ctx context.Context, in mktapi.UpdateInput) ([]mktapi.Update, error) {
	if err := refuseWhenLocked("update"); err != nil {
		return nil, err
	}
	ups, err := s.Outdated(ctx)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, n := range in.Names {
		want[n] = true
	}
	var moves []mktapi.Update
	for _, u := range ups {
		if len(in.Names) == 0 || want[u.Name] {
			moves = append(moves, u)
		}
	}
	ctx, st := beginSteps(ctx, len(moves))
	var done []mktapi.Update
	for _, u := range moves {
		st.begin("Updating " + u.Name + " to " + u.Latest)
		// Move the STORE and every universe that runs a store version of the
		// item: an update that left them on the old version
		// left them on a program the store may no longer hold. A universe
		// installed from a solution pins its members on purpose: it keeps a
		// version that still exists and is reported behind, for its
		// solution's update or `universe wire --universe <name>`.
		rep, err := s.setVersion(ctx, mktapi.SetVersionInput{Name: u.Name, Version: u.Latest})
		if err != nil {
			return done, err
		}
		u.Universes, u.Behind = rep.moved, rep.behind
		done = append(done, u)
		st.end("Updated " + u.Name + " to " + u.Latest)
	}
	return done, nil
}

// ItemVersions lists an item's catalog versions newest-first, qualified by the
// item's installed marketplace when it is installed (so a name served by several
// marketplaces resolves the one the install came from).
func (s *service) ItemVersions(ctx context.Context, in mktapi.ItemVersionsInput) (mktapi.ItemVersionsOutput, error) {
	mkt := ""
	if ins, err := s.installedItem(in.Name); err == nil {
		mkt = ins.Marketplace
	}
	it, _, _, err := s.findItem(ctx, in.Name, mkt)
	if err != nil {
		return mktapi.ItemVersionsOutput{}, err
	}
	out := make([]string, len(it.Versions))
	for i, v := range it.Versions {
		out[i] = v.Version
	}
	return mktapi.ItemVersionsOutput{Versions: out}, nil
}

// SetVersion makes version current (install idempotently — no re-fetch of an
// already-placed version, newer versions kept) and repoints the universe pin
// when the item is wired. version "" = latest.
func (s *service) SetVersion(ctx context.Context, in mktapi.SetVersionInput) (mktapi.Installed, error) {
	ctx, st := beginSteps(ctx, 1)
	st.begin("Setting " + itemLabel(in.Name, in.Version))
	rep, err := s.setVersion(ctx, in)
	if err == nil {
		st.end(itemLabel(rep.inst.Name, rep.inst.Version) + " is current")
	}
	return rep.inst, err
}

// versionMove is what a version change did: the installed record, the
// universes repinned to it and the solutions' universes left behind.
type versionMove struct {
	inst          mktapi.Installed
	moved, behind []string
}

// setVersion is SetVersion. Given a universe, it repoints that universe's pin;
// given none (the Marketplace's update, a version picked for the machine), it
// repoints every universe that runs another store version of the item
// (wire.RepinToInstalled) and restarts, through each universe's hub, the
// repinned members that run.
func (s *service) setVersion(ctx context.Context, in mktapi.SetVersionInput) (versionMove, error) {
	var out versionMove
	inst, err := s.setVersionOne(ctx, in)
	out.inst = inst
	if err != nil || in.UniverseHome != "" {
		return out, err
	}
	moved, behind, err := wire.RepinToInstalled(inst)
	for _, b := range behind {
		out.behind = append(out.behind, b.Name)
	}
	byHome := map[string][]string{}
	for _, r := range moved {
		out.moved = append(out.moved, r.Universe.Name)
		byHome[r.Universe.Home] = append(byHome[r.Universe.Home], inst.Name)
	}
	if err != nil {
		return out, fmt.Errorf("installed %s@%s but failed to repoint the universes that run it: %w", inst.Name, inst.Version, err)
	}
	s.restartRepinned(ctx, byHome)
	return out, nil
}

func (s *service) setVersionOne(ctx context.Context, in mktapi.SetVersionInput) (mktapi.Installed, error) {
	mkt := ""
	if ins, err := s.installedItem(in.Name); err == nil {
		mkt = ins.Marketplace
		// Moving an installed item's version is an update.
		if ins.Version != in.Version {
			if err := refuseWhenLocked("set-version " + in.Name); err != nil {
				return mktapi.Installed{}, err
			}
		}
	}
	inst, err := s.Install(ctx, mktapi.ItemRef{Name: in.Name, Version: in.Version, Marketplace: mkt})
	if err != nil {
		return inst, err
	}
	if wire.WiredNames(in.UniverseHome)[in.Name] {
		if _, err := s.WireIntoUniverse(ctx, mktapi.WireIntoUniverseInput{Name: in.Name, UniverseHome: in.UniverseHome}); err != nil {
			return inst, fmt.Errorf("installed %s@%s but failed to repoint the universe pin: %w", in.Name, inst.Version, err)
		}
	}
	return inst, nil
}

// ── Configure → USED (drive each sidecar's own config surface) ───────────

func (s *service) ConfigureItem(ctx context.Context, in mktapi.ConfigureItemInput) (sidecarapi.ConfigCheckReport, error) {
	ins, err := s.installedItem(in.Name)
	if err != nil {
		return sidecarapi.ConfigCheckReport{}, err
	}
	return s.configureInstalled(ctx, in.UniverseHome, ins)
}

// configureInstalled drives an installed item's own config surface. Compiled
// sidecars run `<binary> sidecar config …`; content items run the same verbs
// through the generic host (`ktw-host --folder <folder> …`) so a files-only
// sidecar's declared config (from its ktw-host.yaml) is checked identically.
//
// home is the universe whose config dir the templates must land in. Pass "" ONLY
// when the caller genuinely means the universe this process is running in — over
// KTSP that is ADMIN, so anything acting on a named universe must pass its home.
func (s *service) configureInstalled(ctx context.Context, home string, ins mktapi.Installed) (sidecarapi.ConfigCheckReport, error) {
	if isFolderMember(ins) {
		// Nothing runs a folder member: its config: block is checked in
		// process. It ships no config file defaults to set.
		return checkFolderMember(ctx, home, ins)
	}
	bin, pre, err := configSurface(ins)
	if err != nil {
		return sidecarapi.ConfigCheckReport{}, err
	}
	return configure.ConfigureWith(ctx, home, bin, pre)
}

// checkInstalled is the read-only variant of configureInstalled (no `config
// set`), used by Doctor.
func (s *service) checkInstalled(ctx context.Context, home string, ins mktapi.Installed) (sidecarapi.ConfigCheckReport, error) {
	if isFolderMember(ins) {
		return checkFolderMember(ctx, home, ins)
	}
	bin, pre, err := configSurface(ins)
	if err != nil {
		return sidecarapi.ConfigCheckReport{}, err
	}
	return configure.CheckWith(ctx, home, bin, pre)
}

// isFolderMember says whether an installed content item is wired as a folder
// member (no scripts/: the hub reads it, nothing runs).
func isFolderMember(ins mktapi.Installed) bool {
	if ins.Kind != "content" || ins.FolderPath == "" {
		return false
	}
	fi, err := os.Stat(filepath.Join(ins.FolderPath, "scripts"))
	return err != nil || !fi.IsDir()
}

// checkFolderMember checks a folder member's declared config (its ktw-host.yaml
// `config:` block) in process, as the generic host would report it.
func checkFolderMember(ctx context.Context, home string, ins mktapi.Installed) (sidecarapi.ConfigCheckReport, error) {
	var decl struct {
		Config sidecar.FolderConfig `yaml:"config"`
	}
	raw, err := os.ReadFile(filepath.Join(ins.FolderPath, "ktw-host.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return sidecarapi.ConfigCheckReport{}, fmt.Errorf("%q: read ktw-host.yaml: %w", ins.Name, err)
	}
	if len(raw) > 0 {
		if err := config.DecodeBytes(raw, marshal.YAML, &decl); err != nil {
			return sidecarapi.ConfigCheckReport{}, fmt.Errorf("%q: ktw-host.yaml: %w", ins.Name, err)
		}
	}
	return sidecar.CheckFolderConfig(ctx, home, ins.Name, decl.Config)
}

// configSurface resolves HOW an installed item's own `sidecar config …` verbs
// are reached: the item's binary for a compiled sidecar, or the generic host
// with `--folder <folder>` pre-args for a files-only content item.
func configSurface(ins mktapi.Installed) (binary string, pre []string, err error) {
	if ins.Kind == "content" {
		if ins.FolderPath == "" {
			return "", nil, fmt.Errorf("%q: content item has no folder path", ins.Name)
		}
		return hostBinaryPath(), []string{"--folder", ins.FolderPath}, nil
	}
	if ins.BinaryPath == "" {
		return "", nil, fmt.Errorf("%q is a %s item, not a runnable binary", ins.Name, ins.Kind)
	}
	// A host binary answers no `sidecar` verb: ktw-display, a web server, would
	// start serving and hold the call until its timeout.
	if orchcatalog.HostBinaries[ins.Name] {
		return "", nil, fmt.Errorf("%q has no configuration of its own", ins.Name)
	}
	return ins.BinaryPath, nil, nil
}

// ── Raw config-file editing (the text-editor path) ────────────────────────

func (s *service) GetItemConfig(ctx context.Context, in mktapi.GetItemConfigInput) (mktapi.GetItemConfigOutput, error) {
	if in.File == "" {
		return mktapi.GetItemConfigOutput{}, fmt.Errorf("%q: config file name is required", in.Name)
	}
	ins, err := s.installedItem(in.Name)
	if err != nil {
		return mktapi.GetItemConfigOutput{}, err
	}
	bin, pre, err := configSurface(ins)
	if err != nil {
		return mktapi.GetItemConfigOutput{}, err
	}
	got, err := configure.Get(ctx, in.UniverseHome, bin, pre, in.File, in.Default)
	if err != nil {
		return mktapi.GetItemConfigOutput{}, err
	}
	return mktapi.GetItemConfigOutput{Name: in.Name, File: in.File, Path: got.Path, Content: got.Content}, nil
}

func (s *service) SetItemConfig(ctx context.Context, in mktapi.SetItemConfigInput) (mktapi.SetItemConfigOutput, error) {
	if in.File == "" {
		return mktapi.SetItemConfigOutput{}, fmt.Errorf("%q: config file name is required", in.Name)
	}
	ins, err := s.installedItem(in.Name)
	if err != nil {
		return mktapi.SetItemConfigOutput{}, err
	}
	bin, pre, err := configSurface(ins)
	if err != nil {
		return mktapi.SetItemConfigOutput{}, err
	}
	set, err := configure.SetContent(ctx, in.UniverseHome, bin, pre, in.File, in.Content)
	if err != nil {
		return mktapi.SetItemConfigOutput{}, err
	}
	out := mktapi.SetItemConfigOutput{Name: in.Name, File: in.File}
	for _, f := range set.Files {
		if f.Name == in.File {
			out.Path = f.Path
		}
	}
	// The verdict after the write, so the caller can refresh its report without a
	// second round-trip. A failing re-check is not a failed write.
	if rep, err := configure.CheckWith(ctx, in.UniverseHome, bin, pre); err == nil {
		out.Report = rep
	}
	return out, nil
}

func (s *service) Doctor(ctx context.Context, in mktapi.DoctorInput) ([]mktapi.ItemCheck, error) {
	home := universeTarget(in.UniverseHome)
	installed, err := available.List()
	if err != nil {
		return nil, err
	}
	var todo []mktapi.Installed
	for _, ins := range installed {
		// Only katyoti-installed items — never exec arbitrary dev/PATH binaries
		// (some start a server on a subcommand and would hang). Content items are
		// safe: they run through the fast, non-server generic host.
		if ins.Source != "marketplace" {
			continue
		}
		// NEVER exec a control-plane binary. install-infra records all four in the
		// installed registry (InfraOutdated reads them to know what this machine
		// runs), and they carry source=marketplace like anything else — so doctor
		// happily ran `ktw-gui sidecar config check`, and ktw-gui is a Wails
		// desktop app: `katyoti doctor` OPENED A WINDOW. `ktw-hub` is worse, it
		// would try to serve.
		//
		// Nothing is lost by skipping them. doctor reports on sidecars' own config
		// surfaces; the control plane has none, and its health is what
		// `katyoti status` and `katyoti outdated` already answer.
		if controlplane.IsInfra(ins.Name) {
			continue
		}
		todo = append(todo, ins)
	}
	// In parallel, a few at a time: each check starts the item's binary, and a
	// first start waits for the antivirus to scan it — one after another, the
	// installation's sidecars outlast the call.
	ctx, st := beginSteps(ctx, len(todo))
	reps := make([]*mktapi.ItemCheck, len(todo))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, ins := range todo {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rep, err := s.checkInstalled(ctx, home, ins)
			if err != nil {
				st.end(ins.Name + ": not checked")
				return // skip items that can't be checked (e.g. a hub that would block)
			}
			reps[i] = &mktapi.ItemCheck{Name: ins.Name, Report: rep}
			st.end(ins.Name + ": checked")
		}()
	}
	wg.Wait()
	var out []mktapi.ItemCheck
	for _, r := range reps {
		if r != nil {
			out = append(out, *r)
		}
	}
	return out, nil
}

// hostBinaryPath resolves the generic host used to run + check content items.
// Prefers a `ktw-host` co-located with the running exe (dev `bin/`, a bundled
// instance folder), else falls back to PATH.
// hostBinaryPath delegates to wire.HostBinaryPath — ONE resolver for the generic
// host, owned by the package that writes membership specs.
//
// Two copies is how this broke: this one (sibling-of-exe) was used by the
// configure path and worked, while wire hardcoded the bare name "ktw-host" and
// produced a member the hub could not stat. Every surface reported success and
// the sidecar never spawned.
var hostBinaryPath = wire.HostBinaryPath // a variable: the tests that prove a path runs no host stand it in

func (s *service) installedItem(name string) (mktapi.Installed, error) {
	installed, err := available.List()
	if err != nil {
		return mktapi.Installed{}, err
	}
	for _, ins := range installed {
		if ins.Name == name {
			return ins, nil
		}
	}
	return mktapi.Installed{}, fmt.Errorf("%q is not installed", name)
}

// WireIntoUniverse wires an installed item into a universe + configures it.
func (s *service) WireIntoUniverse(ctx context.Context, in mktapi.WireIntoUniverseInput) (mktapi.WireIntoUniverseOutput, error) {
	ins, err := s.installedItem(in.Name)
	if err != nil {
		return mktapi.WireIntoUniverseOutput{}, err
	}
	home, err := wire.Into(ins, in.UniverseHome)
	if err != nil {
		return mktapi.WireIntoUniverseOutput{Home: home}, err
	}
	// Configure now (best-effort; remaining next-steps surface via `configure`/
	// `doctor`). Content items configure through the generic host.
	_, _ = s.configureInstalled(ctx, home, ins)
	return mktapi.WireIntoUniverseOutput{Home: home}, nil
}

func pickVersion(it mktapi.Item, version string) (string, mktapi.ItemVersion, error) {
	if len(it.Versions) == 0 {
		return "", mktapi.ItemVersion{}, fmt.Errorf("%s: no versions in catalog", it.Name)
	}
	if version == "" {
		return it.Versions[0].Version, it.Versions[0], nil
	}
	for _, v := range it.Versions {
		if v.Version == version {
			return v.Version, v, nil
		}
	}
	avail := make([]string, len(it.Versions))
	for i, v := range it.Versions {
		avail[i] = v.Version
	}
	return "", mktapi.ItemVersion{}, fmt.Errorf("%s: version %q not found (available: %s)", it.Name, version, strings.Join(avail, ", "))
}

func pickAsset(iv mktapi.ItemVersion) (mktapi.Asset, error) {
	for _, a := range iv.Assets {
		if (a.OS == "" || a.OS == runtime.GOOS) && (a.Arch == "" || a.Arch == runtime.GOARCH) {
			return a, nil
		}
	}
	return mktapi.Asset{}, fmt.Errorf("no asset for %s/%s", runtime.GOOS, runtime.GOARCH)
}

// titleCaseName derives a display name from a universe name — the same shape
// `universe create` produces, so a reproduced universe is named like any other.
func titleCaseName(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// defaultReleaseHome is where a release lands when the caller names no universe:
// a NEW universe under the global universes root, named after the release.
//
// Explicitly NOT Env().WorkspaceDir(). That is what wire.CreateUniverse falls
// back to, and it is correct for a CLI standing in a project folder — but this
// method is served by the ADMIN universe's hub, where "the process workspace"
// IS admin. An empty home therefore rewrote admin's own Univers.yaml with the
// release's members, deleting ktw-univers (the sidecar answering the call) and
// hanging the request with the control plane destroyed.
func defaultReleaseHome(releaseName string) string {
	return filepath.Join(application.Env().GlobalHomeDir(), "universes", releaseName)
}

// emptyItemSummaries and emptyInstalled return EMPTY, NON-NIL slices.
//
// A Go nil slice marshals to JSON `null`; the generated TypeScript client
// declares these outputs as arrays and iterates them directly. `for (const x
// of null)` throws "is not iterable" and takes the whole panel down with it —
// which is what the Marketplace view did on any machine with no marketplace
// subscribed and nothing installed.
//
// Named helpers rather than a bare `[]T{}` at each site so the REASON travels
// with the value, and so a test can assert the contract in one place.
func emptyItemSummaries() []mktapi.ItemSummary { return []mktapi.ItemSummary{} }
func emptyInstalled() []mktapi.Installed       { return []mktapi.Installed{} }

// registeredName is the name a universe home is registered under (a home
// registered under another name than its folder's keeps that name); its
// folder's name otherwise.
func registeredName(home string) string {
	if regPath, err := orchcatalog.DefaultRegistryPath(); err == nil {
		if reg, err := orchcatalog.LoadRegistry(regPath); err == nil {
			for _, e := range reg.List() {
				if strings.EqualFold(filepath.Clean(e.HomePath), filepath.Clean(home)) {
					return e.Name
				}
			}
		}
	}
	return filepath.Base(home)
}

// universeTarget resolves where a solution goes (or which universe is meant): a folder (absolute, or naming
// a path) is taken as it is; a bare name is the registered universe of that
// name, or a new one under the global universes root — so a person (the GUI's
// form, a CLI) names a universe without knowing where universes live.
func universeTarget(target string) string {
	t := strings.TrimSpace(target)
	if t == "" || filepath.IsAbs(t) || strings.ContainsAny(t, `/\`) {
		return t
	}
	if regPath, err := orchcatalog.DefaultRegistryPath(); err == nil {
		if reg, err := orchcatalog.LoadRegistry(regPath); err == nil {
			if e, err := reg.Find(t); err == nil && e.HomePath != "" {
				return e.HomePath
			}
		}
	}
	return defaultReleaseHome(t)
}

// RefuseNewerFormats refuses an item whose formats name a kind in a newer
// MAJOR than host reads, naming each and saying to update the hub; a newer
// minor is installed (its unknown keys are ignored, with a finding, at read).
func RefuseNewerFormats(name, version string, formats, host map[string]string) error {
	refused, _ := contribapi.FormatFindings(name+"@"+version, host, formats)
	if len(refused) == 0 {
		return nil
	}
	return fmt.Errorf("%s@%s needs a newer hub: %s", name, version, strings.Join(refused, "; "))
}
