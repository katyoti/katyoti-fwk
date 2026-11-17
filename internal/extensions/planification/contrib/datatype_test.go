// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The pack's field-group data-types, each rendered from the Go type its
// service decodes it with — the one source of the contract.
var dataTypes = []datatypegen.Source{
	{
		Name:        "cap-module-uses",
		Version:     "1.0.0",
		Root:        "CapModuleUses",
		Origin:      "api.ModuleUsesFields (internal/extensions/planification/api/plan.go)",
		Description: "Field-group for the `module-uses` capability: the module edges the planner learned from the steps it submitted, who showed each and when it was last seen.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.ModuleUsesFields]() },
	},
	{
		Name:    "cap-plan",
		Version: "1.0.0",
		Root:    "CapPlan",
		Origin:  "api.PlanFields (internal/extensions/planification/api/plan.go)",
		Description: "Field-group for the `plan` capability: where the work on an object stands —\n" +
			"its own state and trail in the plan workflow, beside the object's lifecycle —\n" +
			"its priority, a note on why it is blocked or idle, and one record per step its\n" +
			"to-dos did. The steps themselves are the scheduler's to-dos.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.PlanFields]() },
	},
	{
		Name:    "cap-request",
		Version: "1.0.0",
		Root:    "CapRequest",
		Origin:  "api.RequestFields (internal/extensions/planification/api/request.go)",
		Description: "Field-group for the `request` capability: work asked for on an\n" +
			"object — where it comes from, the details, what the work must deliver and\n" +
			"whether each is verified, the kind, priority and budget. Its state is its\n" +
			"plan's. The one-line ask is the object's title.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.RequestFields]() },
	},
	{
		Name:    "cap-release",
		Version: "1.1.0",
		Root:    "CapRelease",
		Origin:  "api.ReleaseFields (internal/extensions/planification/api/release.go)",
		Description: "Field-group for the `release` capability: the version it ships as, its\n" +
			"planned date, and the targets it was frozen with. Its name is the object's name;\n" +
			"its state is the lifecycle; its work targets it by edge.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.ReleaseFields]() },
	},
	{
		Name:    "cap-change",
		Version: "1.0.0",
		Root:    "CapChange",
		Origin:  "api.ChangeFields (internal/extensions/planification/api/change.go)",
		Description: "Field-group for the `change` capability: a change-log entry's number on the\n" +
			"pages, its domain, value, maturity and who it is for, the modules it changed,\n" +
			"the requests it delivers and when it was recorded. Its title is the object's\n" +
			"name and its text the object's body; its release is its parent.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.ChangeFields]() },
	},
	{
		Name:        "cap-step",
		Version:     "1.0.0",
		Root:        "CapStep",
		Origin:      "api.StepFields (internal/extensions/planification/api/planning.go)",
		Description: "Field-group for the `step` capability: what reaching the sync point produces.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.StepFields]() },
	},
	{
		Name:    "cap-gate-wait",
		Version: "1.0.0",
		Root:    "CapGateWait",
		Origin:  "api.GateWaitFields (internal/extensions/planification/api/gates.go)",
		Description: "Field-group for the `gate-wait` capability: a move a named gate refused —\n" +
			"the gate, the transition, who asked — and the requests and inbox validations\n" +
			"the planner filed to meet its unmet requirements.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.GateWaitFields]() },
	},
	{
		Name:    "stream",
		Version: "1.1.0",
		Root:    "Stream",
		Origin:  "api.StreamFields (internal/extensions/planification/api/stream.go)",
		Description: "The stream type's own fields: its color in the drawings, its rank and its role;\n" +
			"its version scheme and its release mode.\n",
		AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.StreamFields]() },
	},
}

// withDisplay renders a delivery type with its display: the ui-* extensions
// it declares once beside it (api.DeliveryDisplay).
func withDisplay[T any](root string) func() (*schemaapi.Type, error) {
	return func() (*schemaapi.Type, error) {
		t, err := ktwreflect.TypeAST[T]()
		if err != nil {
			return nil, err
		}
		t.Extensions = append(t.Extensions, api.DeliveryDisplay[root]...)
		return t, nil
	}
}

// The delivery data types: the rows the Delivery view
// and the stream, release and increment panes show, each with its display.
var deliveryTypes = []datatypegen.Source{
	{Name: "stream-lane", Version: "1.0.0", Root: "StreamLane", Origin: "api.StreamLane (internal/extensions/planification/api/delivery_rows.go)",
		Description: "A stream on the Delivery timeline: its mode and next cut, its releases by date as cards, its backlog at the lane's start.\n",
		AST:         withDisplay[api.StreamLane]("StreamLane"),
		Named: []datatypegen.NamedType{
			{Name: "ReleaseCard", Description: "A release on the lane.", AST: withDisplay[api.ReleaseCard]("ReleaseCard")},
			{Name: "BacklogItem", Description: "An increment of the lane's backlog.", AST: withDisplay[api.BacklogItem]("BacklogItem")},
		}},
	{Name: "release-card", Version: "1.0.0", Root: "ReleaseCard", Origin: "api.ReleaseCard (internal/extensions/planification/api/delivery_rows.go)",
		Description: "A release as a lane and the release level show it: its stage, date, what it ships done of the total, a dot per gate, what is planned for it.\n",
		AST:         withDisplay[api.ReleaseCard]("ReleaseCard")},
	{Name: "release-rows", Version: "1.0.0", Root: "ReleaseRows", Origin: "api.ReleaseRows (internal/extensions/planification/api/delivery_rows.go)",
		Description: "A release's level: its card, its stage stepper and next action, the increments that ship in it and those planned for it.\n",
		AST:         withDisplay[api.ReleaseRows]("ReleaseRows"),
		Named: []datatypegen.NamedType{
			{Name: "ReleaseCard", Description: "The release.", AST: withDisplay[api.ReleaseCard]("ReleaseCard")},
			{Name: "IncrementRow", Description: "An increment of the release level.", AST: withDisplay[api.IncrementRow]("IncrementRow")},
		}},
	{Name: "increment-row", Version: "1.0.0", Root: "IncrementRow", Origin: "api.IncrementRow (internal/extensions/planification/api/delivery_rows.go)",
		Description: "An increment as a row of the release level: priority, title and id, where it stands, its requests as one bar, what it waits on.\n",
		AST:         withDisplay[api.IncrementRow]("IncrementRow")},
	{Name: "increment-detail", Version: "1.0.0", Root: "IncrementDetail", Origin: "api.IncrementDetail (internal/extensions/planification/api/delivery_rows.go)",
		Description: "An increment's detail - its drawer and its pane: where it is assigned and where it can move, its proposal, each delivery goal down to its requests.\n",
		AST:         withDisplay[api.IncrementDetail]("IncrementDetail"),
		Named: []datatypegen.NamedType{
			{Name: "IncrementRow", Description: "The increment's row.", AST: withDisplay[api.IncrementRow]("IncrementRow")},
		}},
}

// The increment drawer's lines, each a data type of its own so a page draws
// it by its view.
var drawerTypes = []datatypegen.Source{
	{Name: "goal-line", Version: "1.0.0", Root: "GoalLine", Origin: "api.GoalLine (internal/extensions/planification/api/delivery_rows.go)",
		Description: "A delivery goal of an increment and what it became.\n",
		AST:         withDisplay[api.GoalLine]("GoalLine"),
		Named: []datatypegen.NamedType{
			{Name: "HeldLine", Description: "What it became.", AST: withDisplay[api.HeldLine]("HeldLine")},
			{Name: "RequestLine", Description: "A request serving it.", AST: withDisplay[api.RequestLine]("RequestLine")},
		}},
	{Name: "held-line", Version: "1.0.0", Root: "HeldLine", Origin: "api.HeldLine (internal/extensions/planification/api/delivery_rows.go)",
		Description: "A goal or a feature an increment holds and the requests serving it.\n",
		AST:         withDisplay[api.HeldLine]("HeldLine"),
		Named: []datatypegen.NamedType{
			{Name: "RequestLine", Description: "A request serving it.", AST: withDisplay[api.RequestLine]("RequestLine")},
		}},
	{Name: "request-line", Version: "1.0.0", Root: "RequestLine", Origin: "api.RequestLine (internal/extensions/planification/api/delivery_rows.go)",
		Description: "A request serving what an increment holds: its steps and where it is in its work.\n",
		AST:         withDisplay[api.RequestLine]("RequestLine")},
}

func init() { dataTypes = append(append(dataTypes, deliveryTypes...), drawerTypes...) }

// TestDataTypesAreCurrent refuses a data-type that no longer says what its Go
// type says. KTW_DATATYPE_UPDATE=1 rewrites them.
func TestDataTypesAreCurrent(t *testing.T) {
	for _, src := range dataTypes {
		file := "embed/cwm/data-types/" + src.Name + ".dsl.yaml"
		if !strings.HasPrefix(src.Name, "cap-") {
			file = "embed/cwm/data-types/" + src.Name + ".dsl.yaml"
		}
		want, err := datatypegen.RenderAs(src, datatypegen.Header{
			Tool:       "the planification contrib test",
			Regenerate: "`KTW_DATATYPE_UPDATE=1 go test ./internal/extensions/planification/contrib/`",
			URI:        "ktw-agents:/data-type/" + src.Name,
		})
		if err != nil {
			t.Fatal(err)
		}
		if os.Getenv("KTW_DATATYPE_UPDATE") == "1" {
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%v — run with KTW_DATATYPE_UPDATE=1", err)
		}
		if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
			t.Errorf("%s is stale: %s changed — run with KTW_DATATYPE_UPDATE=1", file, src.Origin)
		}
	}
}
