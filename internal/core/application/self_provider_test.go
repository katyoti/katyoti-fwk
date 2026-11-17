// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"testing"
)

// TestSelfProviderFromContext_NoBinding confirms the function returns
// `("", false)` cleanly when no override and no Application is bound —
// callers that need a provider can detect "missing" and error with a
// specific message instead of getting an empty string they confuse with
// a wildcard.
func TestSelfProviderFromContext_NoBinding(t *testing.T) {
	got, ok := SelfProviderFromContext(context.Background())
	if ok {
		t.Errorf("ok = true on empty ctx, want false")
	}
	if got != "" {
		t.Errorf("got = %q, want empty", got)
	}
}

// TestWithSelfProvider_RoundTrip confirms the explicit override path:
// WithSelfProvider stores a value; SelfProviderFromContext reads it
// back. Empty name is a no-op (ctx unchanged).
func TestWithSelfProvider_RoundTrip(t *testing.T) {
	ctx := WithSelfProvider(context.Background(), "test-binary")
	got, ok := SelfProviderFromContext(ctx)
	if !ok {
		t.Fatalf("ok = false, want true after WithSelfProvider")
	}
	if got != "test-binary" {
		t.Errorf("got = %q, want %q", got, "test-binary")
	}

	// Empty name = no-op
	ctx2 := WithSelfProvider(context.Background(), "")
	if _, ok := SelfProviderFromContext(ctx2); ok {
		t.Errorf("empty WithSelfProvider should be a no-op")
	}
}

// TestSelfProvider_AppFallback confirms that when no explicit override
// is in ctx but an Application IS bound, its Provider() value wins.
// This is the production path: appcore.NewAppExt records AppName via
// app.SetProvider, then callers read it via SelfProviderFromContext.
func TestSelfProvider_AppFallback(t *testing.T) {
	app := &Application{}
	app.SetProvider("ktw-test")

	ctx := WithApplication(context.Background(), app)
	got, ok := SelfProviderFromContext(ctx)
	if !ok {
		t.Fatalf("ok = false, want true with Application bound")
	}
	if got != "ktw-test" {
		t.Errorf("got = %q, want %q", got, "ktw-test")
	}
}

// TestSelfProvider_OverrideWinsOverApp confirms precedence: when both
// WithSelfProvider AND an Application with Provider() are bound, the
// explicit override wins. Lets tests / sub-process bridges forge a
// provider identity that differs from the host binary's AppName.
func TestSelfProvider_OverrideWinsOverApp(t *testing.T) {
	app := &Application{}
	app.SetProvider("ktw-host")

	ctx := WithApplication(context.Background(), app)
	ctx = WithSelfProvider(ctx, "forged-name")

	got, _ := SelfProviderFromContext(ctx)
	if got != "forged-name" {
		t.Errorf("got = %q, want override %q (Application's %q should be overridden)",
			got, "forged-name", "ktw-host")
	}
}
