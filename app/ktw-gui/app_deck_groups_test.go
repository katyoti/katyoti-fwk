// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// « Insérer un groupe de slides » lists the project's groups first, then the
// sidecars', each kept in name order, with what the preview needs.
func TestSlideGroupInfosListTheProjectFirst(t *testing.T) {
	g := func(provider, name string) mbapi.Attributed[deckapi.SlideGroupBody] {
		return mbapi.Attributed[deckapi.SlideGroupBody]{Provider: provider, Body: deckapi.SlideGroupBody{Name: name, Description: name + " d", Slides: "## " + name}}
	}
	got := slideGroupInfos([]mbapi.Attributed[deckapi.SlideGroupBody]{
		g("ktw-docs", "a-sidecar"), g(deckapi.ProviderProject, "b-project"), g("ktw-docs", "c-sidecar"), g(deckapi.ProviderProject, "d-project"),
	})
	want := []string{"b-project", "d-project", "a-sidecar", "c-sidecar"}
	for i, w := range want {
		if got[i].Name != w {
			t.Fatalf("order: got %+v, want %v", got, want)
		}
	}
	if got[0].Description != "b-project d" || got[0].Slides != "## b-project" || got[0].Provider != deckapi.ProviderProject {
		t.Errorf("info: %+v", got[0])
	}
}
