//go:build windows

package edge

import "strings"

// ExtraBrowserArgs are browser arguments a program adds to every WebView2
// environment it creates, beside its framework's own (Wails fills
// AdditionalBrowserArgs from its options only, and the loader clears
// WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS). Set before the first window opens.
//
// katyoti patch of go-webview2 v1.0.22 - see PATCH.md at the module root.
var ExtraBrowserArgs []string

// browserArguments joins a Chromium's own arguments and ExtraBrowserArgs.
func browserArguments(own []string) string {
	return strings.Join(append(append([]string{}, own...), ExtraBrowserArgs...), " ")
}
