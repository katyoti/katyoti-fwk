// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// TestTheShippedEventTypesAreCWMsOwn: ktw-agents ships one event-type file per
// built-in event type, saying what the code does (its point, its payload, not
// published), each with its French texts.
func TestTheShippedEventTypesAreCWMsOwn(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "..", "app", "ktw-agents", "embed", "cwm", "event-types")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	shipped := map[string]api.EventType{}
	translated := map[string]bool{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".i18n.yaml") {
			translated[strings.TrimSuffix(e.Name(), ".i18n.yaml")] = true
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var et api.EventType
		if err := config.DecodeBytes(raw, marshal.YAML, &et); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if et.Label == "" || et.Description == "" {
			t.Errorf("%s: a shipped event type says what it is (label, description)", e.Name())
		}
		shipped[et.Name] = et
	}
	if len(shipped) != len(api.BuiltinEventTypes) {
		t.Errorf("shipped %d event types, the code has %d", len(shipped), len(api.BuiltinEventTypes))
	}
	for _, b := range api.BuiltinEventTypes {
		s, ok := shipped[b.Name]
		if !ok || s.Point != b.Point || s.Payload != b.Payload || s.Published {
			t.Errorf("%s: shipped %+v, code %+v (and not published)", b.Name, s, b)
		}
		if !translated[b.Name] {
			t.Errorf("%s: no %s.i18n.yaml", b.Name, b.Name)
		}
	}
	// The pack loader reads them the same way the catalog publishes them.
	loaded, err := storage.LoadEventTypesLayered(os.DirFS(filepath.Dir(dir)))
	if err != nil || len(loaded) != len(api.BuiltinEventTypes) {
		t.Errorf("LoadEventTypesLayered = %d, %v", len(loaded), err)
	}
}
