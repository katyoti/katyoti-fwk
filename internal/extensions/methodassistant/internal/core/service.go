// SPDX-License-Identifier: Apache-2.0

// Package core implements the method-assistant service over the CWM Object
// API: it gathers the method's objects in an object's scope, runs the pure
// method-core derivation (code/assistant) on them, and writes the alignment
// fixes back through CWM. Nothing is stored here.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/assistant"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	featuresapi "github.com/katyoti/katyoti-fwk/internal/extensions/features/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// Service implements api.MethodAssistantService.
type Service struct {
	listRequests RequestLister
	readScope    ScopeReader
	listReleases ReleaseLister
}

// RequestLister lists a module's requests (planification's List). The caller
// wires it - over the bus, in local/ - so this package speaks to no transport.
type RequestLister func(ctx context.Context, in planapi.ListInput) (planapi.ListOutput, error)

// New returns the service; list answers What's Next's requests to triage
// (nil: none are shown).
func New(list RequestLister) *Service { return &Service{listRequests: list} }

var _ api.MethodAssistantService = (*Service)(nil)

// FeatureType is the method's want node under a goal — the type the
// assistant creates (shape), the Generate Features task creates, and the only
// type it reads as one.
const FeatureType = "feature"

// CriterionType is the method's smallest gradeable unit: an acceptance
// criterion, under the feature it proves. VerdictType is the record of its
// last run, under it.
const (
	CriterionType = "acceptance-criterion"
	VerdictType   = "verdict"
)

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	return application.UseService[cwmapi.CWMService](ctx)
}

// Overview is What's Next on the object: the counts, findings and ranked
// actions of its scope, and the goals and features with their criteria.
func (s *Service) Overview(ctx context.Context, in api.ObjectInput) (api.OverviewOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.OverviewOutput{}, err
	}
	if in.ID == "" {
		return api.OverviewOutput{}, fmt.Errorf("%w: id is required", cwmapi.ErrInvalidInput)
	}
	snap, err := Gather(ctx, cwm, in.ID)
	if err != nil {
		return api.OverviewOutput{}, err
	}
	if snap, _, err = s.scoped(ctx, cwm, snap, in.Release); err != nil {
		return api.OverviewOutput{}, err
	}
	out := api.OverviewOutput{
		Overview: assistant.ComputeOverview(snap),
		Findings: assistant.Findings(snap),
		Actions:  assistant.NextActions(snap),
		Act:      api.ActURI,
	}
	out.Goals, out.Stories, out.LooseCriteria = views(snap)
	return out, nil
}

// Act runs the top action of the asked kind in the object's scope — the same
// scope its Overview showed.
func (s *Service) Act(ctx context.Context, in api.ActInput) (api.ActOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ActOutput{}, err
	}
	if in.ID == "" {
		return api.ActOutput{}, fmt.Errorf("%w: id is required", cwmapi.ErrInvalidInput)
	}
	snap, err := Gather(ctx, cwm, in.ID)
	if err != nil {
		return api.ActOutput{}, err
	}
	if snap, _, err = s.scoped(ctx, cwm, snap, in.Release); err != nil {
		return api.ActOutput{}, err
	}
	a, found := TopAction(snap, in.Kind)
	if !found {
		return api.ActOutput{Message: "no matching action"}, nil
	}
	msg, ok := RunAction(ctx, cwm, a, snap)
	return api.ActOutput{Action: a.Kind, Message: msg, OK: ok}, nil
}

// TopAction is the highest-ranked action of the kind (of any kind when kind
// is empty).
func TopAction(snap assistant.Snapshot, kind string) (assistant.Action, bool) {
	for _, a := range assistant.NextActions(snap) {
		if kind == "" || a.Kind == kind {
			return a, true
		}
	}
	return assistant.Action{}, false
}

// Composition lists the object's direct goals and features, read whole so
// the view has their fields.
func (s *Service) Composition(ctx context.Context, in api.ObjectInput) (api.CompositionOutput, error) {
	if in.ID == "" {
		return api.CompositionOutput{}, fmt.Errorf("%w: id is required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.CompositionOutput{}, err
	}
	listed, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: in.ID})
	if err != nil {
		return api.CompositionOutput{}, err
	}
	objs, err := cwmapi.WholeObjects(ctx, cwm, listed.Objects, "goal", FeatureType)
	if err != nil {
		return api.CompositionOutput{}, err
	}
	out := api.CompositionOutput{Goals: []api.Item{}, Features: []api.Item{}}
	titles := map[string]string{}
	var served []string
	for _, o := range objs {
		it := api.Item{ID: o.ID, Type: o.Type, Title: o.DisplayTitle(), Tags: o.Tags, Summary: o.Description, Fields: o.Fields}
		if l, ok := o.Lifecycle(); ok {
			it.Status = l.State
		}
		switch o.Type {
		case "goal":
			titles[o.ID] = it.Title
			out.Goals = append(out.Goals, it)
		case FeatureType:
			for _, e := range o.Edges {
				if string(e.Kind) == featuresapi.EdgeServes {
					it.Serves = append(it.Serves, api.Ref{ID: e.To})
					served = append(served, e.To)
				}
			}
			out.Features = append(out.Features, it)
		}
	}
	nameServed(ctx, cwm, in.ID, out.Features, served, titles, in.Language)
	if out.Tree, err = planningTree(ctx, cwm, in.ID, in.Advanced); err != nil {
		return api.CompositionOutput{}, err
	}
	out.Advanced = in.Advanced
	return out, nil
}

// planningTree is the goals and features under an object as planning reads
// them: each goal at the top (or under the goal above it), each feature under
// the feature it breaks down, else under the first goal of the scope it
// serves, else at the top. A row is its title and state; advanced adds its
// cost of delay (and its parts) and its reachability.
func planningTree(ctx context.Context, cwm cwmapi.CWMService, id string, advanced bool) ([]api.TreeRow, error) {
	tree, err := cwm.ObjectTree(ctx, cwmapi.ObjectTreeInput{Root: id})
	if err != nil {
		return nil, err
	}
	whole := cwmapi.NewWholeReader(ctx, cwm, "goal", FeatureType)
	var rows []api.TreeRow
	inScope := map[string]bool{}
	var walk func(nodes []cwmapi.ObjectTreeNode, above string)
	walk = func(nodes []cwmapi.ObjectTreeNode, above string) {
		for _, n := range nodes {
			next := above
			if n.Object.Type == "goal" || n.Object.Type == FeatureType {
				o, err := whole.One(n.Object)
				if err != nil {
					o = n.Object
				}
				r := api.TreeRow{ID: o.ID, Parent: above, Title: o.DisplayTitle(), Type: o.Type, Meta: o.Type}
				if l, ok := o.Lifecycle(); ok {
					r.State = l.State
				}
				if r.State == "" {
					r.State = "idea"
				}
				if o.Type == FeatureType && above == "" {
					for _, e := range o.Edges {
						if string(e.Kind) == featuresapi.EdgeServes {
							r.Parent = "serves:" + e.To // resolved once every goal is known
							break
						}
					}
				}
				if advanced {
					r.Meta += planningFields(o)
				}
				inScope[o.ID] = true
				rows = append(rows, r)
				next = o.ID
			}
			walk(n.Children, next)
		}
	}
	walk(tree.Roots, "")
	for i := range rows {
		if g, ok := strings.CutPrefix(rows[i].Parent, "serves:"); ok {
			rows[i].Parent = ""
			if inScope[g] {
				rows[i].Parent = g
			}
		}
	}
	return rows, nil
}

// planningFields is a row's planning fields: its cost of delay and its parts,
// and its reachability.
func planningFields(o cwmapi.Object) string {
	c := featuresapi.CoD(o)
	s := fmt.Sprintf(" · CoD %d (value %d, time %d, risk %d)", c.Sum(), c.BusinessValue, c.TimeCriticality, c.RiskOpportunity)
	if w, ok := o.Capabilities[cwmapi.CapWant]["reachability"].(string); ok && w != "" {
		s += " · " + w
	}
	return s
}

// nameServed titles the goals each feature under holder serves — a goal of
// another module included — and says them as the line shown under the
// feature. A goal the index does not know keeps its id. The goals are read in
// holder's project (a serves edge never crosses one): with several projects
// served, a bare id would answer every project's homonym.
func nameServed(ctx context.Context, cwm cwmapi.CWMService, holder string, features []api.Item, served []string, titles map[string]string, lang string) {
	var missing []string
	for _, id := range served {
		if _, ok := titles[id]; !ok {
			missing = append(missing, id)
		}
	}
	if ends, err := paneapi.EdgeEnds(ctx, cwm, holder, missing); err == nil {
		if rows, err := paneapi.ReadRowsOf(ctx, cwm, ends); err == nil {
			for _, r := range rows {
				titles[r.ID] = r.Name
			}
		}
	}
	label := "Serves: "
	if lang == "fr" {
		label = "Sert : "
	}
	for i := range features {
		var names []string
		for j, g := range features[i].Serves {
			title := titles[g.ID]
			if title == "" {
				title = g.ID
			}
			features[i].Serves[j].Title = title
			names = append(names, title)
		}
		if len(names) > 0 {
			features[i].ServesText = label + strings.Join(names, " · ")
		}
	}
}

// Gather assembles the assistant Snapshot from the subtree under rootID (the
// whole index when rootID is empty): its goals and features read whole (the
// index is resident-only), a feature's serves from its own edges, and each
// acceptance criterion with the nearest feature above it and the status of
// its verdict. A criterion counts for every feature above it, so a feature's
// criteria are those of its sub-features too. It is the one gather: the
// panels' verbs and the method-overview / method-act commands read the
// method through it.
func Gather(ctx context.Context, cwm cwmapi.CWMService, rootID string) (assistant.Snapshot, error) {
	tree, err := cwm.ObjectTree(ctx, cwmapi.ObjectTreeInput{Root: rootID})
	if err != nil {
		return assistant.Snapshot{}, err
	}
	if tree.Total == 0 && rootID != "" {
		return assistant.Snapshot{}, fmt.Errorf("%w: %s is not an object", cwmapi.ErrInvalidInput, rootID)
	}
	within := rootID
	// A PROJECT holds its modules through the index, not through
	// container.parent — a module object is parentless — so its own subtree is
	// the project alone. Its scope is everything the index holds, as the
	// objects tree draws it.
	if len(tree.Roots) == 1 && tree.Roots[0].Level == cwmapi.CapProject && tree.Total == 1 {
		if all, err := cwm.ObjectTree(ctx, cwmapi.ObjectTreeInput{}); err == nil && all.Total > 0 {
			tree, within = all, ""
		}
	}
	verdicts := verdictsWithin(ctx, cwm, within)
	var snap assistant.Snapshot
	servesInv := map[string][]string{}
	featureAt := map[string]int{}
	whole := cwmapi.NewWholeReader(ctx, cwm, "goal", FeatureType)
	// above is the chain of features over the nodes walked, nearest last.
	var walk func(nodes []cwmapi.ObjectTreeNode, above []string)
	walk = func(nodes []cwmapi.ObjectTreeNode, above []string) {
		for _, n := range nodes {
			under := above
			switch n.Object.Type {
			case "goal", FeatureType:
				// An object whose file cannot be read still counts, from
				// its resident record.
				o, err := whole.One(n.Object)
				if err != nil {
					o = n.Object
				}
				if o.Type == "goal" {
					body := o.FieldsJSON()
					snap.Goals = append(snap.Goals, assistant.Goal{ID: o.ID, Title: o.DisplayTitle(), Scope: bodyStr(body, "scope"), CoD: featuresapi.FieldsCoD(body)})
					break
				}
				var serves []string
				for _, e := range o.Edges {
					if string(e.Kind) == featuresapi.EdgeServes {
						serves = append(serves, e.To)
						servesInv[e.To] = append(servesInv[e.To], o.ID)
					}
				}
				featureAt[o.ID] = len(snap.Features)
				snap.Features = append(snap.Features, assistant.Feature{ID: o.ID, Title: o.DisplayTitle(), CoD: featuresapi.CoD(o), Serves: serves})
				under = append(append([]string(nil), above...), o.ID)
			case CriterionType:
				c := assistant.Criterion{ID: n.Object.ID, Title: n.Object.DisplayTitle(), Status: verdicts[n.Object.ID], ParentID: n.Object.ParentID()}
				if len(above) > 0 {
					c.FeatureID = above[len(above)-1]
				}
				for _, f := range above {
					i := featureAt[f]
					snap.Features[i].Criteria = append(snap.Features[i].Criteria, c.ID)
				}
				snap.Criteria = append(snap.Criteria, c)
			}
			walk(n.Children, under)
		}
	}
	walk(tree.Roots, nil)
	for i := range snap.Goals {
		snap.Goals[i].ServingUS = servesInv[snap.Goals[i].ID]
	}
	return snap, nil
}

// verdictsWithin is the status of each criterion's verdict under within (the
// whole index when empty), by criterion: a verdict sits under the criterion
// it records. None when the search fails: every criterion then reads as
// never run, never as passing.
func verdictsWithin(ctx context.Context, cwm cwmapi.CWMService, within string) map[string]string {
	out := map[string]string{}
	got, err := cwm.ObjectSearch(ctx, cwmapi.ObjectSearchInput{Within: within, Types: []string{VerdictType}})
	if err != nil {
		return out
	}
	for _, v := range got.Objects {
		if st, ok := v.Capabilities[VerdictType]["status"].(string); ok {
			out[v.ParentID()] = st
		}
	}
	return out
}

// views turns the snapshot into the navigable view model: the goal roll-ups,
// the features with their criteria inline (highest CoD first), and the
// criteria under no feature.
func views(snap assistant.Snapshot) ([]api.GoalView, []api.StoryView, []api.CriterionView) {
	byID := map[string]assistant.Criterion{}
	status := map[string]string{}
	for _, c := range snap.Criteria {
		byID[c.ID] = c
		status[c.ID] = c.Status
	}
	delivered := map[string]bool{}
	stories := make([]api.StoryView, 0, len(snap.Features))
	for _, f := range snap.Features {
		pass, total := 0, len(f.Criteria)
		acs := make([]api.CriterionView, 0, total)
		for _, id := range f.Criteria {
			c := byID[id]
			if assistant.Passes(c.Status) {
				pass++
			}
			acs = append(acs, criterionView(c))
		}
		done := assistant.Delivered(f, status)
		if done {
			delivered[f.ID] = true
		}
		stories = append(stories, api.StoryView{
			ID: f.ID, Title: f.Title, CoD: f.CoD.Sum(),
			Delivered: done, PassCount: pass, TotalCount: total, Criteria: acs,
		})
	}
	sort.SliceStable(stories, func(i, j int) bool { return stories[i].CoD > stories[j].CoD })

	goals := make([]api.GoalView, 0, len(snap.Goals))
	for _, g := range snap.Goals {
		gv := api.GoalView{ID: g.ID, Title: g.Title, ServingUS: len(g.ServingUS)}
		for _, id := range g.ServingUS {
			if delivered[id] {
				gv.DeliveredUS++
			}
		}
		goals = append(goals, gv)
	}

	loose := []api.CriterionView{}
	for _, c := range snap.Criteria {
		if c.FeatureID == "" {
			loose = append(loose, criterionView(c))
		}
	}
	return goals, stories, loose
}

func criterionView(c assistant.Criterion) api.CriterionView {
	status := c.Status
	if assistant.NoTest(status) {
		status = "no-test"
	}
	return api.CriterionView{ID: c.ID, Title: c.Title, Status: status, NodeID: c.ParentID}
}

// RunAction runs one action. The alignment fixes are precise CWM writes,
// after which the finding is gone on the next Overview; creative work is
// named, not done.
func RunAction(ctx context.Context, cwm cwmapi.CWMService, a assistant.Action, snap assistant.Snapshot) (string, bool) {
	switch a.Kind {
	case "shape-us":
		return shapeFeature(ctx, cwm, a)
	case "review-waste":
		if len(snap.Goals) == 0 {
			return "no goal available to serve", false
		}
		g := snap.Goals[0]
		if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: a.Subject, To: g.ID, Kind: "serves"}); err != nil {
			return "serves edge failed: " + err.Error(), false
		}
		return fmt.Sprintf("Linked feature → goal %q (serves). WASTE resolved.", g.Title), true
	case "retro-link":
		return fmt.Sprintf("Criterion %q sits under no feature: which feature it proves is a judgement — move it under that feature (cwm object-move), or retire it.", a.Title), false
	default:
		return fmt.Sprintf("'%s' is creative work — launch an agent with the matching skill (plan→plan-increment, write-probe→author a test).", a.Kind), false
	}
}

// shapeFeature is "shape-us": a feature beside the goal (same module, same
// parent) that serves it — a stub for its author to fill, as the Generate
// Features task creates one. Its authored id comes from its title, so it is
// findable by what demanded it.
func shapeFeature(ctx context.Context, cwm cwmapi.CWMService, a assistant.Action) (string, bool) {
	module, err := cwmapi.ModuleOfObjectID(a.Subject)
	if err != nil {
		return fmt.Sprintf("the goal %q is not an object: %v", a.Subject, err), false
	}
	g, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: a.Subject})
	if err != nil {
		return "could not read the goal: " + err.Error(), false
	}
	name := "Serve: " + a.Title
	f := cwmapi.NewRecordObject(module, "f-"+slugish(a.Title), FeatureType, g.Object.ParentID(), name, nil)
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: f})
	if err != nil {
		return "could not create the feature: " + err.Error(), false
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: created.Object.ID, To: a.Subject, Kind: "serves"}); err != nil {
		return "created the feature but the serves edge failed: " + err.Error(), false
	}
	return fmt.Sprintf("Created feature %q → serves the goal. GAP resolved (a stub — fill its content and its acceptance criteria next).", name), true
}

// slugish makes a title the local half of an authored id: lowercase
// alphanumerics and single hyphens.
func slugish(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "feature"
	}
	return s
}

func bodyStr(body json.RawMessage, key string) string {
	var m map[string]any
	if len(body) == 0 || json.Unmarshal(body, &m) != nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}
