// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"errors"
	"fmt"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/busauth"
)

// authorize is the stamp of a slot call: on a bus that checks
// keys, the caller is whose key signed the message - never a header it wrote.
// A call is refused when it is unsigned or signed by a key the hub did not
// mint, when its tenant header disagrees with the tenant of the caller's key
// (the account's), and when its cause says « person » without the
// attestation of a person's client (a sidecar or an agent's process cannot
// make up the person's act; it relays one it was given, attestation and
// all). The served context carries the caller (busauth.CallerFrom) and the
// cause's attestation, for the calls the method makes in turn.
//
// A bus that checks no keys (a test bus, a hub of a version before keys)
// stamps nothing: ctx is returned as it came. A keyed call with no tenant
// header is served in its caller's tenant.
func authorize(ctx context.Context, msg mbapi.Message, tenantID string) (context.Context, string, *ktsp.RPCError) {
	caller, err := busauth.CallerOf(msg)
	if errors.Is(err, busauth.ErrOpenBus) {
		return ctx, tenantID, nil
	}
	if err != nil {
		return ctx, tenantID, refused(err.Error())
	}
	if tenantID == "" {
		tenantID = caller.Tenant
	}
	if tenantID != caller.Tenant {
		return ctx, tenantID, refused(fmt.Sprintf(
			"the call's tenant header %q disagrees with its caller %s, of tenant %q", tenantID, caller, caller.Tenant))
	}
	ctx = busauth.WithCaller(ctx, caller)
	c, ok := CauseFromMessage(msg)
	if !ok {
		return ctx, tenantID, nil
	}
	stamped, _ := msg.(busauth.Stamped)
	if profileapi.IsPerson(cwmapi.Actor{ID: c.ID, Kind: c.Kind}) {
		if _, err := stamped.CauseAttester(); err != nil {
			return ctx, tenantID, refused(fmt.Sprintf(
				"the call from %s says it is the person %s's act: %v", caller, c.ID, err))
		}
	}
	if h := msg.Header(); h != nil {
		if att := h[busauth.HeaderCauseSig]; len(att) > 0 {
			ctx = busauth.WithCauseAttestation(ctx, h[HeaderCause][0], att[0])
		}
	}
	return ctx, tenantID, nil
}

// refused is the answer to a call the stamp refuses: an invalid request whose
// message says why, so the caller reads it in the error it gets.
func refused(why string) *ktsp.RPCError {
	return &ktsp.RPCError{Code: ktsp.CodeInvalidRequest, Message: "Refused by the bus: " + why}
}
