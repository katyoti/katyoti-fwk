// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
)

// ObjectProvider is the object-backed provider (decision D1): it serves the
// entries of one kind a project publishes from its objects, under the
// project's provider token (review R-A1: one provider per token and kind). It never reads CWM: the process hosting CWM pushes an
// object's entry on each object event (Put, Remove), so Enumerate touches no
// runtime service. Its source changes while the process runs: it is mutable.
//
// Two objects publishing the same entry are both withheld, with a clash
// naming them (api.Clashes); the entry of an object that is put again replaces
// its previous one whole.
//
// An entry that overrides another provider's entry is never enumerated: it is
// reported apart (OverrideEntries), for the manifest builder to apply over
// the entry it names (contribapi.OverrideEntryReporter).
type ObjectProvider struct {
	token, kind string

	mu      sync.Mutex
	trusted bool
	objects map[string]api.ObjectEntry // by object id
}

var (
	_ contribapi.ContributionProvider  = (*ObjectProvider)(nil)
	_ contribapi.OverrideEntryReporter = (*ObjectProvider)(nil)
)

// NewObjectProvider is the provider of kind's object entries under token.
func NewObjectProvider(token, kind string) *ObjectProvider {
	return &ObjectProvider{token: token, kind: kind, objects: map[string]api.ObjectEntry{}}
}

// Type implements contribapi.ContributionProvider.
func (p *ObjectProvider) Type() string { return p.kind }

// Provider implements contribapi.ContributionProvider: the project's token.
func (p *ObjectProvider) Provider() string { return p.token }

// Identity implements contribapi.ContributionProvider: objects change while
// the process runs.
func (p *ObjectProvider) Identity() contribapi.Identity { return contribapi.Identity{Mutable: true} }

// Put records an object's entry, replacing the one the object published
// before. An entry of another token or kind is refused.
func (p *ObjectProvider) Put(e api.ObjectEntry) error {
	if e.Entry.Provider != p.token || e.Entry.Type != p.kind {
		return fmt.Errorf("object %s: entry %s is not of %s:/%s", e.Object, e.URI(), p.token, p.kind)
	}
	if e.Object == "" {
		return fmt.Errorf("entry %s: no object", e.URI())
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.objects[e.Object] = e
	return nil
}

// Remove forgets an object's entry (the object was deleted, or no longer
// carries the capability or this kind). Nothing when it published none.
func (p *ObjectProvider) Remove(object string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.objects, object)
}

// Entries are the object entries held, by URI then object - clashing ones
// included (Clashes names them).
func (p *ObjectProvider) Entries() []api.ObjectEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]api.ObjectEntry, 0, len(p.objects))
	for _, e := range p.objects {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URI() != out[j].URI() {
			return out[i].URI() < out[j].URI()
		}
		return out[i].Object < out[j].Object
	})
	return out
}

// Clashes are the entries two objects publish: neither is served.
func (p *ObjectProvider) Clashes() []api.Clash { return api.Clashes(nil, p.Entries()) }

// SetTrusted says whether the universe trusts the project: its override
// entries carry it (contribapi.OverrideEntry.Trusted). Untrusted until said.
func (p *ObjectProvider) SetTrusted(trusted bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trusted = trusted
}

// served are the entries held without those two objects publish.
func (p *ObjectProvider) served() []api.ObjectEntry {
	clashing := map[string]bool{}
	for _, c := range p.Clashes() {
		clashing[c.URI] = true
	}
	var out []api.ObjectEntry
	for _, e := range p.Entries() {
		if !clashing[e.URI()] {
			out = append(out, e)
		}
	}
	return out
}

// Enumerate implements contribapi.ContributionProvider: the entries held,
// sorted by id, without those two objects publish and without the override
// entries (OverrideEntries).
func (p *ObjectProvider) Enumerate(context.Context) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, e := range p.served() {
		if e.Overrides == "" {
			out = append(out, e.Entry)
		}
	}
	return out, nil
}

// OverrideEntries implements contribapi.OverrideEntryReporter: the entries
// held that override another provider's entry, without those two objects
// publish, by URI.
func (p *ObjectProvider) OverrideEntries() []contribapi.OverrideEntry {
	p.mu.Lock()
	trusted := p.trusted
	p.mu.Unlock()
	var out []contribapi.OverrideEntry
	for _, e := range p.served() {
		if e.Overrides != "" {
			out = append(out, e.AsOverride(trusted))
		}
	}
	return out
}

// Open implements contribapi.ContributionProvider from the enumeration.
func (p *ObjectProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}
