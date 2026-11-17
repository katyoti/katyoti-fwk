// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"errors"
	"io"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
)

// moduleFiles is a module's files by path, read as CWM reads them.
type moduleFiles map[string][]byte

func (m moduleFiles) ObjectFileRead(_ context.Context, in cwmapi.ObjectFileReadInput) (cwmapi.ObjectFileReadOutput, error) {
	b, ok := m[in.Path]
	if !ok {
		return cwmapi.ObjectFileReadOutput{}, errors.New("no file " + in.Path)
	}
	return cwmapi.ObjectFileReadOutput{Content: b}, nil
}

// The provider serves what the CWM host pushes, replaced whole on each put,
// gone on remove, and withholds an entry two objects publish.
func TestTheObjectProviderServesPushedEntries(t *testing.T) {
	p := NewObjectProvider("archi-dossiers", "deck-theme")
	files := moduleFiles{"themes/acme-blue/logo.svg": []byte("<svg/>")}
	e, err := api.EntryOfObject(context.Background(), cwmapi.Object{ID: "acme-blue", Type: "deck-theme",
		Capabilities: map[string]map[string]any{
			"deck-theme":       {"name": "acme-blue"},
			api.CapContributes: {"kind": "deck-theme", "layers": map[string]any{"data": "capabilities.deck-theme", "logo": "files/themes/acme-blue/logo.svg"}},
		}}, "archi-dossiers", files)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Put(e); err != nil {
		t.Fatal(err)
	}
	providertest.Run(t, p)
	rc, _, err := p.Open(context.Background(), "acme-blue", "logo")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	if string(b) != "<svg/>" {
		t.Fatalf("logo = %q", b)
	}

	twin := e
	twin.Object = "copy-of-acme-blue"
	if err := p.Put(twin); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.Enumerate(context.Background()); len(got) != 0 {
		t.Fatalf("served %d entries, want the clashing entry withheld", len(got))
	}
	if c := p.Clashes(); len(c) != 1 {
		t.Fatalf("clashes = %+v", c)
	}
	p.Remove("copy-of-acme-blue")
	if got, _ := p.Enumerate(context.Background()); len(got) != 1 {
		t.Fatalf("served %d entries after the twin went, want 1", len(got))
	}
	p.Remove("acme-blue")
	if _, _, err := p.Open(context.Background(), "acme-blue", "data"); !errors.Is(err, contribapi.ErrContributionNotFound{}) {
		t.Fatalf("open after remove: %v, want not found", err)
	}
	other := api.ObjectEntry{Entry: contribapi.Contribution{Provider: "archi-dossiers", Type: "persona", ID: "x"}, Object: "x"}
	if err := p.Put(other); err == nil {
		t.Fatal("an entry of another kind was put")
	}
}

// An override entry is never served as an entry of its own: the provider
// reports it apart, with the trust the member says, for the manifest builder
// to apply over the entry it names.
func TestAnOverrideEntryIsReportedApart(t *testing.T) {
	m := NewMember("acme", nil)
	put := func(object, overrides string) {
		t.Helper()
		e := api.ObjectEntry{Entry: contribapi.Contribution{Provider: "acme", Type: "persona", ID: object}, Object: object, Overrides: overrides}
		e.Entry.SetLayer("prompt", []byte("text of "+object), "text/markdown")
		if err := m.Apply(api.EntriesPush{Token: "acme", Op: api.PushPut, Object: object, Entry: &e}); err != nil {
			t.Fatal(err)
		}
	}
	put("own", "")
	put("reviewer-fr", "ktw-example:/persona/reviewer")
	ps, err := m.Providers(context.Background())
	if err != nil || len(ps) != 1 {
		t.Fatalf("providers = %d, %v", len(ps), err)
	}
	got, _ := ps[0].Enumerate(context.Background())
	if len(got) != 1 || got[0].ID != "own" {
		t.Fatalf("enumerated %+v; want the own entry only", got)
	}
	r, ok := ps[0].(contribapi.OverrideEntryReporter)
	if !ok {
		t.Fatal("the object provider reports no override entry")
	}
	ovs := r.OverrideEntries()
	if len(ovs) != 1 || ovs[0].Overrides != "ktw-example:/persona/reviewer" || ovs[0].Origin != "project:acme" || ovs[0].Trusted {
		t.Fatalf("override entries = %+v; want reviewer-fr, from project:acme, untrusted until said", ovs)
	}
	m.SetTrusted(true)
	if ovs := r.OverrideEntries(); len(ovs) != 1 || !ovs[0].Trusted {
		t.Fatalf("after SetTrusted: %+v", ovs)
	}
}
