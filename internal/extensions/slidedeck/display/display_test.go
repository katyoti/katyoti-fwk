// SPDX-License-Identifier: Apache-2.0

package display

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// A fake display server theme store: what PublishTheme talks to.
type fakeThemeStore struct {
	mu      sync.Mutex
	held    map[string]string // name → version
	posts   int
	lastRaw map[string]any
	token   string
}

func (f *fakeThemeStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Query().Get("token") != f.token {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/widget/deck/theme":
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.posts++
		f.lastRaw = in
		f.held[in["name"].(string)] = in["version"].(string)
		_ = json.NewEncoder(w).Encode(map[string]string{"name": in["name"].(string), "version": in["version"].(string)})
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/theme.json"):
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/widget/deck/theme/"), "/theme.json")
		v, ok := f.held[name]
		if !ok {
			http.Error(w, `{"error":"not published"}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": name, "version": v, "body": map[string]any{}})
	default:
		http.NotFound(w, r)
	}
}

func TestPublishTheme(t *testing.T) {
	store := &fakeThemeStore{held: map[string]string{}, token: "tok"}
	srv := httptest.NewServer(store)
	defer srv.Close()
	theme := deckapi.ResolvedTheme{
		Body:   deckapi.ThemeBody{Name: "demo", Description: "Demo."},
		Assets: map[string][]byte{"logo.png": {0x89, 'P', 'N', 'G', '\r', '\n'}},
	}

	v, err := PublishTheme(context.Background(), srv.Client(), srv.URL, "tok", theme)
	if err != nil || v != theme.Version() || store.posts != 1 {
		t.Fatalf("first publish: version=%q err=%v posts=%d", v, err, store.posts)
	}
	// The files travel as base64, byte for byte (a PNG's CR LF included).
	got, _ := base64.StdEncoding.DecodeString(store.lastRaw["assets"].(map[string]any)["logo.png"].(string))
	if string(got) != string(theme.Assets["logo.png"]) {
		t.Fatalf("asset bytes changed on the wire: %q", got)
	}

	// The store holds that version: nothing is sent again.
	if _, err := PublishTheme(context.Background(), srv.Client(), srv.URL, "tok", theme); err != nil || store.posts != 1 {
		t.Fatalf("an unchanged theme was re-sent: err=%v posts=%d", err, store.posts)
	}

	// A changed theme is sent.
	theme.Assets["logo.png"] = []byte("new")
	if _, err := PublishTheme(context.Background(), srv.Client(), srv.URL, "tok", theme); err != nil || store.posts != 2 {
		t.Fatalf("a changed theme was not sent: err=%v posts=%d", err, store.posts)
	}

	// A refusal comes back as an error naming the store's reason.
	if _, err := PublishTheme(context.Background(), srv.Client(), srv.URL, "wrong", theme); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want a 401 error, got %v", err)
	}
}
