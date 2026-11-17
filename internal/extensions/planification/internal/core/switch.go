// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// Pause stops planning: the planning task group — the planner's and its
// steps' — starts no run until resumed; what is submitted queues.
func (p *Plans) Pause(ctx context.Context, in api.PlanningSwitchInput) (api.PlanningSwitchOutput, error) {
	return p.switchPlanning(ctx, in, false)
}

// Resume starts planning again.
func (p *Plans) Resume(ctx context.Context, in api.PlanningSwitchInput) (api.PlanningSwitchOutput, error) {
	return p.switchPlanning(ctx, in, true)
}

func (p *Plans) switchPlanning(ctx context.Context, in api.PlanningSwitchInput, on bool) (api.PlanningSwitchOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.PlanningSwitchOutput{}, err
	}
	tasks, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return api.PlanningSwitchOutput{}, fmt.Errorf("the task service: %w", err)
	}
	g, err := tasks.SetGroup(ctx, taskapi.SetGroupRequest{Group: api.PlanGroup, On: &on})
	if err != nil {
		return api.PlanningSwitchOutput{}, err
	}
	return api.PlanningSwitchOutput{Group: api.PlanGroup, On: g.On}, nil
}
