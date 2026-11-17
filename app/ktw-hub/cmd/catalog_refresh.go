// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	pcapi "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
	pccontrib "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/contrib"
	"github.com/spf13/cobra"
)

// catalogRefresher exposes the optional Reload method on the published
// Catalog so the refresh command can prompt a re-read of the tier-1
// manifest cache after writing a member's folder. Mirrors CatalogReloader in
// internal/extensions/manifest/commands/catalog.go.
type catalogRefresher interface {
	Reload() error
}

// newCatalogRefreshCommand returns the `catalog refresh <name>` Cobra
// subcommand. It re-probes the member through refreshMember — the same
// exec-and-kill probe hub boot uses (`<binary> sc resources manifest`,
// appcore.RunResourcesList) — saves the Manifest into the member's tier-1
// cache folder (<CacheDir>/sidecars/<name>@<key>/), and asks the Catalog to
// reload.
//
// Lives in ktw-hub/cmd/ rather than internal/extensions/manifest/commands/
// because it bridges connmgr + manifest — the manifest extension
// deliberately doesn't depend on connmgr (layering), and connmgr
// shouldn't host a "catalog" subcommand.
func newCatalogRefreshCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh <name|all>",
		Short: "Re-probe a sidecar (or 'all'), pull its manifest, cache it",
		Long: `Runs the named sidecar's binary as a short-lived subprocess
('<binary> sc resources manifest', the same exec probe hub boot uses — no
message bus involved, so it works without a live hub), writes the returned
manifest into the tier-1 manifest cache (<CacheDir>/sidecars/<name>@<key>/), and
asks the catalog to reload.

The binary is the one named by the sidecar's entry in the 'sidecars:' block
of the universe's Univers.yaml, else a ktw-* binary of that name next to
ktw-hub.

Pass 'all' to refresh every sidecar declared in the universe's Univers.yaml
in turn; the command continues past individual failures and reports a
summary at the end.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "all" {
				return runCatalogRefreshAll(cmd.Context(), cmd)
			}
			return runCatalogRefresh(cmd.Context(), cmd, args[0])
		},
	}
}

// runCatalogRefreshAll refreshes every sidecar declared in the universe's
// Univers.yaml, one at a time. It continues past individual failures so one
// unreachable sidecar doesn't abort the whole sweep, and returns a non-nil
// error only if at least one refresh failed (with a summary count).
func runCatalogRefreshAll(ctx context.Context, cmd *cobra.Command) error {
	reg, err := universeMembers()
	if err != nil {
		return fmt.Errorf("load universe sidecars: %w", err)
	}
	if len(reg.Sidecars) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no sidecars declared in this universe")
		return nil
	}
	var failed int
	for _, s := range reg.Sidecars {
		if err := runCatalogRefresh(ctx, cmd, s.Name); err != nil {
			failed++
			fmt.Fprintf(cmd.ErrOrStderr(), "refresh %s: %v\n", s.Name, err)
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "refreshed %s\n", s.Name)
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d sidecar(s) failed to refresh", failed, len(reg.Sidecars))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "refreshed all %d sidecar(s)\n", len(reg.Sidecars))
	return nil
}

func runCatalogRefresh(ctx context.Context, cmd *cobra.Command, name string) error {
	manifest, err := refreshMember(ctx, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"refreshed %s: %s (resources=%d, slots=%d, hash=%s)\n",
		name, manifest.Version,
		len(manifest.Contributions), len(manifest.Slots), manifest.Hash)
	return nil
}

// refreshMember re-probes ONE member, re-caches its manifest and asks the
// catalog to re-read — the refresh itself, with no CLI in it, so the content
// watcher can drive exactly what the operator command drives rather than a
// parallel path that could drift from it.
func refreshMember(ctx context.Context, name string) (*mbapi.Manifest, error) {
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog service not published: %w", err)
	}

	// The hub's own record is not probed: it is built in process, with the
	// system set it alone carries (saveHubOwnManifest). Probing the hub's
	// executable like a member would save it without the set.
	if name == hubProvider {
		cache := universeCache(filepath.Join(application.Env().CacheDir(), "sidecars"))
		if err := saveHubOwnManifest(ctx, cache); err != nil {
			return nil, fmt.Errorf("record the hub's own manifest: %w", err)
		}
		exe, err := hubOwnExecutable()
		if err != nil {
			return nil, fmt.Errorf("own executable: %w", err)
		}
		own, _, err := cache.Load(hubProvider, exe)
		if err != nil {
			return nil, fmt.Errorf("load the hub's own manifest: %w", err)
		}
		if r, ok := cat.(catalogRefresher); ok {
			if err := r.Reload(); err != nil {
				return nil, fmt.Errorf("reload catalog after refresh: %w", err)
			}
		}
		return own, nil
	}

	// Resolve the binary on disk. The descriptor is pulled by running the
	// binary's own `sc resources manifest` as a SUBPROCESS — no message bus
	// is involved, so this works whether or not NATS is running. (The old
	// implementation spawned the sidecar over the bus, which made the CLI
	// command unusable outside a live hub: "manager has no bus scope".)
	if spec, ok := memberSpec(name); ok && spec.IsFolder() {
		return refreshFolderMember(cat, spec)
	}
	exePath := resolveSidecarBinary(name)
	if exePath == "" {
		return nil, fmt.Errorf("cannot locate a binary for %q — not in this universe's Univers.yaml sidecars, and no %q binary found next to ktw-hub", name, name)
	}

	// System meta-model dedup: every binary embeds the identical `system:/`
	// entries; the hub's own manifest is their one carrier, so a member is
	// always probed WITHOUT them (e26). The record says so (include_system:
	// false), whatever universe asked for the refresh.
	// The cache, and the reads half of the key, BEFORE the probe: a skill
	// saved while the probe runs must read as a change at the next check.
	root := filepath.Join(application.Env().CacheDir(), "sidecars")
	cache := universeCache(root)
	digests := sourceDigestsFor(name)
	digests.ReadRoots, digests.Reads = cache.ReadsBeforeProbe(name, exePath)
	manifest, err := probeManifest(exePath, contentRootOf(name), false)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest from %s (%s): %w", name, exePath, err)
	}
	if manifest == nil {
		return nil, fmt.Errorf("%s returned a nil manifest", name)
	}

	// The sidecar cache is shared across universes under `<CacheDir>/sidecars/`
	// and keyed by (name, binary) — and by universe for a record that depends
	// on it (a declared read root, an override tree), which Save decides.
	// Cache.Save stats exePath to record exe_size+mtime as the invalidation key.
	if err := cache.Save(name, manifest, exePath, digests); err != nil {
		return nil, fmt.Errorf("save manifest for %s: %w", name, err)
	}

	// Ask the catalog to re-read so the new file is visible to consumers.
	if r, ok := cat.(catalogRefresher); ok {
		if err := r.Reload(); err != nil {
			return nil, fmt.Errorf("reload catalog after refresh: %w", err)
		}
	}
	return manifest, nil
}

// memberSpec is the universe's declaration of a member.
func memberSpec(name string) (appcore.SidecarSpec, bool) {
	if reg, err := universeMembers(); err == nil {
		for _, s := range reg.Sidecars {
			if s.Name == name {
				return s, true
			}
		}
	}
	return appcore.SidecarSpec{}, false
}

// refreshFolderMember re-reads a folder member in process (no process to
// probe), saves its record under its folder and reloads the catalog.
func refreshFolderMember(cat mbapi.Catalog, spec appcore.SidecarSpec) (*mbapi.Manifest, error) {
	reg, err := universeMembers()
	if err != nil {
		return nil, err
	}
	cache := universeCache(filepath.Join(application.Env().CacheDir(), "sidecars"))
	root := recordSource(spec)
	manifest, err := folderManifest(spec.Name, root, universeKinds(cache, reg.Sidecars))
	if err != nil {
		return nil, fmt.Errorf("read folder member %s (%s): %w", spec.Name, root, err)
	}
	if err := cache.Save(spec.Name, manifest, root, sourceDigestsIn(spec.Name, reg.Sidecars)); err != nil {
		return nil, fmt.Errorf("save manifest for %s: %w", spec.Name, err)
	}
	if r, ok := cat.(catalogRefresher); ok {
		if err := r.Reload(); err != nil {
			return nil, fmt.Errorf("reload catalog after refresh: %w", err)
		}
	}
	return manifest, nil
}

// resolveSidecarBinary finds the on-disk binary for a sidecar/host name:
// the universe's Univers.yaml `binary:` field first, then a scan of ktw-*
// binaries colocated with the running ktw-hub binary (covers the host —
// e.g. ktw-agents — which sits in bin/ but isn't a Univers.yaml sidecar entry).
func resolveSidecarBinary(name string) string {
	if reg, err := universeMembers(); err == nil {
		for _, s := range reg.Sidecars {
			if s.Name == name && s.Binary != "" {
				return s.Binary
			}
		}
	}
	for _, d := range appcore.DiscoverSidecars() {
		if d.Name == name {
			return d.Path
		}
	}
	return ""
}

// projectMembers are the project members the hub serves, by provider token
// (review R-A1): each holds its folder's providers and the object entries the
// process hosting CWM pushes for its token. universeMembership keeps the
// attached projects'.
var projectMembers = newProjectMemberRegistry()

type projectMemberRegistry struct {
	mu        sync.Mutex
	members   map[string]*pccontrib.Member
	trusted   map[string]bool // the trust each member was last told
	trustMove map[string]bool // the tokens whose trust moved, not yet taken
}

func newProjectMemberRegistry() *projectMemberRegistry {
	return &projectMemberRegistry{members: map[string]*pccontrib.Member{},
		trusted: map[string]bool{}, trustMove: map[string]bool{}}
}

// keepAll holds the member of every project served, and only them: a member
// already held keeps the entries pushed to it, another project's (one
// detached) is dropped. Each member kept is told whether the universe trusts
// its project (trusted, of the project root): an override entry of an
// executable kind applies only from a trusted project. It returns the tokens
// of the members held before whose trust moved, and keeps them until
// takeTrustMoved.
func (r *projectMemberRegistry) keepAll(served []pcapi.ProjectMember, trusted func(projectRoot string) bool) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	want := make(map[string]string, len(served))
	for _, m := range served {
		if m.Token != "" {
			want[m.Token] = m.Project
		}
	}
	for t := range r.members {
		if _, ok := want[t]; !ok {
			delete(r.members, t)
			delete(r.trusted, t)
			delete(r.trustMove, t)
		}
	}
	var moved []string
	for t, root := range want {
		trust := trusted != nil && trusted(root)
		m := r.members[t]
		if m == nil {
			m = pccontrib.NewMember(t, nil)
			r.members[t] = m
		} else if r.trusted[t] != trust {
			moved = append(moved, t)
			r.trustMove[t] = true
		}
		m.SetTrusted(trust)
		r.trusted[t] = trust
	}
	sort.Strings(moved)
	return moved
}

// takeTrustMoved is the members whose trust moved since last taken, sorted:
// their records are rebuilt, whichever read of the membership saw the move.
func (r *projectMemberRegistry) takeTrustMoved() []*pccontrib.Member {
	r.mu.Lock()
	defer r.mu.Unlock()
	tokens := make([]string, 0, len(r.trustMove))
	for t := range r.trustMove {
		tokens = append(tokens, t)
	}
	sort.Strings(tokens)
	out := make([]*pccontrib.Member, 0, len(tokens))
	for _, t := range tokens {
		if m := r.members[t]; m != nil {
			out = append(out, m)
		}
	}
	r.trustMove = map[string]bool{}
	return out
}

// get is the member of token; nil when the hub serves no project under it.
func (r *projectMemberRegistry) get(token string) *pccontrib.Member {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.members[token]
}

// tokens are the tokens of the members held, sorted.
func (r *projectMemberRegistry) tokens() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.members))
	for t := range r.members {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// memberProviders are what the folder member name serves from root: its
// folder's providers, and for a project's member the object entries pushed to
// it too, an entry served twice withheld on both sides (contrib.Member).
func memberProviders(ctx context.Context, name, root string, kinds contribapi.Kinds) ([]contribapi.ContributionProvider, error) {
	ps, err := folderProviders(name, root, kinds)
	if err != nil {
		return nil, err
	}
	m := projectMembers.get(name)
	if m == nil {
		return ps, nil
	}
	m.SetFolder(ps)
	return m.Providers(ctx)
}

// projectPushHandler applies the pushes of the project's object entries
// (pcapi.EntriesPush) to their member and rebuilds its record: saved in the
// cache, the catalog reloaded (contributions-changed), no restart.
type projectPushHandler struct {
	members  *projectMemberRegistry
	sidecars func() ([]appcore.SidecarSpec, error)
	cache    *manifestlocal.Cache
	reload   func()

	mu       sync.Mutex          // one rebuild at a time: no interleaved records
	findings map[string][]string // the clashes last logged, by token
}

// handle applies one push. A push for a token no member answers to, or one
// that does not validate, changes nothing.
func (h *projectPushHandler) handle(ctx context.Context, payload []byte) error {
	var p pcapi.EntriesPush
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("push does not decode: %w", err)
	}
	m := h.members.get(p.Token)
	if m == nil {
		return fmt.Errorf("push for %q: no project member of this hub answers to it", p.Token)
	}
	if err := m.Apply(p); err != nil {
		return err
	}
	return h.rebuild(ctx, m)
}

// rebuildTrustMoved reads the membership again (each member told its
// project's trust) and rebuilds the record of every member whose trust moved.
func rebuildTrustMoved(ctx context.Context, h *projectPushHandler, errw io.Writer) {
	if _, err := universeMembers(); err != nil {
		fmt.Fprintf(errw, "ktw-hub: projects.yaml reload failed: %v\n", err)
		return
	}
	for _, m := range h.members.takeTrustMoved() {
		if err := h.rebuild(ctx, m); err != nil {
			fmt.Fprintf(errw, "ktw-hub: the record of %s is not rebuilt after its trust moved: %v\n", m.Token(), err)
		}
	}
}

// rebuild saves the record of m's folder member, logs the clashes that are
// new, then reloads the catalog.
func (h *projectPushHandler) rebuild(ctx context.Context, m *pccontrib.Member) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	sidecars, err := h.sidecars()
	if err != nil {
		return err
	}
	token := m.Token()
	for _, s := range sidecars {
		if s.Name != token || !s.IsFolder() {
			continue
		}
		root := recordSource(s)
		manifest, err := folderManifest(token, root, universeKinds(h.cache, sidecars))
		if err != nil {
			return fmt.Errorf("member %s: %w", token, err)
		}
		if err := h.cache.Save(token, manifest, root, sourceDigestsIn(token, sidecars)); err != nil {
			return fmt.Errorf("save manifest for %s: %w", token, err)
		}
		h.logFindings(ctx, m) // recorded before the reload signals the record is rebuilt
		if h.reload != nil {
			h.reload()
		}
		return nil
	}
	return nil // no folder member yet: its record is built with the member's entries when it is
}

func (h *projectPushHandler) logFindings(ctx context.Context, m *pccontrib.Member) {
	found, err := m.Findings(ctx)
	if err != nil {
		slog.Warn("hub: the project's clashes do not read", "token", m.Token(), "error", err)
		return
	}
	if h.findings == nil {
		h.findings = map[string][]string{}
	}
	seen := map[string]bool{}
	for _, f := range h.findings[m.Token()] {
		seen[f] = true
	}
	for _, f := range found {
		if !seen[f] {
			slog.Warn("hub: an entry the project serves twice is withheld: "+f, "token", m.Token())
		}
	}
	h.findings[m.Token()] = found
}

// serveProjectPushes subscribes the push subject of every project of the
// universe, then asks the process hosting CWM for the snapshot of each
// project the hub serves.
func serveProjectPushes(ctx context.Context, bus mbapi.MessageBus, tenant, universe string, h *projectPushHandler) (mbapi.Subscription, error) {
	sub, err := bus.Subscribe(ctx, pcapi.PushSubjectAll(tenant, universe), func(_ context.Context, msg mbapi.Message) {
		if err := h.handle(ctx, msg.Data()); err != nil {
			slog.Warn("hub: a push of project contributions is not applied", "subject", msg.Subject(), "error", err)
		}
	})
	if err != nil {
		return nil, err
	}
	for _, token := range h.members.tokens() {
		if err := bus.Publish(ctx, pcapi.SnapshotRequestSubject(tenant, universe, token), nil); err != nil {
			slog.Warn("hub: the project's snapshot was not asked", "token", token, "error", err)
		}
	}
	return sub, nil
}
