// SPDX-License-Identifier: Apache-2.0

package remote_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// fakeSender answers catalog calls and records what was asked.
//
// Records the PATHS rather than just counting, because the thing most worth
// asserting here is that a consumer asked the aggregator once per page and not
// once per entry — a count alone would pass for the wrong reason.
type fakeSender struct {
	paths   []string
	filters []contribapi.ListFilter // the filter of every ListContributions call, in order
	pages   [][]contribapi.Contribution
	bodies  map[string][]byte
	opens   map[string]mbapi.OpenLayerOutput // key: uri + "#" + layer
	names   mbapi.ListNamesOutput
	// manifests answer GetManifest; a provider in split answers its slot
	// types by name first, as the aggregator does past one message.
	manifests map[string]*mbapi.Manifest
	split     map[string]bool
}

func (f *fakeSender) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }

func (f *fakeSender) Call(_ context.Context, path string, params any, _ ...ktsapi.CallOption) (json.RawMessage, error) {
	f.paths = append(f.paths, path)
	switch {
	case strings.HasSuffix(path, "ListContributions"):
		in := params.(mbapi.CatalogListContributionsInput)
		f.filters = append(f.filters, in.Filter)
		idx := 0
		if in.Cursor != "" {
			idx = int(in.Cursor[0] - '0')
		}
		// The aggregator applies the filter server-side and stamps every
		// page with its offset; the fake does the same, so a reader that
		// asked for less than the universe receives less.
		offset := 0
		for k := 0; k < idx; k++ {
			offset += len(f.pages[k])
		}
		var page []contribapi.Contribution
		for _, e := range f.pages[idx] {
			if in.Filter.IsEmpty() || in.Filter.Matches(e.TypedURI()) {
				page = append(page, e)
			}
		}
		out := mbapi.CatalogListContributionsOutput{Contributions: page, Pagination: mbapi.Pagination{Offset: offset, PageSize: len(page)}}
		if idx+1 < len(f.pages) {
			out.Pagination.HasMore = true
			out.Pagination.NextCursor = string(rune('0' + idx + 1))
		}
		return json.Marshal(out)
	case strings.HasSuffix(path, "ListNames"):
		return json.Marshal(f.names)
	case strings.HasSuffix(path, "OpenLayer"):
		in := params.(mbapi.OpenLayerInput)
		return json.Marshal(f.opens[in.URI+"#"+in.Layer])
	case strings.HasSuffix(path, "GetManifest"):
		in := params.(mbapi.GetManifestInput)
		m := *f.manifests[in.Provider]
		switch {
		case in.SlotType != "":
			var one []mbapi.SlotType
			for _, st := range m.SlotTypes {
				if st.Type == in.SlotType {
					one = append(one, st)
				}
			}
			m.SlotTypes = one
		case f.split[in.Provider]:
			names := make([]mbapi.SlotType, len(m.SlotTypes))
			for i, st := range m.SlotTypes {
				names[i] = mbapi.SlotType{Type: st.Type, Version: st.Version}
			}
			m.SlotTypes = names
			return json.Marshal(mbapi.GetManifestOutput{Manifest: &m, SplitSlotTypes: true})
		}
		return json.Marshal(mbapi.GetManifestOutput{Manifest: &m})
	case strings.HasSuffix(path, "GetBody"):
		in := params.(mbapi.GetBodyInput)
		return json.Marshal(mbapi.GetBodyOutput{Content: f.bodies[in.Hash]})
	}
	return nil, nil
}

func entry(provider, kind, id string, body []byte) contribapi.Contribution {
	c := contribapi.Contribution{Provider: provider, Type: kind, ID: id}
	if body != nil {
		c.SetData(body)
		c.DropInline() // the index plane: a descriptor, no content
	}
	return c
}

// The cursor loop lives here, once. It used to be copied into every consumer,
// and a paging bug is invisible until a universe crosses a page boundary.
func TestIndexFollowsEveryPage(t *testing.T) {
	f := &fakeSender{pages: [][]contribapi.Contribution{
		{entry("a", "skill", "one", nil)},
		{entry("a", "skill", "two", nil)},
		{entry("b", "task", "three", nil)},
	}}
	got, err := remote.New(f).Index(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("read %d entries across 3 pages, want 3", len(got))
	}
	if n := len(f.paths); n != 3 {
		t.Errorf("made %d calls for 3 pages, want 3", n)
	}
}

// Grouped by the contribution's OWN provider, never by the manifest that
// carried it: attributing a layer provider's entry to its carrier gives one
// logical entry two URIs, and dedupe breaks silently.
func TestListGroupsByTheContributionsOwnProvider(t *testing.T) {
	f := &fakeSender{pages: [][]contribapi.Contribution{{
		entry("ktw-docs", "skill", "one", nil),
		entry("system", "contribution-type", "skill", nil),
		entry("ktw-docs", "skill", "two", nil),
	}}}
	ms, err := remote.New(f).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byProvider := map[string]int{}
	for _, m := range ms {
		byProvider[m.Provider] = len(m.Contributions)
	}
	if byProvider["ktw-docs"] != 2 || byProvider["system"] != 1 {
		t.Errorf("grouped as %v, want ktw-docs=2 system=1", byProvider)
	}
}

// The whole reason this package exists: a consumer holding nothing but a
// Sender can be handed to the query surface.
func TestTheQuerySurfaceAcceptsARemoteReader(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"requires": []string{"workspace"}})
	e := entry("ktw-docs", "skill", "doc-create", body)
	f := &fakeSender{
		pages:  [][]contribapi.Contribution{{e}},
		bodies: map[string][]byte{e.DataHash(): body},
	}

	var cat mbapi.CatalogReader = remote.New(f)
	got, err := mbapi.QueryTyped[mbapi.SkillBody](context.Background(),
		mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "skill", Catalog: cat})
	if err != nil {
		t.Fatalf("Query over a remote reader: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if len(got[0].Body.Requires) != 1 || got[0].Body.Requires[0] != "workspace" {
		t.Errorf("body did not decode: %+v", got[0].Body)
	}
	if got[0].URI != "ktw-docs:/skill/doc-create" {
		t.Errorf("URI = %q", got[0].URI)
	}
}

// A reader satisfies the READ contract. It must not be asked for the
// aggregator's write protocol — that is the split that lets it be light.
func TestAReaderIsNotACatalog(t *testing.T) {
	var r any = remote.New(&fakeSender{})
	if _, ok := r.(mbapi.CatalogReader); !ok {
		t.Error("remote.Catalog no longer satisfies CatalogReader")
	}
	if _, ok := r.(mbapi.Catalog); ok {
		t.Error("remote.Catalog implements the full Catalog — it should not have to")
	}
}
