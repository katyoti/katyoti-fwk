// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
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

func (f *fakeSender) Notify(context.Context, string, any, ...ktsp.CallOption) error { return nil }

func TestRemoteAddressesTheAdminProfileSlot(t *testing.T) {
	f := &fakeSender{resp: json.RawMessage(`{"id":"john-doe","email":"john.doe@example.com","source":"placeholder"}`)}
	p, err := NewRemote(f).Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.path != "ktw-univers:profile/get" || p.ID != "john-doe" || p.Source != api.SourcePlaceholder {
		t.Fatalf("path %q, profile %+v", f.path, p)
	}

	f.resp = json.RawMessage(`{"id":"jdoe","email":"c@io.example","source":"local"}`)
	in := api.UpdateInput{ID: "jdoe", Email: "c@io.example"}
	if _, err := NewRemote(f).Update(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if f.path != "ktw-univers:profile/update" || f.in != in {
		t.Fatalf("path %q, input %#v", f.path, f.in)
	}

	f.resp = json.RawMessage(`{"id":"jdoe","emails":[{"email":"c@io.example","source":"git"}]}`)
	s, err := NewRemote(f).Suggest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.path != "ktw-univers:profile/suggest" || len(s.Emails) != 1 {
		t.Fatalf("path %q, suggestion %+v", f.path, s)
	}
}
