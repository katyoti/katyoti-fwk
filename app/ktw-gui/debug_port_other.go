// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

// openDebugPort does nothing outside Windows: only WebView2 is driven by its
// debugging port.
func openDebugPort() {}
