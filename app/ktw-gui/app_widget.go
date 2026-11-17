// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"

	bridgeapi "github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
)

// GetWidgetURL composes the iframe URL for a (kind, schemaName, value, mode)
// tuple in the named active universe. Reaches the universe's core via
// KTSP and routes to the displaybridge.SchemaBridge service, which (in
// Phase 1) forwards to bun's /api/widget-url handler.
//
// The returned URL has the per-session token already embedded as a
// `?token=<...>` query param (bun's auth gate for /widget/* routes
// expects it there — iframes can't set custom headers).
//
// Wails-bound: window.go.main.App.GetWidgetURL(universeName, kind,
// schemaName, value, mode). Errors propagate as Promise rejections in TS.
//
// Required: universeName + kind. schemaName / value / mode are optional
// (empty strings are dropped before being forwarded to bun).
func (a *App) GetWidgetURL(universeName, kind, schemaName, value, mode string) (string, error) {
	if kind == "" {
		return "", fmt.Errorf("kind is required")
	}
	sender, err := a.universeSender(universeName)
	if err != nil {
		return "", err
	}
	in := bridgeapi.GetWidgetURLInput{
		Kind:       kind,
		SchemaName: schemaName,
		Value:      value,
		Mode:       mode,
	}
	// displaybridge is hosted by ktw-agents.
	respRaw, err := sender.Call(a.ctx, "ktw-agents:/displaybridge/get-widget-url", in)
	if err != nil {
		return "", fmt.Errorf("KTSP displaybridge/get-widget-url: %w", err)
	}
	var out bridgeapi.WidgetURLOutput
	if err := json.Unmarshal(respRaw, &out); err != nil {
		return "", fmt.Errorf("decode displaybridge response: %w", err)
	}
	return out.URL, nil
}
