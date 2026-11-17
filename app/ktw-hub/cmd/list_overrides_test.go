// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// The hub indexes what override trees served: ListOverrides gathers the
// Overrides of every cached manifest, from one tree when asked.
func TestTheHubListsTheOverridesOfItsMembers(t *testing.T) {
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{
		{Provider: "ktw-compose", Overrides: []contribapi.Override{
			{URI: "ktw-compose:/skill/greet", Layer: "source", From: "project:acme"},
			{URI: "ktw-compose:/skill/greet", Layer: "translations", From: "universe"},
		}},
		{Provider: "ktw-docs", Overrides: []contribapi.Override{
			{URI: "ktw-docs:/guide-recipe/tour", Layer: "translations", From: "project:acme"},
		}},
		{Provider: "ktw-plain"},
	}, nil)
	s := &catalogServiceImpl{cat: cat}
	all, err := s.ListOverrides(context.Background(), mbapi.ListOverridesInput{})
	if err != nil || len(all.Overrides) != 3 {
		t.Fatalf("all = %+v, %v; want the three", all.Overrides, err)
	}
	mine, err := s.ListOverrides(context.Background(), mbapi.ListOverridesInput{From: "project:acme"})
	if err != nil || len(mine.Overrides) != 2 {
		t.Fatalf("project:acme's = %+v, %v; want two", mine.Overrides, err)
	}
	for _, o := range mine.Overrides {
		if o.From != "project:acme" {
			t.Errorf("another tree's override: %+v", o)
		}
	}
}
