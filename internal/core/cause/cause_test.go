// SPDX-License-Identifier: Apache-2.0

package cause

import (
	"context"
	"testing"
)

// A cause set on a context is read back as set; none set reads as none, at
// depth 0; a zero cause is never carried.
func TestWithCauseRoundTrip(t *testing.T) {
	ctx := context.Background()
	if c, ok := CauseFrom(ctx); ok || !c.IsZero() || DepthFrom(ctx) != 0 {
		t.Fatalf("a bare context carries %+v (%v), want none at depth 0", c, ok)
	}
	want := Cause{Kind: KindAgent, ID: "run-1", Depth: 2}
	ctx = WithCause(ctx, want)
	got, ok := CauseFrom(ctx)
	if !ok || got != want || DepthFrom(ctx) != 2 {
		t.Fatalf("CauseFrom = %+v (%v), want %+v", got, ok, want)
	}
	if got.String() != "agent:run-1@2" {
		t.Errorf("String = %q, want agent:run-1@2", got.String())
	}
	if c, ok := CauseFrom(WithCause(ctx, Cause{})); !ok || c != want {
		t.Errorf("a zero cause replaced %+v with %+v (%v)", want, c, ok)
	}
	if (Cause{}).String() != "" {
		t.Error("a zero cause renders as something")
	}
}
