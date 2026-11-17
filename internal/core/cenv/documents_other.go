// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package cenv

// knownDocumentsDir: no known folder outside Windows.
func knownDocumentsDir(string) string { return "" }
