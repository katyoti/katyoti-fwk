// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/api"
	acceptancelocal "github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/local"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	verificationlocal "github.com/katyoti/katyoti-fwk/internal/extensions/verification/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// ACCEPTANCE, end to end: the verification service walks a use-case, the `checkable`
// capability's verify hook reaches this service, which runs each criterion's
// check and records its verdict.

const (
	hookURI  = "ktw-methods:acceptance/verify"
	probeURI = "ktw-methods:acceptance/probe"
)

// routeActions is the ActionService for the test: the verify hook's URI goes
// to the acceptance service in process — what the bus does between ktw-agents
// and ktw-methods.
type routeActions struct{}

func (routeActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (routeActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	svc, err := application.UseService[api.AcceptanceService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	var res api.VerifyResult
	if in.URI == probeURI {
		var pin api.ProbeInput
		if err := json.Unmarshal(in.Input, &pin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		res, err = svc.Probe(ctx, pin)
	} else {
		var vin api.VerifyInput
		if err := json.Unmarshal(in.Input, &vin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		res, err = svc.Verify(ctx, vin)
	}
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	raw, _ := json.Marshal(res)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

func pack(withVerdict bool) fstest.MapFS {
	fs := fstest.MapFS{
		"capabilities/container.yaml": {Data: []byte("name: container\n")},
		"capabilities/content.yaml":   {Data: []byte("name: content\n")},
		"capabilities/verdict.yaml":   {Data: []byte("name: verdict\n")},
		"capabilities/checkable.yaml": {Data: []byte("name: checkable\nhooks:\n  verify:\n    - uri: " + hookURI + "\n")},
		"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container]\n")},
		"object-types/use-case.yaml":  {Data: []byte("name: use-case\ncapabilities: [container, content]\n")},
		"object-types/criterion.yaml": {Data: []byte("name: criterion\ncapabilities: [container, content, checkable]\n")},
	}
	if withVerdict {
		fs["object-types/verdict.yaml"] = &fstest.MapFile{Data: []byte("name: verdict\nfolder: verdicts\ncapabilities: [container, content, verdict]\n")}
	}
	return fs
}

func world(t *testing.T, withVerdict bool) (context.Context, cwmapi.CWMService) {
	t.Helper()
	return worldOf(t, pack(withVerdict))
}

func worldOf(t *testing.T, fsys fstest.MapFS) (context.Context, cwmapi.CWMService) {
	t.Helper()
	ctx, cwm := cwmapp.New(t,
		verificationlocal.Extension(),
		cwmapp.Pack(fsys),
		acceptancelocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return routeActions{}, nil
		}),
	)
	mk := func(o cwmapi.Object, parent string, checkable map[string]any) {
		o.Name = o.ID
		o.Parent = parent
		o.Name = o.ID
		if checkable != nil {
			_ = o.SetCapability(cwmapi.CapCheckable, checkable)
		}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	mk(cwmapi.Object{ID: "m", Type: "module"}, "", nil)
	mk(cwmapi.Object{ID: "m.uc", Type: "use-case"}, "m", nil)
	mk(cwmapi.Object{ID: "m.ac-1", Type: "criterion"}, "m.uc", map[string]any{"kind": "expression", "probe": "1 == 1"})
	mk(cwmapi.Object{ID: "m.ac-2", Type: "criterion"}, "m.uc", map[string]any{"kind": "expression", "probe": "1 == 2"})
	mk(cwmapi.Object{ID: "m.ac-3", Type: "criterion"}, "m.uc", map[string]any{"kind": "command", "probe": "exit 3"})
	mk(cwmapi.Object{ID: "m.ac-4", Type: "criterion"}, "m.uc", map[string]any{"kind": "none"})
	return ctx, cwm
}

// TestVerifyRecordsOneVerdictPerCriterion: from the MODULE, the walk reaches
// the criteria; pass and fail are counted — a command probe runs in the project
// and fails on its exit status —, an empty one is unproven, and each criterion
// has its verdict beside it.
func TestVerifyRecordsOneVerdictPerCriterion(t *testing.T) {
	ctx, cwm := world(t, true)
	out, err := verificationOf(t, ctx).Verify(ctx, verificationapi.VerifyInput{ID: "m"})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Objects != 4 || out.Runs != 4 || out.Passed != 1 {
		t.Fatalf("objects %d runs %d passed %d, want 4 / 4 / 1", out.Objects, out.Runs, out.Passed)
	}
	failed := map[string]bool{}
	for _, f := range out.Failed {
		failed[f.ObjectID] = true
	}
	if len(out.Failed) != 2 || !failed["m.ac-2"] || !failed["m.ac-3"] {
		t.Errorf("failed = %+v, want ac-2 (false) and ac-3 (exit 3)", out.Failed)
	}
	status := map[string]string{}
	for _, u := range out.Unproven {
		status[u.ObjectID] = u.Status
	}
	if status["m.ac-4"] != "no-probe" || len(out.Unproven) != 1 {
		t.Errorf("unproven = %+v, want ac-4 no-probe", out.Unproven)
	}
	for _, ac := range []string{"m.ac-1", "m.ac-2", "m.ac-3", "m.ac-4"} {
		vid := "m.verdict-" + strings.TrimPrefix(ac, "m.")
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: vid})
		if err != nil {
			t.Fatalf("no verdict for %s: %v", ac, err)
		}
		v := got.Object
		if c := v; c.Parent != ac || v.Type != api.VerdictType {
			t.Errorf("%s: type %s parent %s, want a verdict under %s", vid, v.Type, c.Parent, ac)
		}
		grp, _ := v.Capability(api.CapVerdict)
		if _, of := grp["of"]; of || grp["status"] == "" || grp["ran_at"] == "" {
			t.Errorf("%s: verdict group %+v", vid, grp)
		}
		if ac == "m.ac-3" && grp["probe"] != "exit 3" {
			t.Errorf("the probe as it ran is not recorded: %+v", grp)
		}
	}
}

// TestARerunReplacesTheVerdictsAndNeverGradesThem: a second run REPLACES the
// verdict (the probe changed, so does the recorded status) and still leaves
// four; the verdicts — which carry no checkable — are never verified.
func TestARerunReplacesTheVerdictsAndNeverGradesThem(t *testing.T) {
	ctx, cwm := world(t, true)
	if _, err := verificationOf(t, ctx).Verify(ctx, verificationapi.VerifyInput{ID: "m.uc"}); err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.ac-1"})
	if err != nil {
		t.Fatal(err)
	}
	ac := got.Object
	_ = ac.SetCapability(cwmapi.CapCheckable, map[string]any{"kind": "expression", "probe": "1 == 3"})
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: ac}); err != nil {
		t.Fatal(err)
	}
	out, err := verificationOf(t, ctx).Verify(ctx, verificationapi.VerifyInput{ID: "m.uc"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Objects != 4 {
		t.Fatalf("the rerun verified %d objects, want the 4 criteria only", out.Objects)
	}
	v, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.verdict-ac-1"})
	if err != nil {
		t.Fatal(err)
	}
	if grp, _ := v.Object.Capability(api.CapVerdict); grp["status"] != "fail" || grp["probe"] != "1 == 3" {
		t.Fatalf("verdict after the rerun = %+v, want the new probe failing", grp)
	}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: api.VerdictType})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Objects) != 4 {
		t.Fatalf("verdicts = %d after two runs, want 4 (replaced, not added)", len(list.Objects))
	}
}

// TestTheShippedCheckableDeclaresTheHook: ktw-methods' checkable, as shipped,
// makes this service its verify hook.
func TestTheShippedCheckableDeclaresTheHook(t *testing.T) {
	var c cwmapi.Capability
	if err := config.DecodeBytes(cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/capabilities/checkable.yaml"), marshal.YAML, &c); err != nil {
		t.Fatal(err)
	}
	hooks := c.Hooks[cwmapi.HookVerify]
	if len(hooks) != 1 || hooks[0].URI != hookURI {
		t.Fatalf("checkable's verify hooks = %+v, want %s", hooks, hookURI)
	}
}

// verificationOf is the verification service of the test's application: it
// reads the lint and verify points CWM runs.
func verificationOf(t *testing.T, ctx context.Context) verificationapi.VerificationService {
	t.Helper()
	v, err := application.UseService[verificationapi.VerificationService](ctx)
	if err != nil {
		t.Fatalf("resolve VerificationService: %v", err)
	}
	return v
}

// A POLICY IS MEASURED BY A VERIFY ENTRY: acceptance's probe, its probe the
// entry's input, answering a verify answer and recording no verdict. An
// expression is evaluated over the object; a command is left to the harness.
func TestAPolicysProbeIsAVerifyAnswerWithNoVerdict(t *testing.T) {
	fsys := pack(true)
	fsys["policies/named.yaml"] = &fstest.MapFile{Data: []byte(`name: named
applies: {types: [use-case]}
hooks:
  verify:
    - uri: ` + probeURI + `
      input: {id: "${id}", probe: "id == 'm.uc'"}
`)}
	fsys["policies/scripted.yaml"] = &fstest.MapFile{Data: []byte(`name: scripted
applies: {types: [use-case]}
hooks:
  verify:
    - uri: ` + probeURI + `
      input: {id: "${id}", kind: command, probe: "exit 7"}
`)}
	ctx, cwm := worldOf(t, fsys)
	out, err := verificationOf(t, ctx).Verify(ctx, verificationapi.VerifyInput{ID: "m.uc"})
	if err != nil {
		t.Fatal(err)
	}
	bySource := map[string][]string{}
	for _, o := range out.Outcomes {
		if o.ObjectID == "m.uc" {
			bySource[o.Source] = append(bySource[o.Source], o.Status)
		}
	}
	if strings.Join(bySource["policy:named"], ",") != "pass" || strings.Join(bySource["policy:scripted"], ",") != "needs-harness" {
		t.Fatalf("the policies' answers on m.uc: %v", bySource)
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.verdict-uc"}); err == nil {
		t.Error("a policy's probe recorded a verdict")
	}
}
