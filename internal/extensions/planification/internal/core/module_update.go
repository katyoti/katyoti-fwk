// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A REQUEST THAT CHANGES A MODULE ENDS WITH ITS MODULE UP TO DATE — the work
// of a template that changes a module (build, fix, maintain, feature-build)
// serves a want (a goal, a feature, a use case: the one it is filed on, else
// the one it points at), and its module-update step leaves the module saying
// what is true: an acceptance check added or changed to prove the change, the
// change log naming the request (or why no user sees it). One function,
// moduleUpdated, answers what is missing: the end of the request's last step
// asks it before the node moves — a refusal plans the module-update step
// again with what failed — and the `module-updated` gate on the plan's finish
// asks it for any other end (a move by hand). Exempt: another template, a
// request filed before the configured date (none: the gate is off), one a
// gate filed, a split demand, and one a person waived.

// The gate, the step and the skill of a module update.
const (
	// GateModuleUpdated is the gate on a plan's finish.
	GateModuleUpdated = "module-updated"
	// StepModuleUpdate is the recipes' module-update step.
	StepModuleUpdate = "module-update"
	// stepModuleUpdateAgain is the step planned again when the end was refused.
	stepModuleUpdateAgain = StepModuleUpdate + " (again)"
	// ModuleUpdateSkill is the skill its worker follows.
	ModuleUpdateSkill = "ktw-agents:/skill/module-update"
	// maxModuleUpdateRounds bounds the times the step is planned again before
	// the request is blocked for a person (who brings it up to date or
	// waives it).
	maxModuleUpdateRounds = 3
)

// moduleUpdateInstructions is what a module-update step planned again does.
const moduleUpdateInstructions = "Bring the module up to date with this change, following the skill module-update: " +
	"the feature it serves and its use cases, the acceptance check that proves the change, the module pages (in place), " +
	"the change log naming the request — then record it with requests module-update."

// changeKinds are the request kinds whose template changes a module.
var changeKinds = []string{api.KindBuild, api.KindFix, api.KindMaintain}

// changesModule reports a template whose work changes a module: one chosen by
// a changing kind (build, fix, maintain, at any depth) or feature-build.
func changesModule(r api.Recipe) bool {
	return r.Name == "feature-build" || (len(r.Types) == 0 && slices.Contains(changeKinds, r.Kind))
}

// carriesWant reports an object whose effective capabilities include `want`.
func carriesWant(ctx context.Context, cwm cwmapi.CWMService, id string) bool {
	eff, err := cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: id})
	if err != nil {
		return false
	}
	return slices.ContainsFunc(eff.Capabilities, func(c cwmapi.CapabilityRef) bool { return c.ID == cwmapi.CapWant })
}

// servedOf is the want a request serves: the object carrying it when that is
// a want (a request filed on a feature), else the target of its serves — then
// about — edge that carries one; "" when none.
func servedOf(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) string {
	if carriesWant(ctx, cwm, o.ID) {
		return o.ID
	}
	for _, kind := range []string{api.EdgeServes, api.EdgeAbout} {
		for _, e := range o.Edges {
			if e.Kind == kind && carriesWant(ctx, cwm, e.To) {
				return e.To
			}
		}
	}
	return ""
}

// DIRECT WORK — a bug fix or a small change (a label, a message, a small UI or
// doc fix) is a fix, or a build at depth quick, that serves no want: it runs its
// quick recipe and its module is up to date without one — a test case proves
// it when its behaviour changes (or why none can), its change log as any. It
// never files a feature: one that needs a feature serves it, and is release
// work.

// directWork reports a request that is direct work.
func directWork(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) bool {
	f := fieldsOf(o)
	if f.Kind != api.KindFix && (f.Kind != api.KindBuild || f.Depth != api.DepthQuick) {
		return false
	}
	return servedOf(ctx, cwm, o) == ""
}

// proposeServes finds the wants a request that serves none could serve: those
// of its module matching its title, else its module's wants. Nothing is
// written.
func proposeServes(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) *api.ServesProposal {
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	p := &api.ServesProposal{Note: "It changes a module and serves no want: name the goal, feature or use case it serves (assess with serves), " +
		"file the feature it needs if none fits, or ask its requester."}
	seen := map[string]bool{o.ID: true}
	add := func(objs []cwmapi.Object) {
		for _, c := range objs {
			if seen[c.ID] || len(p.Candidates) >= 5 {
				continue
			}
			seen[c.ID] = true
			p.Candidates = append(p.Candidates, api.ServeCandidate{ID: c.ID, Type: c.Type, Title: c.DisplayTitle()})
		}
	}
	if got, err := cwm.ObjectSearch(ctx, cwmapi.ObjectSearchInput{Query: titleOf(o), Mode: "fulltext", Modules: []string{module},
		Capabilities: []string{cwmapi.CapWant}, Limit: 5}); err == nil {
		add(got.Objects)
	}
	if len(p.Candidates) == 0 {
		if got, err := cwm.ObjectSearch(ctx, cwmapi.ObjectSearchInput{Modules: []string{module}, Capabilities: []string{cwmapi.CapWant}, Limit: 10}); err == nil {
			add(got.Objects)
		}
	}
	return p
}

// planificationConfig is planification's configuration; none is the zero one.
func planificationConfig(ctx context.Context) api.PlanificationConfig {
	cfg, _ := application.UseConfig[api.PlanificationConfig](ctx)
	return cfg
}

// parseSince reads the configured date: a day or a time.
func parseSince(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// filingStart is when the request's current filing began: when it was filed
// or filed again (filed_at, rewritten by a refile), else its latest refile or
// first plan in its trail — whichever is the latest.
func filingStart(o cwmapi.Object, f api.RequestFields) time.Time {
	start, _ := time.Parse(time.RFC3339Nano, f.FiledAt)
	pf, _ := planOf(o)
	for _, e := range pf.Trail {
		t, err := time.Parse(time.RFC3339Nano, e.At)
		switch {
		case err != nil:
		case e.Transition == "plan" && start.IsZero():
			start = t // filed before filed_at was recorded: its first plan
		case e.Transition == "refile" && t.After(start.Truncate(time.Second)):
			// A trail time is to the second: a refile moves the start only
			// from a later second than the filed time holds.
			start = t
		}
	}
	return start
}

// verifyServed runs the verification of a want and everything under it; ok
// false when the binary has no verification service. A variable: a test
// stands a verification of its own for it.
var verifyServed = func(ctx context.Context, id string) (verificationapi.VerifyOutput, bool, error) {
	v, err := application.UseService[verificationapi.VerificationService](ctx)
	if err != nil || v == nil {
		return verificationapi.VerifyOutput{}, false, nil
	}
	out, err := v.Verify(ctx, verificationapi.VerifyInput{ID: id})
	return out, true, err
}

// verdictsUnder is the status each criterion under a want had, read from its
// verdict object (a verdict sits under the criterion it records).
func verdictsUnder(ctx context.Context, cwm cwmapi.CWMService, served string) map[string]string {
	out := map[string]string{}
	got, err := cwm.ObjectSearch(ctx, cwmapi.ObjectSearchInput{Within: served, Types: []string{"verdict"}})
	if err != nil {
		return out
	}
	for _, v := range got.Objects {
		if st, ok := v.Capabilities["verdict"]["status"].(string); ok {
			out[v.Parent] = st
		}
	}
	return out
}

// newReds is what fails now and did not before: a criterion failing whose
// verdict, before the run, was anything but a fail. A red that was already
// there is not the request's.
func newReds(before map[string]string, out verificationapi.VerifyOutput) []string {
	var reds []string
	for _, o := range out.Outcomes {
		if o.Status == "fail" && before[o.ObjectID] != "fail" && !slices.Contains(reds, o.ObjectID) {
			reds = append(reds, o.ObjectID)
		}
	}
	return reds
}

// moduleExempt says why a request is not held to its module update; "" when
// it is.
func moduleExempt(ctx context.Context, o cwmapi.Object, f api.RequestFields) string {
	since := strings.TrimSpace(planificationConfig(ctx).ModuleUpdate.Since)
	if since == "" {
		return "the module-updated gate is off (no module_update.since)"
	}
	if f.Waiver != nil {
		return "waived by " + f.Waiver.By.ID + ": " + f.Waiver.Note
	}
	if f.Source != nil && f.Source.Kind == api.SourceGate {
		return "filed by a gate"
	}
	if planState(o) == api.PlanSplit {
		return "split: its parts carry its work"
	}
	tmpl, err := templateFor(ctx, o)
	if err != nil || !changesModule(tmpl) {
		return "its template changes no module"
	}
	from, err := parseSince(since)
	if err != nil {
		return fmt.Sprintf("module_update.since %q is not a date: the gate is off", since)
	}
	if start := filingStart(o, f); !start.IsZero() && start.Before(from) {
		return "filed before " + since
	}
	return ""
}

// moduleUpdated answers what keeps a request's module from being up to date —
// nil when it is, or when the request is exempt (why, then). With verify, the
// want it serves is verified once and a check failing that did not fail
// before is a new red; a guard never runs probes, so the gate does not.
func moduleUpdated(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, verify bool) ([]string, string) {
	if !hasRequest(o) {
		return nil, "it carries no request"
	}
	f := fieldsOf(o)
	if why := moduleExempt(ctx, o, f); why != "" {
		return nil, why
	}
	var fails []string
	served := servedOf(ctx, cwm, o)
	direct := served == "" && directWork(ctx, cwm, o)
	if served == "" && !direct {
		fails = append(fails, "it serves no want: name the goal, feature or use case it serves (requests assess --serves), or file the want it needs")
	} else if verify {
		before := verdictsUnder(ctx, cwm, served)
		out, ran, err := verifyServed(ctx, served)
		switch {
		case err != nil:
			fails = append(fails, fmt.Sprintf("the checks of %s could not run: %v", served, err))
		case ran:
			if reds := newReds(before, out); len(reds) > 0 {
				fails = append(fails, fmt.Sprintf("a new red under %s: %s fails now", served, strings.Join(reds, ", ")))
			}
		}
	}
	mu := f.ModuleUpdate
	switch {
	case direct && mu != nil && len(mu.Checks) == 0 && mu.NoCheck != "":
		// Nothing a test can see changed: it says why.
	case mu == nil || len(mu.Checks) == 0:
		fails = append(fails, "no acceptance check added or changed to prove it: record the one that does (requests module-update --checks)")
	default:
		fails = append(fails, checksChanged(ctx, cwm, mu.Checks, filingStart(o, f))...)
	}
	if missing := untracked(ctx, cwm, recordedObjects(o)); len(missing) > 0 {
		fails = append(fails, "what the module update records is not committed - untracked in git: "+strings.Join(missing, ", "))
	}
	if mu == nil || !mu.Recorded() {
		fails = append(fails, "no change-log entry recorded: record the entry that names it (changes add, then requests module-update --changelog <its id>), or that no user sees it and why (--internal --reason)")
	} else if isChangeID(mu.Changelog) {
		if why := entryNames(ctx, cwm, mu.Changelog, o.ID); why != "" {
			fails = append(fails, why)
		}
	}
	return fails, ""
}

// checksChanged says which of the checks recorded is not an acceptance check,
// or has not changed since the request was filed.
func checksChanged(ctx context.Context, cwm cwmapi.CWMService, checks []string, since time.Time) []string {
	journal, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil || journal == nil {
		return []string{"no journal says when its checks changed"}
	}
	var fails []string
	for _, id := range checks {
		if _, err := loadObject(ctx, cwm, id); err != nil {
			fails = append(fails, fmt.Sprintf("check %s does not exist", id))
			continue
		}
		if !carriesCapability(ctx, cwm, id, capCheckable) {
			fails = append(fails, fmt.Sprintf("%s is not an acceptance check (it carries no checkable)", id))
			continue
		}
		t, err := journal.Times(ctx, journalapi.TimesInput{ID: id})
		if err != nil || t.UpdatedAt.IsZero() || !t.UpdatedAt.After(since) {
			fails = append(fails, fmt.Sprintf("check %s has not changed since the request was filed", id))
		}
	}
	return fails
}

// capCheckable is the method's acceptance-check capability.
const capCheckable = "checkable"

// carriesCapability reports an object whose effective capabilities include c.
func carriesCapability(ctx context.Context, cwm cwmapi.CWMService, id, c string) bool {
	eff, err := cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: id})
	if err != nil {
		return false
	}
	return slices.ContainsFunc(eff.Capabilities, func(r cwmapi.CapabilityRef) bool { return r.ID == c })
}

// replanModuleUpdate plans the module-update step again on a request whose
// end was refused, with what failed as its feedback — after maxModuleUpdateRounds
// the request is blocked for a person instead.
func replanModuleUpdate(ctx context.Context, o cwmapi.Object, done []api.WorkTodo, fails []string, blocked func(string) error) error {
	feedback := "its module is not up to date: " + strings.Join(fails, "; ")
	var base *api.WorkTodo
	rounds, last := 0, 0
	for i := range done {
		t := done[i]
		last = max(last, t.Position)
		switch {
		case t.Packet.Step == stepModuleUpdateAgain:
			rounds++
			base = &done[i]
		case t.Packet.Step == StepModuleUpdate:
			base = &done[i]
		case base == nil && t.Packet.Doer != api.DoerPerson && t.Packet.Step != api.StepIntake:
			base = &done[i] // a template folding the update into its one step
		}
	}
	if base == nil || rounds >= maxModuleUpdateRounds {
		return blocked(feedback + " — bring it up to date, or a person waives it (requests waive)")
	}
	p := base.Packet
	p.Step, p.Starts, p.Moves, p.AskedIn, p.Stale, p.EndsOn, p.Verifies, p.Confirms = stepModuleUpdateAgain, "", "", "", "", "", "", ""
	p.Gate, p.Refines, p.HeldBy = GateModuleUpdated, false, nil
	p.Params = map[string]any{"feedback": feedback}
	p.Instructions = moduleUpdateInstructions + "\n\nDo it again: " + feedback
	p.Added(base.Packet, PlanningActor, feedback)
	_, err := put(ctx, api.WorkTodo{Todo: schedapi.Todo{
		Module: base.Module, Target: base.Target, Request: todoRequestRef(ctx, base.Todo), Title: stepModuleUpdateAgain + " — " + titleOf(o),
		For: base.For, Skill: ModuleUpdateSkill, Group: base.Group, Start: base.Start, Priority: base.Priority,
		State: schedapi.StateWaiting, Since: base.Since, Position: last + 1, Reads: base.Reads, Writes: base.Writes,
	}, Packet: p})
	if err != nil {
		return err
	}
	if !collecting(ctx) {
		return settleRequest(ctx, o.ID, PlanningActor)
	}
	return nil
}

// ModuleUpdate records what a request's module-update step did: the change
// log entry naming it — or that no user sees the change, and why — and the
// acceptance checks it added or changed (added to those already recorded).
func (s *Service) ModuleUpdate(ctx context.Context, in api.ModuleUpdateInput) (api.RequestOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.RequestOutput{}, err
	}
	invisible := in.Internal
	switch {
	case invisible && in.Changelog != "":
		return api.RequestOutput{}, invalid("ModuleUpdate: a change no user sees has no change-log entry — give changelog, or internal with a reason")
	case invisible && strings.TrimSpace(in.Reason) == "":
		return api.RequestOutput{}, invalid("ModuleUpdate: say why no user sees the change (reason)")
	}
	o, f, err := openRequest(ctx, "ModuleUpdate", in.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RequestOutput{}, err
	}
	noCheck := strings.TrimSpace(in.NoCheck)
	if noCheck != "" && !directWork(ctx, cwm, o) {
		return api.RequestOutput{}, invalid("ModuleUpdate: no_check is for direct work only (a fix or a quick build serving no want) - %s needs the acceptance check that proves it", in.ID)
	}
	if c := strings.TrimSpace(in.Changelog); c != "" {
		if got, err := loadObject(ctx, cwm, c); err != nil || got.Type != api.ChangeType {
			return api.RequestOutput{}, invalid("ModuleUpdate: the change log is an entry - %q is not one: add it (changes add --requests %s) and give its id", c, in.ID)
		}
	}
	for _, id := range in.Checks {
		if _, err := loadObject(ctx, cwm, id); err != nil {
			return api.RequestOutput{}, invalid("ModuleUpdate: check %q is not the id of an object of the project", id)
		}
		if !carriesCapability(ctx, cwm, id, capCheckable) {
			return api.RequestOutput{}, invalid("ModuleUpdate: %s is not an acceptance check (it carries no checkable)", id)
		}
	}
	mu := api.ModuleUpdate{}
	if f.ModuleUpdate != nil {
		mu = *f.ModuleUpdate
	}
	switch {
	case in.Changelog != "":
		mu.Changelog, mu.UserVisible, mu.Reason = strings.TrimSpace(in.Changelog), nil, ""
	case invisible:
		visible := false
		mu.Changelog, mu.UserVisible, mu.Reason = "", &visible, strings.TrimSpace(in.Reason)
	}
	for _, id := range in.Checks {
		if !slices.Contains(mu.Checks, id) {
			mu.Checks = append(mu.Checks, id)
		}
	}
	if noCheck != "" {
		mu.NoCheck = noCheck
	}
	mu.At = time.Now().UTC().Format(time.RFC3339)
	f.ModuleUpdate = &mu
	out, err := saveRequest(ctx, o, f)
	if err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(out)}, nil
}

// Waive waives the module-updated gate on one open request: by a person only
// — a call from an agent's session is refused whatever by says — saying why.
// The module-update steps planned again and not started leave the queue, and
// a request whose steps are all done ends.
func (s *Service) Waive(ctx context.Context, in api.WaiveInput) (api.RequestOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.RequestOutput{}, err
	}
	if in.By.Kind != "person" {
		return api.RequestOutput{}, invalid("Waive: a person waives a module update, not %s %s", in.By.Kind, in.By.ID)
	}
	relayed, err := personsAct(ctx, in.By, "waive:"+in.ID, "Waive: a person waives it themselves (the request view, or their own command)")
	if err != nil {
		return api.RequestOutput{}, err
	}
	if strings.TrimSpace(in.Note) == "" {
		return api.RequestOutput{}, invalid("Waive: say why its module needs no update (note)")
	}
	o, f, err := openRequest(ctx, "Waive", in.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	f.Waiver = &api.Waiver{By: in.By, At: time.Now().UTC().Format(time.RFC3339), Note: strings.TrimSpace(in.Note), Relayed: relayed}
	out, err := saveRequest(ctx, o, f)
	if err != nil {
		return api.RequestOutput{}, err
	}
	open, err := todosOf(ctx, o.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	for _, t := range open {
		if notStarted(t) && t.Packet.Step == stepModuleUpdateAgain {
			if err := archive(ctx, t.ID, "waived by "+in.By.ID+": "+f.Waiver.Note); err != nil {
				return api.RequestOutput{}, err
			}
		}
	}
	if out, err = endIfDone(ctx, out, f, in.By); err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: toRequest(out)}, nil
}

// CheckModuleUpdated is the module-updated gate's requirement: accepted when
// the request's module is up to date or the request is exempt, refused naming
// what is missing. Read-only, and it runs no probe.
func (s *Service) CheckModuleUpdated(ctx context.Context, in api.ModuleUpdatedInput) (api.CheckReadyOutput, error) {
	id := in.ID
	if id == "" && in.Object != nil {
		id = in.Object.ID
	}
	if id == "" {
		return api.CheckReadyOutput{}, invalid("CheckModuleUpdated: id or object required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.CheckReadyOutput{}, err
	}
	o, err := loadObject(ctx, cwm, id)
	if err != nil {
		return api.CheckReadyOutput{}, err
	}
	fails, exempt := moduleUpdated(ctx, cwm, o, false)
	if len(fails) > 0 {
		return api.CheckReadyOutput{Reason: strings.Join(fails, "; ")}, nil
	}
	return api.CheckReadyOutput{Accept: true, Reason: exempt}, nil
}

// isChangeID reports a module update naming a change-log entry (an object of
// type change) - a record made before the change log was data names a page.
func isChangeID(ref string) bool {
	return strings.Contains(ref, ".CH-")
}

// entryNames says why the change-log entry a module update names does not
// stand for the request: it does not exist, or it does not name it; "" when
// it does.
func entryNames(ctx context.Context, cwm cwmapi.CWMService, id, request string) string {
	o, err := loadObject(ctx, cwm, id)
	if err != nil || o.Type != api.ChangeType {
		return fmt.Sprintf("the change-log entry %s does not exist", id)
	}
	if !slices.Contains(changeFields(o).Requests, request) {
		return fmt.Sprintf("the change-log entry %s does not name %s: changes update --id %s --requests %s", id, request, id, request)
	}
	return ""
}
