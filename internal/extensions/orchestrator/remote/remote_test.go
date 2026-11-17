// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

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

func TestRemoteOrch_AddressesAdminOrchestratorSlot(t *testing.T) {
	f := &fakeSender{resp: json.RawMessage(`{"universes":[{"name":"acme"}]}`)}
	out, err := NewRemote(f).ListUniverses(context.Background(), api.ListUniversesInput{})
	if err != nil {
		t.Fatalf("ListUniverses: %v", err)
	}
	if f.path != "ktw-univers:orchestrator/list-universes" {
		t.Errorf("path = %q, want ktw-univers:orchestrator/list-universes", f.path)
	}
	if len(out.Universes) != 1 || out.Universes[0].Name != "acme" {
		t.Errorf("decoded = %+v", out)
	}
}

func TestRemoteOrch_KebabPaths(t *testing.T) {
	cases := map[string]func(*fakeSender){
		"ktw-univers:orchestrator/create-universe": func(f *fakeSender) {
			NewRemote(f).CreateUniverse(context.Background(), api.CreateUniverseInput{Name: "x", DisplayName: "X"})
		},
		"ktw-univers:orchestrator/delete-universe": func(f *fakeSender) {
			NewRemote(f).DeleteUniverse(context.Background(), api.DeleteUniverseInput{Name: "x"})
		},
		"ktw-univers:orchestrator/add-sidecar": func(f *fakeSender) { NewRemote(f).AddSidecar(context.Background(), api.AddSidecarInput{Universe: "x"}) },
		"ktw-univers:orchestrator/attach-project": func(f *fakeSender) {
			NewRemote(f).AttachProject(context.Background(), api.AttachProjectInput{ProjectDir: "d", Universe: "x"})
		},
		"ktw-univers:orchestrator/scan-inventory": func(f *fakeSender) { NewRemote(f).ScanInventory(context.Background(), api.ScanInventoryInput{}) },
	}
	for wantPath, invoke := range cases {
		f := &fakeSender{resp: json.RawMessage(`{}`)}
		invoke(f)
		if f.path != wantPath {
			t.Errorf("path = %q, want %q", f.path, wantPath)
		}
	}
}
