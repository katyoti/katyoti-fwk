// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/challenges/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// homonymCWM serves alpha and beta (one project, alpha, when single): both
// hold the decision and the challenger; only alpha's has
// challengers - and, when missing, that no project holds.
type homonymCWM struct {
	cwmapi.CWMService
	single, missing bool
}

func (c homonymCWM) EdgeList(_ context.Context, in cwmapi.EdgeListInput) (cwmapi.EdgeListOutput, error) {
	edges := []cwmapi.Edge{{From: "d.OD-2", To: "d.OD-1", Kind: api.EdgeChallenges}}
	if c.missing {
		edges = append(edges, cwmapi.Edge{From: "d.OD-9", To: "d.OD-1", Kind: api.EdgeChallenges})
	}
	return cwmapi.EdgeListOutput{Edges: edges}, nil
}

func (c homonymCWM) ObjectGet(_ context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	out := cwmapi.ObjectGetOutput{Object: cwmapi.Object{ID: in.ID, Type: "decision"}}
	if !c.single {
		out.Project = "alpha"
	}
	return out, nil
}

func (c homonymCWM) ObjectSearch(context.Context, cwmapi.ObjectSearchInput) (cwmapi.ObjectSearchOutput, error) {
	if c.single {
		return cwmapi.ObjectSearchOutput{Objects: []cwmapi.Object{
			{ID: "d.OD-1", Type: "decision", Name: "alpha decision"},
			{ID: "d.OD-2", Type: "decision", Name: "alpha challenger"},
		}}, nil
	}
	return cwmapi.ObjectSearchOutput{
		Objects: []cwmapi.Object{
			{ID: "d.OD-1", Type: "decision", Name: "alpha decision"},
			{ID: "d.OD-2", Type: "decision", Name: "alpha challenger"},
			{ID: "d.OD-1", Type: "decision", Name: "beta decision"},
			{ID: "d.OD-2", Type: "decision", Name: "beta challenger"},
		},
		Projects: []string{"alpha", "alpha", "beta", "beta"},
	}, nil
}

func (homonymCWM) ObjectTypeGet(context.Context, cwmapi.ObjectTypeGetInput) (cwmapi.ObjectTypeGetOutput, error) {
	return cwmapi.ObjectTypeGetOutput{}, nil
}

// A DECISION LISTS ITS OWN PROJECT'S CHALLENGERS. The
// challenges edges CWM answers name their ends bare; with two projects
// holding the challenger's id, the Challenged by tab lists the challenger of
// the decision's project alone, its count right, and a challenger no project
// holds is counted unresolved - never a negative count hiding the badge. One
// project: unchanged.
func TestChallengersAreTheChallengedObjectsProjects(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name       string
		cwm        homonymCWM
		unresolved string
	}{
		{"two projects", homonymCWM{}, ""},
		{"two projects, one challenger missing", homonymCWM{missing: true}, "1 not in this project"},
		{"one project", homonymCWM{single: true}, ""},
	} {
		def, err := challengers(ctx, c.cwm, api.ObjectInput{ID: "d.OD-1"})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		b, _ := json.Marshal(def.DSLTree)
		page := string(b)
		if !strings.Contains(page, "alpha challenger") || strings.Contains(page, "beta challenger") {
			t.Errorf("%s: want alpha's challenger alone: %s", c.name, page)
		}
		if !strings.Contains(page, `"badge":1`) {
			t.Errorf("%s: want a count of 1: %s", c.name, page)
		}
		if c.unresolved != "" && !strings.Contains(page, c.unresolved) {
			t.Errorf("%s: want %q: %s", c.name, c.unresolved, page)
		}
		if c.unresolved == "" && strings.Contains(page, "not in this project") {
			t.Errorf("%s: no challenger is unresolved: %s", c.name, page)
		}
	}
}
