// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A WAIT REPLACED IS NEVER ABSENT: the same move
// refused twice by two actors (an agent's verify, planification's own verify
// of a realized feature whose criteria pass) replaces the wait the first
// refusal wrote. A reader between any two writes of that replacement still
// finds the object waiting - taken off and given again, it found none.

// oneObjectCWM keeps one object in memory, with a revision per write; after
// each write it notes whether the object carries a gate wait. conflicts
// refuses that many conditional updates first, as a concurrent write would.
type oneObjectCWM struct {
	cwmapi.CWMService
	obj       cwmapi.Object
	rev       int
	conflicts int
	unwaited  int
	writes    int
}

func (c *oneObjectCWM) note() {
	c.writes++
	if _, ok := c.obj.Capabilities[api.CapGateWait]; !ok {
		c.unwaited++
	}
}

func (c *oneObjectCWM) ObjectGet(_ context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	if in.ID != c.obj.ID {
		return cwmapi.ObjectGetOutput{}, cwmapi.ErrNotFound
	}
	return cwmapi.ObjectGetOutput{Object: c.copy(), Revision: fmt.Sprint(c.rev)}, nil
}

func (c *oneObjectCWM) ObjectUpdate(_ context.Context, in cwmapi.ObjectUpdateInput) (cwmapi.ObjectUpdateOutput, error) {
	if in.Revision != "" && in.Revision != fmt.Sprint(c.rev) {
		return cwmapi.ObjectUpdateOutput{}, cwmapi.ErrConflict
	}
	if c.conflicts > 0 {
		c.conflicts--
		c.rev++ // someone else wrote meanwhile
		return cwmapi.ObjectUpdateOutput{}, cwmapi.ErrConflict
	}
	c.obj, c.rev = in.Object, c.rev+1
	c.note()
	return cwmapi.ObjectUpdateOutput{Object: c.copy()}, nil
}

func (c *oneObjectCWM) ObjectAddCapability(_ context.Context, in cwmapi.ObjectAddCapabilityInput) (cwmapi.ObjectAddCapabilityOutput, error) {
	o := c.copy()
	if err := o.SetCapability(in.Capability, in.Fields); err != nil {
		return cwmapi.ObjectAddCapabilityOutput{}, err
	}
	c.obj, c.rev = o, c.rev+1
	c.note()
	return cwmapi.ObjectAddCapabilityOutput{Object: c.copy()}, nil
}

func (c *oneObjectCWM) ObjectRemoveCapability(_ context.Context, in cwmapi.ObjectRemoveCapabilityInput) (cwmapi.ObjectRemoveCapabilityOutput, error) {
	o := c.copy()
	delete(o.Capabilities, in.Capability)
	c.obj, c.rev = o, c.rev+1
	c.note()
	return cwmapi.ObjectRemoveCapabilityOutput{Object: c.copy()}, nil
}

func (c *oneObjectCWM) copy() cwmapi.Object {
	o := c.obj
	o.Capabilities = map[string]map[string]any{}
	for k, v := range c.obj.Capabilities {
		o.Capabilities[k] = v
	}
	return o
}

func TestAWaitReplacedByASecondRefusalIsNeverAbsent(t *testing.T) {
	ctx := context.Background()
	agent := cwmapi.Actor{ID: "an-agent", Kind: "agent"}
	first := api.GateWaitFields{Gate: "gui-tested", Transition: "verify", To: "done", By: agent, Since: "2026-11-09T10:00:00Z"}
	for _, conflicts := range []int{0, 2} {
		c := &oneObjectCWM{obj: cwmapi.Object{ID: "billing.F-001", Type: "feature"}, conflicts: 0}
		if err := saveGateWait(ctx, c, c.obj, first, false); err != nil {
			t.Fatal(err)
		}
		c.conflicts, c.unwaited, c.writes = conflicts, 0, 0

		second := first
		second.By = PlanningActor
		if err := saveGateWait(ctx, c, c.obj, second, true); err != nil {
			t.Fatalf("%d conflicts: replacing the wait: %v", conflicts, err)
		}
		if c.unwaited > 0 {
			t.Errorf("%d conflicts: %d of the %d writes replacing the wait left billing.F-001 carrying no wait; want it waiting after every write", conflicts, c.unwaited, c.writes)
		}
		got, ok := gateWaitOf(c.obj)
		if !ok || got.By != PlanningActor || got.Since != first.Since {
			t.Errorf("%d conflicts: the wait after its replacement is %+v (carried %v); want it by %v since %s", conflicts, got, ok, PlanningActor, first.Since)
		}
	}
}
