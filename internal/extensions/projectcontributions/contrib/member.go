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

// Member is the project's one member under its provider token (review R-A1):
// its contributions folder's providers AND the object entries the process
// hosting CWM pushes (api.EntriesPush), one ObjectProvider per kind. An entry
// served twice - a folder file and an object, or two objects, with the same
// kind and id - is withheld on both sides, with a clash naming both
// (api.Clashes). Keyed by its token: a hub serving several projects holds one
// per token.
type Member struct {
	token string

	mu      sync.Mutex
	trusted bool
	folder  []contribapi.ContributionProvider
	objects map[string]*ObjectProvider // by kind
}

// NewMember is the member of the project published under token, serving
// folder (its folder's providers; none for a project with no folder yet).
func NewMember(token string, folder []contribapi.ContributionProvider) *Member {
	return &Member{token: token, folder: folder, objects: map[string]*ObjectProvider{}}
}

// Token is the project's provider token.
func (m *Member) Token() string { return m.token }

// SetTrusted says whether the universe trusts the project (projects.yaml
// trusted): an override entry of an executable kind from an untrusted project
// is refused by the manifest builder. Untrusted until said.
func (m *Member) SetTrusted(trusted bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trusted = trusted
	for _, op := range m.objects {
		op.SetTrusted(trusted)
	}
}

// SetFolder replaces the folder's providers (the folder was read again).
func (m *Member) SetFolder(folder []contribapi.ContributionProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.folder = folder
}

// Apply takes one push: a put replaces what the object published (under any
// kind), a remove forgets it, a snapshot replaces the whole set. A push of
// another token, or one that does not validate, is refused and changes
// nothing.
func (m *Member) Apply(p api.EntriesPush) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Token != m.token {
		return fmt.Errorf("push for %s reached the member %s", p.Token, m.token)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	switch p.Op {
	case api.PushPut:
		return m.put(*p.Entry, true)
	case api.PushRemove:
		for _, op := range m.objects {
			op.Remove(p.Object)
		}
	case api.PushSnapshot:
		kept := m.objects
		m.objects = map[string]*ObjectProvider{}
		for _, e := range p.Entries {
			if err := m.put(e, false); err != nil {
				m.objects = kept
				return err
			}
		}
	}
	return nil
}

// put records e under its kind, the object's entries of other kinds removed
// first when replace. Callers hold mu.
func (m *Member) put(e api.ObjectEntry, replace bool) error {
	if e.Entry.Provider != m.token {
		return fmt.Errorf("object %s: entry %s is not under %s", e.Object, e.URI(), m.token)
	}
	op, ok := m.objects[e.Entry.Type]
	if !ok {
		op = NewObjectProvider(m.token, e.Entry.Type)
		op.SetTrusted(m.trusted)
	}
	if err := op.Put(e); err != nil {
		return err
	}
	if replace {
		for kind, other := range m.objects {
			if kind != e.Entry.Type {
				other.Remove(e.Object)
			}
		}
	}
	m.objects[e.Entry.Type] = op
	return nil
}

// ObjectEntries are the object entries held, every kind, by URI then object.
func (m *Member) ObjectEntries() []api.ObjectEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.objectEntries()
}

func (m *Member) objectEntries() []api.ObjectEntry {
	var out []api.ObjectEntry
	for _, op := range m.objects {
		out = append(out, op.Entries()...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URI() != out[j].URI() {
			return out[i].URI() < out[j].URI()
		}
		return out[i].Object < out[j].Object
	})
	return out
}

// Clashes are the entries the project serves twice - a folder file and an
// object, or two objects: none of them is served.
func (m *Member) Clashes(ctx context.Context) ([]api.Clash, error) {
	m.mu.Lock()
	folder, objects := append([]contribapi.ContributionProvider(nil), m.folder...), m.objectEntries()
	m.mu.Unlock()
	var files []contribapi.Contribution
	for _, p := range folder {
		items, err := p.Enumerate(ctx)
		if err != nil {
			return nil, fmt.Errorf("member %s: %s: %w", m.token, p.Type(), err)
		}
		for _, c := range items {
			if c.Provider == "" || c.Type == "" {
				c.Provider, c.Type = p.Provider(), p.Type()
			}
			files = append(files, c)
		}
	}
	return api.Clashes(files, objects), nil
}

// Findings are the clashes as a person reads them, each naming both sources.
func (m *Member) Findings(ctx context.Context) ([]string, error) {
	clashes, err := m.Clashes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(clashes))
	for _, c := range clashes {
		out = append(out, c.Finding())
	}
	return out, nil
}

// Providers are what the member serves: the folder's providers and one
// provider per kind of object entries, each without the entries a clash
// withholds. The hub builds the member's manifest from them and opens its
// layers through them.
func (m *Member) Providers(ctx context.Context) ([]contribapi.ContributionProvider, error) {
	clashes, err := m.Clashes(ctx)
	if err != nil {
		return nil, err
	}
	withheld := map[string]bool{}
	for _, c := range clashes {
		withheld[c.URI] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]contribapi.ContributionProvider, 0, len(m.folder)+len(m.objects))
	for _, p := range m.folder {
		out = append(out, withholding(p, withheld))
	}
	kinds := make([]string, 0, len(m.objects))
	for k := range m.objects {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		out = append(out, withholding(m.objects[k], withheld))
	}
	return out, nil
}

// withholding serves p without the entries whose URI withheld names.
func withholding(p contribapi.ContributionProvider, withheld map[string]bool) contribapi.ContributionProvider {
	if len(withheld) == 0 {
		return p
	}
	return &withheldProvider{ContributionProvider: p, withheld: withheld}
}

type withheldProvider struct {
	contribapi.ContributionProvider
	withheld map[string]bool
}

func (w *withheldProvider) uri(id string) string {
	return w.Provider() + ":/" + w.Type() + "/" + id
}

func (w *withheldProvider) Enumerate(ctx context.Context) ([]contribapi.Contribution, error) {
	items, err := w.ContributionProvider.Enumerate(ctx)
	if err != nil {
		return nil, err
	}
	out := items[:0:0]
	for _, c := range items {
		if !w.withheld[w.uri(c.ID)] {
			out = append(out, c)
		}
	}
	return out, nil
}

// OverrideEntries forwards the wrapped provider's override entries, without
// those withheld.
func (w *withheldProvider) OverrideEntries() []contribapi.OverrideEntry {
	r, ok := w.ContributionProvider.(contribapi.OverrideEntryReporter)
	if !ok {
		return nil
	}
	var out []contribapi.OverrideEntry
	for _, ov := range r.OverrideEntries() {
		if !w.withheld[ov.Entry.ResolvedURI()] {
			out = append(out, ov)
		}
	}
	return out
}

func (w *withheldProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	if w.withheld[w.uri(id)] {
		return nil, contribapi.LayerRef{}, contribapi.ErrContributionNotFound{URI: w.uri(id)}
	}
	return w.ContributionProvider.Open(ctx, id, layer)
}
