// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// countingScheduler counts the scheduler's listings of every to-do.
type countingScheduler struct {
	schedapi.TodoService
	all atomic.Int64
}

func (c *countingScheduler) List(ctx context.Context, in schedapi.ListInput) (schedapi.TodosOutput, error) {
	if reflect.DeepEqual(in, schedapi.ListInput{}) {
		c.all.Add(1)
	}
	return c.TodoService.List(ctx, in)
}

// « À FAIRE » LISTS THE SCHEDULER'S TO-DOS ONCE: its
// own items, the demands and the open intake questions are read from one
// listing of every to-do for the whole open - read again for each, the view
// spent most of its second under its one-second budget re-reading the to-do
// folders. The inbox's waiting list reads its own.
func TestÀFaireListsTheSchedulersToDosOnce(t *testing.T) {
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson})
	toReview(t, ctx, r.ID)
	if _, err := req.Ask(ctx, api.AskInput{ID: file(t, ctx, req, api.FileInput{Title: "Mail invoices", Module: "billing"}).ID, Question: "Paper or mail?", By: intake}); err != nil {
		t.Fatal(err)
	}
	app := application.AppFromContext(ctx)
	counted := &countingScheduler{TodoService: schedOf(t, ctx)}
	application.PutService[schedapi.TodoService](app, "default", schedapi.TodoService(counted))
	if _, err := views(t, ctx).PersonQueuePage(ctx, api.PersonQueueInput{}); err != nil {
		t.Fatal(err)
	}
	if n := counted.all.Load(); n != 2 {
		t.Errorf("one open of « À faire » listed every to-do %d times; want 2 (planification's one listing, the inbox's waiting list)", n)
	}
}
