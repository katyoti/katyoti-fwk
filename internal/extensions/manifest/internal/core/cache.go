// SPDX-License-Identifier: Apache-2.0

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Cache is the per-sidecar on-disk manifest cache that replaces the
// flat-dir `Inventory` (2026-06-05 redesign).
//
// Layout under <Root>:
//
//	<Root>/<sidecar>@<key>/metadata.yaml   # exe stat + manifest hash + indexed_at + include_system
//	<Root>/<sidecar>@<key>/manifest.json   # the full denormalized manifest
//
// A record is addressed by (member name, binary): `<key>` is ExeKey of the
// binary the record was taken from. The cache root is shared by every
// universe on the host, and a universe declares its members by name, so the
// name alone is not an identity: two universes naming `ktw-x` at two binaries
// hold two records, and neither ever reads the other's as fresh (blind audit
// H1). Two universes naming the same binary share one record, which is the
// point of a shared cache.
//
// Each record is rewritten atomically (write to sibling temp then rename)
// when its content changes. Stat-based invalidation (exe_size + exe_mtime)
// makes "is this cache stale?" cheap: one os.Stat per sidecar at hub
// boot, no spawn.
//
// Replaces the prior shape (`<Root>/<name>-<version>-<hash>.json` flat)
// which had two structural problems: lex-order sort on content-
// addressed filenames silently served stale entries (see
// `Inventory.LoadLatest` doc), and no GC — the dir grew unboundedly.
//
// A record that depends on the universe as well as on the binary — one whose
// manifest declares a universe read root, or that was taken with a universe's
// override tree — is a different record in every universe: it is kept in its
// own folder, `<name>@<key>~<universe key>`, and a universe never reads another
// one's (ForUniverse). Every other record stays shared.
type Cache struct {
	Root string // absolute path to the cache root directory

	// scope is the key of the universe this cache is read for ("" for an
	// unscoped cache, which reads and writes shared records only); anchors
	// resolve a record's read roots in that universe (ReadRoot.Anchor → dir).
	scope    string
	scopeDir string
	anchors  map[string]string
}

// ForUniverse binds the cache to one universe: its read roots resolve against
// universeDir and globalDir, and records that depend on the universe are read
// from and written to that universe's own folder. Returns the cache.
func (c *Cache) ForUniverse(universeDir, globalDir string) *Cache {
	c.scopeDir = universeDir
	c.scope = scopeKey(universeDir)
	c.anchors = map[string]string{
		mbapi.ReadRootUniverse: universeDir,
		mbapi.ReadRootGlobal:   globalDir,
	}
	return c
}

// scopeSep separates a universe-scoped record's binary key from its universe
// key. After recordSep, so splitRecordDir still reads the member name.
const scopeSep = "~"

// scopeKey is the universe key of a folder: one universe reached by two
// spellings of its path (a relative path, `d:` and `D:` on Windows) is one
// scope, never two records probed twice.
func scopeKey(dir string) string {
	if dir == "" {
		return ""
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if runtime.GOOS == "windows" {
		dir = strings.ToLower(dir)
	}
	return ExeKey(dir)
}

// Metadata is the per-sidecar stat + integrity record stored at
// `<Root>/<sidecar>/metadata.yaml`. Read once at hub boot to decide
// "fresh cache or re-fetch?".
//
// `ExeSize` + `ExeModTime` together drive invalidation: a rebuild
// almost always changes one or both. False positives (mtime touched
// without content change) are tolerable — they just cost one extra
// re-fetch. False negatives (content changed but stat looks same) need
// the same size AND the same stamp at the filesystem's own resolution;
// where that resolution is a whole second the stat is reported undecided
// (StatUndecided) and the recorded provider digests break the tie.
//
// `ManifestHash` mirrors the `Manifest.Hash` field; lets us detect
// integrity drift between metadata.yaml and manifest.json (rare;
// indicates a partial-write race). When they disagree the cache is
// treated as stale.
//
// `IndexedAt` is informational — surfaced in the GUI for "last
// refreshed" timestamps. NOT used for invalidation.
// metadataFormat is the current Metadata layout; bump when a field that IsStale
// compares is added or the record key changes, so older records re-probe
// instead of reading fresh. 2 added include_system; 3 keyed the record by
// (name, binary) — a record of the older key copied into a keyed folder is
// re-probed rather than trusted; 4 probes members without their `local:`
// entries (blind audit M3) — a record taken with them is the wrong record of
// the right binary, and re-probes rather than being served and filtered; 5
// records the read roots and their digest, and keeps a universe-dependent
// record per universe — a shared record of such a member is another
// universe's.
const metadataFormat = 5

type Metadata struct {
	ExePath      string    `yaml:"exe_path"`
	ExeSize      int64     `yaml:"exe_size"`
	ExeModTime   time.Time `yaml:"exe_mtime"`
	ManifestHash string    `yaml:"manifest_hash"`
	IndexedAt    time.Time `yaml:"indexed_at"`
	// ContentDigest fingerprints a SERVED FOLDER — content the binary reads
	// at runtime rather than carries. A generic host running someone's folder
	// has the same executable whatever that folder holds, so the exe stat
	// below is blind to every edit in it: the author changes a skill and the
	// aggregator never asks again. Empty for a binary that serves no folder.
	ContentDigest string `yaml:"content_digest,omitempty"`

	// OverwritesDigest fingerprints the sidecar's workspace override tree
	// (`.katyoti/overwrites/<sidecar>/`) at Save time. The exe stat alone
	// can't see override edits — the binary is unchanged — so without this a
	// changed override would serve a stale cached manifest in agent mode
	// (the recurring staleness bug). Empty when the universe gate
	// (allow_overwrites) is off, so prod never refetches on a stray file.
	OverwritesDigest string `yaml:"overwrites_digest,omitempty"`

	// ReadRoots are the folders the manifest declares it reads (Manifest.
	// ReadRoots), and ReadsDigest their fingerprint at Save, resolved in the
	// universe that saved it. The exe stat is blind to an edit there; the
	// digest, recomputed against the same roots, is not.
	ReadRoots   []mbapi.ReadRoot `yaml:"read_roots,omitempty"`
	ReadsDigest string           `yaml:"reads_digest,omitempty"`

	// Scope is the key of the universe a universe-dependent record was taken
	// in, and ScopeDir that universe's folder; both empty for a shared record.
	// GC removes a scoped record whose universe folder is gone.
	Scope    string `yaml:"scope,omitempty"`
	ScopeDir string `yaml:"scope_dir,omitempty"`

	// IncludeSystem records how the manifest was taken: true when it carries
	// the embedded `system` meta-model, false when it was probed with
	// `--include-system=false`. Since e26 the answer depends on the record's
	// owner alone — true for the hub's own manifest, the one carrier of the
	// system set, false for every probed member — never on the universe that
	// probed it. It is still recorded and compared because caches are shared
	// and outlive the rule: a member record written before e26 as its
	// universe's carrier holds the set, and must read stale rather than serve
	// a second copy beside the hub's. Always written, never omitted, so the
	// record states the setting rather than implying it; a record that
	// predates the field reads false.
	IncludeSystem bool `yaml:"include_system"`

	// Format is the record layout version. A record written before the layout
	// carried include_system decodes that field as false whatever the probe
	// was run with; such a record could serve a second copy of the system set
	// after an upgrade, so IsStale re-probes any record below the current
	// format (re-audit N20, 2026-09-25).
	Format int `yaml:"format"`
	// Folder says the record was taken from a FOLDER member's root (ExePath
	// names the folder, the hub read it itself): GC sweeps it once the folder
	// is gone, as nothing will ever read it again.
	Folder bool `yaml:"folder,omitempty"`
}

// SourceDigests fingerprints the sources a binary's contributions depend on but
// whose changes its EXECUTABLE cannot show.
//
// The exe stat is the cheap, correct staleness key for a compiled sidecar: a
// rebuild changes it. It is blind, though, to anything the binary READS rather
// than carries — a workspace override tree, or a folder a generic host serves.
// Each of those gets a digest here, and a change in any of them invalidates the
// cached manifest exactly as a rebuild would.
//
// Named rather than passed as loose strings: two adjacent digest parameters of
// the same type can be swapped silently, and the result would be a staleness
// check that compares the wrong source against the wrong record.
type SourceDigests struct {
	// Binary is the executable the universe DECLARES for the member — the
	// one the probe would run. A record was taken from one binary
	// (Metadata.ExePath) and is the record of that binary only: a universe
	// naming the same member at another path must not read it as fresh,
	// however that other binary's stat looks (blind audit H1). Required —
	// IsStale refuses an empty value rather than statting whatever path the
	// record happens to name.
	Binary string
	// Overwrites fingerprints `.katyoti/overwrites/<sidecar>/`.
	Overwrites string
	// Content fingerprints a served folder's tree.
	Content string
	// IncludeSystem is the system setting the record must have been taken
	// with: true for the hub's own manifest (the one carrier), false for
	// every member (e26). Not a digest but the same kind of key — the binary
	// is unchanged and a record taken the other way is not the one wanted —
	// so it travels with the digests, is recorded by Save and compared by
	// IsStale.
	IncludeSystem bool
	// ReadRoots and Reads are the read roots of the member's previous record
	// and their fingerprint, taken BEFORE the probe (Cache.ReadsBeforeProbe).
	// Save records Reads when the probed manifest declares the same roots: an
	// edit landing while the probe runs then reads as a change at the next
	// check, instead of being fingerprinted as seen without being enumerated.
	ReadRoots []mbapi.ReadRoot
	Reads     string
}

// DigestDir fingerprints a directory tree from its files' relative paths,
// sizes, and mtimes (sorted for determinism). It is the override-tree staleness
// key folded into Metadata.OverwritesDigest. Returns "" for a missing or empty
// dir — the prod-safe value (matches a gate-off "" digest, so no spurious
// refetch). Stat-based (not content-hashed) for cheapness: an override edit
// always bumps size or mtime, the same false-negative bound the exe stat takes.
func DigestDir(dir string) string {
	if dir == "" {
		return ""
	}
	type ent struct {
		rel  string
		size int64
		mod  int64
	}
	var ents []ent
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // tolerate a vanished subtree; absent → empty digest
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return nil
		}
		ents = append(ents, ent{rel: filepath.ToSlash(rel), size: info.Size(), mod: info.ModTime().UnixNano()})
		return nil
	})
	if err != nil || len(ents) == 0 {
		return ""
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].rel < ents[j].rel })
	h := sha256.New()
	for _, e := range ents {
		h.Write([]byte(e.rel))
		h.Write([]byte{0})
		h.Write([]byte(strconv.FormatInt(e.size, 10)))
		h.Write([]byte{0})
		h.Write([]byte(strconv.FormatInt(e.mod, 10)))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// NewCache constructs a Cache rooted at the given directory. Caller
// passes `<DataDir>/cache`. The directory is created on first Save —
// Load tolerates absence and returns (nil, nil, nil).
func NewCache(root string) *Cache {
	return &Cache{Root: root}
}

// Member is one entry of a catalog's membership: the name a universe declares
// and the binary it declares for that name. Together they address the
// member's record — the name alone does not, since the cache root is shared
// by every universe on the host. A member declared with no binary has no
// record (nothing is probed for it) and loads as a clean miss.
type Member struct {
	Name   string
	Binary string
	// Folder is a folder member's content root: the hub builds its record
	// from the folder itself (no Binary).
	Folder string
}

// recordSep separates the member name from the binary key in a record
// folder's name. Outside the kebab alphabet of a member name, so the folder
// name splits unambiguously.
const recordSep = "@"

// ExeKey is the folder-name component that identifies a binary: a prefix of
// the sha256 of its cleaned path. Two declarations of one path key alike;
// two paths never collide in practice (48 bits). Empty for an empty path.
func ExeKey(exe string) string {
	if exe == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(filepath.Clean(exe)))
	return hex.EncodeToString(sum[:6])
}

// sameBinary answers whether two paths name the same binary, as the key does:
// by cleaned path, the same reading on both sides of every compare.
func sameBinary(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

// RecordDir is the folder holding the record of `name` taken from `exe`:
// `<Root>/<name>@<ExeKey(exe)>`. Exported so tests and the operator's
// `cache show` can name the folder without re-deriving the key.
func (c *Cache) RecordDir(name, exe string) string {
	return filepath.Join(c.Root, name+recordSep+ExeKey(exe))
}

// scopedRecordDir is the folder of the record of `name` taken from `exe` in
// this cache's universe; "" for an unscoped cache.
func (c *Cache) scopedRecordDir(name, exe string) string {
	if c.scope == "" {
		return ""
	}
	return c.RecordDir(name, exe) + scopeSep + c.scope
}

// RecordPath is the folder Load reads the record of `name` taken from `exe`
// from: this universe's own record when it has one, else the shared one.
func (c *Cache) RecordPath(name, exe string) string {
	if dir := c.scopedRecordDir(name, exe); dir != "" {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	return c.RecordDir(name, exe)
}

// splitRecordDir reads a record folder's name back into the member name.
// ok is false for anything that is not a record: a `.tmp` leftover, or a
// folder of the older name-only key.
func splitRecordDir(folder string) (name string, ok bool) {
	if strings.HasSuffix(folder, ".tmp") {
		return "", false
	}
	i := strings.LastIndex(folder, recordSep)
	if i <= 0 || i == len(folder)-1 {
		return "", false
	}
	return folder[:i], true
}

// Load reads the cached manifest + metadata for the given sidecar, as taken
// from `exe`, the binary the universe declares for it. Returns (nil, nil,
// nil) when the cache has no such record — callers treat absence as "need
// refetch" without distinguishing it from other errors. An empty `exe` is a
// member nothing is probed for: a clean miss.
//
// A record in the folder whose metadata names another binary is refused
// with an error naming both paths, like the hash cross-check below: it is
// not the record of the declared binary, whatever put it there.
//
// A partial-write race (manifest written but metadata missing, or
// vice versa) returns (nil, nil, error) — defensive: the cache for
// that sidecar is unusable, the caller must re-fetch.
func (c *Cache) Load(sidecar, exe string) (*mbapi.Manifest, *Metadata, error) {
	if exe == "" {
		return nil, nil, nil
	}
	manifest, meta, err := c.loadDir(sidecar, c.RecordPath(sidecar, exe))
	if err != nil || manifest == nil {
		return nil, nil, err
	}
	if !sameBinary(meta.ExePath, exe) {
		return nil, nil, fmt.Errorf("cache %s: record was taken from %s, not the declared binary %s — re-fetch required",
			sidecar, meta.ExePath, exe)
	}
	// A shared record written before records were kept per universe may be
	// another universe's (a member that reads its universe was shared then).
	// A universe's cache never serves it, even when the probe that should
	// replace it fails: a miss, not another universe's entries.
	if c.scope != "" && meta.Scope == "" && meta.Format < metadataFormat {
		return nil, nil, nil
	}
	return manifest, meta, nil
}

// LoadMetadata is Load without the manifest: the record's metadata, under
// Load's rules — the declared binary's record only, never another universe's
// record written before records were kept per universe, nothing when the
// manifest beside it is gone. What a watch round asks (does the record still
// match its sources?) needs only these few lines; the manifest can be
// megabytes, and decoding it every round kept an idle hub busy. The pair's
// hash cross-check is Load's: a round that finds the record stale re-probes
// through it.
func (c *Cache) LoadMetadata(sidecar, exe string) (*Metadata, error) {
	if exe == "" {
		return nil, nil
	}
	dir := c.RecordPath(sidecar, exe)
	meta, err := readMetadata(dir)
	if err != nil {
		return nil, fmt.Errorf("cache %s: read metadata: %w", sidecar, err)
	}
	if meta == nil {
		return nil, nil // clean miss
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		return nil, fmt.Errorf("cache %s: read manifest: %w", sidecar, err)
	}
	if !sameBinary(meta.ExePath, exe) {
		return nil, fmt.Errorf("cache %s: record was taken from %s, not the declared binary %s — re-fetch required",
			sidecar, meta.ExePath, exe)
	}
	if c.scope != "" && meta.Scope == "" && meta.Format < metadataFormat {
		return nil, nil
	}
	return meta, nil
}

// loadDir reads and cross-checks the pair in one record folder. The binary
// check is Load's; Records lists a record whatever binary it was taken from.
func (c *Cache) loadDir(sidecar, dir string) (*mbapi.Manifest, *Metadata, error) {
	metaPath := filepath.Join(dir, "metadata.yaml")
	manPath := filepath.Join(dir, "manifest.json")

	metaBytes, metaErr := os.ReadFile(metaPath)
	manBytes, manErr := os.ReadFile(manPath)
	if os.IsNotExist(metaErr) && os.IsNotExist(manErr) {
		return nil, nil, nil // clean miss
	}
	if metaErr != nil {
		return nil, nil, fmt.Errorf("cache %s: read metadata: %w", sidecar, metaErr)
	}
	if manErr != nil {
		return nil, nil, fmt.Errorf("cache %s: read manifest: %w", sidecar, manErr)
	}

	var meta Metadata
	if err := config.DecodeBytes(metaBytes, marshal.YAML, &meta); err != nil {
		return nil, nil, fmt.Errorf("cache %s: decode metadata: %w", sidecar, err)
	}
	var manifest mbapi.Manifest
	if err := json.Unmarshal(manBytes, &manifest); err != nil {
		return nil, nil, fmt.Errorf("cache %s: decode manifest: %w", sidecar, err)
	}
	// Integrity check — if metadata and manifest disagree on hash, the
	// pair was probably written by two concurrent processes interleaving
	// or a previous run crashed mid-rename. Treat as stale; caller will
	// re-fetch.
	if meta.ManifestHash != "" && manifest.Hash != "" && meta.ManifestHash != manifest.Hash {
		return nil, nil, fmt.Errorf("cache %s: hash mismatch (metadata=%q manifest=%q) — re-fetch required",
			sidecar, meta.ManifestHash, manifest.Hash)
	}
	return &manifest, &meta, nil
}

// IsStale reports whether the cached metadata is the record of the sources
// `live` names and still matches the declared binary's stat. Returns (false,
// "") when fresh; (true, "<reason>") when invalidated.
//
// The declared binary is compared first: a record taken from another path is
// another binary's record, whatever its stat, and the reason names both paths.
// Then ExeSize and ExeModTime are compared against a fresh os.Stat of the
// DECLARED binary — never of whatever path the record names; if it no longer
// resolves (sidecar binary moved or deleted), staleness is flagged with
// reason="exe missing".
//
// A "true" return doesn't itself trigger a re-fetch — the caller is
// expected to combine it with a spawn-and-refetch step.
func (c *Cache) IsStale(meta *Metadata, live SourceDigests) (bool, string) {
	if meta == nil {
		return true, "no metadata"
	}
	if meta.ExePath == "" {
		return true, "metadata has empty exe_path"
	}
	if live.Binary == "" {
		return true, "no declared binary to compare the record against"
	}
	if !sameBinary(meta.ExePath, live.Binary) {
		return true, fmt.Sprintf("declared binary is %s; the record was taken from %s", live.Binary, meta.ExePath)
	}
	// Sources the exe stat cannot see, compared next. Both are cases where
	// the binary is byte-identical and its contributions are not.
	if live.Overwrites != meta.OverwritesDigest {
		return true, fmt.Sprintf("overwrites changed: %q → %q", meta.OverwritesDigest, live.Overwrites)
	}
	if live.Content != meta.ContentDigest {
		return true, fmt.Sprintf("served content changed: %q → %q", meta.ContentDigest, live.Content)
	}
	// A record that depends on the universe is the record of the universe it
	// was taken in — another universe's is not this universe's, whatever its
	// digests say.
	if universeDependent(meta.ReadRoots, meta.OverwritesDigest) && meta.Scope != c.scope {
		return true, fmt.Sprintf("record taken in universe %q, read in %q", meta.Scope, c.scope)
	}
	// The folders the binary declared it reads, fingerprinted again here, in
	// this universe: an edit there leaves the binary byte-identical.
	if now := c.readsDigest(meta.ReadRoots); now != meta.ReadsDigest {
		return true, fmt.Sprintf("read folders changed: %q → %q", meta.ReadsDigest, now)
	}
	// Saved with the other system setting: right binary, wrong record — in
	// practice a member record written before e26 as its universe's system
	// carrier, which would be a second copy beside the hub's.
	if meta.Format < metadataFormat {
		return true, fmt.Sprintf("record format %d predates %d (include_system unknown): re-probe", meta.Format, metadataFormat)
	}
	if live.IncludeSystem != meta.IncludeSystem {
		return true, fmt.Sprintf("include_system changed: %t → %t", meta.IncludeSystem, live.IncludeSystem)
	}
	info, err := os.Stat(live.Binary)
	if err != nil {
		if os.IsNotExist(err) {
			return true, "exe missing: " + live.Binary
		}
		return true, fmt.Sprintf("stat %s: %v", live.Binary, err)
	}
	if info.Size() != meta.ExeSize {
		return true, fmt.Sprintf("exe_size changed: %d → %d", meta.ExeSize, info.Size())
	}
	// Compared exactly, at whatever resolution the filesystem offers (100 ns
	// on NTFS, ns on ext4 and APFS, 2 s on FAT): the record and the compare
	// stat the same file on the same host, so both see the same rounding.
	// It used to truncate both to the second, and a same-size rebuild inside
	// one second read fresh (blind audit M2). Where the filesystem keeps
	// only whole seconds the stat is UNDECIDED rather than fresh, and
	// StatUndecided says so; the recorded provider digests then break the
	// tie (ProvidersMoved) on the re-probe the caller runs.
	if !info.ModTime().Equal(meta.ExeModTime) {
		return true, fmt.Sprintf("exe_mtime changed: %s → %s",
			meta.ExeModTime.Format(time.RFC3339Nano), info.ModTime().Format(time.RFC3339Nano))
	}
	return false, ""
}

// StatUndecided reports whether the declared binary's stat matches the record
// WITHOUT being able to tell a rebuild from it: same size, same mtime, and an
// mtime with no sub-second part — a filesystem that keeps whole seconds (FAT),
// or a stamp a tool set to one (an archive's), so that two builds inside that
// second share it. On a filesystem that keeps a finer stamp the stat decides
// alone, and this is false whatever IsStale said; it is also false whenever
// IsStale already answers stale.
//
// It is the caller's cue to run the tie-breaker (ProvidersMoved) on a
// re-probe: the tier-1 identity stays the stat, and the digests are consulted
// only where the stat has this one gap (OD-2, 2026-09-26).
func (c *Cache) StatUndecided(meta *Metadata, live SourceDigests) bool {
	if stale, _ := c.IsStale(meta, live); stale {
		return false
	}
	info, err := os.Stat(live.Binary)
	if err != nil {
		return false
	}
	return info.ModTime().Nanosecond() == 0
}

// ProvidersMoved is the tie-breaker for the stat's one gap: the immutable
// providers a member's manifest recorded (Manifest.Providers, digest-backed)
// against the ones a fresh probe of the declared binary reports. A digest that
// differs, a provider that appeared or one that disappeared means the binary's
// sources moved although its stat did not; the reason names the provider and
// both digests. A mutable provider carries no digest and never decides.
//
// A method of the cache, whose identity decision it completes; it reads no
// record itself — the caller holds both manifests.
func (c *Cache) ProvidersMoved(recorded, probed []mbapi.ProviderInfo) (bool, string) {
	key := func(p mbapi.ProviderInfo) string { return p.Name + "/" + p.Type }
	was := make(map[string]string, len(recorded))
	for _, p := range recorded {
		if !p.Mutable {
			was[key(p)] = p.Digest
		}
	}
	seen := make(map[string]bool, len(probed))
	for _, p := range probed {
		if p.Mutable {
			continue
		}
		k := key(p)
		seen[k] = true
		before, known := was[k]
		if !known {
			return true, fmt.Sprintf("provider %s appeared (digest %s)", k, p.Digest)
		}
		if before != p.Digest {
			return true, fmt.Sprintf("provider %s digest changed: %s → %s", k, before, p.Digest)
		}
	}
	for k, d := range was {
		if !seen[k] {
			return true, fmt.Sprintf("provider %s gone (was digest %s)", k, d)
		}
	}
	return false, ""
}

// Save atomically writes the manifest + metadata for the given
// sidecar. Stats the binary at exePath to populate ExeSize +
// ExeModTime; IndexedAt = wall-clock now.
//
// Atomicity: writes to a sibling temp dir then renames into place.
// On crash mid-write the next Load sees the old folder intact (or
// no folder, if first save crashed).
func (c *Cache) Save(sidecar string, manifest *mbapi.Manifest, exePath string, digests SourceDigests) error {
	if manifest == nil {
		return fmt.Errorf("cache %s: nil manifest", sidecar)
	}
	if exePath == "" {
		return fmt.Errorf("cache %s: empty exe_path", sidecar)
	}
	info, err := os.Stat(exePath)
	if err != nil {
		return fmt.Errorf("cache %s: stat %s: %w", sidecar, exePath, err)
	}

	meta := Metadata{
		ExePath:          exePath,
		ExeSize:          info.Size(),
		ExeModTime:       info.ModTime(),
		ManifestHash:     manifest.Hash,
		IndexedAt:        time.Now().UTC(),
		OverwritesDigest: digests.Overwrites,
		ContentDigest:    digests.Content,
		IncludeSystem:    digests.IncludeSystem,
		ReadRoots:        manifest.ReadRoots,
		ReadsDigest:      readsTakenBefore(manifest.ReadRoots, digests),
		Format:           metadataFormat,
		Folder:           info.IsDir(),
	}
	// Where the record goes: its universe's own folder when it depends on the
	// universe, the shared one otherwise — and then this universe's own folder
	// of the member, if an earlier probe left one, goes, so Load does not
	// keep preferring it.
	finalDir := c.RecordDir(sidecar, exePath)
	staleScoped := ""
	if c.scope != "" {
		if universeDependent(manifest.ReadRoots, digests.Overwrites) {
			meta.Scope, meta.ScopeDir = c.scope, c.scopeDir
			finalDir = c.scopedRecordDir(sidecar, exePath)
		} else {
			staleScoped = c.scopedRecordDir(sidecar, exePath)
		}
	}

	metaBytes, err := yaml.Marshal(&meta)
	if err != nil {
		return fmt.Errorf("cache %s: marshal metadata: %w", sidecar, err)
	}
	manBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("cache %s: marshal manifest: %w", sidecar, err)
	}

	// Atomic two-file write: build a sibling temp folder, populate it,
	// rename over the live folder. The rename is atomic for the
	// destination NAME — both files become visible together (or neither,
	// if rename fails before completing).
	if err := os.MkdirAll(c.Root, 0o755); err != nil {
		return fmt.Errorf("cache: mkdir root %s: %w", c.Root, err)
	}
	tmpDir := finalDir + ".tmp"
	// Clean any leftover from a prior aborted save.
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return fmt.Errorf("cache %s: mkdir tmp %s: %w", sidecar, tmpDir, err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "metadata.yaml"), metaBytes, 0o644); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("cache %s: write metadata: %w", sidecar, err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "manifest.json"), manBytes, 0o644); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("cache %s: write manifest: %w", sidecar, err)
	}
	// Swap the new record in WITHOUT destroying the live one first. os.Rename
	// onto a non-empty dir is not portable, so the live folder is moved aside,
	// the new one renamed into place, and only then is the old one deleted.
	//
	// It used to be RemoveAll(live) then Rename(tmp). On Windows a reader
	// holding either file open (the catalog reloading on the very fsnotify event
	// a save causes) makes RemoveAll fail HALFWAY: one file gone, the other
	// kept, the save reported failed — and the previous good record destroyed
	// with it (e18, TestCache_ConcurrentRefresh_NoTornRead). Moving aside fails
	// whole or not at all, and is retried while a reader finishes.
	oldDir := finalDir + ".old.tmp" // `.tmp`: ListNames skips it, GC sweeps it
	_ = os.RemoveAll(oldDir)
	if _, err := os.Stat(finalDir); err == nil {
		if err := retrySharing(func() error { return os.Rename(finalDir, oldDir) }); err != nil {
			_ = os.RemoveAll(tmpDir)
			return fmt.Errorf("cache %s: move live record aside %s: %w", sidecar, finalDir, err)
		}
	}
	if err := retrySharing(func() error { return os.Rename(tmpDir, finalDir) }); err != nil {
		// A failed save must cost nothing: put the previous record back.
		_ = os.Rename(oldDir, finalDir)
		return fmt.Errorf("cache %s: rename %s → %s: %w", sidecar, tmpDir, finalDir, err)
	}
	_ = os.RemoveAll(oldDir) // best effort; a leftover is swept by GC
	if staleScoped != "" {
		if err := retrySharing(func() error { return os.RemoveAll(staleScoped) }); err != nil {
			return fmt.Errorf("cache %s: remove this universe's former record %s: %w", sidecar, staleScoped, err)
		}
	}
	return nil
}

// universeDependent answers whether a record is a function of the universe as
// well as of the binary: it declares a read root, or it was taken with the
// universe's override tree. A global root counts too: the global home is the
// same for every universe of one installation, but two hubs of one cache root
// may still resolve it apart (a sandboxed universe has a home of its own), and
// a record shared between them would be re-probed by each on every round.
func universeDependent(roots []mbapi.ReadRoot, overwritesDigest string) bool {
	return overwritesDigest != "" || len(roots) > 0
}

// readsUnknown is the reads digest of a record whose roots were not known
// before its probe (the first probe of a binary on the host, or a rebuild that
// declares other roots): no fingerprint equals it, so the next check re-probes
// once with the roots now recorded, and that probe's record is exact.
const readsUnknown = "unknown: the read roots were learnt by this probe"

// readsTakenBefore is the reads digest a record keeps: the fingerprint taken
// before the probe, when the probed manifest declares the roots it was taken
// over, else readsUnknown. Never a fingerprint taken after the probe — an edit
// landing while the probe ran would read as seen without being enumerated.
func readsTakenBefore(roots []mbapi.ReadRoot, before SourceDigests) string {
	if len(roots) == 0 {
		return ""
	}
	if !slices.Equal(roots, before.ReadRoots) {
		return readsUnknown
	}
	return before.Reads
}

// ReadsBeforeProbe is the reads half of the staleness key, taken before a
// probe: the read roots of the member's record — this universe's, else the
// shared one, else any other universe's of the same binary, since the roots a
// binary declares are the binary's — and their fingerprint now. Empty when no
// record of the binary declares any.
func (c *Cache) ReadsBeforeProbe(sidecar, exe string) ([]mbapi.ReadRoot, string) {
	if exe == "" {
		return nil, ""
	}
	dirs := []string{c.RecordPath(sidecar, exe)}
	others, _ := filepath.Glob(filepath.Join(c.Root, sidecar+recordSep+ExeKey(exe)+scopeSep+"*"))
	dirs = append(dirs, others...)
	for _, dir := range dirs {
		if strings.HasSuffix(dir, ".tmp") {
			continue
		}
		meta, err := readMetadata(dir)
		if err != nil || meta == nil || !sameBinary(meta.ExePath, exe) || len(meta.ReadRoots) == 0 {
			continue
		}
		return meta.ReadRoots, c.readsDigest(meta.ReadRoots)
	}
	return nil, ""
}

// readMetadata decodes one record folder's metadata.yaml; (nil, nil) when the
// folder has none.
func readMetadata(dir string) (*Metadata, error) {
	b, err := os.ReadFile(filepath.Join(dir, "metadata.yaml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var meta Metadata
	if err := config.DecodeBytes(b, marshal.YAML, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// readsDigest fingerprints the declared read roots as this cache's universe
// resolves them: "" when none is declared or every folder is absent or empty,
// so a binary that reads nothing keeps the empty value it always had. A root
// whose anchor this cache cannot resolve (an unscoped cache) counts as absent.
func (c *Cache) readsDigest(roots []mbapi.ReadRoot) string {
	h := sha256.New()
	found := false
	for _, r := range roots {
		base, ok := c.anchors[r.Anchor]
		if !ok || base == "" {
			continue
		}
		d := DigestDir(filepath.Join(base, filepath.FromSlash(r.Path)))
		if d == "" {
			continue
		}
		found = true
		h.Write([]byte(r.Anchor + "/" + r.Path + "=" + d + "\n"))
	}
	if !found {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ReadsMoved answers, from the record's metadata alone, whether a folder the
// member declared it reads changed since the record was taken. The hub's
// member watch asks it every round, beside the executable's stat: a manifest
// can be megabytes, the metadata is a few lines. A member with no record, or
// none it can read, is hydration's, not the watch's: false.
func (c *Cache) ReadsMoved(sidecar, exe string) (bool, string) {
	if exe == "" {
		return false, ""
	}
	meta, err := readMetadata(c.RecordPath(sidecar, exe))
	if err != nil || meta == nil || len(meta.ReadRoots) == 0 {
		return false, ""
	}
	if now := c.readsDigest(meta.ReadRoots); now != meta.ReadsDigest {
		return true, fmt.Sprintf("read folders changed: %q → %q", meta.ReadsDigest, now)
	}
	return false, ""
}

// retrySharing runs a filesystem operation that a concurrent reader can block
// transiently — on Windows a folder cannot be renamed while a file in it is
// open — for up to about a second. Readers of the cache hold a file only for
// the length of one ReadFile, so the window closes quickly.
func retrySharing(op func() error) error {
	var err error
	for attempt := 0; attempt < 100; attempt++ {
		if err = op(); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return err
}

// Invalidate removes every record of the given sidecar, whatever binary each
// was taken from, and their `.tmp` leftovers. Idempotent — no folder is not
// an error. Used by `catalog cache invalidate <name>`.
func (c *Cache) Invalidate(sidecar string) error {
	entries, err := os.ReadDir(c.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("cache %s: invalidate: read root %s: %w", sidecar, c.Root, err)
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), sidecar+recordSep) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.Root, e.Name())); err != nil {
			return fmt.Errorf("cache %s: invalidate %s: %w", sidecar, e.Name(), err)
		}
	}
	return nil
}

// GCResult is the report from a Cache.GC sweep. Surfaces the two
// classes of stale state separately so the operator can decide how
// loud to be about each.
type GCResult struct {
	// TmpDirsRemoved counts `<sidecar>.tmp` leftovers from crashed
	// prior Save calls. Expected to be 0 on a healthy host; a non-zero
	// count points at a recent crash + recovery (informational).
	TmpDirsRemoved []string
	// OrphansRemoved counts cache entries for sidecars that no longer
	// appear in the `keep` allowlist (typically `Univers.yaml`'s
	// `sidecars:` block). Expected to be 0 in steady state; non-zero
	// means a sidecar was removed from the universe and its cache is
	// now reclaimed.
	OrphansRemoved []string
}

// GC sweeps the cache root for two classes of stale state:
//
//   - `<sidecar>.tmp` directories from crashed prior `Save` calls.
//     Save always tries to remove its own .tmp on retry, but a kill
//     between mkdir and rename leaves the dir behind. GC unconditionally
//     deletes them — they're never the authoritative source.
//
//   - Records of names NOT in `keep`, whatever binary each was taken from.
//     Pass the current `sidecars:` list (typically from
//     `appcore.LoadUniverseSidecars()`). Entries absent from the allowlist
//     are removed. A folder of the older name-only key (no `@`) is never a
//     record again and is swept as an orphan in the same pass.
//     So is a universe's own record (`~<universe key>`) whose universe
//     folder no longer exists, and a folder member's record whose folder is
//     gone.
//
// Pass an empty `keep` slice to ONLY sweep tmp dirs (orphan detection
// skipped). The hub's `cache gc` subcommand uses the universe sidecar
// list; tests / forced-clean ops pass empty to limit blast radius.
//
// Returns the lists of removed paths for the operator-facing report.
// Errors removing individual entries are logged on the result via the
// returned error wrap chain; partial progress is preserved.
func (c *Cache) GC(keep []string) (*GCResult, error) {
	entries, err := os.ReadDir(c.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return &GCResult{}, nil
		}
		return nil, fmt.Errorf("cache: read root %s: %w", c.Root, err)
	}

	keepSet := make(map[string]bool, len(keep))
	for _, name := range keep {
		keepSet[name] = true
	}

	result := &GCResult{}
	var firstErr error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		path := filepath.Join(c.Root, name)
		// .tmp leftovers: always remove. Never authoritative.
		if len(name) > 4 && name[len(name)-4:] == ".tmp" {
			if err := os.RemoveAll(path); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("cache GC: remove tmp %s: %w", path, err)
				}
				continue
			}
			result.TmpDirsRemoved = append(result.TmpDirsRemoved, name)
			continue
		}
		// Orphan sweep — only when caller provided an allowlist. A folder
		// that is not a record (older name-only key) has no member.
		member, isRecord := splitRecordDir(name)
		if len(keepSet) > 0 && (!isRecord || !keepSet[member] || universeGone(path) || folderGone(path)) {
			if err := os.RemoveAll(path); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("cache GC: remove orphan %s: %w", path, err)
				}
				continue
			}
			result.OrphansRemoved = append(result.OrphansRemoved, name)
		}
	}
	return result, firstErr
}

// folderGone answers whether a record was taken from a folder member's root
// that no longer exists.
func folderGone(dir string) bool {
	meta, err := readMetadata(dir)
	if err != nil || meta == nil || !meta.Folder {
		return false
	}
	_, err = os.Stat(meta.ExePath)
	return os.IsNotExist(err)
}

// universeGone answers whether a record folder is a universe's own record
// whose universe folder no longer exists — a universe deleted or moved leaves
// its records behind, and nothing would ever read them again.
func universeGone(dir string) bool {
	if !strings.Contains(filepath.Base(dir), scopeSep) {
		return false
	}
	meta, err := readMetadata(dir)
	if err != nil || meta == nil || meta.ScopeDir == "" {
		return false
	}
	_, err = os.Stat(meta.ScopeDir)
	return os.IsNotExist(err)
}

// ListNames enumerates the distinct member names that have a record in the
// cache, whatever binary each record was taken from. Useful for the
// `cache gc --dry-run` orphan report. Returns nil on an absent cache root.
func (c *Cache) ListNames() ([]string, error) {
	entries, err := os.ReadDir(c.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cache: read root %s: %w", c.Root, err)
	}
	seen := make(map[string]bool, len(entries))
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Not a record: a temp leftover from an aborted Save (`.tmp`
		// suffix) or a folder of the older name-only key.
		name, ok := splitRecordDir(e.Name())
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out, nil
}

// Record is one record as Records lists it: the member's name and the loaded
// pair, or the error that refused it. A record that fails to load still
// surfaces, so the operator can see it exists.
type Record struct {
	Name     string
	Manifest *mbapi.Manifest
	Meta     *Metadata
	Err      error
}

// Records loads every record in the cache, one per `<name>@<key>` folder, in
// folder order. This is the operator's view (`cache list`): one row per
// record, the binary each was taken from in its metadata. Returns nil on an
// absent cache root.
func (c *Cache) Records() ([]Record, error) {
	entries, err := os.ReadDir(c.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cache: read root %s: %w", c.Root, err)
	}
	var out []Record
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name, ok := splitRecordDir(e.Name())
		if !ok {
			continue
		}
		r := Record{Name: name}
		r.Manifest, r.Meta, r.Err = c.loadDir(name, filepath.Join(c.Root, e.Name()))
		out = append(out, r)
	}
	return out, nil
}
