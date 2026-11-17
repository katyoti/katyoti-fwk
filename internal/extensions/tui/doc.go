// SPDX-License-Identifier: Apache-2.0

// Package tui is the terminal UI of ktw-agents: the Bubbletea views (chat,
// form, task picker, multi-pane console), their styles and the Glamour
// markdown rendering. They live in the private build only (files tagged
// !oss); the published build (tag oss) keeps the package's API in oss.go,
// where IsTerminal is false and the interactive entry points answer
// ErrNotInBuild, so callers take their plain output path.
package tui

import "errors"

// ErrNotInBuild is what the terminal UI's entry points answer in a build
// that does not carry it (the published build, tag oss).
var ErrNotInBuild = errors.New("the terminal UI is not part of this build")
