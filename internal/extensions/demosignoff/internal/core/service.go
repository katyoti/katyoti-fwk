// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/demosignoff/api"
)

// Service implements api.SignoffService over the CWMService in ctx (in
// ktw-demo, the bus client of the CWM ktw-agents hosts).
type Service struct {
	now func() time.Time
}

// New returns the service.
func New() *Service { return &Service{now: time.Now} }

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	c, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil || c == nil {
		return nil, fmt.Errorf("signoff: no CWMService: %v", err)
	}
	return c, nil
}

// load reads an object and its type's sign-off settings.
func load(ctx context.Context, c cwmapi.CWMService, id string) (cwmapi.ObjectGetOutput, api.SignoffSettings, error) {
	if id == "" {
		return cwmapi.ObjectGetOutput{}, api.SignoffSettings{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.ObjectGetOutput{}, api.SignoffSettings{}, err
	}
	got, err := c.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return got, api.SignoffSettings{}, err
	}
	return got, settings(ctx, c, got.Object.Type), nil
}

// settings is a type's sign-off section; the defaults when the type writes none
// or cannot be read.
func settings(ctx context.Context, c cwmapi.CWMService, typ string) api.SignoffSettings {
	t, err := c.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: typ})
	if err != nil {
		return api.SignoffSettings{}
	}
	return SettingsOf(t.ObjectType)
}

// contesters are the objects that contest id.
func contesters(ctx context.Context, c cwmapi.CWMService, id string) ([]string, error) {
	edges, err := c.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: id, Kind: api.EdgeContests, Direction: cwmapi.DirectionIn})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range edges.Edges {
		if e.To == id && e.From != "" {
			out = append(out, e.From)
		}
	}
	return out, nil
}

// CheckWrite refuses a sign-off written approved without the approvals its
// type requires.
func (s *Service) CheckWrite(ctx context.Context, in cwmapi.HookCall) (cwmapi.HookAccept, error) {
	if in.Object == nil {
		return cwmapi.HookAccept{Accept: true}, nil
	}
	c, err := cwmOf(ctx)
	if err != nil {
		return cwmapi.HookAccept{}, err
	}
	return CheckWrite(GroupOf(*in.Object), settings(ctx, c, in.Object.Type)), nil
}

// Record appends the sign-off's state to the object's sign-off log, a named file
// of the signoff-log capability: a write never touches the object itself.
func (s *Service) Record(ctx context.Context, in cwmapi.HookCall) (api.RecordOutput, error) {
	if in.Object == nil {
		return api.RecordOutput{}, nil
	}
	c, err := cwmOf(ctx)
	if err != nil {
		return api.RecordOutput{}, err
	}
	g := GroupOf(*in.Object)
	line := fmt.Sprintf("%s %s: %s", s.now().UTC().Format(time.RFC3339), in.Point, State(g, settings(ctx, c, in.Object.Type)))
	if _, err := c.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{
		ID: in.Object.ID, Capability: api.LogCapability, Name: api.LogFile, Content: []byte(line + "\n"), Append: true,
	}); err != nil {
		return api.RecordOutput{}, err
	}
	return api.RecordOutput{Line: line}, nil
}

// Lint reports what the sign-off lacks.
func (s *Service) Lint(_ context.Context, in cwmapi.HookCall) (api.LintOutput, error) {
	if in.Object == nil {
		return api.LintOutput{Status: "ok", Findings: []api.LintFinding{}}, nil
	}
	return Lint(GroupOf(*in.Object)), nil
}

// Verify answers the verdict of a rule over the object handed, else read.
func (s *Service) Verify(ctx context.Context, in api.VerifyInput) (api.VerifyOutput, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return api.VerifyOutput{}, err
	}
	var obj cwmapi.Object
	var set api.SignoffSettings
	if in.Object != nil {
		obj, set = *in.Object, settings(ctx, c, in.Object.Type)
	} else {
		got, st, err := load(ctx, c, in.ID)
		if err != nil {
			return api.VerifyOutput{}, err
		}
		obj, set = got.Object, st
	}
	return Verify(GroupOf(obj), set, in.Rule), nil
}

// Digest is the signoff_digest point: the sign-off and how many contest it,
// in one finding of level info.
func (s *Service) Digest(ctx context.Context, in cwmapi.HookCall) (api.LintOutput, error) {
	if in.Object == nil {
		return api.LintOutput{Status: "ok", Findings: []api.LintFinding{}}, nil
	}
	c, err := cwmOf(ctx)
	if err != nil {
		return api.LintOutput{}, err
	}
	from, err := contesters(ctx, c, in.Object.ID)
	if err != nil {
		return api.LintOutput{}, err
	}
	return api.LintOutput{Status: "findings", Findings: []api.LintFinding{{
		Rule: api.RuleDigest, Severity: "info", Message: Digest(GroupOf(*in.Object), settings(ctx, c, in.Object.Type), len(from)),
	}}}, nil
}

// SignedoffFunction is signedoff(): the object has the approvals its type
// requires.
func (s *Service) SignedoffFunction(ctx context.Context, in cwmapi.FunctionCall) (bool, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return false, err
	}
	got, set, err := load(ctx, c, in.Object)
	if err != nil {
		return false, err
	}
	return SignedOff(GroupOf(got.Object), set), nil
}

// Log reads the object's sign-off log.
func (s *Service) Log(ctx context.Context, in api.ObjectInput) (api.LogOutput, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return api.LogOutput{}, err
	}
	return readLog(ctx, c, in.ID)
}

func readLog(ctx context.Context, c cwmapi.CWMService, id string) (api.LogOutput, error) {
	out := api.LogOutput{ID: id, Lines: []string{}}
	f, err := c.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: id, Capability: api.LogCapability, Name: api.LogFile})
	if err != nil {
		return out, err
	}
	for _, l := range strings.Split(strings.TrimSpace(string(f.Content)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out.Lines = append(out.Lines, l)
		}
	}
	return out, nil
}

// Approve adds a reviewer's approval; refused, as a page result, for someone
// who is not a reviewer.
func (s *Service) Approve(ctx context.Context, in api.ApproveInput) (render.Result, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return render.Result{}, err
	}
	got, _, err := load(ctx, c, in.ID)
	if err != nil {
		return render.Result{}, err
	}
	g := GroupOf(got.Object)
	who := strings.TrimSpace(in.Reviewer)
	isReviewer := false
	for _, r := range g.Reviewers {
		isReviewer = isReviewer || strings.TrimSpace(r) == who
	}
	if !isReviewer {
		return render.Result{OK: false, Message: fmt.Sprintf("%q is not a reviewer of %s", who, in.ID)}, nil
	}
	for _, a := range g.Approvals {
		if strings.TrimSpace(a) == who {
			return render.Result{OK: true, Message: who + " had already approved"}, nil
		}
	}
	g.Approvals = append(g.Approvals, who)
	obj := got.Object
	SetGroup(&obj, g)
	if _, err := c.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Object: obj, Revision: got.Revision}); err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	return render.Result{OK: true, Message: who + " approved " + in.ID}, nil
}

// Write sets an object's verdict and notes. CWM runs the before_update hook
// on the write: its refusal, with the reason, is this act's answer.
func (s *Service) Write(ctx context.Context, in api.WriteInput) (render.Result, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return render.Result{}, err
	}
	got, _, err := load(ctx, c, in.ID)
	if err != nil {
		return render.Result{}, err
	}
	g := GroupOf(got.Object)
	g.Verdict, g.Notes = in.Verdict, in.Notes
	obj := got.Object
	SetGroup(&obj, g)
	if _, err := c.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Object: obj, Revision: got.Revision}); err != nil {
		return render.Result{OK: false, Message: "refused: " + err.Error()}, nil
	}
	return render.Result{OK: true, Message: fmt.Sprintf("%s: verdict %s", in.ID, verdictOrPending(in.Verdict))}, nil
}

// VerifyNotes is the signoff-has-notes policy's verify hook.
func (s *Service) VerifyNotes(ctx context.Context, in api.VerifyInput) (api.VerifyOutput, error) {
	in.Rule = "notes"
	return s.Verify(ctx, in)
}

// Contest records that one object contests another: an edge from the
// contester, so the contested object's Contests panel lists it.
func (s *Service) Contest(ctx context.Context, in api.ContestInput) (render.Result, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return render.Result{}, err
	}
	if in.ID == "" || in.Contester == "" {
		return render.Result{OK: false, Message: "the object and its contester are both required"}, nil
	}
	if in.ID == in.Contester {
		return render.Result{OK: false, Message: "an object does not contest itself"}, nil
	}
	if _, err := c.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: in.Contester, To: in.ID, Kind: api.EdgeContests}); err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	return render.Result{OK: true, Message: in.Contester + " contests " + in.ID, Open: in.ID + "#contests"}, nil
}

// EnsureDesk is the bootstrap hook: the module's welcome sign-off item exists.
// A module that does not exist yet is left alone.
func (s *Service) EnsureDesk(ctx context.Context, in api.EnsureDeskInput) (api.EnsureOutput, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return api.EnsureOutput{}, err
	}
	if in.Module == "" {
		return api.EnsureOutput{}, fmt.Errorf("%w: module required", cwmapi.ErrInvalidInput)
	}
	id := cwmapi.QualifyObjectID(in.Module, api.WelcomeLocal)
	if _, err := c.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: in.Module, ID: id}); err == nil {
		return api.EnsureOutput{ID: id}, nil
	} else if !notFound(err) {
		return api.EnsureOutput{}, err
	}
	if _, err := c.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: in.Module, ID: in.Module}); err != nil {
		if notFound(err) {
			return api.EnsureOutput{}, nil
		}
		return api.EnsureOutput{}, err
	}
	obj := cwmapi.Object{
		ID: id, Type: api.ItemType, Parent: in.Module, Name: "Welcome sign-off",
		Description:  "What a sign-off item is: reviewers, approvals, a verdict, contests.",
		Capabilities: map[string]map[string]any{},
	}
	SetGroup(&obj, api.SignoffGroup{Verdict: api.VerdictPending})
	if _, err := c.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: in.Module, Object: obj}); err != nil {
		return api.EnsureOutput{}, err
	}
	return api.EnsureOutput{ID: id, Created: true}, nil
}

func notFound(err error) bool {
	return errors.Is(err, cwmapi.ErrNotFound) || strings.Contains(err.Error(), "not found")
}
