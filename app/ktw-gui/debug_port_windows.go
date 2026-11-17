// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"log"
	"os"

	"github.com/wailsapp/go-webview2/pkg/edge"
)

// openDebugPort gives WebView2 its debugging port when KTW_GUI_DEBUG_PORT asks
// one, through the patched go-webview2 (third_party/go-webview2/PATCH.md):
// Wails has no option for it and the loader clears WebView2's own variable.
func openDebugPort() {
	args, err := debugPortArgs(os.Getenv(DebugPortEnv))
	if err != nil {
		log.Printf("[main] %v", err)
		return
	}
	if len(args) > 0 {
		edge.ExtraBrowserArgs = append(edge.ExtraBrowserArgs, args...)
		log.Printf("[main] WebView2 opens its debugging port: %v", args)
	}
}
