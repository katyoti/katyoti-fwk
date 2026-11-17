// SPDX-License-Identifier: Apache-2.0

package render

// displayWidth is the printed width of s on a terminal: in the published build
// (no lipgloss) its runes with ANSI escapes stripped, one column each.
func displayWidth(s string) int { return runeWidth(s) }
