// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// A hand-over runs only the stages it names, in the case's order - the
// installer and the GUI, then the machine is the person's (:
// make sandbox-release / sandbox-current); a stage the case lacks is refused,
// and with no name every stage runs.
func TestOnlyRunsTheNamedStagesInTheCasesOrder(t *testing.T) {
	m := &api.CaseMachine{Stages: []api.MachineStage{{ID: "m1-install", Install: true}, {ID: "m2-default-universe"}, {ID: "m5-keys"}, {ID: "m8-gui"}}}
	got, err := stagesOf(m, []string{"m8-gui", "m1-install"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "m1-install,m8-gui" {
		t.Errorf("only m8-gui and m1-install: %v; want them in the case's order", ids)
	}
	if _, err := stagesOf(m, []string{"m1-install", "m9-nothing"}); err == nil || !strings.Contains(err.Error(), "m9-nothing") {
		t.Errorf("a stage the case lacks: %v; want it refused, named", err)
	}
	if all, _ := stagesOf(m, nil); len(all) != 4 {
		t.Errorf("no name: %d stages; want every one", len(all))
	}
}
