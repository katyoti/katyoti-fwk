// SPDX-License-Identifier: Apache-2.0

package person

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
)

// The open source build accepts a relayed act confirmed at the prompt, by a
// code or by the phone's approval only: no prover is served, an entra
// confirmation is no confirmation, and the refusal does not offer it.
func TestTheOpenSourceBuildServesNoProver(t *testing.T) {
	if len(provers) != 0 {
		t.Fatalf("provers %v; want none in the open source build", provers)
	}
	ctx := cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindAgent, ID: "claude-code",
		Relay: &cause.Relay{Person: "jdoe", Act: "decide:x", Confirmed: cause.ConfirmedEntra, Proof: "p-1"}})
	r, err := acceptRelayed(ctx, "jdoe", "decide:x", "decide x", func() bool { return true })
	if err == nil || r != nil || !strings.Contains(err.Error(), "is no confirmation (prompt, totp, passkey)") {
		t.Fatalf("confirmed entra: %+v, %v; want refused, prompt, totp and passkey offered", r, err)
	}
}
