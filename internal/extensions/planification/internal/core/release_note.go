// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A RELEASE'S NOTE IS THE SUM OF ITS INCREMENTS' CHANGES (
// decision OD-014) - each change entry belongs to the increment whose work
// made it (its requests); a release's note is the entries of the increments it
// carries and of its fixes, generated, never written by hand. Each candidate
// gets its letter: the main points only, a few lines per increment, in French.

// Note answers a release's note.
func (r *Releases) Note(ctx context.Context, in api.GetInput) (api.NoteOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.NoteOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.NoteOutput{}, err
	}
	return noteOf(ctx, cwm, rel)
}

func noteOf(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object) (api.NoteOutput, error) {
	out := api.NoteOutput{Release: rel.ID, Increments: []api.IncrementNote{}, Fixes: []api.Change{}}
	carried := carriedIncrements(ctx, cwm, rel)
	if releaseState(rel) == api.ReleasePlanned {
		// A planned release's note counts what rides along in it.
		if sm := edgeTo(rel, api.EdgeInStream); sm != "" {
			if next, ok := nextPlannedRelease(ctx, cwm, sm); ok && next == rel.ID {
				carried = append(carried, ridingAlong(ctx, cwm, sm)...)
			}
		}
	}
	index := map[string]int{}
	for _, inc := range carried {
		index[inc.ID] = len(out.Increments)
		out.Increments = append(out.Increments, api.IncrementNote{ID: inc.ID, Title: titleOf(inc), Changes: []api.Change{}})
	}
	all, err := NewChanges().List(ctx, api.ListChangesInput{})
	if err != nil {
		return out, err
	}
	direct, _ := workOf(ctx, cwm, rel.ID)
	incOf := map[string]string{} // a request's increment, read once
	increment := func(rq string) string {
		if inc, ok := incOf[rq]; ok {
			return inc
		}
		inc := ""
		if o, err := loadObject(ctx, cwm, rq); err == nil {
			inc, _ = releaseOfWork(ctx, cwm, o)
		}
		incOf[rq] = inc
		return inc
	}
	for _, c := range all.Changes {
		// An increment it carries wins over the release itself (as
		// releaseOfWork does); the entry is counted once.
		into, fix := -1, false
		for _, rq := range c.Requests {
			if i, ok := index[increment(rq)]; ok {
				into = i
				break
			}
			if rq == rel.ID || slices.Contains(direct, rq) {
				fix = true
			}
		}
		switch {
		case into >= 0:
			out.Increments[into].Changes = append(out.Increments[into].Changes, c)
		case fix:
			out.Fixes = append(out.Fixes, c)
		}
	}
	return out, nil
}

// ExportLetter writes a candidate's letter to a file: what a delivery's build
// publishes as its solution's « Quoi de neuf ». A candidate
// with no letter is refused (releases letter writes it).
func (r *Releases) ExportLetter(ctx context.Context, in api.ExportLetterInput) (api.ExportLetterOutput, error) {
	if strings.TrimSpace(in.To) == "" {
		return api.ExportLetterOutput{}, invalid("ExportLetter: the file to write (to) is required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ExportLetterOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ExportLetterOutput{}, err
	}
	cands := releaseFields(rel).Candidates
	i := slices.IndexFunc(cands, func(c api.Candidate) bool { return c.Name == in.Candidate })
	if i < 0 {
		return api.ExportLetterOutput{}, invalid("%s has no candidate %q (releases deliver makes one)", in.ID, in.Candidate)
	}
	c := cands[i]
	if strings.TrimSpace(c.Letter) == "" {
		return api.ExportLetterOutput{}, invalid("%s %s has no letter (releases letter writes it)", in.ID, in.Candidate)
	}
	if err := os.WriteFile(in.To, []byte(c.Letter), 0o644); err != nil {
		return api.ExportLetterOutput{}, err
	}
	return api.ExportLetterOutput{Path: in.To, SignedOff: c.SignedOff != nil}, nil
}

// Letter writes a candidate's letter from the release's note.
func (r *Releases) Letter(ctx context.Context, in api.CandidateInput) (api.ReleaseOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.ReleaseOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	note, err := noteOf(ctx, cwm, rel)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	letter := letterOf(titleOf(rel), releaseFields(rel).Version, in.Candidate, note)
	return onCandidate(ctx, in.ID, in.Candidate, func(c *api.Candidate) error {
		if c.SignedOff != nil {
			return invalid("Letter: %s %s is signed off - its letter is what was approved", in.ID, in.Candidate)
		}
		c.Letter = letter
		return nil
	})
}

// letterOf is a delivery's « Quoi de neuf »: per increment its title and the
// titles of its major and notable changes (its first ones when it has none),
// then its corrections in one line.
func letterOf(name, version, candidate string, note api.NoteOutput) string {
	var b strings.Builder
	// A release named by its version (the default) is not named twice.
	if version != "" && name != version {
		name += " " + version
	}
	fmt.Fprintf(&b, "# Quoi de neuf - %s", name)
	if candidate != "" {
		fmt.Fprintf(&b, " (%s)", candidate)
	}
	b.WriteString("\n")
	main := func(cs []api.Change) []api.Change {
		var out []api.Change
		for _, c := range cs {
			if c.Value == api.ValueMajor || c.Value == api.ValueNotable {
				out = append(out, c)
			}
		}
		return out // the main points only: an increment with none is not in the letter
	}
	for _, in := range note.Increments {
		points := main(in.Changes)
		if len(points) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n", in.Title)
		for _, c := range points {
			fmt.Fprintf(&b, "- %s\n", c.Title)
		}
	}
	switch n := len(note.Fixes); {
	case n == 1:
		b.WriteString("\n**Corrections** : 1 correction, détaillée dans le journal des changements.\n")
	case n > 1:
		fmt.Fprintf(&b, "\n**Corrections** : %d corrections, détaillées dans le journal des changements.\n", n)
	}
	return b.String()
}
