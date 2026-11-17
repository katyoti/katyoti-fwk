// SPDX-License-Identifier: Apache-2.0

package contribution

import "embed"

// systemFS is the build-time embedded tree of system-scope
// contributions. Every binary that links against this package carries
// the same bytes — they're the bootstrap metadata each sidecar needs
// to be fully self-describing per contribution-meta-model.md §4.
//
// Layout (per the ADR's `embed/<type>/<id>.yaml` convention):
//
//	embed/contribution-type/<id>.yaml   → system:/contribution-type/<id>
//	embed/data-type/<id>.yaml           → system:/data-type/<id>
//	embed/service/<id>.yaml             → system:/service/<id>
//
// `<id>` is the file basename without `.yaml`. The system loader
// (system_loader.go) walks this tree at AppInit and emits one
// Contribution per file.
//
// Adding a new system contribution means: drop a YAML file in the
// right subdirectory, rebuild. No code change.
//
// Some data-types are GENERATED from the Go type consumers decode them
// with (datatypegen.Registry) — `system:/data-type/object-panel` from
// objectpaneapi.PanelDecl. Their files carry a header saying so; regenerate with
// `go generate ./internal/core/contribution/`, and
// TestGeneratedDataTypesAreCurrent refuses a stale one.
//
//go:generate go run ../../../tools/gen-data-type ./embed/data-type ../../../app/ktw-gui/frontend/src/generated
//go:embed embed
var systemFS embed.FS

// SystemFS returns the embedded system-contributions filesystem so
// callers (snapshot tests, tooling) can introspect what's bundled
// without touching the loader.
func SystemFS() embed.FS { return systemFS }
