// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"encoding/json"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// fakeSender records the last call + returns a canned response — proves the client
// addresses the right admin-universe marketplace slot + decodes, with no NATS/hub.
type fakeSender struct {
	path string
	in   any
	resp json.RawMessage
}

func (f *fakeSender) Call(_ context.Context, path string, params any, _ ...ktsp.CallOption) (json.RawMessage, error) {
	f.path, f.in = path, params
	return f.resp, nil
}
func (f *fakeSender) Notify(_ context.Context, _ string, _ any, _ ...ktsp.CallOption) error {
	return nil
}

func TestRemote_NoInputMethodAddressesAdminSlot(t *testing.T) {
	f := &fakeSender{resp: json.RawMessage(`[{"name":"acme","enabled":true}]`)}
	mkts, err := NewRemote(f).ListMarketplaces(context.Background())
	if err != nil {
		t.Fatalf("ListMarketplaces: %v", err)
	}
	if f.path != "ktw-univers:marketplace/list-marketplaces" {
		t.Errorf("path = %q, want ktw-univers:marketplace/list-marketplaces", f.path)
	}
	if len(mkts) != 1 || mkts[0].Name != "acme" {
		t.Errorf("decoded = %+v, want one marketplace 'acme'", mkts)
	}
}

func TestRemote_InputMethodPassesTypedInput(t *testing.T) {
	f := &fakeSender{resp: json.RawMessage(`{"name":"acme"}`)}
	out, err := NewRemote(f).AddMarketplace(context.Background(), mktapi.AddMarketplaceInput{Name: "acme", URL: "https://x.example/cat.yaml"})
	if err != nil {
		t.Fatalf("AddMarketplace: %v", err)
	}
	if f.path != "ktw-univers:marketplace/add-marketplace" {
		t.Errorf("path = %q, want …/add-marketplace", f.path)
	}
	in, ok := f.in.(mktapi.AddMarketplaceInput)
	if !ok || in.Name != "acme" || in.URL != "https://x.example/cat.yaml" {
		t.Errorf("input not forwarded: %#v (ok=%v)", f.in, ok)
	}
	if out.Name != "acme" {
		t.Errorf("output decode = %+v", out)
	}
}

// The universe-targeting inputs carry UniverseHome verbatim: over KTSP the
// serving hub's own workspace is ADMIN, so a dropped home silently retargets
// the write (the GUI wire/unwire/configure bug of 2026-09-18).
func TestRemote_UniverseHomeIsForwarded(t *testing.T) {
	f := &fakeSender{resp: json.RawMessage(`{}`)}
	NewRemote(f).ConfigureItem(context.Background(), mktapi.ConfigureItemInput{Name: "x", UniverseHome: "/u/one"})
	if in, ok := f.in.(mktapi.ConfigureItemInput); !ok || in.UniverseHome != "/u/one" {
		t.Errorf("ConfigureItem input = %#v, want UniverseHome=/u/one", f.in)
	}
	NewRemote(f).SetItemConfig(context.Background(), mktapi.SetItemConfigInput{Name: "x", File: "X.yaml", Content: "a: 1\n", UniverseHome: "/u/one"})
	if in, ok := f.in.(mktapi.SetItemConfigInput); !ok || in.UniverseHome != "/u/one" {
		t.Errorf("SetItemConfig input = %#v, want UniverseHome=/u/one", f.in)
	}
}

// The kebab wire names must match ToKebab(GoMethod) exactly (Slot.Handle is exact).
func TestRemote_KebabPathsForRepresentativeMethods(t *testing.T) {
	cases := map[string]func(*fakeSender){
		"ktw-univers:marketplace/search-items": func(f *fakeSender) { NewRemote(f).SearchItems(context.Background(), mktapi.SearchItemsInput{}) },
		"ktw-univers:marketplace/set-version":  func(f *fakeSender) { NewRemote(f).SetVersion(context.Background(), mktapi.SetVersionInput{Name: "x"}) },
		"ktw-univers:marketplace/wire-into-universe": func(f *fakeSender) {
			NewRemote(f).WireIntoUniverse(context.Background(), mktapi.WireIntoUniverseInput{Name: "x"})
		},
		"ktw-univers:marketplace/configure-item": func(f *fakeSender) {
			NewRemote(f).ConfigureItem(context.Background(), mktapi.ConfigureItemInput{Name: "x"})
		},
		"ktw-univers:marketplace/get-item-config": func(f *fakeSender) {
			NewRemote(f).GetItemConfig(context.Background(), mktapi.GetItemConfigInput{Name: "x", File: "X.yaml"})
		},
		"ktw-univers:marketplace/set-item-config": func(f *fakeSender) {
			NewRemote(f).SetItemConfig(context.Background(), mktapi.SetItemConfigInput{Name: "x", File: "X.yaml", Content: "a: 1\n"})
		},
	}
	for wantPath, invoke := range cases {
		f := &fakeSender{resp: json.RawMessage(`{}`)}
		invoke(f)
		if f.path != wantPath {
			t.Errorf("path = %q, want %q", f.path, wantPath)
		}
	}
}
