// SPDX-License-Identifier: Apache-2.0

// Package core implements the reviews service over the CWMService in its
// context: it reads reviews, findings and the other children of a review with
// the Object API and draws the two panels' pages. It writes when a review is
// filed (file.go: the review and its findings), when the older reviews of a
// type are folded (supersede.go), and starts runs (start.go) through the
// action service.
//
// A review's findings are its own children (the imported reviews' and an
// audit's) and the findings filed under the object it reviewed whose found-in
// or seen-in edge points at it: a finding stays one object across the reviews
// of its type.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// Service implements api.ReviewsService.
type Service struct{}

// New returns the service.
func New() *Service { return &Service{} }

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	c, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil || c == nil {
		return nil, fmt.Errorf("reviews: no CWMService: %v", err)
	}
	return c, nil
}

// get reads one object by its id.
func get(ctx context.Context, c cwmapi.CWMService, id string) (cwmapi.Object, error) {
	if id == "" {
		return cwmapi.Object{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	got, err := c.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return got.Object, nil
}

// children reads the direct children of parent of one type, whole: a listing
// carries what the index holds, not the fields and the state the pages show.
func children(ctx context.Context, c cwmapi.CWMService, parent, typ string) ([]cwmapi.Object, error) {
	out, err := c.ObjectList(ctx, cwmapi.ObjectListInput{Parent: parent, Type: typ})
	if err != nil {
		return nil, err
	}
	objs := make([]cwmapi.Object, 0, len(out.Objects))
	for _, o := range out.Objects {
		full, err := get(ctx, c, o.ID)
		if err != nil {
			return nil, err
		}
		objs = append(objs, full)
	}
	return objs, nil
}

// ReviewOf is a review as the pages read it: its fields, its state and its
// findings.
type ReviewOf struct {
	Object   cwmapi.Object
	Fields   api.ReviewFields
	State    string
	Findings []FindingOf
}

// FindingOf is a finding as the pages read it: FoundIn is the review that
// raised it, SeenIn the latest review that saw it (both empty for a finding
// filed under its review).
type FindingOf struct {
	Object  cwmapi.Object
	Fields  api.FindingFields
	State   string
	About   string
	FoundIn string
	SeenIn  string
}

func readFinding(f cwmapi.Object) FindingOf {
	fo := FindingOf{Object: f, State: stateOf(f, api.FindingOpen), About: f.EdgeTo(api.EdgeAbout),
		FoundIn: f.EdgeTo(api.EdgeFoundIn), SeenIn: f.EdgeTo(api.EdgeSeenIn)}
	decodeFields(f, &fo.Fields)
	return fo
}

// Open says whether the finding still asks for something: open or addressed.
func (f FindingOf) Open() bool {
	return f.State == "" || f.State == api.FindingOpen || f.State == api.FindingAddressed
}

func decodeFields(o cwmapi.Object, into any) {
	if len(o.Fields) == 0 {
		return
	}
	b, err := json.Marshal(o.Fields)
	if err == nil {
		_ = json.Unmarshal(b, into)
	}
}

func stateOf(o cwmapi.Object, initial string) string {
	if l, ok := o.Lifecycle(); ok && l.State != "" {
		return l.State
	}
	return initial
}

// ReadReview reads a review's fields, state and findings: its children, and
// the findings under the object it reviewed that it raised or saw.
func ReadReview(ctx context.Context, c cwmapi.CWMService, o cwmapi.Object) (ReviewOf, error) {
	r := ReviewOf{Object: o, State: stateOf(o, "draft")}
	decodeFields(o, &r.Fields)
	fs, err := children(ctx, c, o.ID, api.FindingType)
	if err != nil {
		return r, err
	}
	for _, f := range fs {
		r.Findings = append(r.Findings, readFinding(f))
	}
	if o.Parent != "" {
		under, err := children(ctx, c, o.Parent, api.FindingType)
		if err != nil {
			return r, err
		}
		for _, f := range under {
			if fo := readFinding(f); fo.FoundIn == o.ID || fo.SeenIn == o.ID {
				r.Findings = append(r.Findings, fo)
			}
		}
	}
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := SeverityRank(r.Findings[i].Fields.Severity), SeverityRank(r.Findings[j].Fields.Severity)
		if a != b {
			return a < b
		}
		return r.Findings[i].Object.ID < r.Findings[j].Object.ID
	})
	return r, nil
}

// ReviewsOf is the Reviews panel: the reviews filed under in.ID.
func (s *Service) ReviewsOf(ctx context.Context, in api.ObjectInput) (render.Def, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	obj, err := get(ctx, c, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	list, err := children(ctx, c, obj.ID, api.ReviewType)
	if err != nil {
		return render.Def{}, err
	}
	reviews := make([]ReviewOf, 0, len(list))
	for _, o := range list {
		r, err := ReadReview(ctx, c, o)
		if err != nil {
			return render.Def{}, err
		}
		reviews = append(reviews, r)
	}
	var picker render.Node
	if types, err := s.TypesFor(ctx, api.TypesInput{ID: obj.ID}); err == nil {
		provider, _ := application.SelfProviderFromContext(ctx)
		if provider == "" {
			provider = "ktw-agents"
		}
		picker = startPicker(in.Language, provider, obj.ID, types)
	}
	dimOf := map[string]string{}
	if types, err := ReviewTypes(ctx); err == nil {
		for _, t := range types {
			dimOf[t.Name] = t.Dimension
		}
	}
	return ReviewsPage(in.Language, obj, reviews, picker, scorecard(in.Language, dimOf, obj, reviews)), nil
}

// Review is the Review panel: a review, its findings, its documents and decks.
func (s *Service) Review(ctx context.Context, in api.ObjectInput) (render.Def, error) {
	c, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	obj, err := get(ctx, c, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	if obj.Type != api.ReviewType {
		return render.Def{}, fmt.Errorf("%w: %s is a %s, not a review", cwmapi.ErrInvalidInput, obj.ID, obj.Type)
	}
	r, err := ReadReview(ctx, c, obj)
	if err != nil {
		return render.Def{}, err
	}
	docs, err := children(ctx, c, obj.ID, api.DocumentType)
	if err != nil {
		return render.Def{}, err
	}
	decks, err := children(ctx, c, obj.ID, api.DeckType)
	if err != nil {
		return render.Def{}, err
	}
	cmp, err := CompareOf(ctx, c, r)
	if err != nil {
		return render.Def{}, err
	}
	return ReviewPage(in.Language, r, docs, decks, compareSection(in.Language, r, cmp)), nil
}
