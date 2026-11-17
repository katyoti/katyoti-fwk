// SPDX-License-Identifier: Apache-2.0

// Package index is the neutral marketplace-index engine: it BUILDS a
// katyoti-marketplace.yaml catalog from built sidecars (running each
// `<binary> describe`, hashing the asset) + a publisher-supplied base URL. The
// consumer side (parse/install) lives in the marketplace api + core; this is
// the producer side, imported by the ktw-compose publisher (ADR §9).
//
// One catalog carries every platform: an item version holds one Asset per
// OS/arch, and the consumer picks the one matching its own runtime (see
// marketplace/internal/core.selectAsset). A source tree laid out as
// <root>/<goos>-<goarch>/ is indexed as that matrix in a single pass.
//
// Describe metadata (name, display name, description, icon, version) is
// platform-independent, so it is read ONCE from the host-platform folder — a
// foreign binary cannot be executed on the publisher's machine. Only the
// sha256 is computed per platform, which is a pure file read.
package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/appcore/selfupdate"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// describeTimeout bounds each `<binary> describe` call — a misbehaving binary
// (one that starts a server on bootstrap, hangs, etc.) is killed + skipped
// rather than blocking the whole build.
//
// Generous on purpose: a freshly built binary's FIRST execution pays for an
// on-access antivirus scan of tens of megabytes, which overran a tighter
// budget and silently dropped the item from the catalog. Publishing is not a
// hot path — a slow describe must cost seconds, never an item.
const describeTimeout = 45 * time.Second

// Platform is one folder of built binaries and the target it was built for.
type Platform struct {
	OS   string // GOOS the binaries in Dir target
	Arch string // GOARCH the binaries in Dir target
	Dir  string // folder holding the built binaries
	Sub  string // path segment under baseURL for these assets ("" = baseURL root)
}

// knownGOOS bounds what DiscoverPlatforms will accept as a <goos>-<goarch>
// folder, so an unrelated subfolder in the source tree is never mistaken for a
// build target.
var knownGOOS = map[string]bool{"windows": true, "darwin": true, "linux": true}

// DiscoverPlatforms reads a source root laid out as <root>/<goos>-<goarch>/ and
// returns one Platform per recognised subfolder. Returns nil when root holds no
// such subfolder — the caller then treats root itself as a single platform.
func DiscoverPlatforms(root string) ([]Platform, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", root, err)
	}
	var out []Platform
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		goos, arch, ok := strings.Cut(e.Name(), "-")
		if !ok || !knownGOOS[goos] || arch == "" {
			continue
		}
		out = append(out, Platform{OS: goos, Arch: arch, Dir: filepath.Join(root, e.Name()), Sub: e.Name()})
	}
	return out, nil
}

// Skip records a candidate binary that did NOT make it into the catalog, and
// why. Skips are returned rather than swallowed: a silently dropped item
// produces a valid, signed marketplace that is quietly missing a sidecar, and
// the cause (a slow describe, a binary with no describe verb) is invisible at
// the point it matters. Callers surface these to the publisher.
type Skip struct {
	Binary string
	Reason string
	// Failed: a sidecar that should have been indexed and could not be (its
	// describe failed) — not a deliberate skip like the control plane's.
	Failed bool
}

// Layout decides where an asset is served from, relative to the catalog.
type Layout int

const (
	// LayoutFlat keeps assets beside the catalog, under their platform folder
	// (or directly beside it when there is only one platform). For a
	// self-contained BUNDLE: one folder, one version, binaries used in place.
	LayoutFlat Layout = iota
	// LayoutVersioned puts each asset under <version>/<goos>-<goarch>/.
	// For a MARKETPLACE, where several versions are advertised at once and
	// v1's bytes must survive v2 being published.
	LayoutVersioned
)

// StagedAsset is one file to publish: where it sits in the build output, and
// the path it must be served from (which is exactly its URL when the catalog
// is its sibling). Returned so the caller can place files without re-deriving
// paths the indexer already computed.
type StagedAsset struct {
	Src string
	Rel string
}

// BuildOutput is everything one indexing pass produced.
type BuildOutput struct {
	Catalog mktapi.Catalog
	Staged  []StagedAsset
	Skips   []Skip
}

// Build indexes one or more platform folders into a single catalog. For each
// `ktw-*` (or `katyoti`) executable in the HOST platform's folder it runs
// `<binary> describe --out json` for the metadata, then attaches one asset per
// platform that ships the same binary — hashing each file, no foreign exec.
//
// Requires the host platform to be among platforms, since that is the only
// folder whose binaries can be executed here. Binaries that don't describe
// cleanly, and those no platform ships, are reported in the returned skips.
func Build(platforms []Platform, marketplaceName, baseURL string, layout Layout) (BuildOutput, error) {
	host := -1
	for i, p := range platforms {
		if p.OS == runtime.GOOS && p.Arch == runtime.GOARCH {
			host = i
			break
		}
	}
	if host == -1 {
		return BuildOutput{}, fmt.Errorf(
			"no %s/%s binaries among the platforms to index — describe metadata can only be read from a binary this machine can run; build the host platform too",
			runtime.GOOS, runtime.GOARCH)
	}

	// A published version is immutable, so every platform's bytes must come
	// from the SAME build. Rebuilding one platform and publishing freezes the
	// other platforms' stale binaries under the new version — undetectable
	// afterwards, because the catalog records whatever it was handed.
	if layout == LayoutVersioned {
		if err := verifyOneBuild(platforms); err != nil {
			return BuildOutput{}, err
		}
	}

	entries, err := os.ReadDir(platforms[host].Dir)
	if err != nil {
		return BuildOutput{}, fmt.Errorf("read dir %s: %w", platforms[host].Dir, err)
	}
	out := BuildOutput{Catalog: mktapi.Catalog{
		APIVersion:  1,
		Marketplace: mktapi.CatalogMeta{Name: marketplaceName},
	}}
	for _, e := range entries {
		if e.IsDir() || !isSidecarBinary(e.Name()) {
			continue
		}
		// The control-plane binaries are NOT catalog items. What to install is
		// compiled in (controlplane.InfraBinaries); only where and which
		// version are open, and that is what the infra channel answers.
		//
		// Publishing them here is what let three subscribed marketplaces each
		// serve "katyoti", making the ONE binary that must always resolve the
		// one that could not -- and let any subscription quietly offer a
		// different katyoti.exe, the binary that installs everything else.
		if controlplane.IsInfra(strings.TrimSuffix(e.Name(), ".exe")) {
			out.Skips = append(out.Skips, Skip{Binary: e.Name(), Reason: "control-plane binary: published to the infra channel, not the catalog"})
			continue
		}
		desc, err := runDescribe(filepath.Join(platforms[host].Dir, e.Name()))
		if err != nil {
			out.Skips = append(out.Skips, Skip{Binary: e.Name(), Reason: err.Error(), Failed: true})
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".exe")
		assets, staged := collectAssets(platforms, base, baseURL, desc.Name, desc.Version, layout)
		if len(assets) == 0 {
			out.Skips = append(out.Skips, Skip{Binary: e.Name(), Reason: "no platform ships this binary"})
			continue
		}
		out.Catalog.Items = append(out.Catalog.Items, assembleItem(desc, assets))
		out.Staged = append(out.Staged, staged...)
	}
	return out, nil
}

// BuildMarkerFile records which build produced a platform folder. Written by
// scripts/build-cross.sh; read back to prove every platform in a release came
// from one build.
const BuildMarkerFile = ".katyoti-build"

// VerifyBuild fails unless every platform folder of a built tree came from one
// build — what a release refuses, asked before anything is published.
func VerifyBuild(dist string) error {
	platforms, err := DiscoverPlatforms(dist)
	if err != nil {
		return err
	}
	return verifyOneBuild(platforms)
}

// verifyOneBuild fails unless every platform folder carries the same build
// marker. A tree with no markers anywhere is accepted — it was assembled by
// hand rather than by the build script, and there is nothing to compare.
func verifyOneBuild(platforms []Platform) error {
	markers := make(map[string]string, len(platforms))
	var missing []string
	for _, p := range platforms {
		data, err := os.ReadFile(filepath.Join(p.Dir, BuildMarkerFile))
		if err != nil {
			missing = append(missing, p.Sub)
			continue
		}
		markers[p.Sub] = strings.TrimSpace(string(data))
	}
	if len(markers) == 0 {
		return nil
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"platform(s) %s have no %s marker while others do — they are left over from an earlier build; rebuild every platform (make dist-all) before publishing",
			strings.Join(missing, ", "), BuildMarkerFile)
	}
	var first, firstSub string
	for sub, m := range markers {
		if first == "" {
			first, firstSub = m, sub
			continue
		}
		if m != first {
			return fmt.Errorf(
				"platforms disagree on which build they came from (%s=%q vs %s=%q) — a published version is immutable, so all platforms must be one build; run `make dist-all` before publishing",
				firstSub, first, sub, m)
		}
	}
	return nil
}

// AssetPath is where a published asset lives, relative to the catalog:
// `<version>/<goos>-<goarch>/<filename>`.
//
// The version leads the path because a catalog advertises several versions at
// once. Without it, publishing v2 would overwrite v1's bytes while v1's entry
// still records v1's sha256 — the catalog would resolve, download, then fail
// the hash check.
//
// There is deliberately NO per-item folder: the filename already identifies the
// item (binaries are unique within one build's platform folder), so an item
// prefix only lengthened every URL. Version-first also makes the common cleanup
// trivial — retiring a whole release is one `rm -rf <version>/` instead of a
// walk across every item. The trade-off is that a `<version>/<arch>/` directory
// is now SHARED by every item, so pruning ONE item's version must delete that
// item's files and GC the directory only once it is empty (see
// authoring.Release) — never RemoveAll the version folder.
// SidecarsDir is the top-level folder holding every sidecar asset. A deploy
// root has one per kind — `infra/`, `sidecars/`, `config-sets/`, `seeds/`, `vaults/`,
// `solutions/` — beside
// the catalog and its key, so what a tree contains is readable from one `ls`
// instead of inferred from a scatter of version folders at the root.
const SidecarsDir = "sidecars"

// AssetPath is where one platform's binary is served from, inside the catalog's
// own tree.
//
// VERSION-FIRST, not item-first: every sidecar of one build shares a version,
// so `sidecars/<version>/` is the whole release and deleting it retires that
// release completely. An item-first layout would scatter one release across
// twenty folders and make retention a per-item walk.
func AssetPath(version, goos, arch, filename string) string {
	return strings.Join([]string{SidecarsDir, version, goos + "-" + arch, filename}, "/")
}

// ContentAssetPath is where a files-only sidecar's bundle is served from.
//
// It sits beside the compiled binaries of the same version, with no
// <goos>-<goarch> level: a folder of markdown and YAML is the same bytes
// everywhere. So `sidecars/<version>/` holds both kinds of sidecar, which is
// exactly what they are.
func ContentAssetPath(version, name string) string {
	return strings.Join([]string{SidecarsDir, version, name + ".tar.gz"}, "/")
}

// ConfigSetAssetPath is where a config-set bundle is served from. Same
// version-first rule, its own top-level folder: config-sets are configuration,
// not runnable units, and mixing them into `sidecars/` would blur that.
func ConfigSetAssetPath(version, name string) string {
	return strings.Join([]string{"config-sets", version, name + ".tar.gz"}, "/")
}

// SeedAssetPath is where a release's seed is served from: its own folder,
// version first, like every other asset.
func SeedAssetPath(version, name string) string {
	return strings.Join([]string{"seeds", version, name + ".tar.gz"}, "/")
}

// collectAssets hashes base's binary in every platform folder that ships it and
// returns one Asset each plus where each file must be placed. Missing or
// unreadable files are skipped, so a platform that failed to build simply has
// no asset rather than a broken one.
func collectAssets(platforms []Platform, base, baseURL, item, version string, layout Layout) ([]mktapi.Asset, []StagedAsset) {
	var assets []mktapi.Asset
	var staged []StagedAsset
	for _, p := range platforms {
		filename := base + exeSuffix(p.OS)
		src := filepath.Join(p.Dir, filename)
		sum, err := sha256File(src)
		if err != nil {
			continue
		}
		rel := AssetPath(version, p.OS, p.Arch, filename)
		if layout == LayoutFlat {
			rel = filename
			if p.Sub != "" {
				rel = p.Sub + "/" + filename
			}
		}
		asset := mktapi.Asset{
			OS:     p.OS,
			Arch:   p.Arch,
			URL:    joinURL(baseURL, rel),
			Sha256: sum,
		}
		staged = append(staged, StagedAsset{Src: src, Rel: rel})
		// A companion archive beside the binary ships with it.
		bundleName := base + mktapi.BundleSuffix
		bundleSrc := filepath.Join(p.Dir, bundleName)
		if bsum, berr := sha256File(bundleSrc); berr == nil {
			brel := AssetPath(version, p.OS, p.Arch, bundleName)
			if layout == LayoutFlat {
				brel = bundleName
				if p.Sub != "" {
					brel = p.Sub + "/" + bundleName
				}
			}
			asset.Bundle = &mktapi.AssetBundle{URL: joinURL(baseURL, brel), Sha256: bsum}
			staged = append(staged, StagedAsset{Src: bundleSrc, Rel: brel})
		}
		assets = append(assets, asset)
	}
	return assets, staged
}

func exeSuffix(goos string) string {
	if goos == "windows" {
		return ".exe"
	}
	return ""
}

// joinURL forms an asset URL from the publisher's base and the asset's path.
// An empty baseURL yields a CLEAN relative URL (no leading slash), so the
// consumer resolves it against the catalog's own location — which is what lets
// the whole published folder be copied or hosted anywhere. A non-empty baseURL
// is prepended, for assets hosted apart from the catalog.
func joinURL(baseURL, assetPath string) string {
	if b := strings.TrimRight(baseURL, "/"); b != "" {
		return b + "/" + assetPath
	}
	return assetPath
}

// notSidecars are ktw-* binaries that are NOT marketplace sidecars, mirroring
// the discovery exclusions in appcore/sidecar/client. None has a `describe`
// verb, so indexing them yields nothing — excluding them by name keeps the
// build's skip report free of expected noise, so a real gap stands out.
//
// ktw-gui is deliberately NOT excluded: it is a CONTROL-PLANE binary that
// `katyoti install-infra` must be able to install, so it needs a catalog entry.
// It gained a headless `describe` short-circuit (app/ktw-gui/describe.go) that
// returns before wails.Run, so indexing it no longer opens a WebView window.
// ktw-agents is NOT excluded: it is wired into universes as a member like any
// other sidecar, so a universe that lists it must be able to INSTALL it. It sat
// here because it could not answer `describe` — it builds its app by hand and
// never registered the command — which is a gap to fix, not a reason to keep it
// out of the catalog. It has the verb now (app/ktw-agents/cmd/root.go).
var notSidecars = map[string]bool{
	"ktw-utils": true, // CLI utilities
	"ktw-forms": true, // Wails forms app
}

func isSidecarBinary(name string) bool {
	if strings.HasSuffix(name, mktapi.BundleSuffix) {
		return false // a sidecar's companion archive, published with its binary
	}
	if selfupdate.IsLeftover(name) {
		return false // a swap's leftover (ktw-x.exe.old-…): a stale copy, never published
	}
	base := strings.TrimSuffix(name, ".exe")
	if notSidecars[base] {
		return false
	}
	return base == "katyoti" || strings.HasPrefix(base, "ktw-")
}

// assembleItem builds a catalog Item from describe metadata + its per-platform
// assets. Pure — the exec + hashing are separated so it is unit-testable. The
// version's requires carry the formats it writes and the programs, keys and
// logins it needs, so the catalog shows them before anything is installed.
func assembleItem(desc appcore.SidecarDescription, assets []mktapi.Asset) mktapi.Item {
	req := mktapi.NeedsOf(desc.Name, desc.Tools, desc.APIKeys, desc.Auth)
	req.Formats = desc.Formats
	return mktapi.Item{
		Name:        desc.Name,
		Kind:        mktapi.KindSidecar,
		DisplayName: desc.DisplayName,
		Description: desc.Description,
		Icon:        desc.Icon,
		Versions: []mktapi.ItemVersion{{
			Version:  desc.Version,
			Assets:   assets,
			Requires: req,
		}},
	}
}

func runDescribe(path string) (appcore.SidecarDescription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), describeTimeout)
	defer cancel()
	var d appcore.SidecarDescription
	out, err := exec.CommandContext(ctx, path, "describe", "--out", "json").Output()
	if err != nil {
		return d, fmt.Errorf("run %s describe: %w", path, err)
	}
	if err := json.Unmarshal(out, &d); err != nil {
		return d, fmt.Errorf("parse describe of %s: %w", path, err)
	}
	if d.Name == "" {
		return d, fmt.Errorf("%s describe returned no name", path)
	}
	return d, nil
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// MarshalCatalog serializes a catalog to katyoti-marketplace.yaml bytes.
func MarshalCatalog(cat mktapi.Catalog) ([]byte, error) {
	return yaml.Marshal(cat)
}

// Dropped names one item version that retention removed, so the caller can
// delete its asset folder in the same pass. Catalog entries and assets must go
// together: an entry whose assets are gone fails at DOWNLOAD (after the user
// picked it) instead of simply not being offered.
type Dropped struct {
	Item    string
	Version string
	// Assets are the dropped version's asset URLs exactly as the catalog
	// recorded them. Under the version-first layout a `<version>/<arch>/` folder
	// is shared by every item, so the pruner must delete these specific files
	// rather than the enclosing directory. Absolute URLs (a --base-url publish,
	// where the bytes are hosted elsewhere) are present but not locally prunable.
	Assets []string
}

// Merge folds a freshly built catalog into the one already published,
// accumulating versions newest-first so every previously published version
// stays installable.
//
// Republishing an existing <item>@<version> is refused: a published version is
// immutable, the same rule AddSolution enforces for `kind: solution`. Bump the
// version instead. The same files again under that version (every platform's
// sha256 equal) change nothing: the item is left as published.
//
// keep bounds how many versions each item retains (<= 0 keeps all). Versions
// are ordered by PUBLISH order — newest first, because the fresh one is
// prepended — never by parsing the version string, which is not always a
// semver (a dev build carries a git revision).
func Merge(published, fresh mktapi.Catalog, keep int) (mktapi.Catalog, []Dropped, error) {
	out := published
	// RETRACT infra that an older publisher put in the catalog.
	//
	// Excluding infra from fresh BUILDS does not retract what is already
	// published: an accumulating catalog keeps every version it ever carried,
	// so a tree published before the infra channel existed keeps serving
	// katyoti/ktw-hub/ktw-gui forever -- and keeps reproducing the ambiguity
	// that made install-infra unresolvable. Dropping them on every republish
	// makes an existing tree heal itself the first time it is rebuilt.
	//
	// Safe to drop rather than deprecate: nothing INSTALLS infra from a catalog
	// any more, so a retained entry can only mislead.
	kept := out.Items[:0]
	for _, it := range out.Items {
		if controlplane.IsInfra(it.Name) {
			continue
		}
		kept = append(kept, it)
	}
	out.Items = kept
	if out.APIVersion == 0 {
		out.APIVersion = fresh.APIVersion
	}
	if out.Marketplace.Name == "" {
		out.Marketplace = fresh.Marketplace
	}

	var dropped []Dropped
	for _, item := range fresh.Items {
		if len(item.Versions) != 1 {
			return mktapi.Catalog{}, nil, fmt.Errorf(
				"%s: a freshly built item carries exactly one version, got %d", item.Name, len(item.Versions))
		}
		incoming := item.Versions[0]

		idx := -1
		for i := range out.Items {
			if out.Items[i].Name == item.Name {
				idx = i
				break
			}
		}
		if idx == -1 {
			out.Items = append(out.Items, item)
			continue
		}

		republished := false
		for _, v := range out.Items[idx].Versions {
			if v.Version != incoming.Version {
				continue
			}
			// The same files again (a member a later cut ships unchanged)
			// change nothing; other bytes under a published version would.
			if !sameArtifacts(v, incoming) {
				return mktapi.Catalog{}, nil, fmt.Errorf(
					"%s@%s is already published — published versions are immutable; bump the version",
					item.Name, incoming.Version)
			}
			republished = true
		}
		if republished {
			continue
		}

		// Metadata (display name, description, icon) tracks the newest build.
		merged := item
		merged.Versions = append([]mktapi.ItemVersion{incoming}, out.Items[idx].Versions...)
		var gone []Dropped
		merged.Versions, gone = retain(item.Name, merged.Versions, keep)
		dropped = append(dropped, gone...)
		out.Items[idx] = merged
	}
	// A promoted release shares its candidate's files: a dropped version's
	// file still referenced by a retained version is not to be deleted.
	for i := range dropped {
		dropped[i].Assets = Unreferenced(out, dropped[i].Assets)
	}
	return out, dropped, nil
}

// AddsAVersion is whether fresh carries an <item>@<version> published does
// not: a pass without one republishes what is published.
func AddsAVersion(published, fresh mktapi.Catalog) bool {
	have := map[string]bool{}
	for _, it := range published.Items {
		for _, v := range it.Versions {
			have[it.Name+"@"+v.Version] = true
		}
	}
	for _, it := range fresh.Items {
		for _, v := range it.Versions {
			if !have[it.Name+"@"+v.Version] {
				return true
			}
		}
	}
	return false
}

// sameArtifacts is whether two versions ship the same files: each has assets,
// and every platform's sha256 is the same in both.
func sameArtifacts(a, b mktapi.ItemVersion) bool {
	if len(a.Assets) == 0 || len(a.Assets) != len(b.Assets) {
		return false
	}
	sums := map[string]string{}
	for _, x := range a.Assets {
		sums[x.OS+"/"+x.Arch] = x.Sha256
	}
	for _, x := range b.Assets {
		if s, ok := sums[x.OS+"/"+x.Arch]; !ok || s == "" || s != x.Sha256 {
			return false
		}
	}
	return true
}

// Retain applies retention to a whole catalog: each item keeps its `keep`
// newest versions of each channel (0 keeps all). What it drops carries only
// the files no retained version uses. A canary cut publishes with no
// retention, marks its version canary, then retains: a
// candidate never counts as a stable version.
func Retain(cat mktapi.Catalog, keep int) (mktapi.Catalog, []Dropped) {
	out := cat
	out.Items = make([]mktapi.Item, len(cat.Items))
	var dropped []Dropped
	for i, it := range cat.Items {
		var gone []Dropped
		it.Versions, gone = retain(it.Name, it.Versions, keep)
		out.Items[i] = it
		dropped = append(dropped, gone...)
	}
	for i := range dropped {
		dropped[i].Assets = Unreferenced(out, dropped[i].Assets)
	}
	return out, dropped
}

// retain keeps an item's `keep` newest versions of each channel - release
// candidates never push a stable release out; 0 keeps all.
func retain(item string, versions []mktapi.ItemVersion, keep int) ([]mktapi.ItemVersion, []Dropped) {
	if keep <= 0 {
		return versions, nil
	}
	var retained []mktapi.ItemVersion
	var dropped []Dropped
	counted := map[string]int{}
	for _, v := range versions {
		ch := v.Channel
		if ch == "" {
			ch = mktapi.ChannelStable
		}
		if counted[ch] < keep {
			counted[ch]++
			retained = append(retained, v)
			continue
		}
		urls := make([]string, 0, len(v.Assets))
		for _, a := range v.Assets {
			urls = append(urls, a.URLs()...)
		}
		dropped = append(dropped, Dropped{Item: item, Version: v.Version, Assets: urls})
	}
	return retained, dropped
}

// Unreferenced is the asset URLs no version of the catalog references any
// more - the ones a pruner may delete. A promotion publishes a candidate's
// own files under the official version, so one file can be two versions'.
func Unreferenced(cat mktapi.Catalog, urls []string) []string {
	used := map[string]bool{}
	for _, it := range cat.Items {
		for _, v := range it.Versions {
			for _, a := range v.Assets {
				for _, u := range a.URLs() {
					used[u] = true
				}
			}
		}
	}
	var out []string
	for _, u := range urls {
		if !used[u] {
			out = append(out, u)
		}
	}
	return out
}
