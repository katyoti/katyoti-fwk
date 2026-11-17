// SPDX-License-Identifier: Apache-2.0

// Package cwmembed exposes ktw-agents' CWM type pack — its object types,
// edge types, capabilities, object panels, panel components, hook types,
// event types and the data-types they name — as an embed.FS, published by
// the binary like any sidecar's pack.
//
// The directives below MUST stay aligned with the on-disk layout of this
// directory. If you rename or remove a folder, update this file too.
package cwmembed

import "embed"

// FS holds the pack's folders, one declaration per file; the data-types
// become URIs like `ktw-agents:/data-type/<name>`.
//
// The directive is an explicit ALLOW-LIST, so a new folder here is INVISIBLE
// until it is named — that is how `capabilities/` first shipped as files nothing
// published, the same "declared but not delivered" shape the census exists to
// catch. It also refuses to build on a named folder that is empty or holds only
// dotfiles, so a folder is added here when it has content, not before.
//
//go:embed edge-types object-types data-types capabilities object-panels panel-components hook-types event-types
var FS embed.FS
