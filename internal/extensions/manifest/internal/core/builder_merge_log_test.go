// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// A catalog rebuilds its manifest many times a start: the merge finding is
// warned in one line, once per process for the same URIs, and the copies of
// one provider registered twice are not warned at all (:
// one warning per published service at every build filled a 700 MB log).
func TestDuplicateProviderWarningIsLoggedOnce(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	svc := func(id string) contribapi.Contribution {
		c := contribapi.Contribution{Provider: "ktw-merge", Type: "service", ID: id}
		c.SetData(json.RawMessage(`{}`))
		return c
	}
	cfg := contribapi.Contribution{Provider: "ktw-merge", Type: "config-file", ID: "X.yaml"}
	providers := []contribapi.ContributionProvider{
		// One provider registered twice: every service comes twice.
		staticProvider{provider: "ktw-merge", typ: "service", entries: []contribapi.Contribution{svc("a"), svc("b")}},
		staticProvider{provider: "ktw-merge", typ: "service", entries: []contribapi.Contribution{svc("a"), svc("b")}},
		// Two providers publishing one URI: a real finding.
		staticProvider{provider: "ktw-merge", typ: "config-file", entries: []contribapi.Contribution{cfg}},
		staticProvider{provider: "ktw-merge", typ: "config-file-template", entries: []contribapi.Contribution{cfg}},
	}

	for range 3 {
		if _, err := NewBuilder("merge-log-test", "dev").BuildFromProviders(context.Background(), providers); err != nil {
			t.Fatal(err)
		}
	}

	out := logs.String()
	if n := strings.Count(out, "level=WARN"); n != 1 {
		t.Fatalf("%d warnings over three builds, want exactly one; log was:\n%s", n, out)
	}
	if !strings.Contains(out, "ktw-merge:/config-file/X.yaml") {
		t.Errorf("the warning does not name the URI two providers published; log was:\n%s", out)
	}
	if strings.Contains(out, "ktw-merge:/service/") {
		t.Errorf("the copies of one provider registered twice were warned; log was:\n%s", out)
	}
}
