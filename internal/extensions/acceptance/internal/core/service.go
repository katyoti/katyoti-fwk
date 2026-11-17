// SPDX-License-Identifier: Apache-2.0

// Package core implements acceptance over the CWM Object API.
package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/projectcmd"
	"github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Service verifies one criterion at a time.
//
// WHERE THE EVIDENCE LANDS: a verdict object is filed in the module its
// criterion belongs to (the verdict type's folder), with the criterion as its
// parent, so it travels with what it is about. ONE VERDICT PER CRITERION,
// upserted on a derived id: a re-run replaces the record, so a committed folder
// stays bounded (the history of runs is the journal's).
//
// ONLY A RUN PRODUCES A VERDICT. The probe is run here (runProbe): an
// expression through CWM's ObjectEvaluate, a command in the project the
// universe serves.
type Service struct {
	now          func() time.Time
	probeTimeout time.Duration
}

// New builds the service; a command probe runs up to api.ProbeTimeout, or to
// the bound its checkable declares (`timeout`).
func New() *Service { return &Service{now: time.Now, probeTimeout: api.ProbeTimeout} }

// WithProbeTimeout bounds a command probe that declares no bound at d instead
// (a test's short bound).
func (s *Service) WithProbeTimeout(d time.Duration) *Service {
	s.probeTimeout = d
	return s
}

// Verify runs one criterion's check and upserts its verdict.
func (s *Service) Verify(ctx context.Context, in api.VerifyInput) (api.VerifyResult, error) {
	id := in.ID
	if id == "" && in.Object != nil {
		id = in.Object.ID
	}
	if id == "" {
		return api.VerifyResult{}, fmt.Errorf("%w: id (or object) required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.VerifyResult{}, err
	}
	module, local := splitID(id)
	if module == "" {
		return api.VerifyResult{}, fmt.Errorf("%w: criterion %q names no module, so its verdict has nowhere to be filed", cwmapi.ErrInvalidInput, id)
	}
	// REFUSE RATHER THAN MISFILE: where a verdict is written is its TYPE's to
	// say; without the type, the record would land where nothing decided.
	if _, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: api.VerdictType}); err != nil {
		return api.VerifyResult{}, fmt.Errorf("no object-type %q is declared, so the verdict of %s would be filed where nothing decided it belongs: %w", api.VerdictType, id, err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return api.VerifyResult{}, err
	}
	criterion := got.Object

	kind, probe, declared := probeOf(criterion)
	var res api.VerifyResult
	if bound, extended, err := projectcmd.ParseTimeout(declared); err != nil {
		// A bound that cannot be read is never guessed at: the probe is not run.
		res = api.VerifyResult{Status: "harness-error", Evidence: "the checkable's " + err.Error() + ": the probe was not run"}
	} else if extended && kind == "command" {
		res = runProbe(ctx, cwm, id, kind, probe, true, bound)
		res.Timeout = declared
	} else {
		res = runProbe(ctx, cwm, id, kind, probe, true, s.probeTimeout)
	}
	verdict, err := s.writeVerdict(ctx, cwm, module, local, criterion, kind, probe, res)
	if err != nil {
		return api.VerifyResult{}, fmt.Errorf("record the verdict of %s: %w", id, err)
	}
	res.Verdict = verdict
	return res, nil
}

// Probe runs a probe over an object and records nothing — a policy's verify
// entry, whose probe is the entry's declared input.
func (s *Service) Probe(ctx context.Context, in api.ProbeInput) (api.VerifyResult, error) {
	if in.ID == "" {
		return api.VerifyResult{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.VerifyResult{}, err
	}
	return runProbe(ctx, cwm, in.ID, strings.TrimSpace(in.Kind), strings.TrimSpace(in.Probe), false, 0), nil
}

// probeOf reads a criterion's probe, how it runs and the bound it declares
// from its `checkable` group.
func probeOf(criterion cwmapi.Object) (kind, probe, timeout string) {
	if grp, ok := criterion.Capability(cwmapi.CapCheckable); ok {
		probe, _ = grp["probe"].(string)
		kind, _ = grp["kind"].(string)
		timeout, _ = grp["timeout"].(string)
	}
	return strings.TrimSpace(kind), strings.TrimSpace(probe), strings.TrimSpace(timeout)
}

// runProbe answers one probe: an expression is evaluated by CWM over the
// object. A command runs in the project the universe serves (exit 0 passes,
// its output the evidence; a test command that ran no
// test fails, projectcmd.RanNoTest) when it is an acceptance criterion's — authored for
// that object; a policy's command (Probe, issued onto every object it applies
// to) is the method harness's to run: it answers needs-harness. A probe that
// could not be ASKED is a harness error, never green. A command runs up to
// timeout; one that overruns is stopped and FAILS, its evidence saying it timed
// out ("timed out after 600s: <its output>").
func runProbe(ctx context.Context, cwm cwmapi.CWMService, id, kind, probe string, runCommand bool, timeout time.Duration) api.VerifyResult {
	if kind == "" {
		kind = "expression" // cap-checkable's default
	}
	switch kind {
	case "none":
		return api.VerifyResult{Status: "no-probe", Evidence: "kind: none — nothing declared"}
	case "manual":
		return api.VerifyResult{Status: "harness-error", Evidence: "manual probe: needs a person's verdict"}
	case "command":
		if probe == "" {
			return api.VerifyResult{Status: "no-probe", Evidence: "command kind with no probe"}
		}
		if !runCommand {
			return api.VerifyResult{Status: "needs-harness", Evidence: "a policy's command probe — run by the method harness, not here: " + probe}
		}
		out, err := projectcmd.Run(ctx, probe, timeout)
		if err != nil {
			return api.VerifyResult{Status: "fail", Evidence: strings.TrimSpace(err.Error() + ": " + projectcmd.Tail(out, 1500))}
		}
		if why := projectcmd.RanNoTest(probe, out); why != "" {
			// A probe that runs nothing proves nothing: a fail saying why.
			return api.VerifyResult{Status: "fail", Evidence: strings.TrimSpace(why + ": " + projectcmd.Tail(out, 1500))}
		}
		return api.VerifyResult{Status: "pass", Evidence: projectcmd.Tail(out, 1500)}
	case "expression":
	default:
		return api.VerifyResult{Status: "harness-error", Evidence: fmt.Sprintf("kind %q is not command, expression, manual or none", kind)}
	}
	if probe == "" {
		return api.VerifyResult{Status: "no-probe", Evidence: "no probe declared"}
	}
	out, err := cwm.ObjectEvaluate(ctx, cwmapi.ObjectEvaluateInput{ID: id, Expressions: []string{probe}})
	switch {
	case err != nil:
		return api.VerifyResult{Status: "harness-error", Evidence: err.Error()}
	case len(out.Results) != 1:
		return api.VerifyResult{Status: "harness-error", Evidence: "no result"}
	case out.Results[0].Error != "":
		return api.VerifyResult{Status: "harness-error", Evidence: "probe not evaluable: " + out.Results[0].Error}
	case out.Results[0].Value == true:
		return api.VerifyResult{Status: "pass"}
	case out.Results[0].Value == false:
		return api.VerifyResult{Status: "fail", Evidence: "probe evaluated false"}
	}
	return api.VerifyResult{Status: "harness-error", Evidence: fmt.Sprintf("probe answered %v, not true or false", out.Results[0].Value)}
}

// writeVerdict upserts the one verdict of a criterion, on <module>.verdict-<local>.
func (s *Service) writeVerdict(ctx context.Context, cwm cwmapi.CWMService, module, local string, criterion cwmapi.Object, kind, probe string, res api.VerifyResult) (string, error) {
	id := module + ".verdict-" + local
	if kind == "" {
		kind = "expression" // cap-checkable's default, recorded as it ran
	}
	title := "Verdict: " + criterion.DisplayTitle()
	obj := cwmapi.Object{ID: id, Type: api.VerdictType}
	obj.Name = title
	obj.Parent = criterion.ID // the criterion it is about: its parent, never a field
	grp := map[string]any{
		"status":   res.Status,
		"ran_at":   s.now().UTC().Format(time.RFC3339),
		"probe":    probe,
		"kind":     kind,
		"evidence": res.Evidence,
	}
	if res.Timeout != "" {
		grp["timeout"] = res.Timeout // it ran on an extended bound
	}
	_ = obj.SetCapability(api.CapVerdict, grp)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	err = cwmapi.Restore(err) // CWM may be another process: its error then crosses as text
	switch {
	case err == nil:
		_, err = cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: obj, Revision: got.Revision})
	case errors.Is(err, cwmapi.ErrNotFound):
		_, err = cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: obj})
	}
	return id, err
}

// splitID splits <module>.<local> on the first dot.
func splitID(id string) (module, local string) {
	if i := strings.Index(id, "."); i > 0 {
		return id[:i], id[i+1:]
	}
	return "", id
}
