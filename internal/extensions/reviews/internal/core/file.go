// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// File files one review type's result on the object reviewed. The review is
// created under the object (id minted, RV-NNN) with the result's report as
// its body; then, against the object's open findings of that type:
//   - a result finding whose fingerprint an open finding carries is that
//     finding seen again: its fields refreshed, its seen-in moved here;
//   - any other is a new finding under the object, found-in and seen-in here;
//   - an open finding of the type the result does not hold is resolved:
//     addressed then closed, the note naming this review.
//
// A manual review is held in-review for a person; an automatic one is closed.
// Then the older reviews of the type are folded away (Supersede). Files names
// every file the filing wrote or removed, the fold's included.
func (s *Service) File(ctx context.Context, in api.FileInput) (api.FileOutput, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return api.FileOutput{}, err
	}
	if in.Object == "" || in.Type == "" {
		return api.FileOutput{}, fmt.Errorf("%w: object and type required", cwmapi.ErrInvalidInput)
	}
	obj, err := get(ctx, c, in.Object)
	if err != nil {
		return api.FileOutput{}, err
	}
	module, err := cwmapi.ModuleOfObjectID(obj.ID)
	if err != nil {
		return api.FileOutput{}, err
	}
	if in.Result.Coverage == api.CoverageNone {
		// A run that checked nothing judges nothing: no review, the previous
		// one and its open findings stand.
		why := in.Result.Summary
		if why == "" {
			why = "the run checked nothing"
		}
		return api.FileOutput{Skipped: why}, nil
	}
	trigger := in.Trigger
	if trigger == "" {
		trigger = api.TriggerManual
	}
	by := cwmapi.Actor{ID: "reviews", Kind: cwmapi.ActorAgent}

	fields := api.ReviewFields{Type: in.Type, Scope: obj.ID, ReviewedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Commit: in.Commit, Trigger: trigger, RequestedBy: in.RequestedBy, Run: in.Run, Reviewers: in.Reviewers,
		Rating: in.Result.Rating, Score: in.Result.Score, Summary: in.Result.Summary}
	review := cwmapi.Object{Type: api.ReviewType, Parent: obj.ID, Name: reviewName(in.Type, fields.ReviewedAt),
		Capabilities: map[string]map[string]any{}}
	review.Fields = asMap(fields)
	review.Body = withoutEchoes(in.Result.Report, review.Fields)
	created, err := c.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: review})
	if err != nil {
		return api.FileOutput{}, fmt.Errorf("file review: %w", err)
	}
	rid := created.Object.ID
	out := api.FileOutput{Review: rid}

	under, err := children(ctx, c, obj.ID, api.FindingType)
	if err != nil {
		return out, err
	}
	open := map[string]FindingOf{}
	for _, f := range under {
		fo := readFinding(f)
		if fo.Fields.FoundBy == in.Type && fo.Open() && fo.Fields.Fingerprint != "" {
			open[fo.Fields.Fingerprint] = fo
		}
	}
	seen := map[string]bool{}
	for _, rf := range in.Result.Findings {
		key := rf.Key
		if key == "" {
			key = rf.Statement
		}
		fp := api.Fingerprint(in.Type, rf.Location, key)
		if seen[fp] {
			continue // the same finding twice in one result
		}
		seen[fp] = true
		ff := api.FindingFields{Severity: rf.Severity, Statement: rf.Statement, Location: rf.Location,
			Recommendation: rf.Recommendation, FoundBy: in.Type, Fingerprint: fp}
		if prev, ok := open[fp]; ok {
			if err := refresh(ctx, c, module, prev.Object.ID, ff, rf.Evidence, rid); err != nil {
				return out, err
			}
			out.Persisting = append(out.Persisting, prev.Object.ID)
			continue
		}
		f := cwmapi.Object{Type: api.FindingType, Parent: obj.ID, Name: findingName(rf.Statement),
			Capabilities: map[string]map[string]any{},
			Edges:        []cwmapi.ObjectEdge{{Kind: api.EdgeAbout, To: obj.ID}, {Kind: api.EdgeFoundIn, To: rid}, {Kind: api.EdgeSeenIn, To: rid}}}
		f.Fields = asMap(ff)
		f.Body = withoutEchoes(rf.Evidence, f.Fields)
		got, err := c.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: f})
		if err != nil {
			return out, fmt.Errorf("file finding %q: %w", rf.Statement, err)
		}
		out.New = append(out.New, got.Object.ID)
	}
	for fp, f := range open {
		if seen[fp] || in.Result.Coverage == api.CoveragePartial {
			continue // a partial run says nothing of what it did not check
		}
		note := "not seen by " + rid
		for _, to := range []string{api.FindingAddressed, api.FindingClosed} {
			if f.State == api.FindingAddressed && to == api.FindingAddressed {
				continue
			}
			if _, err := c.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f.Object.ID, To: to, Note: note, By: by}); err != nil {
				return out, fmt.Errorf("resolve %s: %w", f.Object.ID, err)
			}
		}
		out.Resolved = append(out.Resolved, f.Object.ID)
	}

	states := []string{"in-review"}
	if trigger == api.TriggerAutomatic {
		states = append(states, "closed")
	}
	for _, to := range states {
		if _, err := c.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: rid, To: to, Note: "review filed", By: by}); err != nil {
			return out, fmt.Errorf("review %s to %s: %w", rid, to, err)
		}
	}
	// The files of what the filing wrote, resolved before the fold can
	// remove any of them; then the fold's own.
	files := newFileSet(ctx, c)
	for _, ids := range [][]string{{rid}, out.New, out.Persisting, out.Resolved} {
		for _, id := range ids {
			files.add(id)
		}
	}
	sup, err := s.Supersede(ctx, api.SupersedeInput{ID: rid})
	if err != nil {
		return out, err
	}
	out.Supersede = sup
	files.merge(sup.Files)
	out.Files = files.sorted()
	return out, nil
}

// refresh writes a finding seen again: its fields from this result, its
// evidence when the result gives one, its seen-in moved to the review.
func refresh(ctx context.Context, c cwmapi.CWMService, module, id string, ff api.FindingFields, evidence, review string) error {
	got, err := c.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return err
	}
	o := got.Object
	o.Fields = asMap(ff)
	if evidence != "" {
		o.Body = evidence
	}
	o.Body = withoutEchoes(o.Body, o.Fields)
	edges := make([]cwmapi.ObjectEdge, 0, len(o.Edges)+1)
	for _, e := range o.Edges {
		if e.Kind != api.EdgeSeenIn {
			edges = append(edges, e)
		}
	}
	o.Edges = append(edges, cwmapi.ObjectEdge{Kind: api.EdgeSeenIn, To: review})
	_, err = c.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o, Revision: got.Revision})
	return err
}

func asMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

// withoutEchoes is the body a review or a finding is filed with: the
// runner's prose without any of the object's field values it restates. A
// datum lives in its field OR in the prose, never both (CWM refuses an
// object whose body repeats a field), and a runner's report commonly opens
// with its summary — go-checks' is its whole body when nothing ran. Only
// values long enough to be prose (as CWM counts them) are taken out, the
// longest first; the blank lines a removal leaves are folded.
func withoutEchoes(body string, fields map[string]any) string {
	var values []string
	for _, v := range fields {
		if s, ok := v.(string); ok && len(strings.TrimSpace(s)) >= 20 {
			values = append(values, strings.TrimSpace(s))
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	cut := false
	for _, v := range values {
		if strings.Contains(body, v) {
			body = strings.ReplaceAll(body, v, "")
			cut = true
		}
	}
	if !cut {
		return body
	}
	lines := strings.Split(body, "\n")
	kept := lines[:0]
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" && len(kept) > 0 && kept[len(kept)-1] == "" {
			continue
		}
		kept = append(kept, l)
	}
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	if out == "" {
		return ""
	}
	return out + "\n"
}

func reviewName(typ, at string) string { return typ + " review, " + day(at) }

func findingName(statement string) string {
	s := strings.TrimSpace(statement)
	if r := []rune(s); len(r) > 80 {
		s = string(r[:79]) + "…"
	}
	if s == "" {
		return "Finding"
	}
	return s
}
