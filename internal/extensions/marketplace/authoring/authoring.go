// SPDX-License-Identifier: Apache-2.0

// Package authoring is the neutral publisher-side engine for CREATING and
// maintaining a marketplace: index a source folder, keygen, sign the index, add
// a universe release, publish. It is shared by the ktw-compose CLI (thin verb
// wrappers) and ktw-gui's publisher bindings — one implementation, no
// duplication (mirrors the CONSUMER engine at marketplace/local.NewService).
//
// It is the "ComposeMarketplace service" of the marketplace-authoring-node-type
// design (D2): stateless — all state is the source + output folders + the key
// files on disk.
package authoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/appcore/frozenhost"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configset"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/infra"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/memberversion"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/prereqs"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/seed"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/signing"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// Service is the marketplace-authoring engine.
type Service struct{}

// New builds the authoring engine.
func New() *Service { return &Service{} }

// --- keygen ---

// Keygen generates an Ed25519 signing keypair. Returns the minisign-format
// public-key file content (hand to consumers to pin), the UNENCRYPTED secret-key
// file content (keep private), and the bare public-key line (the compact form a
// subscription pins).
func (s *Service) Keygen(comment string) (pubFile, secFile, pubLine string, err error) {
	pk, sk, err := signing.Generate()
	if err != nil {
		return "", "", "", err
	}
	return signing.EncodePublicKey(pk, comment), signing.EncodeSecretKey(sk, comment), signing.PublicKeyLine(pk), nil
}

// --- build ---

// BuildResult reports a completed index build. Skipped names the candidate
// binaries that did NOT make the catalog, with the reason — an unreported skip
// ships a signed marketplace that is quietly missing a sidecar.
type BuildResult struct {
	CatalogPath string
	ItemCount   int
	Skipped     []string
}

// Build indexes a source folder of binaries into a katyoti-marketplace.yaml.
// baseURL is where assets are served ("" = the source folder itself, for
// local/dev); out is the catalog path ("" = <source>/katyoti-marketplace.yaml).
//
// A source laid out as <source>/<goos>-<goarch>/ is indexed as ONE catalog
// covering every platform found; a source holding binaries directly is indexed
// as the host platform alone.
func (s *Service) Build(source, name, baseURL, out string) (BuildResult, error) {
	// Empty base-url → RELATIVE asset URLs (portable: the catalog + assets move
	// together; the consumer resolves each asset against the catalog's own
	// location). Pass a base-url only when assets are hosted at a separate fixed
	// URL (release assets / CDN). Note: for relative URLs to resolve, the assets
	// must live in the catalog's folder tree — the default (catalog written into
	// `source`) and the multi-platform `dist/` layout both satisfy this.
	platforms, err := index.DiscoverPlatforms(source)
	if err != nil {
		return BuildResult{}, err
	}
	if len(platforms) == 0 {
		platforms = []index.Platform{{OS: runtime.GOOS, Arch: runtime.GOARCH, Dir: source}}
	}
	// LayoutFlat: Build serves a SELF-CONTAINED folder — the binaries stay
	// where they are and the catalog points at them in place. Publishing a
	// marketplace that accumulates versions is Release, below.
	built, err := index.Build(platforms, name, baseURL, index.LayoutFlat)
	if err != nil {
		return BuildResult{}, err
	}
	cat := built.Catalog
	skipped := make([]string, 0, len(built.Skips))
	for _, s := range built.Skips {
		skipped = append(skipped, s.Binary+": "+s.Reason)
	}
	data, err := index.MarshalCatalog(cat)
	if err != nil {
		return BuildResult{}, err
	}
	dest := out
	if dest == "" {
		dest = filepath.Join(source, "katyoti-marketplace.yaml")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return BuildResult{}, fmt.Errorf("create output dir for %s: %w", dest, err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{CatalogPath: dest, ItemCount: len(cat.Items), Skipped: skipped}, nil
}

// --- release (publish into an accumulating marketplace) ---

// ReleaseResult reports one publish into the marketplace tree.
type ReleaseResult struct {
	CatalogPath string
	ItemCount   int // items in the merged catalog
	Published   int // items whose new version was added by THIS pass
	AssetCount  int // asset files copied in
	Skipped     []string
	Pruned      []string // "<item>@<version>" removed by retention, assets and all
}

// Release publishes a freshly built `source` tree (<goos>-<goarch>/ folders)
// into the marketplace at `outDir`, ACCUMULATING versions rather than replacing
// them.
//
// The published tree is:
//
//	<outDir>/katyoti-marketplace.yaml            the catalog — a STABLE URL
//	<outDir>/<version>/<goos>-<goarch>/<bin>      immutable assets
//
// The catalog sits above the versioned asset folders, so subscribers keep one
// unchanging URL while each version's bytes live at their own immutable path,
// reachable by a relative URL. Republishing an existing version is refused:
// other bytes under it, or a pass that adds no version at all.
//
// keep bounds versions per item (<= 0 keeps all); pruned versions lose their
// catalog entry AND their asset folder in the same pass, never one without the
// other.
func (s *Service) Release(source, name, baseURL, outDir string, keep int) (ReleaseResult, error) {
	platforms, err := index.DiscoverPlatforms(source)
	if err != nil {
		return ReleaseResult{}, err
	}
	if len(platforms) == 0 {
		platforms = []index.Platform{{OS: runtime.GOOS, Arch: runtime.GOARCH, Dir: source}}
	}
	built, err := index.Build(platforms, name, baseURL, index.LayoutVersioned)
	if err != nil {
		return ReleaseResult{}, err
	}

	catalogPath := filepath.Join(outDir, "katyoti-marketplace.yaml")
	published, err := loadCatalogFile(catalogPath)
	if err != nil {
		return ReleaseResult{}, err
	}
	// Merge lets a member a cut ships unchanged through; a
	// pass that adds no version at all republishes what is published. Asked
	// before Merge, which reuses published's items.
	if len(built.Catalog.Items) > 0 && !index.AddsAVersion(published, built.Catalog) {
		return ReleaseResult{}, fmt.Errorf("every version of %s is already published — published versions are immutable; bump the version", source)
	}
	merged, dropped, err := index.Merge(published, built.Catalog, keep)
	if err != nil {
		return ReleaseResult{}, err
	}

	// Assets first: a catalog referring to files that are not there yet would
	// be briefly wrong, and a crash between the two would leave it wrong.
	for _, a := range built.Staged {
		dest := filepath.Join(outDir, filepath.FromSlash(a.Rel))
		if err := copyFile(a.Src, dest); err != nil {
			return ReleaseResult{}, fmt.Errorf("stage %s: %w", a.Rel, err)
		}
	}

	data, err := index.MarshalCatalog(merged)
	if err != nil {
		return ReleaseResult{}, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return ReleaseResult{}, err
	}
	if err := os.WriteFile(catalogPath, data, 0o644); err != nil {
		return ReleaseResult{}, err
	}

	// Retention last, so a failure above leaves the old versions intact.
	//
	// Under the version-first layout a `<version>/<arch>/` folder is SHARED by
	// every item, so this deletes the dropped item's FILES and then GCs the
	// directories only once they are empty. RemoveAll on the version folder
	// would take other items' assets with it.
	pruned := make([]string, 0, len(dropped))
	for _, d := range dropped {
		for _, rel := range d.Assets {
			if isAbsoluteURL(rel) {
				continue // hosted elsewhere (--base-url): nothing local to prune
			}
			assetPath := filepath.Join(outDir, filepath.FromSlash(rel))
			if err := os.Remove(assetPath); err != nil && !os.IsNotExist(err) {
				return ReleaseResult{}, fmt.Errorf("prune %s@%s: %w", d.Item, d.Version, err)
			}
			pruneEmptyDirs(outDir, filepath.Dir(assetPath))
		}
		pruned = append(pruned, d.Item+"@"+d.Version)
	}

	skipped := make([]string, 0, len(built.Skips))
	for _, sk := range built.Skips {
		skipped = append(skipped, sk.Binary+": "+sk.Reason)
	}
	return ReleaseResult{
		CatalogPath: catalogPath,
		ItemCount:   len(merged.Items),
		Published:   len(built.Catalog.Items),
		AssetCount:  len(built.Staged),
		Skipped:     skipped,
		Pruned:      pruned,
	}, nil
}

// copyFile places src at dest, creating parents. Written to a temp sibling and
// renamed so an interrupted publish never leaves a half-written asset whose
// sha256 will not match.
func copyFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// --- sign ---

// Sign signs the catalog index at catalogPath with the secret-key file, writing
// a minisign-compatible <catalogPath>.minisig. Consumers verify it against the
// pinned public key before install.
func (s *Service) Sign(catalogPath, secretKeyPath, trustedComment string) (sigPath string, err error) {
	indexBytes, err := os.ReadFile(catalogPath)
	if err != nil {
		return "", fmt.Errorf("read catalog: %w", err)
	}
	keyData, err := os.ReadFile(secretKeyPath)
	if err != nil {
		return "", fmt.Errorf("read secret key: %w", err)
	}
	sk, err := signing.ParseSecretKey(keyData)
	if err != nil {
		return "", err
	}
	sigPath = catalogPath + ".minisig"
	if err := os.WriteFile(sigPath, []byte(signing.Sign(indexBytes, sk, trustedComment)), 0o644); err != nil {
		return "", err
	}
	return sigPath, nil
}

// --- add-release ---

// AddSolutionResult reports a completed snapshot-into-catalog.
type AddSolutionResult struct {
	// Warning is non-empty when the catalog was signed and could not be
	// re-signed here. See StaleSignatureWarning.
	Warning  string
	Name     string
	Version  string
	Members  []mktapi.PinnedRef
	Unpinned []string // members not wired from the versioned store (no version pinned)
	// Seed is the pinned seed (name@version) and its file count; empty when
	// the release carries none.
	Seed      string
	SeedFiles int
	// Infra is the infra version the release records (empty: no channel
	// published beside the catalog).
	Infra string
	// Documents is how many documents the solution delivers.
	Documents int
	// Universes are the solution's universes by name, the main one first;
	// OtherSeeds the other universes' seeds published (name@version).
	Universes  []string
	OtherSeeds []string
}

// SolutionSeed names what a release's seed is packed from: the project folder
// and the universe's tasks folder (optional), and the seed item's name (empty:
// <release>-seed). A zero SolutionSeed: no seed.

// AddSolution freezes a live universe into an immutable `kind: solution` item in
// the catalog at catalogPath (created if absent). Pins each member to the
// version its store path resolves to; captures zero secrets. Republishing an
// existing version is refused (immutability). name "" = the universe name.
// pinnedRefs parses `<name>@<version>` references (what flag names them, for
// the message); none is fine.
func pinnedRefs(flag string, refs []string) ([]mktapi.PinnedRef, error) {
	var out []mktapi.PinnedRef
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		name, version, ok := strings.Cut(ref, "@")
		if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
			return nil, fmt.Errorf("--%s must be <name>@<version>, got %q", flag, ref)
		}
		out = append(out, mktapi.PinnedRef{Name: name, Version: version})
	}
	return out, nil
}

// SolutionSeed is the seed a solution publishes: its project, its tasks, its
// name.
type SolutionSeed struct {
	Project, Tasks, Name string
	// Demo marks the seed as demo data (mktapi.Item.Demo).
	Demo bool
}

// SolutionSpec is what a solution pins beyond its members and seed, and the
// documents it delivers: ConfigSets and Vaults as <name>@<version>; Docs a
// folder of `<lang>/*.md`; WhatsNew a version's change log made into
// « Quoi de neuf » (French).
type SolutionSpec struct {
	ConfigSets, Vaults []string
	Docs, WhatsNew     string
	// LetterCovers are the change logs a written « Quoi de neuf » covers
	// (its version's and the KaTyoti version it ships); empty: WhatsNew alone.
	// LetterRequests is how many requests their changes name, -1 or 0 when
	// unknown (not checked) -.
	LetterCovers   []string
	LetterRequests int
	// DisplayName and Icon replace the universe's own when given; Description
	// is the catalog entry's (default « Solution: <name> »).
	DisplayName, Icon, Description string
	// GUI replaces the universe's own `gui` section when given (a scenario
	// case declares it in its test-case.yaml).
	GUI *cenv.UniverseGUI
	// Tags replace the universe's own tags when given.
	Tags []string
	// FirstGuide replaces the main universe's own first guide when given.
	FirstGuide string
	// Others are the solution's other universes, beside the main one (the
	// universe snapshotted): each captured from its home, its members pinned
	// with the main one's. Empty: a solution of one universe.
	Others []OtherUniverse
}

// OtherUniverse is a universe a solution composes beside its main one: its
// name in the solution, the home it is captured from, its config-sets
// (<name>@<version>), its seed and what replaces its own display.
type OtherUniverse struct {
	Name, Home string
	ConfigSets []string
	// Seed is its own seed (a project and tasks), published as an item of its
	// own — <solution>-<name>-seed unless named — made with that universe's
	// members, and pinned as its seed. Zero: none.
	Seed SolutionSeed
	// DisplayName, Icon, GUI and Tags replace the universe's own when given.
	DisplayName, Icon string
	GUI               *cenv.UniverseGUI
	Tags              []string
}

func (s *Service) AddSolution(universeHome, version, name, catalogPath, secretKeyPath string, sd SolutionSeed, spec SolutionSpec) (AddSolutionResult, error) {
	cfgRefs, err := pinnedRefs("config-set", spec.ConfigSets)
	if err != nil {
		return AddSolutionResult{}, err
	}
	vaultRefs, err := pinnedRefs("vault", spec.Vaults)
	if err != nil {
		return AddSolutionResult{}, err
	}
	if version == "" {
		return AddSolutionResult{}, fmt.Errorf("version is required")
	}
	m, err := catalog.LoadManifest(universeHome)
	if err != nil {
		return AddSolutionResult{}, fmt.Errorf("load universe at %s: %w", universeHome, err)
	}
	displayName, icon := m.DisplayName, m.Icon
	if spec.DisplayName != "" {
		displayName = spec.DisplayName
	}
	if spec.Icon != "" {
		icon = spec.Icon
	}
	gui := m.GUI
	if spec.GUI != nil {
		gui = spec.GUI
	}
	tags := m.Tags
	if spec.Tags != nil {
		tags = spec.Tags
	}
	firstGuide := m.FirstGuide
	if spec.FirstGuide != "" {
		firstGuide = spec.FirstGuide
	}
	if name == "" {
		name = m.Name
	}
	description := "Solution: " + name
	if spec.Description != "" {
		description = spec.Description
	}
	cat, err := loadCatalogFile(catalogPath)
	if err != nil {
		return AddSolutionResult{}, err
	}

	includes, sidecars, unpinned := captureMembers(cat, m)
	if len(includes) == 0 {
		return AddSolutionResult{}, fmt.Errorf("universe %q has no sidecars to snapshot", m.Name)
	}
	universes := []mktapi.SolutionUniverse{{Name: m.Name, Main: true, DisplayName: displayName, Icon: icon, Sidecars: sidecars, GUI: taggedGUI(gui, tags), Tags: tags, FirstGuide: firstGuide}}
	// The other universes: each captured, its members pinned with the main
	// one's (a member two universes share is pinned once, at one version).
	var otherSeeds []otherSeed
	for _, o := range spec.Others {
		om, err := catalog.LoadManifest(o.Home)
		if err != nil {
			return AddSolutionResult{}, fmt.Errorf("load universe %s at %s: %w", o.Name, o.Home, err)
		}
		oRefs, err := pinnedRefs("config-set", o.ConfigSets)
		if err != nil {
			return AddSolutionResult{}, err
		}
		oIncludes, oSidecars, oUnpinned := captureMembers(cat, om)
		unpinned = append(unpinned, oUnpinned...)
		for _, ref := range oIncludes {
			if have, ok := pinnedVersion(includes, ref.Name); ok {
				if have != ref.Version {
					return AddSolutionResult{}, fmt.Errorf("universe %s wires %s@%s, another universe of the solution %s@%s: one version per member", o.Name, ref.Name, ref.Version, ref.Name, have)
				}
				continue
			}
			includes = append(includes, ref)
		}
		ou := mktapi.SolutionUniverse{Name: o.Name, ConfigSets: oRefs, DisplayName: om.DisplayName, Icon: om.Icon, Sidecars: oSidecars, GUI: om.GUI, Tags: om.Tags, FirstGuide: om.FirstGuide}
		if o.DisplayName != "" {
			ou.DisplayName = o.DisplayName
		}
		if o.Icon != "" {
			ou.Icon = o.Icon
		}
		if o.GUI != nil {
			ou.GUI = o.GUI
		}
		if o.Tags != nil {
			ou.Tags = o.Tags
		}
		ou.GUI = taggedGUI(ou.GUI, ou.Tags)
		universes = append(universes, ou)
		if o.Seed.Project != "" {
			otherSeeds = append(otherSeeds, otherSeed{at: len(universes) - 1, seed: o.Seed, members: oIncludes, displayName: ou.DisplayName})
		}
	}
	// REFUSE an unreproducible solution. A `kind: solution` exists to be
	// reproduced exactly; a member with no store version cannot be, so the
	// artifact would publish cleanly and fail on the consumer's machine at
	// bootstrap. Refusing here costs the publisher one message; not refusing
	// costs whoever tries to use it a debugging session on a broken release
	// they did not build.
	if len(unpinned) > 0 {
		return AddSolutionResult{}, fmt.Errorf(
			"cannot snapshot %q: %s %s", m.Name, strings.Join(unpinned, ", "), unreproducibleHint)
	}

	rel := mktapi.ItemVersion{
		Version:    version,
		Immutable:  true,
		Includes:   includes,
		Requires:   membersNeeds(cat, includes),
		Universes:  universes,
		ConfigSets: cfgRefs,
		Vaults:     vaultRefs,
		Infra:      publishedInfra(catalogPath),
	}
	if err := rel.ValidateUniverses(); err != nil {
		return AddSolutionResult{}, fmt.Errorf("solution %s@%s: %w", name, version, err)
	}

	if cat.APIVersion == 0 {
		cat.APIVersion = 1
	}
	if cat.Marketplace.Name == "" {
		cat.Marketplace.Name = "solutions"
	}
	idx := -1
	for i := range cat.Items {
		if cat.Items[i].Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		cat.Items = append(cat.Items, mktapi.Item{
			Name: name, Kind: mktapi.KindSolution, DisplayName: displayName, Icon: icon,
			Description: description,
		})
		idx = len(cat.Items) - 1
	}
	if cat.Items[idx].Kind != mktapi.KindSolution {
		return AddSolutionResult{}, fmt.Errorf("item %q already exists as kind %q, not solution", name, cat.Items[idx].Kind)
	}
	for _, v := range cat.Items[idx].Versions {
		if v.Version == version {
			return AddSolutionResult{}, fmt.Errorf("solution %s@%s already exists — solutions are immutable (bump the version)", name, version)
		}
	}
	// The seeds, each published as an item of its own (kind: seed) BEFORE the
	// release pins it: the main universe's, its members the release's; each
	// other universe's, its members that universe's. A publish rewrote the
	// catalog: reload it and find the release item again.
	var seedRes BuildSeedResult
	var otherSeedRefs []string
	if sd.Project != "" {
		seedName := sd.Name
		if seedName == "" {
			seedName = name + "-seed"
		}
		seedRes, err = s.buildSeedInto(catalogPath, sd.Project, sd.Tasks, seedName, version, m.DisplayName,
			"The content of the solution "+name, includes, 0, secretKeyPath, sd.Demo)
		if err != nil {
			return AddSolutionResult{}, err
		}
		rel.Seed = &mktapi.PinnedRef{Name: seedName, Version: version}
	}
	for _, ot := range otherSeeds {
		u := &rel.Universes[ot.at]
		seedName := ot.seed.Name
		if seedName == "" {
			seedName = name + "-" + u.Name + "-seed"
		}
		if _, err := s.buildSeedInto(catalogPath, ot.seed.Project, ot.seed.Tasks, seedName, version, ot.displayName,
			"The content of the universe "+u.Name+" of the solution "+name, ot.members, 0, secretKeyPath, ot.seed.Demo); err != nil {
			return AddSolutionResult{}, fmt.Errorf("universe %s: %w", u.Name, err)
		}
		u.Seed = &mktapi.PinnedRef{Name: seedName, Version: version}
		otherSeedRefs = append(otherSeedRefs, seedName+"@"+version)
	}
	if rel.Seed != nil || len(otherSeeds) > 0 {
		if cat, err = loadCatalogFile(catalogPath); err != nil {
			return AddSolutionResult{}, err
		}
		idx = -1
		for i := range cat.Items {
			if cat.Items[i].Name == name {
				idx = i
				break
			}
		}
		if idx == -1 {
			cat.Items = append(cat.Items, mktapi.Item{
				Name: name, Kind: mktapi.KindSolution, DisplayName: displayName, Icon: icon,
				Description: description,
			})
			idx = len(cat.Items) - 1
		}
	}
	// The documents, beside the catalog, before it names them.
	if rel.Documents, err = publishDocuments(filepath.Dir(catalogPath), name, version, spec); err != nil {
		return AddSolutionResult{}, err
	}
	if spec.Description != "" {
		cat.Items[idx].Description = description
	}
	cat.Items[idx].Versions = append([]mktapi.ItemVersion{rel}, cat.Items[idx].Versions...) // newest first

	data, err := index.MarshalCatalog(cat)
	if err != nil {
		return AddSolutionResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		return AddSolutionResult{}, fmt.Errorf("create output dir for %s: %w", catalogPath, err)
	}
	if err := os.WriteFile(catalogPath, data, 0o644); err != nil {
		return AddSolutionResult{}, err
	}
	warning, err := s.resignOrWarn(catalogPath, secretKeyPath, "solution "+name+"@"+version)
	if err != nil {
		return AddSolutionResult{}, err
	}
	if err := writeSolutionFile(filepath.Dir(catalogPath), name, rel); err != nil {
		return AddSolutionResult{}, err
	}
	res := AddSolutionResult{Name: name, Version: version, Members: includes, Unpinned: unpinned, Warning: warning, SeedFiles: seedRes.Files, Infra: rel.Infra, Documents: len(rel.Documents)}
	if rel.Seed != nil {
		res.Seed = rel.Seed.Name + "@" + rel.Seed.Version
	}
	for _, u := range rel.Universes {
		res.Universes = append(res.Universes, u.Name)
	}
	res.OtherSeeds = otherSeedRefs
	return res, nil
}

// otherSeed is the seed of one of a solution's other universes: its position
// in the solution's universes, what it is packed from, the members it is made
// with (that universe's) and its display name.
type otherSeed struct {
	at          int
	seed        SolutionSeed
	members     []mktapi.PinnedRef
	displayName string
}

// captureMembers pins each sidecar a universe wires (its store version, else
// the catalog version whose asset is its very bytes) and captures its portable
// wiring; it answers the members it could not pin.
func captureMembers(cat mktapi.Catalog, m *catalog.Manifest) ([]mktapi.PinnedRef, []mktapi.SolutionSidecar, []string) {
	var includes []mktapi.PinnedRef
	var sidecars []mktapi.SolutionSidecar
	var unpinned []string
	for _, sc := range m.Sidecars {
		ver := memberversion.Derive(sc)
		if ver == "" {
			// Not wired from the store (a scenario case runs copies in its
			// bin/): pinned by content — the catalog version whose asset is
			// these very bytes.
			ver = pinByContent(cat, sc)
		}
		if ver == "" {
			unpinned = append(unpinned, sc.Name)
		}
		includes = append(includes, mktapi.PinnedRef{Name: sc.Name, Version: ver})
		// The binary's NAME only: an install derives each member's path from
		// its own store, and the publisher's paths mean nothing elsewhere.
		sidecars = append(sidecars, mktapi.SolutionSidecar{Name: sc.Name, Transport: sc.Transport, Binary: filepath.Base(sc.Binary), Args: sc.Args})
	}
	return includes, sidecars, unpinned
}

// pinnedVersion is the version refs pin name at.
func pinnedVersion(refs []mktapi.PinnedRef, name string) (string, bool) {
	for _, r := range refs {
		if r.Name == name {
			return r.Version, true
		}
	}
	return "", false
}

// packSeed stages a seed (seed.Stage: its secrets refused, its derived folders
// left out) and packs it, in memory; it answers the bytes, their sha256 and the
// file count.
func packSeed(project, tasks string) ([]byte, string, int, error) {
	stage, err := os.MkdirTemp("", "katyoti-seed-")
	if err != nil {
		return nil, "", 0, err
	}
	defer os.RemoveAll(stage)
	n, err := seed.Stage(project, tasks, stage)
	if err != nil {
		return nil, "", 0, err
	}
	// A project CWM never opened has no project object yet, only its seed in
	// its store (a typed project, a scenario case declaring its project's
	// type): the seed carries it, or the install would get the default type.
	if _, err := seed.WithProjectSeed(stage, filepath.Join(application.EnvOf(project).HomeDir(), cwmapi.ProjectSeedFile)); err != nil {
		return nil, "", 0, fmt.Errorf("the project object's seed: %w", err)
	}
	data, err := frozenhost.TarGzDir(stage)
	if err != nil {
		return nil, "", 0, fmt.Errorf("pack the seed: %w", err)
	}
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:]), n, nil
}

// BuildSeedResult reports one seed publish.
type BuildSeedResult struct {
	CatalogPath, Name, Version, AssetPath, Sha256, Warning string
	Files                                                  int
	Pruned                                                 []string
}

// BuildSeed packs a project folder (and a universe's tasks folder) as a
// `kind: seed` version and accumulates it into the marketplace catalog at
// outDir: the CONTENT of a universe — its CWM objects and the whole project
// folder — applied to a universe like a config-set, pinned by a solution.
// requires are the members it was made with. The new version is merged in
// memory first (an existing version is refused: immutable), then its bytes are
// written, then the catalog, then re-signed — never a catalog naming bytes
// that are not there.
// demo marks its content as demo data (mktapi.Item.Demo).
func (s *Service) BuildSeed(project, tasks, name, version, displayName, description string, requires []mktapi.PinnedRef, outDir string, keep int, secretKeyPath string, demo bool) (BuildSeedResult, error) {
	return s.buildSeedInto(filepath.Join(outDir, "katyoti-marketplace.yaml"), project, tasks, name, version, displayName, description, requires, keep, secretKeyPath, demo)
}

// buildSeedInto is BuildSeed into a named catalog file (a release publishes
// its seed into its own catalog, whatever its name); the seed's bytes go
// beside it.
func (s *Service) buildSeedInto(catalogPath, project, tasks, name, version, displayName, description string, requires []mktapi.PinnedRef, keep int, secretKeyPath string, demo bool) (BuildSeedResult, error) {
	outDir := filepath.Dir(catalogPath)
	res := BuildSeedResult{Name: name, Version: version}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
		return res, fmt.Errorf("build-seed: both a name and a version are required")
	}
	data, sha, n, err := packSeed(project, tasks)
	if err != nil {
		return res, err
	}
	res.Sha256, res.Files = sha, n
	rel := index.SeedAssetPath(version, name)
	res.AssetPath = rel
	if displayName == "" {
		displayName = name
	}
	fresh := mktapi.Catalog{APIVersion: 1, Items: []mktapi.Item{{
		Name: name, Kind: mktapi.KindSeed, DisplayName: displayName, Description: description, Demo: demo,
		Versions: []mktapi.ItemVersion{{
			Version:  version,
			Assets:   []mktapi.Asset{{URL: rel, Sha256: sha}},
			Requires: mktapi.Requires{Members: requires},
		}},
	}}}
	res.CatalogPath = catalogPath
	published, err := loadCatalogFile(catalogPath)
	if err != nil {
		return res, err
	}
	merged, dropped, err := index.Merge(published, fresh, keep)
	if err != nil {
		return res, err
	}
	dest := filepath.Join(outDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return res, err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return res, fmt.Errorf("write %s: %w", rel, err)
	}
	out, err := index.MarshalCatalog(merged)
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(catalogPath, out, 0o644); err != nil {
		return res, err
	}
	for _, d := range dropped {
		for _, u := range d.Assets {
			if isAbsoluteURL(u) {
				continue
			}
			p := filepath.Join(outDir, filepath.FromSlash(u))
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return res, fmt.Errorf("prune %s@%s: %w", d.Item, d.Version, err)
			}
			pruneEmptyDirs(outDir, filepath.Dir(p))
		}
		res.Pruned = append(res.Pruned, d.Item+"@"+d.Version)
	}
	res.Warning, err = s.resignOrWarn(catalogPath, secretKeyPath, "seed "+name+"@"+version)
	return res, err
}

// pinByContent pins a member wired from a binary outside the store to the
// catalog version whose asset for this OS/arch has the binary's sha256: the
// same bytes, so the pin is exact by construction. Empty when no version
// matches (the binary was not published) or the member is no binary.
func pinByContent(cat mktapi.Catalog, sc orchapi.SidecarSpec) string {
	if sc.Binary == "" || !filepath.IsAbs(sc.Binary) {
		return ""
	}
	data, err := os.ReadFile(sc.Binary)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])
	for _, it := range cat.Items {
		if it.Name != sc.Name || it.Kind != mktapi.KindSidecar {
			continue
		}
		for _, v := range it.Versions {
			for _, a := range v.Assets {
				if a.OS == runtime.GOOS && a.Arch == runtime.GOARCH && strings.EqualFold(a.Sha256, want) {
					return v.Version
				}
			}
		}
	}
	return ""
}

// membersNeeds is what a solution needs: the sum of the programs, keys and
// logins its pinned members' catalog versions require (prereqs.SumRequires —
// once each, required when any member requires it, the highest version). A
// member the catalog does not hold adds nothing.
func membersNeeds(cat mktapi.Catalog, includes []mktapi.PinnedRef) mktapi.Requires {
	var rs []mktapi.Requires
	for _, ref := range includes {
		for _, it := range cat.Items {
			if it.Name != ref.Name || (it.Kind != mktapi.KindSidecar && it.Kind != mktapi.KindContent) {
				continue
			}
			for _, v := range it.Versions {
				if v.Version == ref.Version {
					rs = append(rs, v.Requires)
				}
			}
		}
	}
	return prereqs.SumRequires(rs...)
}

// publishedInfra is the version of the infra channel published beside the
// catalog (`<dir>/infra/channel.yaml`); empty when there is none.
func publishedInfra(catalogPath string) string {
	ch := infra.Channel{Root: filepath.Join(filepath.Dir(catalogPath), "infra")}
	v, err := ch.LatestVersion(context.Background())
	if err != nil {
		return ""
	}
	return v
}

// --- publish ---

// Publish is the pre-distribution GATE: it confirms the marketplace is
// actually ready to hand out — the index exists + parses, it is signed, and
// (when pubKeyPath is given) the signature VERIFIES against the public key
// consumers will pin. It returns an actionable summary. It does NOT itself
// upload/push — hosting the OUTPUT folder (git repo / static server) is the
// operator's step; an automated git promote is a follow-up (team-marketplace.md §3).
func (s *Service) Publish(output, pubKeyPath string) (string, error) {
	catalogPath := output
	if fi, err := os.Stat(output); err == nil && fi.IsDir() {
		catalogPath = filepath.Join(output, "katyoti-marketplace.yaml")
	}
	index, err := os.ReadFile(catalogPath)
	if err != nil {
		return "", fmt.Errorf("no catalog at %s — run Build first", catalogPath)
	}
	var cat mktapi.Catalog
	if err := yaml.Unmarshal(index, &cat); err != nil {
		return "", fmt.Errorf("catalog at %s does not parse: %w", catalogPath, err)
	}
	// Every file of the tree reaches every subscriber: a swap's leftover
	// (katyoti.exe.old-…) is never handed out.
	if stray, err := Leftovers(filepath.Dir(catalogPath)); err != nil {
		return "", err
	} else if len(stray) > 0 {
		return "", fmt.Errorf("the published tree holds leftovers that are not published items — DO NOT distribute; remove them: %s", strings.Join(stray, ", "))
	}

	sigPath := catalogPath + ".minisig"
	_, sigErr := os.Stat(sigPath)
	status := "SIGNED"
	if sigErr != nil {
		status = "UNSIGNED — team/public consumers (trust: pinned) will REFUSE it; run Sign first"
	} else if pubKeyPath != "" {
		// Verify the signature the way a consumer will, so a broken/mismatched
		// signature is caught HERE, not on someone else's install.
		pkData, err := os.ReadFile(pubKeyPath)
		if err != nil {
			return "", fmt.Errorf("read public key %s: %w", pubKeyPath, err)
		}
		pk, err := signing.ParsePublicKey(pkData)
		if err != nil {
			return "", fmt.Errorf("public key %s: %w", pubKeyPath, err)
		}
		sig, err := os.ReadFile(sigPath)
		if err != nil {
			return "", err
		}
		if err := signing.Verify(index, sig, pk); err != nil {
			return "", fmt.Errorf("signature does NOT verify against %s — DO NOT distribute: %w", filepath.Base(pubKeyPath), err)
		}
		status = "SIGNED + signature VERIFIED"
		// The public key beside the catalog: what a consumer pins (setup reads
		// it there, by the name the infra pointer gives).
		if err := os.WriteFile(filepath.Join(filepath.Dir(catalogPath), filepath.Base(pubKeyPath)), pkData, 0o644); err != nil {
			return "", fmt.Errorf("publish the public key: %w", err)
		}
	}

	return fmt.Sprintf("%d item(s), %s. Ready at %s — host %s (git repo / static server) and share the URL; consumers: `katyoti marketplace add <name> <url> --pubkey <key>.key.pub --trust pinned`",
		len(cat.Items), status, catalogPath, filepath.Dir(catalogPath)), nil
}

func loadCatalogFile(path string) (mktapi.Catalog, error) {
	var cat mktapi.Catalog
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cat, nil
	}
	if err != nil {
		return cat, err
	}
	if err := yaml.Unmarshal(data, &cat); err != nil {
		return cat, fmt.Errorf("parse %s: %w", path, err)
	}
	return cat, nil
}

// contentNeeds are the programs, keys and logins a content folder declares
// (its ktw-host.yaml `config:` block), as its catalog item's requires — what
// a compiled sidecar's describe gives the indexer.
func contentNeeds(folder, name string) (mktapi.Requires, error) {
	var decl struct {
		Config sidecar.FolderConfig `yaml:"config"`
	}
	path := filepath.Join(folder, frozenhost.ManifestFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return mktapi.Requires{}, fmt.Errorf("read %s: %w", path, err)
	}
	if err := config.DecodeBytes(raw, marshal.YAML, &decl); err != nil {
		return mktapi.Requires{}, fmt.Errorf("parse %s: %w", path, err)
	}
	tools, keys, auth := decl.Config.Requirements()
	return mktapi.NeedsOf(name, tools, keys, auth), nil
}

// isAbsoluteURL reports whether a catalog asset URL points somewhere other than
// beside the catalog (a --base-url publish). Such assets have no local file to
// prune.
func isAbsoluteURL(u string) bool {
	return strings.Contains(u, "://")
}

// pruneEmptyDirs removes dir and its parents while they are empty, stopping at
// (and never removing) root. Used after deleting a pruned asset so the shared
// `<version>/<arch>/` folders disappear once the last item leaves them, without
// ever touching a folder another item still occupies.
func pruneEmptyDirs(root, dir string) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return
	}
	for {
		absDir, err := filepath.Abs(dir)
		if err != nil || absDir == absRoot || !strings.HasPrefix(absDir, absRoot) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// RemoveVersionResult reports what one remove-version pass did.
type RemoveVersionResult struct {
	CatalogPath   string
	Item          string
	Version       string
	RemovedAssets []string
	ItemRemoved   bool // the version was the item's last, so the item is gone too
	SignaturePath string
}

// RemoveVersion retires ONE published version of ONE item: it drops the version
// from the catalog, deletes that version's asset files, GCs any directory the
// removal emptied, and re-signs the catalog.
//
// solutionsPinning reports the published releases that pin item@version as a
// member, as "<release>@<version>" strings.
func solutionsPinning(cat mktapi.Catalog, item, version string) []string {
	var out []string
	for _, it := range cat.Items {
		if it.Kind != mktapi.KindSolution {
			continue
		}
		for _, v := range it.Versions {
			for _, m := range v.Includes {
				if m.Name == item && m.Version == version {
					out = append(out, it.Name+"@"+v.Version)
				}
			}
		}
	}
	return out
}

// This is the counterpart to retention pruning, for the case where a specific
// release must be withdrawn rather than aged out. It is a first-class,
// re-signing operation precisely because a manual `rm` would leave the catalog
// advertising bytes that are gone AND leave the signature covering a catalog
// that no longer matches — consumers pinned to the key would then fail.
//
// Ordering is the INVERSE of publishing: the catalog is rewritten BEFORE the
// files are deleted. Publishing writes assets first so the catalog never
// advertises missing bytes; removing must stop advertising them first, so a
// crash mid-way leaves harmless orphan files rather than a catalog pointing at
// nothing.
//
// secretKeyPath may be empty to skip re-signing (useful in tests); in normal use
// it is required, since an unsigned or stale-signed catalog is not distributable.
func (s *Service) RemoveVersion(outDir, item, version, secretKeyPath, comment string, force bool) (RemoveVersionResult, error) {
	res := RemoveVersionResult{Item: item, Version: version}
	if strings.TrimSpace(item) == "" || strings.TrimSpace(version) == "" {
		return res, fmt.Errorf("remove-version: both an item and a version are required (item@version)")
	}

	catalogPath := filepath.Join(outDir, "katyoti-marketplace.yaml")
	res.CatalogPath = catalogPath
	cat, err := loadCatalogFile(catalogPath)
	if err != nil {
		return res, err
	}

	idx := -1
	for i := range cat.Items {
		if cat.Items[i].Name == item {
			idx = i
			break
		}
	}
	if idx == -1 {
		return res, fmt.Errorf("%s is not published in %s", item, catalogPath)
	}

	var removed *mktapi.ItemVersion
	kept := make([]mktapi.ItemVersion, 0, len(cat.Items[idx].Versions))
	for i, v := range cat.Items[idx].Versions {
		if v.Version == version {
			removed = &cat.Items[idx].Versions[i]
			continue
		}
		kept = append(kept, v)
	}
	if removed == nil {
		return res, fmt.Errorf("%s@%s is not published (have: %s)", item, version, versionList(cat.Items[idx].Versions))
	}
	// A published RELEASE pins exact member versions. Withdrawing one of those
	// leaves the release listed, offered in the GUI's picker, and impossible to
	// bootstrap — and the publisher never finds out, because the failure lands on
	// the consumer's machine at the moment they try to use it.
	//
	// Reproducibility is the entire promise of a release, and every other
	// operation around it is enforced: republishing a version is refused, catalog
	// mutations re-sign so pinned consumers keep verifying. This closes the one
	// verb that could quietly break it.
	if users := solutionsPinning(cat, item, version); len(users) > 0 && !force {
		return res, fmt.Errorf(
			"%s@%s is pinned by published release(s): %s. "+
				"Withdrawing it would leave them listed but impossible to bootstrap. "+
				"Withdraw the release(s) first, or pass --force to withdraw anyway",
			item, version, strings.Join(users, ", "))
	}
	for _, a := range removed.Assets {
		res.RemovedAssets = append(res.RemovedAssets, a.URLs()...)
	}

	if len(kept) == 0 {
		cat.Items = append(cat.Items[:idx], cat.Items[idx+1:]...)
		res.ItemRemoved = true
	} else {
		cat.Items[idx].Versions = kept
	}

	// A promoted release shares its candidate's files: those still referenced stay.
	res.RemovedAssets = index.Unreferenced(cat, res.RemovedAssets)

	// Catalog first — see the ordering note above.
	data, err := index.MarshalCatalog(cat)
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(catalogPath, data, 0o644); err != nil {
		return res, err
	}

	// Then the bytes. A `<version>/<arch>/` folder is shared by every item, so
	// this deletes only this item's files and collapses directories once empty.
	for _, rel := range res.RemovedAssets {
		if isAbsoluteURL(rel) {
			continue // hosted elsewhere: nothing local to delete
		}
		assetPath := filepath.Join(outDir, filepath.FromSlash(rel))
		if err := os.Remove(assetPath); err != nil && !os.IsNotExist(err) {
			return res, fmt.Errorf("remove asset %s: %w", rel, err)
		}
		pruneEmptyDirs(outDir, filepath.Dir(assetPath))
	}

	if strings.TrimSpace(secretKeyPath) != "" {
		sig, err := s.Sign(catalogPath, secretKeyPath, comment)
		if err != nil {
			return res, fmt.Errorf("re-sign after removal: %w", err)
		}
		res.SignaturePath = sig
	}
	return res, nil
}

// versionList renders the versions an item still advertises, for error text.
func versionList(vs []mktapi.ItemVersion) string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Version)
	}
	return strings.Join(out, ", ")
}

// ConfigSetAssetPath is where a config-set bundle lives, relative to the catalog:
// `config/<name>/<version>/<name>.tar.gz`.
//
// Config-sets get their OWN namespace rather than the version-first
// `<version>/<goos>-<goarch>/` layout the binaries use, for two reasons: a
// config-set has no platform axis (one bundle serves every OS), so an <arch>
// level would be a lie; and keeping them under `config/` means retiring a whole
// binary release with `rm -rf <version>/` can never take a config-set with it.
// ConfigSetAssetPath delegates to the index package, which owns the deploy-root
// layout for every kind. Kept as a re-export because callers and tests already
// reference it here.
func ConfigSetAssetPath(name, version string) string {
	return index.ConfigSetAssetPath(version, name)
}

// BuildConfigSetResult reports one config-set publish.
type BuildConfigSetResult struct {
	CatalogPath string
	Name        string
	Version     string
	AssetPath   string
	Sha256      string
	Keys        []string
	Templates   []string
	Warning     string
	Pruned      []string
}

// BuildConfigSet validates a config-set bundle folder, packs it, and accumulates a
// `kind: config` version into the marketplace catalog.
//
// Validation runs FIRST and is fatal (configset.Validate): a bundle whose template
// references an undeclared ${VAR} is refused at publish time, because the whole
// promise of a config-set is that installing it tells the consumer every key it
// needs in one pass. Shipping an undeclared placeholder would break that promise
// at the far end, where it is much harder to diagnose.
//
// The asset is platform-agnostic — one bundle per version, no os/arch — so the
// consumer's pickAsset takes it on any platform.
func (s *Service) BuildConfigSet(folder, name, version, displayName, description, outDir string, keep int, secretKeyPath string) (BuildConfigSetResult, error) {
	res := BuildConfigSetResult{Name: name, Version: version}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
		return res, fmt.Errorf("build-config-set: both --name and --version are required")
	}

	valid, err := configset.Validate(folder)
	if err != nil {
		return res, err
	}
	res.Keys, res.Templates = valid.Keys, valid.Templates

	data, err := frozenhost.TarGzDir(folder)
	if err != nil {
		return res, fmt.Errorf("pack %s: %w", folder, err)
	}
	sum := sha256.Sum256(data)
	res.Sha256 = hex.EncodeToString(sum[:])

	rel := ConfigSetAssetPath(name, version)
	res.AssetPath = rel

	if displayName == "" {
		displayName = name
	}
	fresh := mktapi.Catalog{
		APIVersion: 1,
		Items: []mktapi.Item{{
			Name:        name,
			Kind:        mktapi.KindConfig,
			DisplayName: displayName,
			Description: description,
			Versions: []mktapi.ItemVersion{{
				Version: version,
				// No OS/Arch: one bundle serves every platform.
				Assets: []mktapi.Asset{{URL: rel, Sha256: res.Sha256}},
			}},
		}},
	}

	catalogPath := filepath.Join(outDir, "katyoti-marketplace.yaml")
	res.CatalogPath = catalogPath
	published, err := loadCatalogFile(catalogPath)
	if err != nil {
		return res, err
	}
	merged, dropped, err := index.Merge(published, fresh, keep)
	if err != nil {
		return res, err
	}

	// Asset first, then the catalog — same ordering rule as Release: the catalog
	// must never advertise bytes that are not there yet.
	dest := filepath.Join(outDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return res, err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return res, fmt.Errorf("write %s: %w", rel, err)
	}

	out, err := index.MarshalCatalog(merged)
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(catalogPath, out, 0o644); err != nil {
		return res, err
	}

	// Retention: same per-asset delete + empty-dir GC as the binary pruner.
	for _, d := range dropped {
		for _, u := range d.Assets {
			if isAbsoluteURL(u) {
				continue
			}
			p := filepath.Join(outDir, filepath.FromSlash(u))
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return res, fmt.Errorf("prune %s@%s: %w", d.Item, d.Version, err)
			}
			pruneEmptyDirs(outDir, filepath.Dir(p))
		}
		res.Pruned = append(res.Pruned, d.Item+"@"+d.Version)
	}
	res.Warning, err = s.resignOrWarn(catalogPath, secretKeyPath, "config-set "+name+"@"+version)
	if err != nil {
		return res, err
	}
	return res, nil
}

// --- content items (a folder published as a files-only sidecar) ---

// ContentAssetPath is where a content bundle lands, in its own namespace so it
// never collides with a binary item of the same name.
// ContentAssetPath delegates to the index package — see the note on
// ConfigSetAssetPath.
func ContentAssetPath(name, version string) string {
	return index.ContentAssetPath(version, name)
}

// BuildContentResult reports one content publish.
type BuildContentResult struct {
	CatalogPath string
	Name        string
	Version     string
	AssetPath   string
	Sha256      string
	Pruned      []string
	// BulkLayers counts the files written to the object store instead of into
	// the archive. Reported because "the pack is small" and "the content is
	// somewhere" are the same fact, and an author who cannot see the second
	// has no way to tell a hoisted pack from a pack that lost its content.
	BulkLayers int
	// Warning is non-empty when the catalog was signed and could not be
	// re-signed here. See StaleSignatureWarning.
	Warning string
}

// BuildContent publishes a FOLDER as a `kind: content` catalog item.
//
// This is the no-Go-toolchain path: a contributor writes skills, tasks,
// personas and references into a folder, declares it with a ktw-host.yaml, and
// ships it. It runs on ktw-host, so nothing is compiled — which is the whole
// point, and why the gap mattered: content items were INSTALLABLE long before
// anything could publish one, so the feature existed for consumers and was
// unreachable for authors.
//
// Identity comes from the folder's own manifest rather than from flags: the
// name in ktw-host.yaml is what the host adopts as its runtime provider, so
// letting a publisher pass a different one on the command line would let the
// catalog and the runtime disagree about what this thing is called.
//
// ONE asset, no platform matrix. A folder of markdown and YAML is the same
// bytes everywhere; the installer's asset picker treats empty OS/Arch as
// "matches anything".
func (s *Service) BuildContent(folder, version, outDir string, keep int, secretKeyPath, hostBinary, publisher string) (BuildContentResult, error) {
	var res BuildContentResult
	if strings.TrimSpace(version) == "" {
		return res, fmt.Errorf("build-content: --version is required")
	}
	id, err := frozenhost.ReadIdentity(folder)
	if err != nil {
		return res, err
	}
	res.Name, res.Version = id.Name, version

	// Stamp the published version INTO the folder before packing it, so the
	// bytes that ship know what they are. Without this a frozen copy of the same
	// folder reported the ktw-host build it was frozen into — a number with no
	// relationship to the content, which changed whenever the host was rebuilt.
	if err := frozenhost.StampVersion(folder, version); err != nil {
		return res, fmt.Errorf("stamp version into %s: %w", folder, err)
	}

	// Assert origin BEFORE packing, so the assertions ship inside the artifact.
	// Packaging is the moment authorship becomes distribution; a folder an
	// author serves themselves is not a distribution event and stays unsigned.
	if _, err := SignFolder(hostBinary, folder, secretKeyPath, publisher); err != nil {
		return res, fmt.Errorf("sign %s: %w", folder, err)
	}

	// Bulk content goes to the store rather than into the archive, which is
	// what keeps a pack bounded by the NUMBER of contributions rather than
	// their size. Hoisting records the descriptors INTO the folder, so it runs
	// before packing — those three lines are what ships in the file's place.
	moved, err := HoistBulkLayers(context.Background(), folder, filepath.Join(outDir, BlobsDir))
	if err != nil {
		return res, fmt.Errorf("hoist bulk layers in %s: %w", folder, err)
	}
	res.BulkLayers = len(moved)

	data, err := frozenhost.TarGzDirExcept(folder, BulkSkipper(moved))
	if err != nil {
		return res, fmt.Errorf("pack %s: %w", folder, err)
	}
	sum := sha256.Sum256(data)
	res.Sha256 = hex.EncodeToString(sum[:])
	rel := ContentAssetPath(id.Name, version)
	res.AssetPath = rel

	needs, err := contentNeeds(folder, id.Name)
	if err != nil {
		return res, err
	}
	fresh := mktapi.Catalog{
		APIVersion: 1,
		Items: []mktapi.Item{{
			Name:        id.Name,
			Kind:        mktapi.KindContent,
			DisplayName: id.DisplayName,
			Description: id.Description,
			Icon:        id.Icon,
			Versions: []mktapi.ItemVersion{{
				Version:  version,
				Assets:   []mktapi.Asset{{URL: rel, Sha256: res.Sha256}},
				Requires: needs,
			}},
		}},
	}

	catalogPath := filepath.Join(outDir, "katyoti-marketplace.yaml")
	res.CatalogPath = catalogPath
	published, err := loadCatalogFile(catalogPath)
	if err != nil {
		return res, err
	}
	merged, dropped, err := index.Merge(published, fresh, keep)
	if err != nil {
		return res, err
	}

	// Asset first, then the catalog: the catalog must never advertise bytes
	// that are not there yet. Same ordering rule as Release and BuildConfigSet.
	dest := filepath.Join(outDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return res, err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return res, fmt.Errorf("write %s: %w", rel, err)
	}
	out, err := index.MarshalCatalog(merged)
	if err != nil {
		return res, err
	}
	if err := os.WriteFile(catalogPath, out, 0o644); err != nil {
		return res, err
	}

	for _, d := range dropped {
		for _, u := range d.Assets {
			if isAbsoluteURL(u) {
				continue
			}
			p := filepath.Join(outDir, filepath.FromSlash(u))
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return res, fmt.Errorf("prune %s@%s: %w", d.Item, d.Version, err)
			}
			pruneEmptyDirs(outDir, filepath.Dir(p))
		}
		res.Pruned = append(res.Pruned, d.Item+"@"+d.Version)
	}
	res.Warning, err = s.resignOrWarn(catalogPath, secretKeyPath, "content "+id.Name+"@"+version)
	if err != nil {
		return res, err
	}
	return res, nil
}

// unreproducibleHint explains a refused snapshot. Named so the message itself is
// testable: a refusal is only useful if it says WHY the member cannot be pinned
// and what to do about it, and a refactor should not be able to hollow that out.
const unreproducibleHint = "wired from a dev/PATH binary, not the versioned store, and no version in this catalog has its bytes (sha256), so the release could not be reproduced.\n" +
	"Publish that very build into the marketplace first, or install each from a marketplace and re-wire it, then snapshot again."

// StaleSignatureWarning is returned by a catalog-mutating verb that changed a
// SIGNED catalog without being given a key to re-sign it with.
//
// This is not pedantry. A publisher who runs `build-content` or
// `snapshot-universe` against a marketplace subscribed with `--trust pinned`
// leaves a catalog whose signature no longer verifies — and the failure lands on
// CONSUMERS, at their next refresh, as "signature did NOT verify: refusing to
// trust it". The publisher sees a success message and learns about it from a
// teammate. Saying it at the moment of the change is the only place it is cheap.
const StaleSignatureWarning = "the catalog is signed but was NOT re-signed — consumers pinned to the key will refuse it; re-run with --key-folder/--key-prefix, or `sign-marketplace` it"

// resignOrWarn re-signs a mutated catalog when a key is available, and reports a
// stale signature when one is not.
//
// Returns "" when there is nothing to say: either it re-signed, or the catalog
// was never signed in the first place (a local/dev tree, where signing is noise).
func (s *Service) resignOrWarn(catalogPath, secretKeyPath, comment string) (string, error) {
	if strings.TrimSpace(secretKeyPath) != "" {
		if _, err := s.Sign(catalogPath, secretKeyPath, comment); err != nil {
			return "", fmt.Errorf("re-sign %s: %w", catalogPath, err)
		}
		return "", nil
	}
	if _, err := os.Stat(catalogPath + ".minisig"); err == nil {
		return StaleSignatureWarning, nil
	}
	return "", nil
}

// taggedGUI is the gui a solution's universe is published with: a universe
// with tags is shown what its tags allow, so its gui.hide is left out (retired
// for a tagged universe) - first and levels kept, and a gui
// still given so that a solution update replaces the installed universe's own
// (whose hide goes with it). A universe with no tags keeps its gui as is.
func taggedGUI(gui *cenv.UniverseGUI, tags []string) *cenv.UniverseGUI {
	if gui == nil || len(tags) == 0 || len(gui.Hide) == 0 {
		return gui
	}
	g := *gui
	g.Hide = nil
	return &g
}
