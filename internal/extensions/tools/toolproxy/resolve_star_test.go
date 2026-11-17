// SPDX-License-Identifier: Apache-2.0

package toolproxy

import (
	"context"
	"sort"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// `tools: ["*"]` is documented as "every advertised tool" and was dropped as
// unparseable until 2026-09-22: it expands over every member the catalog
// holds, advertised methods only, internal ones excluded.
func TestResolveTools_BareStarIsEveryAdvertisedToolOfEveryMember(t *testing.T) {
	cat := fakeCatalog{m: map[string]*mbapi.Manifest{
		"ktw-weather": {Provider: "ktw-weather", SlotTypes: []mbapi.SlotType{{Type: "WeatherService", Methods: []mbapi.SlotMethod{
			{Name: "GetCurrent"}, {Name: "GetForecast"}, {Name: "Reload", Tags: []string{"internal"}},
		}}}},
		"ktw-notify": {Provider: "ktw-notify", SlotTypes: []mbapi.SlotType{{Type: "Notify", Methods: []mbapi.SlotMethod{{Name: "Speak"}}}}},
	}}
	got := resolveTools(context.Background(), cat, []string{"*"})
	uris := make([]string, 0, len(got))
	for _, m := range got {
		uris = append(uris, m.uri.String())
	}
	sort.Strings(uris)
	want := []string{"ktw-notify:notify/speak", "ktw-weather:weather-service/get-current", "ktw-weather:weather-service/get-forecast"}
	if len(uris) != len(want) {
		t.Fatalf("star resolved %v, want %v", uris, want)
	}
	for i := range want {
		if uris[i] != want[i] {
			t.Fatalf("star resolved %v, want %v", uris, want)
		}
	}
	// No catalog: nothing, and no panic.
	if got := resolveTools(context.Background(), nil, []string{"*"}); len(got) != 0 {
		t.Errorf("without a catalog `*` must resolve to nothing, got %d", len(got))
	}
}
