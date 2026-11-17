// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A VIEW ACROSS PROJECTS READS A REQUEST'S TO-DOS:
// two served projects, alpha and beta, both hold. The CWM
// refuses the bare id as ambiguous; each qualified one names its project.

const (
	refAlphaRepo = "0193a0d3-a5ed-71e3-9f71-b810b0466a01"
	refBetaRepo  = "0193a0d3-a5ed-71e3-9f71-c921c1577b02"
)

// twoProjectsCWM answers as the store set does with alpha and beta served,
// counting each ObjectResolve.
type twoProjectsCWM struct {
	cwmapi.CWMService
	resolves int
}

func (c *twoProjectsCWM) ObjectGet(_ context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	key, bare, qualified := strings.Cut(in.ID, ":")
	if !qualified {
		return cwmapi.ObjectGetOutput{}, fmt.Errorf("%w: object %q is in several projects (alpha, beta): name the project - write one of alpha:%s, beta:%s", cwmapi.ErrInvalidInput, in.ID, in.ID, in.ID)
	}
	return cwmapi.ObjectGetOutput{Object: cwmapi.Object{ID: bare, Type: "request"}, Project: key}, nil
}

func (c *twoProjectsCWM) ObjectResolve(_ context.Context, in cwmapi.ObjectResolveInput) (cwmapi.ObjectResolveOutput, error) {
	c.resolves++
	key, _, _ := strings.Cut(in.Ref, ":")
	repo := map[string]string{"alpha": refAlphaRepo, "beta": refBetaRepo}[key]
	if repo == "" {
		return cwmapi.ObjectResolveOutput{}, cwmapi.ErrNotFound
	}
	return cwmapi.ObjectResolveOutput{ProjectID: repo, Project: key}, nil
}

// oneListScheduler lists the same to-dos: one per project of
// and one with no Repository.
type oneListScheduler struct{ schedapi.TodoService }

func (oneListScheduler) List(context.Context, schedapi.ListInput) (schedapi.TodosOutput, error) {
	row := func(id, repo string) schedapi.Todo {
		return schedapi.Todo{ID: id, Module: "billing", Request: "billing.RQ-001", Repository: repo, Owner: api.Owner}
	}
	return schedapi.TodosOutput{Todos: []schedapi.Todo{row("billing.TD-a", refAlphaRepo), row("billing.TD-b", refBetaRepo), row("billing.TD-none", "")}}, nil
}

func TestAViewNamesAnAmbiguousBareRequestAndResolvesEachKeyOnce(t *testing.T) {
	cwm := &twoProjectsCWM{}
	app := application.New(application.Config())
	application.PutService[cwmapi.CWMService](app, "default", cwm)
	application.PutService[schedapi.TodoService](app, "default", oneListScheduler{})
	ctx := withReadCache(application.WithApplication(context.Background(), app))

	// The bare id two projects hold: the ambiguity named, no row of either.
	got, err := todosOf(ctx, "billing.RQ-001")
	if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "several projects") || len(got) != 0 {
		t.Fatalf("bare billing.RQ-001: %d rows, error %v - want the ambiguity named and no row", len(got), err)
	}

	// A qualified id, read many times: its project's row only, its key
	// resolved once for the view.
	for range 5 {
		got, err = todosOf(ctx, "alpha:billing.RQ-001")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "billing.TD-a" {
			t.Fatalf("alpha:billing.RQ-001 kept %v - want alpha's row only (no beta's, no row without a Repository)", ids(got))
		}
	}
	if cwm.resolves != 1 {
		t.Errorf("alpha's key resolved %d times for five reads - want once", cwm.resolves)
	}
	if got, _ = todosOf(ctx, "beta:billing.RQ-001"); len(got) != 1 || got[0].ID != "billing.TD-b" {
		t.Errorf("beta:billing.RQ-001 kept %v - want beta's row only", ids(got))
	}
}

func ids(todos []api.WorkTodo) []string {
	var out []string
	for _, t := range todos {
		out = append(out, t.ID)
	}
	return out
}
