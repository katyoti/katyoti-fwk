// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
)

// maxLabel is how long an option's label may be: a label is what a button
// and a `selected` name, the explanation is its prose section.
const maxLabel = 80

// Prepare writes an open decision's preparation: the labels into `options`,
// one prose section per option under `## Options` (a previous one replaced
// whole), and the proposal, which names one of the labels.
func (s *Service) Prepare(ctx context.Context, in api.PrepareInput) (api.PrepareOutput, error) {
	if in.Actor.ID == "" {
		return api.PrepareOutput{}, fmt.Errorf("%w: decisions.Prepare: actor required - who prepares it", cwmapi.ErrInvalidInput)
	}
	if len(in.Options) < 2 {
		return api.PrepareOutput{}, fmt.Errorf("%w: decisions.Prepare: at least two options - one option is not a choice", cwmapi.ErrInvalidInput)
	}
	labels := make([]string, 0, len(in.Options))
	for i, opt := range in.Options {
		label := strings.TrimSpace(opt.Label)
		if err := checkLabel(label); err != nil {
			return api.PrepareOutput{}, fmt.Errorf("%w: decisions.Prepare: option %d: %v", cwmapi.ErrInvalidInput, i+1, err)
		}
		if slices.Contains(labels, label) {
			return api.PrepareOutput{}, fmt.Errorf("%w: decisions.Prepare: option %q is given twice", cwmapi.ErrInvalidInput, label)
		}
		in.Options[i].Label = label
		labels = append(labels, label)
	}
	proposal := api.Proposal{Option: strings.TrimSpace(in.Proposal.Option), Why: strings.TrimSpace(in.Proposal.Why)}
	if !slices.Contains(labels, proposal.Option) {
		return api.PrepareOutput{}, fmt.Errorf("%w: decisions.Prepare: the proposal %q is not one of the options (%s)", cwmapi.ErrInvalidInput, proposal.Option, strings.Join(labels, ", "))
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.PrepareOutput{}, err
	}
	l, err := loadRev(ctx, cwm, in.ID)
	if err != nil {
		return api.PrepareOutput{}, err
	}
	if !isOpen(l.obj, l.st) {
		return api.PrepareOutput{}, fmt.Errorf("%w: decisions.Prepare: %s is %s, not open; supersede it instead", cwmapi.ErrTypeRuleViolation, in.ID, stateOf(l.obj))
	}
	l.st.Options = labels
	l.st.Proposal = &proposal
	l.obj.Body = setSection(l.obj.Body, api.OptionsHeading, optionsProse(in.Options))
	d, err := saveAt(ctx, cwm, l.module, l.obj, l.st, l.revision)
	if err != nil {
		return api.PrepareOutput{}, err
	}
	return api.PrepareOutput{Decision: d}, nil
}

// Challenge records the points raised against an open decision's proposal:
// who, when, each point. A later challenge replaces the earlier one.
func (s *Service) Challenge(ctx context.Context, in api.ChallengeInput) (api.ChallengeOutput, error) {
	if in.Actor.ID == "" {
		return api.ChallengeOutput{}, fmt.Errorf("%w: decisions.Challenge: actor required - who challenges it", cwmapi.ErrInvalidInput)
	}
	points := dedupe(in.Points)
	if len(points) == 0 {
		return api.ChallengeOutput{}, fmt.Errorf("%w: decisions.Challenge: at least one point", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ChallengeOutput{}, err
	}
	l, err := loadRev(ctx, cwm, in.ID)
	if err != nil {
		return api.ChallengeOutput{}, err
	}
	if !isOpen(l.obj, l.st) {
		return api.ChallengeOutput{}, fmt.Errorf("%w: decisions.Challenge: %s is %s, not open; challenge a decision that applies by opening one with a challenges edge", cwmapi.ErrTypeRuleViolation, in.ID, stateOf(l.obj))
	}
	if l.st.Proposal == nil {
		return api.ChallengeOutput{}, fmt.Errorf("%w: decisions.Challenge: %s has no proposal to challenge yet; prepare it first", cwmapi.ErrTypeRuleViolation, in.ID)
	}
	l.st.Challenge = &api.ProposalChallenge{By: in.Actor.ID, At: s.today(), Points: points}
	d, err := saveAt(ctx, cwm, l.module, l.obj, l.st, l.revision)
	if err != nil {
		return api.ChallengeOutput{}, err
	}
	return api.ChallengeOutput{Decision: d}, nil
}

// checkLabel refuses a label a button or a heading cannot carry.
func checkLabel(label string) error {
	switch {
	case label == "":
		return fmt.Errorf("a label is required")
	case strings.ContainsAny(label, "\r\n"):
		return fmt.Errorf("a label is one line: %q", label)
	case strings.HasPrefix(label, "#"):
		return fmt.Errorf("a label does not start with #: %q", label)
	case strings.Contains(label, "${"):
		return fmt.Errorf("a label does not hold ${ (a page's template): %q", label)
	case utf8.RuneCountInString(label) > maxLabel:
		return fmt.Errorf("%q is %d characters: a label is short (at most %d) and its explanation goes in means", label, utf8.RuneCountInString(label), maxLabel)
	}
	return nil
}

// optionsProse is the `## Options` section: one `### <label>` section per
// option, each line a key the panel reads back.
func optionsProse(opts []api.OptionDetail) string {
	var b strings.Builder
	b.WriteString(api.OptionsHeading + "\n")
	for _, o := range opts {
		b.WriteString("\n### " + o.Label + "\n\n")
		line := func(key, v string) {
			if v = oneLine(v); v != "" {
				b.WriteString("- **" + key + ":** " + v + "\n")
			}
		}
		list := func(key string, vs []string) {
			vs = dedupe(vs)
			if len(vs) == 0 {
				return
			}
			b.WriteString("- **" + key + ":**\n")
			for _, v := range vs {
				b.WriteString("  - " + oneLine(v) + "\n")
			}
		}
		line(api.KeyMeans, o.Means)
		list(api.KeyPros, o.Pros)
		list(api.KeyCons, o.Cons)
		line(api.KeyCost, o.Cost)
		line(api.KeyIfChosen, o.Consequence)
	}
	return strings.TrimRight(b.String(), "\n")
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// setSection replaces the level-2 section headed heading (up to the next
// level-2 heading) by section, or appends section when the body has none.
func setSection(body, heading, section string) string {
	lines := strings.Split(body, "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == heading {
			start = i
			break
		}
	}
	if start < 0 {
		return appendProse(body, section)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	before := strings.TrimRight(strings.Join(lines[:start], "\n"), "\n")
	after := strings.TrimLeft(strings.Join(lines[end:], "\n"), "\n")
	out := section + "\n"
	if before != "" {
		out = before + "\n\n" + out
	}
	if after != "" {
		out += "\n" + after
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
	}
	return out
}

// section is the prose under a level-2 heading, up to the next one; "" when
// the body has none.
func section(body, heading string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != heading {
			continue
		}
		var out []string
		for _, x := range lines[i+1:] {
			if strings.HasPrefix(x, "## ") {
				break
			}
			out = append(out, x)
		}
		return strings.TrimSpace(strings.Join(out, "\n"))
	}
	return ""
}

// lastMove is the decision's latest lifecycle move, as the journal derives it
// (the object stores only its state). Best effort: no journal, no move.
func lastMove(ctx context.Context, id string) (cwmapi.LifecycleEntry, bool) {
	journal, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		return cwmapi.LifecycleEntry{}, false
	}
	got, err := journal.Transitions(ctx, journalapi.TransitionsInput{ID: id})
	if err != nil || len(got.Entries) == 0 {
		return cwmapi.LifecycleEntry{}, false
	}
	return got.Entries[len(got.Entries)-1], true
}

// alreadyDecided is the refusal of a second resolve: who decided, when and
// what, read from the last move (the resolved date when the journal is out of
// reach).
func alreadyDecided(ctx context.Context, o cwmapi.Object, st state) error {
	what := strings.Join(st.Selected, ", ")
	if m, ok := lastMove(ctx, o.ID); ok && m.By.ID != "" {
		return fmt.Errorf("%w: decisions.Resolve: %s is already decided by %s on %s (%s): %s",
			cwmapi.ErrTypeRuleViolation, o.ID, m.By.ID, dateOf(m.At), m.To, what)
	}
	if st.Resolved != "" {
		return fmt.Errorf("%w: decisions.Resolve: %s is already decided on %s (%s): %s", cwmapi.ErrTypeRuleViolation, o.ID, st.Resolved, stateOf(o), what)
	}
	return fmt.Errorf("%w: decisions.Resolve: %s is already %s", cwmapi.ErrTypeRuleViolation, o.ID, stateOf(o))
}

// dateOf is the day of an RFC 3339 time (the value itself when it is not one).
func dateOf(at string) string {
	if len(at) >= len(dateLayout) {
		return at[:len(dateLayout)]
	}
	return at
}
