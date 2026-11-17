// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/api"
)

// servedCWM serves alpha and beta, both holding the goal under its own
// title (alpha alone when single); a feature of module a is alpha's, of b
// beta's.
type servedCWM struct {
	cwmapi.CWMService
	single bool
}

func (c servedCWM) ObjectGet(_ context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	out := cwmapi.ObjectGetOutput{Object: cwmapi.Object{ID: in.ID, Type: FeatureType}}
	if !c.single {
		out.Project = map[string]string{"a": "alpha", "b": "beta"}[in.Module]
	}
	return out, nil
}

func (c servedCWM) ObjectSearch(context.Context, cwmapi.ObjectSearchInput) (cwmapi.ObjectSearchOutput, error) {
	if c.single {
		return cwmapi.ObjectSearchOutput{Objects: []cwmapi.Object{{ID: "g.G-1", Type: "goal", Name: "alpha goal"}}}, nil
	}
	return cwmapi.ObjectSearchOutput{
		Objects:  []cwmapi.Object{{ID: "g.G-1", Type: "goal", Name: "alpha goal"}, {ID: "g.G-1", Type: "goal", Name: "beta goal"}},
		Projects: []string{"alpha", "beta"},
	}, nil
}

func (servedCWM) ObjectTypeGet(context.Context, cwmapi.ObjectTypeGetInput) (cwmapi.ObjectTypeGetOutput, error) {
	return cwmapi.ObjectTypeGetOutput{}, nil
}

// A FEATURE NAMES THE GOAL OF ITS OWN PROJECT. A serves
// edge answers its goal bare; with two projects holding the goal's id, a
// feature reads its own project's title - never the first or the last
// project's. One project: unchanged.
func TestServedGoalsAreNamedInTheFeaturesProject(t *testing.T) {
	for _, c := range []struct {
		name, holder string
		cwm          servedCWM
		want         string
	}{
		{"two projects, alpha's feature", "a.F-1", servedCWM{}, "alpha goal"},
		{"two projects, beta's feature", "b.F-1", servedCWM{}, "beta goal"},
		{"one project", "a.F-1", servedCWM{single: true}, "alpha goal"},
	} {
		features := []api.Item{{ID: c.holder, Serves: []api.Ref{{ID: "g.G-1"}}}}
		nameServed(context.Background(), c.cwm, c.holder, features, []string{"g.G-1"}, map[string]string{}, "en")
		if got := features[0].Serves[0].Title; got != c.want {
			t.Errorf("%s: served goal titled %q, want %q", c.name, got, c.want)
		}
	}
}
