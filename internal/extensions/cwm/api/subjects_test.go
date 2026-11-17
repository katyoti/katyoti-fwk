// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
	"testing"
)

// TestSubjectBuilders_HappyPath asserts every CWM subject builder
// produces the exact wire format documented in
// modules/bus/nats_subjects_list.md § 8.
func TestSubjectBuilders_HappyPath(t *testing.T) {
	const (
		tenant    = "local"
		universe  = "default"
		workspace = "workspace-uuid"
		token     = "tok-1"
		ssID      = "ss-1"
	)
	cases := map[string]struct {
		got  string
		want string
	}{
		"ObjectCreated": {
			ObjectCreatedSubject(tenant, universe, workspace, token),
			"ktw.local.universe.default.workspace.workspace-uuid.object.tok-1.created",
		},
		"ObjectUpdated": {
			ObjectUpdatedSubject(tenant, universe, workspace, token),
			"ktw.local.universe.default.workspace.workspace-uuid.object.tok-1.updated",
		},
		"ObjectDeleted": {
			ObjectDeletedSubject(tenant, universe, workspace, token),
			"ktw.local.universe.default.workspace.workspace-uuid.object.tok-1.deleted",
		},
		"ProjectCaughtUp": {
			ProjectCaughtUpSubject(tenant, universe, workspace),
			"ktw.local.universe.default.workspace.workspace-uuid.project.caught-up",
		},
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", name, c.got, c.want)
		}
	}
}

// TestSubjectBuilders_PanicsOnInvalidToken verifies the charset
// enforcement — any token containing uppercase, dots, or other
// disallowed characters panics rather than producing a broken
// subject silently.
func TestSubjectBuilders_PanicsOnInvalidToken(t *testing.T) {
	cases := []struct {
		name string
		fn   func()
	}{
		{"tenant uppercase", func() { ObjectCreatedSubject("Local", "u", "w", "n") }},
		{"universe contains dot", func() { ObjectCreatedSubject("t", "u.s", "w", "n") }},
		{"workspace contains slash", func() { ObjectCreatedSubject("t", "u", "w/x", "n") }},
		{"token empty", func() { ObjectCreatedSubject("t", "u", "w", "") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Error("expected panic on invalid token, got none")
					return
				}
				msg, _ := r.(string)
				if !strings.Contains(msg, "invalid") {
					t.Errorf("unexpected panic message: %v", r)
				}
			}()
			c.fn()
		})
	}
}
