// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A RESULT THAT ENDS ON A CHOICE — a step reports its options (more.choice);
// planification opens a decision with them in the request's module and the
// person decides it there: the decision's resolution ends their step, is
// recorded on the decision, and passes to the steps after.

// decisionsPrefix is ktw-methods' decision methods, routed in process to the
// decisions service.
const decisionsPrefix = "ktw-methods:decisions/"

// runDecisions runs a decisions method the planner calls.
func runDecisions(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	d, err := application.UseService[decisionsapi.DecisionsService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	var out any
	switch strings.TrimPrefix(in.URI, decisionsPrefix) {
	case "open":
		var x decisionsapi.OpenInput
		if err := json.Unmarshal(in.Input, &x); err != nil {
			return actionsapi.RunOutput{}, err
		}
		out, err = d.Open(ctx, x)
	case "prepare":
		var x decisionsapi.PrepareInput
		if err := json.Unmarshal(in.Input, &x); err != nil {
			return actionsapi.RunOutput{}, err
		}
		out, err = d.Prepare(ctx, x)
	case "challenge":
		var x decisionsapi.ChallengeInput
		if err := json.Unmarshal(in.Input, &x); err != nil {
			return actionsapi.RunOutput{}, err
		}
		out, err = d.Challenge(ctx, x)
	default:
		return actionsapi.RunOutput{}, fmt.Errorf("no route for %s", in.URI)
	}
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	raw, _ := json.Marshal(out)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// aChoice is what an analysis reports when it ends on options.
func aChoice(options ...string) map[string]any {
	var opts []map[string]any
	for _, o := range options {
		opts = append(opts, map[string]any{"label": o, "means": "go with " + o})
	}
	return map[string]any{api.ChoiceKey: map[string]any{
		"question":   "How are the totals rounded?",
		"options":    opts,
		"proposal":   map[string]any{"option": options[0], "why": "the simplest"},
		"challenges": []string{"the ledger rounds differently"},
	}}
}

// decisionAbout is the decision opened about a request, read back.
func decisionAbout(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, request string) cwmapi.Object {
	t.Helper()
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "billing", Type: decisionsapi.DecisionType})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range got.Objects {
		if o.EdgeTo("about") == request {
			return o
		}
	}
	t.Fatalf("no decision about %s among %d", request, len(got.Objects))
	return cwmapi.Object{}
}

// resolve decides a decision as the person, through its own service.
func resolve(t *testing.T, ctx context.Context, id, option string) {
	t.Helper()
	d, err := application.UseService[decisionsapi.DecisionsService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Resolve(ctx, decisionsapi.ResolveInput{ID: id, Selected: []string{option}, Actor: person, Rationale: "it reads well"}); err != nil {
		t.Fatalf("resolve %s: %v", id, err)
	}
}

// An analysis ending on options opens a decision (question, options, the
// proposal, its challenge, linked to the request) and its accept becomes the
// person's decide step on it, asked about the decision; resolving the
// decision ends the step by the person with the option, verifies the request
// and ends it.
func TestAnAnalysisEndingOnAChoiceIsDecidedOnADecision(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing", Kind: api.KindAnalyse, By: person,
		Deliverables: []api.Deliverable{{Title: "How the totals are rounded is decided"}}})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "analyse").ID, session, schedapi.Result{Summary: "two ways to round", More: aChoice("Half up", "Banker's")})

	od := decisionAbout(t, ctx, cwm, r.ID)
	// Its source is a field: its text never repeats it (the decision's guard
	// refuses a body repeating its source).
	if whole, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: od.ID}); err != nil || strings.Contains(whole.Object.Body+whole.Object.Description, r.ID) {
		t.Errorf("the decision's text repeats its source %s (or cannot be read: %v)", r.ID, err)
	}
	var st struct {
		Options   []string                        `json:"options"`
		Proposal  decisionsapi.Proposal           `json:"proposal"`
		Challenge *decisionsapi.ProposalChallenge `json:"challenge"`
		Source    string                          `json:"source"`
	}
	raw, _ := json.Marshal(od.Capabilities[decisionsapi.CapDecision])
	_ = json.Unmarshal(raw, &st)
	if od.Name != "How are the totals rounded?" || strings.Join(st.Options, ",") != "Half up,Banker's" || st.Proposal.Option != "Half up" ||
		st.Challenge == nil || len(st.Challenge.Points) != 1 || st.Source != r.ID {
		t.Fatalf("the decision %s: %q %+v; want the question, both options, the proposal, the challenge, the request as source", od.ID, od.Name, st)
	}
	if got := strings.Join(steps(t, ctx, r.ID), ","); got != "decide" {
		t.Fatalf("steps after the analysis %s; want its accept become the decide step", got)
	}
	d := stepOf(t, ctx, r.ID, "decide")
	if d.Packet.On != od.ID || d.Packet.EndsOn != decisionsapi.StatusApplies || d.Packet.Interaction != api.InteractionDecide ||
		d.Packet.Verifies == "" || d.Packet.Moves == "" || d.Target != r.ID {
		t.Fatalf("the decide step %+v; want it on %s, ending on applies, verifying and moving as the accept did", d.Packet, od.ID)
	}
	if !strings.Contains(d.Packet.Instructions, "Half up (proposed)") || !strings.Contains(d.Packet.Instructions, od.ID) {
		t.Errorf("the decide step does not name the decision and mark the proposal: %s", d.Packet.Instructions)
	}
	eventually(t, "the decide step asked", func() (string, bool) {
		d := stepOf(t, ctx, r.ID, "decide")
		return d.ID, d.State == schedapi.StateReady && d.Packet.Ask != nil
	})
	if ask := stepOf(t, ctx, r.ID, "decide").Packet.Ask; ask.About != od.ID || ask.Kind != inboxapi.KindDecide {
		t.Fatalf("the decide step asks %+v; want a decide question about the decision %s", ask, od.ID)
	}

	resolve(t, ctx, od.ID, "Banker's")
	eventually(t, "the request ended", func() (string, bool) {
		got, err := req.Get(ctx, api.GetInput{ID: r.ID})
		if err != nil {
			return err.Error(), false
		}
		return got.Request.Plan.State, got.Request.Plan.State == api.PlanDone
	})
	done, err := req.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(done.Request.Plan.Steps, func(s api.StepRecord) bool { return strings.HasSuffix(s.Step, "decide") })
	if i < 0 || done.Request.Plan.Steps[i].By == nil || *done.Request.Plan.Steps[i].By != person || !strings.Contains(done.Request.Plan.Steps[i].Result, "Banker's") {
		t.Errorf("the decide step's record %+v; want it done by the person, with the option", done.Request.Plan.Steps)
	}
	for _, dl := range done.Request.Deliverables {
		if !dl.Verified {
			t.Errorf("deliverable %q not verified by the decision", dl.Title)
		}
	}
}

// A choice with no validation after it adds the decide step, which what came
// after waits on and whose option it is handed; a choice that is no choice
// (one option) is refused and opens nothing.
func TestAChoiceAddsADecideStepWhoseOptionPassesOn(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print the totals", Module: "billing", Kind: api.KindBuild, By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{})
	analyse := stepOf(t, ctx, r.ID, "analyse").ID

	s := schedOf(t, ctx)
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: analyse, By: session}); err != nil {
		t.Fatal(err)
	}
	one := schedapi.Result{Status: schedapi.StatusSuccess, Summary: "one way", By: session, More: aChoice("Half up")}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: analyse, Result: one}); err == nil {
		t.Fatal("a choice of one option was collected; want it refused")
	}
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "billing", Type: decisionsapi.DecisionType})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Objects) != 0 {
		t.Fatalf("a refused choice opened %d decision(s)", len(got.Objects))
	}
	// An option the decision's own methods refuse (a label opening with #):
	// the decision opened for it is dropped, the step reported again.
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: analyse, By: session}); err != nil {
		t.Fatal(err)
	}
	bad := schedapi.Result{Status: schedapi.StatusSuccess, Summary: "a bad label", By: session, More: aChoice("Half up", "# Banker's")}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: analyse, Result: bad}); err == nil {
		t.Fatal("a choice with a label its decision refuses was collected; want it refused")
	}
	if got, err = cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "billing", Type: decisionsapi.DecisionType}); err != nil {
		t.Fatal(err)
	}
	for _, o := range got.Objects {
		if l, _ := o.Lifecycle(); l.State != decisionsapi.StatusDropped {
			t.Fatalf("the decision a refused choice opened is %q; want it dropped", l.State)
		}
	}

	pullAndReport(t, ctx, analyse, session, schedapi.Result{Summary: "two ways", More: aChoice("Half up", "Banker's")})
	od := decisionAbout(t, ctx, cwm, r.ID)
	d := stepOf(t, ctx, r.ID, "decide")
	if impl := stepOf(t, ctx, r.ID, "implement"); !slices.Equal(impl.After, []string{d.ID}) {
		t.Fatalf("implement comes after %v; want the decide step %s", impl.After, d.ID)
	}
	if d.Packet.On != od.ID || d.Packet.Moves != "" {
		t.Fatalf("the added decide step %+v; want it on %s, moving nothing (the review does)", d.Packet, od.ID)
	}
	eventually(t, "the decide step asked", func() (string, bool) {
		d := stepOf(t, ctx, r.ID, "decide")
		return d.State, d.State == schedapi.StateReady && d.Packet.Ask != nil
	})
	resolve(t, ctx, od.ID, "Half up")
	eventually(t, "implement handed the option", func() (string, bool) {
		impl := stepOf(t, ctx, r.ID, "implement")
		a := fmt.Sprint(impl.Packet.Params["answer"])
		return impl.State + " " + a, impl.State == schedapi.StateReady && strings.Contains(a, "Half up") && strings.Contains(a, od.ID)
	})
}
