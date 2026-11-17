// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// TestParseTaskEventSubject_RoundTrip pins the subject parser against
// the canonical subject builder. If TaskEventSubject's format ever
// changes (e.g. an extra segment lands per a milestone work item), this
// test catches the off-by-one in the parser's length check and field
// indices.
//
// The bug this guards against: the parser previously expected 11
// tokens; TaskEventSubject produces 10. Every event subject failed
// silently, dropping all task output from chat panels.
func TestParseTaskEventSubject_RoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		tenant    string
		universe  string
		session   string
		taskID    string
		kind      string
		wantRunID string
		wantKind  string
	}{
		{"event", "local", "acme", "default", "abc123", "event", "abc123", "event"},
		{"done", "local", "acme", "default", "abc123", "done", "abc123", "done"},
		{"long taskID", "acme", "client-x", "s1", "deadbeef-feed-face", "event", "deadbeef-feed-face", "event"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			subject := subjects.TaskEventSubject(c.tenant, c.universe, c.session, c.taskID, c.kind)
			runID, kind, ok := parseTaskEventSubject(subject)
			if !ok {
				t.Fatalf("parseTaskEventSubject(%q) returned ok=false", subject)
			}
			if runID != c.wantRunID {
				t.Errorf("runID = %q, want %q", runID, c.wantRunID)
			}
			if kind != c.wantKind {
				t.Errorf("kind = %q, want %q", kind, c.wantKind)
			}
		})
	}
}

// TestParseTaskEventSubject_RejectsMalformed confirms the parser cleanly
// rejects subjects that don't match the canonical shape.
func TestParseTaskEventSubject_RejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"ktw.local.universe.acme", // too few tokens
		"ktw.local.universe.acme.session.default.task.abc.event.kind.extra", // too many tokens
		"NOT.local.universe.acme.session.default.task.abc.event.evt",        // wrong prefix
		"ktw.local.NOPE.acme.session.default.task.abc.event.evt",            // wrong fixed token
		"ktw.local.universe.acme.session.default.NOPE.abc.event.evt",        // 'task' replaced
		"ktw.local.universe.acme.session.default.task.abc.NOPE.evt",         // 'event' replaced
	}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			_, _, ok := parseTaskEventSubject(s)
			if ok {
				t.Errorf("parseTaskEventSubject(%q) = ok=true, want false", s)
			}
		})
	}
}
