// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
)

// stubBun returns an httptest.Server backed by the provided handler and a
// schemaBridge wired to that server. Callers register routes on the
// http.ServeMux passed to handler.
func stubBun(t *testing.T, handler func(mux *http.ServeMux)) (*schemaBridge, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	handler(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	b := newWithEndpoint(
		NewStaticEndpoint(srv.URL, "test-token"),
		withMaxRetry(0),
		withBackoff(1*time.Millisecond),
	)
	return b, srv
}

// expectedAuth asserts the test-token Bearer header was sent.
func expectedAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if got, want := r.Header.Get("Authorization"), "Bearer test-token"; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

// REMOVED 2026-05-22 (schema-realign):
//   - TestParse_*, TestParseFile_*, TestValidate_* (Round 1)
//   - TestGetSchemaAST_*, TestInstallSchema_*, TestListTypes_*,
//     TestGetTypeManifest_*, TestGetTSXFile_*, TestInstallContentType_*,
//     TestGenerateJSONSchema_*, TestGenerateTypeScript_* (Round 2)
//
// All exercised Phase-1 stub forwarders that never had a bun backend and
// had zero live callers. made the Go
// side canonical; the forwarders + their tests + their I/O types were
// removed in two rounds of cleanup. Only GetWidgetURL remains live.

func TestGetWidgetURL_ForwardsToAPIWidgetURL(t *testing.T) {
	b, _ := stubBun(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/widget-url", func(w http.ResponseWriter, r *http.Request) {
			var in api.GetWidgetURLInput
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.Kind != "form" {
				t.Errorf("kind = %q, want form", in.Kind)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(api.WidgetURLOutput{
				URL:   "http://127.0.0.1/widget/form?token=xyz",
				Token: "xyz",
			})
		})
	})
	out, err := b.GetWidgetURL(context.Background(), api.GetWidgetURLInput{Kind: "form"})
	if err != nil {
		t.Fatalf("GetWidgetURL: %v", err)
	}
	if out.Token != "xyz" {
		t.Errorf("Token = %q, want xyz", out.Token)
	}
	if !strings.Contains(out.URL, "?token=xyz") {
		t.Errorf("URL = %q, want widget URL with ?token=", out.URL)
	}
}

// TestSlot_501SurfacesAsNotImplemented — REMOVED 2026-05-22 (schema-realign).
// Exercised the Phase-1 501-stub pattern through Parse, which is gone.
// 501 surfacing is still wired in client.go; if a new bun endpoint ever
// returns 501 it'll be tested through that endpoint's slot.

// Real-bun integration tests live in integration_test.go (skipped unless
// bun is on PATH AND KTW_DISPLAYBRIDGE_INTEGRATION=1). They cover the full
// path: spawn → bridge call → SSE subscribe → cache invalidation.
