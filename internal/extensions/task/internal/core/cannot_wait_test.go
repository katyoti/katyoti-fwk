// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// refusingEvents cannot subscribe anything.
type refusingEvents struct{ eventsapi.EventService }

func (refusingEvents) Subscribe(context.Context, string, string, eventsapi.Handler) (eventsapi.Subscription, error) {
	return nil, errors.New("the bus is down")
}

// A run that waits on another process's event it can never be told — no event
// service, or a subscription refused — fails, and says why. (It waited for
// ever, only a log line saying so.)
func TestARunThatCannotHearItsEventFailsWithWhy(t *testing.T) {
	for name, events := range map[string]func(context.Context) (eventsapi.EventService, error){
		"no event service": func(context.Context) (eventsapi.EventService, error) {
			return nil, errors.New("not registered")
		},
		"could not be subscribed": func(context.Context) (eventsapi.EventService, error) {
			return refusingEvents{}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			old := eventServiceOf
			eventServiceOf = events
			t.Cleanup(func() { eventServiceOf = old })
			svc := groupsService(t, taskapi.TaskGroupsConfig{})
			_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "extwaiter"}, Impl: extWaiterFactory{}})
			svc.tasks["extwaiter"] = &taskapi.Task{Name: "extwaiter", Provider: "test", Type: "extwaiter"}

			r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "extwaiter"})
			if err != nil {
				t.Fatal(err)
			}
			eventually(t, "failed", func() bool { return runInfo(t, svc, r.RunID).Status == taskapi.RunStatusFailed })
			if e := runInfo(t, svc, r.RunID).Error; !strings.Contains(e, name) {
				t.Errorf("error = %q; want it to say %q", e, name)
			}
		})
	}
}
