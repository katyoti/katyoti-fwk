// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
)

// The GUI's runtime: ktw-gui renders in the Microsoft Edge
// WebView2 Runtime on Windows. A clean machine may lack it - a Windows
// Sandbox, a Windows 10 without Edge's runtime - and the GUI would then open on
// WebView2's own download window. Setup makes sure of it before anything opens
// the GUI: present, it is left alone; absent, it is installed in the person's
// profile (no administrator). Elsewhere the GUI needs nothing of the kind.

// GUIRuntime answers what setup did about the GUI's runtime.
const (
	GUIRuntimePresent   = "present"
	GUIRuntimeInstalled = "installed"
	GUIRuntimeNone      = ""
)

// guiRuntime is the GUI's runtime on this platform: present says whether it is
// installed, install installs it. Nil where the GUI needs none.
type guiRuntime struct {
	name    string
	present func() bool
	install func(ctx context.Context) error
}

// ensureGUIRuntime installs the runtime when it is absent and checks it then
// answers present; the answer says what was done.
func ensureGUIRuntime(ctx context.Context, rt *guiRuntime) (string, error) {
	if rt == nil {
		return GUIRuntimeNone, nil
	}
	if rt.present() {
		return GUIRuntimePresent, nil
	}
	if err := rt.install(ctx); err != nil {
		return GUIRuntimeNone, fmt.Errorf("install the %s: %w", rt.name, err)
	}
	if !rt.present() {
		return GUIRuntimeNone, fmt.Errorf("the %s installer ended but the runtime is not found", rt.name)
	}
	return GUIRuntimeInstalled, nil
}
