// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A request comes from any source: a person, an agent, a module, a feature, a
// goal — each an object that exists, of that type — or an external system named
// by its id. Anything else is refused.
func TestARequestComesFromAnySource(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	g := want(t, ctx, cwm, "goal", "Invoices paid on time")
	f := want(t, ctx, cwm, "feature", "Send by mail")

	for name, by := range map[string]cwmapi.Actor{
		"feature":  {ID: f, Kind: api.RequesterFeature},
		"goal":     {ID: g, Kind: api.RequesterGoal},
		"external": {ID: "jira:BILL-42", Kind: api.RequesterExternal},
		"module":   {ID: "billing", Kind: api.RequesterModule},
		"person":   person,
	} {
		out, err := req.File(ctx, api.FileInput{Title: "Asked by a " + name, Module: "billing", By: by})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if out.Request.Requester != by {
			t.Errorf("%s: requester %+v, want %+v", name, out.Request.Requester, by)
		}
	}

	for name, by := range map[string]cwmapi.Actor{
		"a goal as a feature":   {ID: g, Kind: api.RequesterFeature},
		"a missing feature":     {ID: "billing.F-999-nothing", Kind: api.RequesterFeature},
		"an unnamed system":     {Kind: api.RequesterExternal},
		"an unknown kind":       {ID: "x", Kind: "robot"},
		"a module that is none": {ID: f, Kind: api.RequesterModule},
	} {
		_, err := req.File(ctx, api.FileInput{Title: "Refused: " + name, Module: "billing", By: by})
		if err == nil {
			t.Errorf("%s: filed, want refused", name)
		} else if name == "a goal as a feature" || name == "an unnamed system" || name == "an unknown kind" || name == "a module that is none" {
			if !errors.Is(err, cwmapi.ErrInvalidInput) {
				t.Errorf("%s: %v, want an invalid input", name, err)
			}
		}
	}
}
