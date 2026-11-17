// SPDX-License-Identifier: Apache-2.0

package main

import "embed"

// Sidecar-local embedded assets. This binary OWNS all ktw-gocode pack
// content: personas / skills / tasks (relocated here from
// internal/extensions/gocode/local/embed 2026-06-18,
// migrate-pack-content-to-sidecars), the Go-specific CWM type pack
// (embed/cwm — code-symbol kind, relocated 2026-06-17), and the
// sidecar default config files. gocodelocal.Extension() now ships only
// BEHAVIOR (services + LanguagePack), no content.
//
//go:embed embed/config embed/cwm embed/personas embed/skills embed/tasks
var embeddedFS embed.FS
