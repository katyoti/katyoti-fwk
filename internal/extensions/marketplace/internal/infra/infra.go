// SPDX-License-Identifier: Apache-2.0

// Package infra resolves the INFRA CHANNEL -- the control-plane binaries
// (katyoti + ktw-hub + ktw-gui) as a versioned, convention-addressed set rather
// than as catalog items.
//
// WHY THIS IS NOT A MARKETPLACE. A catalog answers "what is available?". For
// infra that answer is compiled in (controlplane.InfraBinaries): we always
// install exactly those three. The only open questions are WHERE and WHICH
// VERSION, so the channel carries a one-line version pointer and nothing else.
//
// Routing infra through the catalog was actively harmful: resolution failed when
// more than one subscribed marketplace served the name, so the ONE thing that
// must always resolve became the one thing that could not -- and a colleague's
// marketplace could quietly offer you a different katyoti.exe, which is the
// binary that installs everything else.
//
// Layout (infra is a SUBFOLDER of a deploy root, so one destination and one
// signing key serve both infra and the sidecar catalog):
//
//	<root>/infra/channel.yaml                      -> version: v0.1.2
//	<root>/infra/<version>/<goos>-<goarch>/<bin>
//	<root>/infra/<version>/<goos>-<goarch>/SHA256SUMS
package infra

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/signing"
)

// EnvChannel overrides channel resolution outright. Used by the installer
// script and by tests.
const EnvChannel = "KTW_INFRA_CHANNEL"

// PointerFile is the only discovery file in a channel. It exists because
// neither HTTP nor a synced-folder share offers a reliable directory listing,
// so "latest" cannot be derived from paths alone. It is a version pointer, not
// a catalog: it never enumerates what to install.
const PointerFile = "channel.yaml"

// SumsFile pins the bytes of every binary in one platform folder. This is
// INTEGRITY, not inventory -- it is what lets an update refuse corrupted or
// swapped bytes.
const SumsFile = "SHA256SUMS"

// registryFile is the machine-managed channel record, a sibling of
// marketplaces.yaml / universes.yaml per the global-layout doctrine.
const registryFile = "infra-channel.yaml"

// Source is a subscribed marketplace, used BOTH to derive the channel location
// and to inherit its trust root: infra published as a subfolder of a
// marketplace is signed by the same key, so a channel derived from a pinned
// marketplace is itself pinned.
type Source struct {
	URL    string
	PubKey string
	Trust  string
}

// Channel is a resolved infra root: everything below it is convention.
type Channel struct {
	// Root is the channel folder itself (the one CONTAINING channel.yaml),
	// either an https:// URL or a local path.
	Root string

	// Source records how Root was found, so an error can be acted on
	// ("set it where?") instead of merely reported.
	Source string

	// Trust + PubKey are the provenance gate, mirroring a marketplace
	// subscription. For TrustPinned, every file this channel serves must carry
	// a `.minisig` that verifies against PubKey before anything is installed.
	//
	// This gate matters more here than anywhere else: infra is the code that
	// installs everything afterwards, so a swapped katyoti.exe compromises
	// every later install rather than one item.
	Trust  string
	PubKey string
}

type pointer struct {
	Version  string `yaml:"version"`
	Previous string `yaml:"previous,omitempty"`
	// Key names the publisher's public key file, beside the catalog.
	Key string `yaml:"key,omitempty"`
	// Solution names the solution a first install opens on
	// (<solution>@<version>).
	Solution string `yaml:"solution,omitempty"`
	// Demo is Solution's former name, still read from an older channel.
	Demo string `yaml:"demo,omitempty"`
}

// Solution is the solution the channel names for a first install (its
// pointer's `solution`, else an older channel's `demo`, verified like the
// version it sits beside); empty when none.
func (c Channel) Solution(ctx context.Context) (string, error) {
	data, err := fetch(ctx, join(c.Root, PointerFile))
	if err != nil {
		return "", err
	}
	if err := c.verify(ctx, PointerFile, data); err != nil {
		return "", err
	}
	var p pointer
	if err := yaml.Unmarshal(data, &p); err != nil {
		return "", fmt.Errorf("parse %s: %w", PointerFile, err)
	}
	if sol := strings.TrimSpace(p.Solution); sol != "" {
		return sol, nil
	}
	return strings.TrimSpace(p.Demo), nil
}

type channelRecord struct {
	Root   string `yaml:"root"`
	Trust  string `yaml:"trust,omitempty"`
	PubKey string `yaml:"pubkey,omitempty"`
}

// Resolve finds the infra channel, in precedence order:
//
//  1. $KTW_INFRA_CHANNEL          -- installer / test override
//  2. <GlobalHomeDir>/infra-channel.yaml
//  3. <dir of a subscribed marketplace index>/infra
//
// Step 3 is what makes the common case need NO configuration: publishing infra
// as a subfolder of the marketplace you already subscribe to means the channel
// is simply there. It applies only when every subscribed marketplace agrees on
// one location -- if they disagree we refuse rather than silently pick, because
// picking wrong means installing someone else's control plane.
func Resolve(sources []Source) (Channel, error) {
	if v := strings.TrimSpace(os.Getenv(EnvChannel)); v != "" {
		return Channel{Root: strings.TrimRight(v, "/"), Source: "$" + EnvChannel}, nil
	}

	recPath := filepath.Join(application.Env().GlobalHomeDir(), registryFile)
	if data, err := os.ReadFile(recPath); err == nil {
		var rec channelRecord
		if err := yaml.Unmarshal(data, &rec); err == nil && strings.TrimSpace(rec.Root) != "" {
			return Channel{
				Root:   strings.TrimRight(rec.Root, "/"),
				Source: recPath,
				Trust:  rec.Trust,
				PubKey: rec.PubKey,
			}, nil
		}
	}

	candidates := map[string]Source{}
	for _, src := range sources {
		if c := join(dir(src.URL), "infra"); c != "" {
			// A pinned source wins over an unpinned one at the same location:
			// inheriting the weaker trust would silently downgrade the gate.
			if prev, seen := candidates[c]; !seen || (prev.Trust != mktapi.TrustPinned && src.Trust == mktapi.TrustPinned) {
				candidates[c] = src
			}
		}
	}
	switch len(candidates) {
	case 1:
		for c, src := range candidates {
			return Channel{
				Root:   c,
				Source: "derived from the subscribed marketplace",
				Trust:  src.Trust,
				PubKey: src.PubKey,
			}, nil
		}
	case 0:
		return Channel{}, fmt.Errorf(
			"no infra channel is configured.\nSet one either way:\n  $%s=<url-or-folder>\n  %s  containing:  root: <url-or-folder>",
			EnvChannel, recPath)
	}
	// Listing the candidates matters: the whole point of refusing is that the
	// user must CHOOSE, and nobody can choose from a count.
	names := make([]string, 0, len(candidates))
	for c := range candidates {
		names = append(names, c)
	}
	sort.Strings(names)
	return Channel{}, fmt.Errorf(
		"the subscribed marketplaces imply %d different infra channels:\n  %s\nPick one and write it to %s as:  root: <the-one-you-want>",
		len(candidates), strings.Join(names, "\n  "), recPath)
}

// Save records the channel so later runs resolve it without deriving.
//
// The TRUST is saved with it. Recording only the location would silently drop
// the pin on the second run: the machine would keep installing from the same
// place, but stop checking that the bytes are signed by the key it accepted at
// install time -- a downgrade nobody would notice.
func Save(c Channel) error {
	p := filepath.Join(application.Env().GlobalHomeDir(), registryFile)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(channelRecord{
		Root:   strings.TrimRight(c.Root, "/"),
		Trust:  c.Trust,
		PubKey: c.PubKey,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// LatestVersion reads the channel's version pointer.
func (c Channel) LatestVersion(ctx context.Context) (string, error) {
	v, _, err := c.Versions(ctx)
	return v, err
}

// Versions reads the pointer: the version to install, and the ROLLBACK TARGET.
//
// The channel retains exactly two versions, because nothing pins infra -- a
// universe pins sidecars in Univers.yaml, but install-infra and update --infra
// always take whatever the pointer names, so version N-2 is ~300MB nobody can
// resolve. Two is not one because rolling back a bad publish means stepping the
// pointer back, which is free only while those bytes still exist.
//
// `previous` is recorded explicitly rather than inferred from folder
// timestamps: a synced share (OneDrive, SharePoint) rewrites mtimes, so
// filesystem metadata cannot be trusted to order publishes.
func (c Channel) Versions(ctx context.Context) (latest, previous string, err error) {
	data, err := fetch(ctx, join(c.Root, PointerFile))
	if err != nil {
		return "", "", fmt.Errorf("read infra channel (%s, via %s): %w", c.Root, c.Source, err)
	}
	// The pointer decides WHICH bytes get installed, so it is verified before
	// it is believed: an attacker who can rewrite it can aim the whole update
	// at a version of their choosing without touching a single binary.
	if err := c.verify(ctx, PointerFile, data); err != nil {
		return "", "", err
	}
	var p pointer
	if err := yaml.Unmarshal(data, &p); err != nil {
		return "", "", fmt.Errorf("parse %s: %w", PointerFile, err)
	}
	if strings.TrimSpace(p.Version) == "" {
		return "", "", fmt.Errorf("%s at %s declares no version", PointerFile, c.Root)
	}
	return p.Version, p.Previous, nil
}

// Platform is the channel's folder name for the running machine.
func Platform() string { return runtime.GOOS + "-" + runtime.GOARCH }

// Assets returns one Asset per infra binary the channel serves for THIS
// platform at the given version, keyed by binary name. Every asset carries the
// sha256 from SHA256SUMS, so the ordinary store install verifies it -- infra
// travels the same provenance path as any sidecar.
//
// A binary listed in SHA256SUMS that the caller does not want is simply never
// looked up; a binary the caller wants that SHA256SUMS omits is reported by the
// caller, which is the side that knows the required set.
func (c Channel) Assets(ctx context.Context, version string) (map[string]mktapi.Asset, error) {
	base := join(join(c.Root, version), Platform())
	data, err := fetch(ctx, join(base, SumsFile))
	if err != nil {
		return nil, fmt.Errorf("infra %s has no %s build (%s): %w", version, Platform(), base, err)
	}
	// SHA256SUMS is what every per-file checksum is trusted FROM, so verifying
	// it is what makes those checksums mean anything: an attacker who can swap
	// a binary can swap its recorded hash just as easily.
	if err := c.verify(ctx, join(version, joinPlat(SumsFile)), data); err != nil {
		return nil, err
	}
	out := map[string]mktapi.Asset{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		sum, file := fields[0], strings.TrimPrefix(fields[1], "*")
		out[baseName(file)] = mktapi.Asset{
			OS:     runtime.GOOS,
			Arch:   runtime.GOARCH,
			URL:    join(base, file),
			Sha256: sum,
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s at %s lists no files", SumsFile, base)
	}
	return out, nil
}

// baseName strips any path and any .exe, so SHA256SUMS entries match the
// catalog-free binary names in controlplane.InfraBinaries.
func baseName(file string) string {
	f := path.Base(filepath.ToSlash(file))
	return strings.TrimSuffix(f, ".exe")
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func dir(u string) string {
	if isURL(u) {
		i := strings.LastIndex(u, "/")
		if i < 0 {
			return ""
		}
		return u[:i]
	}
	return filepath.Dir(strings.TrimPrefix(u, "file://"))
}

func join(base, elem string) string {
	if base == "" {
		return ""
	}
	if isURL(base) {
		return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(filepath.ToSlash(elem), "/")
	}
	return filepath.Join(base, filepath.FromSlash(elem))
}

func fetch(ctx context.Context, loc string) ([]byte, error) {
	if !isURL(loc) {
		return os.ReadFile(strings.TrimPrefix(loc, "file://"))
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, loc, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", loc, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// joinPlat is the platform-relative path of a file inside a version folder.
func joinPlat(name string) string { return Platform() + "/" + name }

// verify enforces the pinned-key gate over one channel file.
//
// This gate was LOST when infra moved off the catalog: the catalog verified a
// pinned signature before any install, and the channel initially checked only
// sha256 -- which an attacker who can replace a file can replace too. Infra is
// the one binary set where a swap compromises every LATER install rather than
// one item, so it fails closed: for a pinned channel, no key, no signature, or
// a bad signature all refuse.
//
// A channel with trust "none" (or empty) is a no-op, matching how an unpinned
// marketplace subscription behaves -- opt-in trust, not silent enforcement.
func (c Channel) verify(ctx context.Context, rel string, body []byte) error {
	if c.Trust != mktapi.TrustPinned {
		return nil
	}
	if strings.TrimSpace(c.PubKey) == "" {
		return fmt.Errorf("infra channel %s is trust=pinned but has no pinned public key", c.Root)
	}
	pk, err := signing.ParsePublicKey([]byte(c.PubKey))
	if err != nil {
		return fmt.Errorf("infra channel %s: invalid pinned public key: %w", c.Root, err)
	}
	sig, err := fetch(ctx, join(c.Root, rel+".minisig"))
	if err != nil {
		return fmt.Errorf("infra channel %s: %s is unsigned, but the channel is pinned: %w", c.Root, rel, err)
	}
	if err := signing.Verify(body, sig, pk); err != nil {
		return fmt.Errorf("infra channel %s: %s signature did NOT verify -- refusing: %w", c.Root, rel, err)
	}
	return nil
}

// SiblingCatalogURL is the marketplace catalog published BESIDE this channel.
//
// The channel lives at <deploy-root>/infra and the catalog at
// <deploy-root>/katyoti-marketplace.yaml, so a machine that just bootstrapped
// from a channel already knows where its sidecars come from. Returning it lets
// first-run subscribe automatically instead of making the user paste a URL they
// were never shown.
func (c Channel) SiblingCatalogURL() string {
	return join(dir(c.Root), "katyoti-marketplace.yaml")
}

// SiblingPubKey fetches the public key published in the deploy root, so a
// first-run subscribe can PIN rather than trust blindly.
//
// Fetching the key from the same tree it verifies is not a trust root on its
// own -- anyone who can rewrite the tree can rewrite the key. It is exactly as
// strong as the channel the user chose to install from, which is the decision
// that already happened: they ran that installer. Pinning it here means every
// LATER fetch is checked against the key seen at install time, so a tree that
// is compromised afterwards is caught.
//
// The pointer names the key file (`key:`), whatever the publisher called it;
// a channel published before it named one falls back to the two names in use.
func (c Channel) SiblingPubKey(ctx context.Context) (string, error) {
	root := dir(c.Root)
	names := []string{"katyoti.key.pub", "team.key.pub"}
	if data, err := fetch(ctx, join(c.Root, PointerFile)); err == nil {
		var p pointer
		if yaml.Unmarshal(data, &p) == nil && p.Key != "" && !strings.ContainsAny(p.Key, `/\`) {
			names = append([]string{p.Key}, names...)
		}
	}
	for _, name := range names {
		if data, err := fetch(ctx, join(root, name)); err == nil {
			return string(data), nil
		}
	}
	return "", fmt.Errorf("no public key published at %s", root)
}
