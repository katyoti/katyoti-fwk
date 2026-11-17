// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Setup leaves the GUI's runtime alone when present, installs it when absent
// , and says a failed or ineffective install.
func TestSetupEnsuresTheGUIRuntime(t *testing.T) {
	ctx := context.Background()
	if got, err := ensureGUIRuntime(ctx, nil); got != GUIRuntimeNone || err != nil {
		t.Fatalf("no runtime needed: %q %v", got, err)
	}
	installs := 0
	present := true
	rt := &guiRuntime{name: "WebView2", present: func() bool { return present }, install: func(context.Context) error {
		installs++
		present = true
		return nil
	}}
	if got, err := ensureGUIRuntime(ctx, rt); got != GUIRuntimePresent || err != nil || installs != 0 {
		t.Fatalf("present: %q %v, %d installs", got, err, installs)
	}
	present = false
	if got, err := ensureGUIRuntime(ctx, rt); got != GUIRuntimeInstalled || err != nil || installs != 1 {
		t.Fatalf("absent: %q %v, %d installs; want installed once", got, err, installs)
	}
	failing := &guiRuntime{name: "WebView2", present: func() bool { return false }, install: func(context.Context) error { return errors.New("offline") }}
	if _, err := ensureGUIRuntime(ctx, failing); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("a failed install is said: %v", err)
	}
	silent := &guiRuntime{name: "WebView2", present: func() bool { return false }, install: func(context.Context) error { return nil }}
	if _, err := ensureGUIRuntime(ctx, silent); err == nil {
		t.Fatal("an install that leaves no runtime is said")
	}
}
