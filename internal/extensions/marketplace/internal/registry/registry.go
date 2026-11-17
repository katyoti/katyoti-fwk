// SPDX-License-Identifier: Apache-2.0

// Package registry resolves the effective marketplace subscription list from a
// beside-exe seed (read-only default) overlaid by a user file in GlobalHomeDir.
// The binary is name-agnostic: the seed is <exe-dir>/<exe-basename>.yaml, so a
// renamed binary (katyoti-northwind, acme-setup, …) reads its own seed (ADR §8).
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// fileShape is the on-disk shape of both the seed and the user overlay.
type fileShape struct {
	Version      int                `yaml:"version,omitempty"`
	Marketplaces []marketplaceEntry `yaml:"marketplaces"`
}

type marketplaceEntry struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url,omitempty"`
	Enabled *bool  `yaml:"enabled,omitempty"` // nil = default enabled
	Trust   string `yaml:"trust,omitempty"`   // "none" | "pinned"
	PubKey  string `yaml:"pubKey,omitempty"`  // pinned trust root (minisign base64 line)
	Channel string `yaml:"channel,omitempty"` // stable (empty) or canary
}

// Registry reads seed + overlay and writes CRUD to the overlay only.
type Registry struct {
	seedPath    string
	overlayPath string
}

// New builds a Registry using the running exe's location (seed) and
// GlobalHomeDir (overlay).
func New() (*Registry, error) {
	seed, err := seedPath()
	if err != nil {
		return nil, err
	}
	root := application.Env().GlobalHomeDir()
	if root == "" {
		return nil, fmt.Errorf("resolve global home dir")
	}
	return forPaths(seed, filepath.Join(root, "marketplaces.yaml")), nil
}

// forPaths builds a Registry with explicit seed + overlay paths (test seam).
func forPaths(seedPath, overlayPath string) *Registry {
	return &Registry{seedPath: seedPath, overlayPath: overlayPath}
}

// seedPath is <exe-dir>/<exe-basename>.yaml (name-agnostic).
func seedPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	base := filepath.Base(exe)
	base = strings.TrimSuffix(base, filepath.Ext(base)) // strip .exe on Windows
	return filepath.Join(filepath.Dir(exe), base+".yaml"), nil
}

// resolveSeedURL makes a relative, non-http seed marketplace URL absolute
// relative to the seed file's directory (portable bundle folders).
func resolveSeedURL(seedPath, url string) string {
	if url == "" || strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	p := strings.TrimPrefix(url, "file://")
	if filepath.IsAbs(p) {
		return url
	}
	return filepath.Join(filepath.Dir(seedPath), p)
}

func loadFile(path string) (fileShape, error) {
	var fsh fileShape
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fileShape{}, nil
	}
	if err != nil {
		return fsh, fmt.Errorf("read %s: %w", path, err)
	}
	if err := config.DecodeBytes(b, marshal.YAML, &fsh); err != nil {
		return fsh, fmt.Errorf("decode %s: %w", path, err)
	}
	return fsh, nil
}

// List returns the effective marketplaces: seed ∪ user. A user entry overrides
// a seed entry by name (may disable it or replace its URL) or adds a new one.
func (r *Registry) List() ([]mktapi.Marketplace, error) {
	seed, err := loadFile(r.seedPath)
	if err != nil {
		return nil, err
	}
	user, err := loadFile(r.overlayPath)
	if err != nil {
		return nil, err
	}
	byName := map[string]mktapi.Marketplace{}
	var order []string
	upsert := func(e marketplaceEntry, src string) {
		enabled := true
		if e.Enabled != nil {
			enabled = *e.Enabled
		}
		prev, hadPrev := byName[e.Name]
		url := e.URL
		if url == "" { // disable-only override inherits the existing URL
			if hadPrev {
				url = prev.URL
			}
		}
		// Trust + PubKey inherit from a prior (seed) entry when this override
		// leaves them empty — so a `disable`/URL-only user edit keeps the pinned
		// key, and an explicit re-pin replaces it.
		trust, pubKey, channel := e.Trust, e.PubKey, e.Channel
		if trust == "" && hadPrev {
			trust = prev.Trust
		}
		if pubKey == "" && hadPrev {
			pubKey = prev.PubKey
		}
		if channel == "" && hadPrev {
			channel = prev.Channel
		}
		if !hadPrev {
			order = append(order, e.Name)
		}
		byName[e.Name] = mktapi.Marketplace{
			Name: e.Name, URL: url, Enabled: enabled, Source: src,
			Trust: trust, PubKey: pubKey, Channel: channel,
		}
	}
	for _, e := range seed.Marketplaces {
		// A relative (non-http) seed URL resolves relative to the seed file —
		// so a bundled instance folder is portable (move it, it still finds its
		// local `katyoti-marketplace.yaml`).
		e.URL = resolveSeedURL(r.seedPath, e.URL)
		upsert(e, "seed")
	}
	for _, e := range user.Marketplaces {
		upsert(e, "user")
	}
	out := make([]mktapi.Marketplace, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out, nil
}

func (r *Registry) saveOverlay(fsh fileShape) error {
	fsh.Version = 1
	if err := os.MkdirAll(filepath.Dir(r.overlayPath), 0o755); err != nil {
		return fmt.Errorf("create overlay dir: %w", err)
	}
	b, err := yaml.Marshal(fsh)
	if err != nil {
		return fmt.Errorf("encode overlay: %w", err)
	}
	if err := os.WriteFile(r.overlayPath, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", r.overlayPath, err)
	}
	return nil
}

func boolPtr(b bool) *bool { return &b }

// Add adds (or updates) a marketplace in the user overlay. pubKey (a minisign
// base64 key line) + trust ("none"|"pinned") set the install-time provenance
// policy; empty values leave any existing policy untouched on update.
func (r *Registry) Add(name, url, pubKey, trust string) error {
	// Trim stray whitespace from a pasted URL / name (a trailing space silently
	// breaks the `<url>.minisig` signature fetch on Windows).
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)
	pubKey = strings.TrimSpace(pubKey)
	trust = strings.TrimSpace(trust)
	if name == "" || url == "" {
		return fmt.Errorf("marketplace name and url are required")
	}
	user, err := loadFile(r.overlayPath)
	if err != nil {
		return err
	}
	for i := range user.Marketplaces {
		if user.Marketplaces[i].Name == name {
			user.Marketplaces[i].URL = url
			user.Marketplaces[i].Enabled = boolPtr(true)
			if trust != "" {
				user.Marketplaces[i].Trust = trust
			}
			if pubKey != "" {
				user.Marketplaces[i].PubKey = pubKey
			}
			return r.saveOverlay(user)
		}
	}
	user.Marketplaces = append(user.Marketplaces, marketplaceEntry{
		Name: name, URL: url, Enabled: boolPtr(true), Trust: trust, PubKey: pubKey,
	})
	return r.saveOverlay(user)
}

// SetChannel makes an installation follow a marketplace's channel: stable
// (end users) or canary (alpha users, who also get the release candidates).
func (r *Registry) SetChannel(name, channel string) error {
	name, channel = strings.TrimSpace(name), strings.TrimSpace(channel)
	if channel != mktapi.ChannelStable && channel != mktapi.ChannelCanary {
		return fmt.Errorf("channel %q: stable or canary", channel)
	}
	all, err := r.List()
	if err != nil {
		return err
	}
	known := false
	for _, m := range all {
		known = known || m.Name == name
	}
	if !known {
		return fmt.Errorf("no marketplace %q", name)
	}
	user, err := loadFile(r.overlayPath)
	if err != nil {
		return err
	}
	if channel == mktapi.ChannelStable {
		channel = "" // the default
	}
	for i := range user.Marketplaces {
		if user.Marketplaces[i].Name == name {
			user.Marketplaces[i].Channel = channel
			return r.saveOverlay(user)
		}
	}
	user.Marketplaces = append(user.Marketplaces, marketplaceEntry{Name: name, Channel: channel})
	return r.saveOverlay(user)
}

// Remove drops a marketplace from the overlay. A seed-only entry is recorded as
// a disabled override (the seed is read-only).
func (r *Registry) Remove(name string) error {
	user, err := loadFile(r.overlayPath)
	if err != nil {
		return err
	}
	var kept []marketplaceEntry
	found := false
	for _, e := range user.Marketplaces {
		if e.Name == name {
			found = true
			continue
		}
		kept = append(kept, e)
	}
	if !found {
		inSeed, err := r.inSeed(name)
		if err != nil {
			return err
		}
		if !inSeed {
			return fmt.Errorf("marketplace %q not found", name)
		}
		kept = append(kept, marketplaceEntry{Name: name, Enabled: boolPtr(false)})
	}
	user.Marketplaces = kept
	return r.saveOverlay(user)
}

// SetEnabled toggles a marketplace on/off via the overlay.
func (r *Registry) SetEnabled(name string, on bool) error {
	user, err := loadFile(r.overlayPath)
	if err != nil {
		return err
	}
	for i := range user.Marketplaces {
		if user.Marketplaces[i].Name == name {
			user.Marketplaces[i].Enabled = boolPtr(on)
			return r.saveOverlay(user)
		}
	}
	inSeed, err := r.inSeed(name)
	if err != nil {
		return err
	}
	if !inSeed {
		return fmt.Errorf("marketplace %q not found", name)
	}
	user.Marketplaces = append(user.Marketplaces, marketplaceEntry{Name: name, Enabled: boolPtr(on)})
	return r.saveOverlay(user)
}

func (r *Registry) inSeed(name string) (bool, error) {
	seed, err := loadFile(r.seedPath)
	if err != nil {
		return false, err
	}
	for _, e := range seed.Marketplaces {
		if e.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// UserRow is one row of the user's subscriptions file: its name and its URL
// (empty for an override of a seed entry).
type UserRow struct {
	Name string
	URL  string
}

// DropUserRows removes from the user's subscriptions file (marketplaces.yaml)
// every row drop answers true for, by its name and its URL - nothing else, the
// seed untouched (the doctor's repair of a test's leftover or a vanished
// subscription). Returns the rows removed.
func DropUserRows(overlayPath string, drop func(UserRow) bool) ([]UserRow, error) {
	r := &Registry{overlayPath: overlayPath}
	user, err := loadFile(overlayPath)
	if err != nil {
		return nil, err
	}
	var kept []marketplaceEntry
	var dropped []UserRow
	for _, e := range user.Marketplaces {
		if row := (UserRow{Name: e.Name, URL: e.URL}); drop(row) {
			dropped = append(dropped, row)
			continue
		}
		kept = append(kept, e)
	}
	if len(dropped) == 0 {
		return nil, nil
	}
	user.Marketplaces = kept
	return dropped, r.saveOverlay(user)
}
