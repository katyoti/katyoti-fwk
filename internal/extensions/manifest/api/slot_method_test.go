// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// oneManifest is a catalog holding one provider's manifest.
type oneManifest struct {
	api.Catalog
	man *api.Manifest
}

func (c oneManifest) Get(_ context.Context, name string) (*api.Manifest, error) {
	if c.man != nil && name == c.man.Provider {
		return c.man, nil
	}
	return nil, errors.New("not in the catalog")
}

// A method's request timeout is the one it declares, read from its provider's
// manifest by its path - kebab or PascalCase, normalised as the sender does
// (: the GUI's PascalCase conversion call was cut); one declaring none, or a path the catalog does
// not know, gets the transport's default (0) -: the GUI's
// doctor and plan-prereqs were cut at the transport's few seconds.
func TestMethodTimeoutIsTheDeclaredOne(t *testing.T) {
	cat := oneManifest{man: &api.Manifest{Provider: "ktw-univers", SlotTypes: []api.SlotType{{
		Type: "Marketplace",
		Methods: []api.SlotMethod{
			{Name: "Doctor", TimeoutS: 300},
			{Name: "PlanPrereqs", TimeoutS: 300},
			{Name: "Status"},
		},
	}}}}
	ctx := context.Background()
	for path, want := range map[string]time.Duration{
		"ktw-univers:marketplace/doctor":       300 * time.Second,
		"ktw-univers:marketplace/plan-prereqs": 300 * time.Second,
		"ktw-univers:marketplace/Doctor":       300 * time.Second,
		"ktw-univers:Marketplace/PlanPrereqs":  300 * time.Second,
		"ktw-univers:marketplace/status":       0,
		"ktw-univers:marketplace/unknown":      0,
		"ktw-other:marketplace/doctor":         0,
		"not a path":                           0,
	} {
		if got := api.MethodTimeout(ctx, cat.Get, path); got != want {
			t.Errorf("%s: %v; want %v", path, got, want)
		}
	}
	if api.MethodTimeout(ctx, nil, "ktw-univers:marketplace/doctor") != 0 {
		t.Error("no catalog: want the transport's default")
	}
}
