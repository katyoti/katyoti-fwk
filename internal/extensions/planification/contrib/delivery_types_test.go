// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The delivery data types parse with no error, carry
// their display, and a row of each - as its source answers it - validates
// against its own schema; a row of the wrong shape does not.
func TestDeliveryRowsValidateAgainstTheirDataTypes(t *testing.T) {
	yes, no := true, false
	left := 12
	card := api.ReleaseCard{ID: "billing.RL-001", Label: "1.1.0", Version: "1.1.0", Date: "2026-11-15", DaysLeft: &left, Stage: api.ReleasePlanned,
		Done: 1, Total: 3, Planned: 2, Next: true, Stream: "billing.SM-001",
		Gates: []api.GateDot{{Name: api.GateIncrements, Passed: &no, Why: "open: Print"}, {Name: api.GateDependencies, Passed: &yes}, {Name: api.GateSignOff, Why: "needs a candidate"}}}
	row := api.IncrementRow{ID: "billing.IN-001", Title: "Send invoices", Priority: "P1", Standing: api.StandingRunning, Progress: []int{2, 1, 1, 0, 0}, Waits: "awaits your review", NeedsYou: 1}
	rows := map[string]any{
		"stream-lane": api.StreamLane{ID: "billing.SM-001", Name: "Product", Mode: api.ModePeriodic, ModeText: "periodic every 14d - next cut 2026-10-31", NextCut: "2026-10-31",
			Releases: []api.ReleaseCard{card}, Backlog: []api.BacklogItem{{ID: "billing.IN-003", Title: "Archive", Priority: "P3", Held: true}}},
		"release-card": card,
		"release-rows": api.ReleaseRows{Release: card, Stepper: []api.StepChip{{Name: "planned", State: "current"}, {Name: "frozen", State: "todo"}},
			NextAction: "freeze it", Ships: []api.IncrementRow{row}, Planned: []api.IncrementRow{}, ShipPlanned: false},
		"increment-row": row,
		"increment-detail": api.IncrementDetail{Row: row, Assigned: "Ships in 1.1.0", Proposal: "approved and applied",
			Options: []api.ReleaseOption{{ID: "billing.RL-001", Label: "1.1.0", Mode: "ships-in"}, {Label: "Backlog (not assigned)", Mode: "backlog"}},
			Goals: []api.GoalLine{{Goal: "I send an invoice by mail", State: "running", Became: []api.HeldLine{{ID: "billing.F-001", Title: "Mail an invoice", State: "active",
				Requests: []api.RequestLine{{ID: "billing.F-001", Title: "Mail an invoice", State: "ready", Steps: []api.StepChip{{Name: "intake", State: "done"}, {Name: "implement", State: "running"}}, Where: "step implement - held by claude"}}}}}}},
	}
	req := api.RequestLine{ID: "billing.F-001", Title: "Mail an invoice", State: "ready", Steps: []api.StepChip{{Name: "intake", State: "done"}}, Where: "step implement"}
	held := api.HeldLine{ID: "billing.F-001", Title: "Mail an invoice", State: "active", Requests: []api.RequestLine{req}}
	rows["goal-line"] = api.GoalLine{Chip: "billing.IN-001·G1", Goal: "I send an invoice by mail", State: "running", Became: []api.HeldLine{held}}
	rows["held-line"] = held
	rows["request-line"] = req
	for _, src := range append(append([]datatypegen.Source{}, deliveryTypes...), drawerTypes...) {
		body, err := os.ReadFile("embed/cwm/data-types/" + src.Name + ".dsl.yaml")
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.Parse(context.Background(), schemaapi.ParseInput{Body: body})
		if err != nil || parsed.Schema == nil {
			t.Fatalf("%s does not parse: %v", src.Name, err)
		}
		for _, d := range parsed.Diagnostics {
			if d.Severity == schemaapi.SeverityError {
				t.Errorf("%s: %s %s at %s", src.Name, d.Code, d.Message, d.Path)
			}
		}
		if !bytes.Contains(body, []byte("type: ui-")) {
			t.Errorf("%s carries no ui-* extension: its display is declared once, with it", src.Name)
		}
		raw, err := parsed.Schema.ToJSONSchema()
		if err != nil {
			t.Fatalf("%s: %v", src.Name, err)
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(src.Name+".json", bytes.NewReader(raw)); err != nil {
			t.Fatal(err)
		}
		schema, err := c.Compile(src.Name + ".json")
		if err != nil {
			t.Fatalf("%s: %v", src.Name, err)
		}
		b, _ := json.Marshal(rows[src.Name])
		var v any
		_ = json.Unmarshal(b, &v)
		if err := schema.Validate(v); err != nil {
			t.Errorf("a %s row does not validate against its data type: %v", src.Name, err)
		}
		// A row of the wrong shape: its id a number.
		bad := map[string]any{}
		_ = json.Unmarshal(b, &bad)
		for _, k := range []string{"id", "release", "row", "goal"} {
			if _, ok := bad[k]; ok {
				bad[k] = 42
				break
			}
		}
		if err := schema.Validate(bad); err == nil {
			t.Errorf("a %s row of the wrong shape validates: the check proves nothing", src.Name)
		}
	}
}
