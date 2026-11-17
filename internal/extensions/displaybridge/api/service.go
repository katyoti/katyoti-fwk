// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// SchemaBridge is the contract Go consumers use to reach the ktw-display
// bun sidecar for the one thing bun must own: iframe URL composition.
// bun knows its own port and mints the per-session auth token, so the
// URL has to be built there.
//
// **2026-05-22 schema-realign cleanup**: this package used to expose 13
// methods as "Phase-1 stubs of refined Architecture C" — forwarders that
// would proxy schema parsing, validation, content-type discovery, codegen,
// and value validation to bun. None of the bun handlers ever landed and
// every forwarder had zero live callers.
// (2026-05-22) made the Go side canonical for everything that wasn't
// inherently TS-side; the forwarders became unreachable dead code and were
// removed. The only legitimate bridge concern that remains is URL+token
// composition, which is a bun responsibility.
//
// **Phase-2 plan is obsolete**: the original design progressed from
// HTTP-bridge (this) → native KTSP. With Go owning the schema spine
// there's no need for native KTSP either — direct in-process calls to
// `application.UseService[SchemaService](ctx)` are the replacement.
type SchemaBridge interface {
	// GetWidgetURL returns the iframe URL (with per-session token embedded
	// as a `?token=<...>` query param) for a widget kind. bun handles
	// composition because it knows its own port + mints the token.
	GetWidgetURL(ctx context.Context, in GetWidgetURLInput) (WidgetURLOutput, error)
}

// SchemaBridgeServiceDef describes the SchemaBridge for auto-publication
// (KTSP / HTTP / Cobra CLI generation).
//
// Slot tags: cli="false" suppresses auto-CLI generation; this is purely a
// bus surface called by ktw-gui's `App.GetWidgetURL` Wails binding.
var SchemaBridgeServiceDef = meta.ServiceDef{
	Name:    "displaybridge",
	Short:   "Go-side bridge to the ktw-display bun sidecar — iframe URL composition only",
	Long:    `Thin bridge for the one bun-owned operation: iframe URL + per-session token composition. Every other former forwarder (Parse, Validate, ListTypes, …) was removed 2026-05-22 in the schema-realign cleanup because the framework's schema decision made Go canonical and the bun-side handlers never landed.`,
	Version: "0.2.0",
	Methods: map[string]meta.MethodDef{
		"GetWidgetURL": meta.MethodLong("Get iframe URL for a widget kind", `Forwards to bun's /api/widget-url handler. bun composes the URL because it owns the port + session token.`),
	},
}

// --- I/O types ---

// GetWidgetURLInput identifies a widget mount.
type GetWidgetURLInput struct {
	Kind       string `json:"kind"                  desc:"Component type the widget renders (the ktw-display route): form | view | table | tree-table | wizard | page | drawio" required:"true"`
	SchemaName string `json:"schemaName,omitempty"`
	Value      string `json:"value,omitempty"       desc:"Optional value reference (record id)"`
	Mode       string `json:"mode,omitempty"        desc:"view | create | update | filter"`
}

// WidgetURLOutput carries the iframe URL plus per-session token.
type WidgetURLOutput struct {
	URL       string `json:"url"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt,omitempty" desc:"Unix seconds; 0 = no expiration"`
}
