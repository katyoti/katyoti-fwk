// SPDX-License-Identifier: Apache-2.0

// Package remote is the KTSP client of the ProfileService served in the admin
// universe (`ktw-univers:profile/<method>`). A caller holding a sender scoped
// to admin (appcore.NewAdminHubSender, the GUI's admin core) calls the same
// ProfileService interface; the hub does every write.
package remote

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// slotBase is the provider:slot prefix of the profile service; the wire method
// names are the kebab-case Go method names.
const slotBase = "ktw-univers:profile/"

// NewRemote returns a ProfileService that dispatches over KTSP via s.
func NewRemote(s ktsapi.Sender) api.ProfileService {
	return &remoteProfile{s: s}
}

type remoteProfile struct{ s ktsapi.Sender }

var _ api.ProfileService = (*remoteProfile)(nil)

func call[Out any](ctx context.Context, s ktsapi.Sender, method string, in any) (Out, error) {
	var out Out
	raw, err := s.Call(ctx, slotBase+method, in)
	if err != nil {
		return out, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return out, nil
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}

func (r *remoteProfile) Get(ctx context.Context) (api.Profile, error) {
	return call[api.Profile](ctx, r.s, "get", nil)
}

func (r *remoteProfile) Update(ctx context.Context, in api.UpdateInput) (api.Profile, error) {
	return call[api.Profile](ctx, r.s, "update", in)
}

func (r *remoteProfile) Suggest(ctx context.Context) (api.Suggestion, error) {
	return call[api.Suggestion](ctx, r.s, "suggest", nil)
}

func (r *remoteProfile) GuideGet(ctx context.Context) (api.GuideState, error) {
	return call[api.GuideState](ctx, r.s, "guide-get", nil)
}

func (r *remoteProfile) GuideComplete(ctx context.Context, in api.GuideCompleteInput) (api.GuideState, error) {
	return call[api.GuideState](ctx, r.s, "guide-complete", in)
}

func (r *remoteProfile) GuideReset(ctx context.Context, in api.GuideResetInput) (api.GuideState, error) {
	return call[api.GuideState](ctx, r.s, "guide-reset", in)
}

func (r *remoteProfile) RelayGet(ctx context.Context) (api.RelayState, error) {
	return call[api.RelayState](ctx, r.s, "relay-get", nil)
}

func (r *remoteProfile) RelaySet(ctx context.Context, in api.RelaySetInput) (api.RelayState, error) {
	return call[api.RelayState](ctx, r.s, "relay-set", in)
}
