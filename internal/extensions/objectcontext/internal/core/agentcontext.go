// SPDX-License-Identifier: Apache-2.0

// Package core resolves an object's context: what a chat started on it
// receives (AgentContext) and which objects of a kind are in force at it
// (Effective). It reads the declarations from the catalog and the objects
// through CWM; it stores nothing.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
)

// Service implements api.ObjectContextService.
type Service struct {
	cfg api.ObjectContextConfig
	cat catalog
}

// New returns the service over its config (ObjectContext.yaml).
func New(cfg api.ObjectContextConfig) *Service { return &Service{cfg: cfg} }

// Invalidate drops the declarations, read again on the next call (the catalog
// changed).
func (s *Service) Invalidate() { s.cat.invalidate() }

func (s *Service) budget() int {
	if s.cfg.MaxBytes > 0 {
		return s.cfg.MaxBytes
	}
	return api.DefaultMaxBytes
}

// AgentContext resolves what a chat started on one object receives.
//
// ORDER, farthest first: the project (ObjectContext.yaml `agent:`), the rules
// in force (each constraint's body is a section), the extensions whose
// objects these are (`agent-context`), the policies (ranked as rules), every
// capability in force (type, aspects, instance), the object's TYPE, the OBJECT
// itself. Every layer is an `agent:` block; a type's `actions` are its
// layer's first tools.
//
// A closer entry with the same key replaces a farther one in its place; a
// closer `when: "false"` removes it. Survivors are filtered by `when` and their
// text interpolated — the object vocabulary, no capability function; what
// cannot be read is reported, not injected. The system sections are bounded by
// the budget, trimmed farthest first, each trim reported.
func (s *Service) AgentContext(ctx context.Context, in api.AgentContextInput) (api.AgentContextOutput, error) {
	if in.ID == "" {
		return api.AgentContextOutput{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	rd, err := readerOf(ctx)
	if err != nil {
		return api.AgentContextOutput{}, err
	}
	obj, err := rd.get(ctx, in.ID)
	if err != nil {
		return api.AgentContextOutput{}, err
	}
	eff, err := rd.cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: in.ID})
	if err != nil {
		return api.AgentContextOutput{}, err
	}
	capIDs := make([]string, 0, len(eff.Capabilities))
	for _, c := range eff.Capabilities {
		capIDs = append(capIDs, c.ID)
	}
	ev := newEvaluator(ctx, rd.cwm, obj.ID)
	r := resolution{
		out: api.AgentContextOutput{
			ObjectID: obj.ID, Type: obj.Type,
			System: []api.Section{}, Tools: []api.Tool{}, Skills: []api.Skill{}, Budget: s.budget(),
		},
		ev:      ev,
		subject: cwmapi.FilterSubject{Type: obj.Type, Capabilities: capIDs, Eval: ev.predicate},
	}
	layers := s.layers(ctx, rd, &r, obj, capIDs)
	ev.prefetch(expressionsOf(layers))
	sections := r.sections(layers)
	sections = append(sections, r.files(ctx, rd, obj.ID, layers)...)
	r.tools(layers)
	r.skills(layers)
	r.budget(sections)
	return r.out, nil
}

// layer is one contributor's declaration and its place in the order.
type layer struct {
	source string
	rank   int
	decl   cwmapi.AgentDecl
}

// Layer ranks, farthest first — the budget trims the lowest rank first.
const (
	rankProject = iota
	rankRule
	rankExtension
	rankCapability
	rankType
	rankObject
)

// layers collects every contributor's declaration, farthest first.
func (s *Service) layers(ctx context.Context, rd reader, r *resolution, obj cwmapi.Object, capIDs []string) []layer {
	var out []layer
	add := func(source string, rank int, d *cwmapi.AgentDecl) {
		if !d.IsZero() {
			out = append(out, layer{source, rank, *d})
		}
	}
	add(api.LayerProject, rankProject, s.cfg.Agent)
	for _, l := range s.rulesInForce(ctx, rd, r, obj) {
		add(l.source, rankRule, &l.decl)
	}
	decl := s.cat.get(ctx)
	// The extensions whose objects these are: every `agent-context` whose
	// filter admits the object. A filter that cannot be read is reported; one
	// that does not match is not a note — it is not about this object.
	r.withEdges(ctx, rd, obj.ID, decl.framings)
	for _, f := range decl.framings {
		ok, why := f.Filter.Matches(r.subject)
		if !ok {
			if strings.HasPrefix(why, "when: ") {
				r.note("system", f.Name, api.FramingSource(f), api.NoteError, why)
			}
			continue
		}
		add(api.FramingSource(f), rankExtension, &f.Agent)
	}
	for _, p := range decl.policiesFor(obj.Type, capIDs) {
		if p.Agent != nil {
			add(api.LayerPolicy+":"+p.Name, rankRule, p.Agent)
		}
	}
	for _, id := range capIDs {
		if c, ok := decl.capabilities[id]; ok {
			add(api.LayerCapability+":"+c.Name, rankCapability, c.Agent)
		}
	}
	typeLabel := obj.Type
	if t, ok := rd.objectType(ctx, obj.Type); ok {
		add(api.LayerType+":"+t.Name, rankType, typeAgent(t))
		if t.Label != "" && t.Label != obj.Type {
			typeLabel = obj.Type + " (" + t.Label + ")"
		}
	}
	own := cwmapi.AgentDecl{System: []cwmapi.AgentSection{currentObjectSection(obj, typeLabel, r.ev)}}
	if obj.Agent != nil {
		own.System = append(own.System, obj.Agent.System...)
		own.Tools, own.Skills, own.Files = obj.Agent.Tools, obj.Agent.Skills, obj.Agent.Files
	}
	add(api.LayerObject, rankObject, &own)
	return out
}

// Rule fields the rules layer reads (the `rule` type's data-type).
const (
	ruleType           = "rule"
	ruleKindConstraint = "constraint"
	ruleScopeRepo      = "repo"
)

// rulesInForce is the rules layer: every constraint rule in force at the
// object, its statement and body a section keyed by the rule's id, plus the
// tools and skills its own `agent:` block declares. In force: the rules of the
// object's ancestors (Effective over `rule`, the rule type's own propagation —
// a rule reaches the subtree of the module it sits in), and every rule whose
// `scope` is `repo`, wherever it sits, farthest first. Only a `constraint`
// (the default kind) is told; a recipe, a reference or an example is fetched
// on demand. A project that declares no `rule` type has no rules.
func (s *Service) rulesInForce(ctx context.Context, rd reader, r *resolution, obj cwmapi.Object) []layer {
	if _, ok := rd.objectType(ctx, ruleType); !ok {
		return nil
	}
	eff, err := s.Effective(ctx, api.EffectiveInput{ID: obj.ID, Type: ruleType})
	if err != nil {
		r.note("system", "rules", api.LayerRule, api.NoteError, err.Error())
		return nil
	}
	// A rule's fields are not resident in the index: each rule is read once,
	// whole, and the reads serve both the scope filter and the section.
	var rules []cwmapi.Object
	read := map[string]bool{}
	get := func(id string) (cwmapi.Object, bool) {
		read[id] = true
		rule, err := rd.get(ctx, id)
		if err != nil {
			r.note("system", id, api.LayerRule, api.NoteError, err.Error())
			return rule, false
		}
		return rule, true
	}
	all, err := rd.cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: ruleType})
	if err != nil {
		r.note("system", "rules", api.LayerRule, api.NoteError, "repo-wide rules: "+err.Error())
	}
	inTree := map[string]bool{}
	for _, b := range eff.Objects {
		inTree[b.ID] = true
	}
	for _, o := range all.Objects {
		if inTree[o.ID] {
			continue // read below, in its place among the tree's rules
		}
		if rule, ok := get(o.ID); ok && scalarString(rule.Fields["scope"]) == ruleScopeRepo {
			rules = append(rules, rule)
		}
	}
	for _, b := range eff.Objects {
		if read[b.ID] {
			continue
		}
		if rule, ok := get(b.ID); ok {
			rules = append(rules, rule)
		}
	}
	var out []layer
	for _, rule := range rules {
		if k := scalarString(rule.Fields["kind"]); k != "" && k != ruleKindConstraint {
			continue // a recipe, a reference, an example is fetched on demand
		}
		out = append(out, layer{source: api.LayerRule + ":" + rule.ID, decl: ruleDecl(rule)})
	}
	return out
}

// ruleDecl is one rule as a section: its statement, then its explanation,
// under its title; a rule with neither IS its title.
func ruleDecl(rule cwmapi.Object) cwmapi.AgentDecl {
	title := rule.DisplayTitle()
	var parts []string
	for _, p := range []string{scalarString(rule.Fields["statement"]), rule.Body} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	sec := cwmapi.AgentSection{ID: api.LayerRule + ":" + rule.ID, Text: title}
	if len(parts) > 0 {
		sec.Title, sec.Text = title, strings.Join(parts, "\n\n")
	}
	d := cwmapi.AgentDecl{System: []cwmapi.AgentSection{sec}}
	if rule.Agent != nil {
		d.Tools, d.Skills = rule.Agent.Tools, rule.Agent.Skills
	}
	return d
}

// typeAgent is a type's declaration with its actions as tools, ahead of the
// tools its `agent:` block names.
func typeAgent(t cwmapi.ObjectType) *cwmapi.AgentDecl {
	var d cwmapi.AgentDecl
	for _, a := range t.Actions {
		if a.URI != "" {
			d.Tools = append(d.Tools, cwmapi.AgentTool{URI: a.URI})
		}
	}
	if t.Agent != nil {
		d.System = append(d.System, t.Agent.System...)
		d.Tools = append(d.Tools, t.Agent.Tools...)
		d.Skills = append(d.Skills, t.Agent.Skills...)
		d.Files = append(d.Files, t.Agent.Files...)
	}
	return &d
}

// currentObjectSection is the object's identity card, the section every chat
// on an object starts from: who it is, where it sits, its state, and its type's
// fields under `type_specific:`.
func currentObjectSection(obj cwmapi.Object, typeLabel string, ev *evaluator) cwmapi.AgentSection {
	cur, _ := ev.value("current")
	curMap, _ := cur.(map[string]any)
	var b strings.Builder
	b.WriteString("You are working on this object; read and change it through the cwm object tools.\n")
	line := func(k string, v any) {
		if s := fmt.Sprint(v); v != nil && s != "" {
			fmt.Fprintf(&b, "%s: %s\n", k, s)
		}
	}
	line("id", obj.ID)
	line("type", typeLabel)
	line("module", curMap["module"])
	line("name", obj.DisplayTitle())
	line("parent_id", obj.ParentID())
	if l, ok := obj.Lifecycle(); ok {
		line("state", l.State)
	}
	line("file", curMap["file"])
	if fields := bodyFields(obj); len(fields) > 0 {
		b.WriteString("type_specific:\n")
		for _, f := range fields {
			fmt.Fprintf(&b, "  %s\n", f)
		}
	}
	return cwmapi.AgentSection{ID: api.SectionCurrentObject, Title: "Current object", Text: strings.TrimRight(b.String(), "\n")}
}

// bodyFields renders the type's fields, sorted, one per line; a list or a map
// is compact JSON, cut at 200 characters.
func bodyFields(obj cwmapi.Object) []string {
	if len(obj.Fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(obj.Fields))
	for k := range obj.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		var str string
		switch t := obj.Fields[k].(type) {
		case nil:
			continue
		case string, bool, float64:
			str = fmt.Sprint(t)
		default:
			raw, _ := json.Marshal(t)
			str = string(raw)
		}
		if len(str) > 200 {
			str = str[:197] + "..."
		}
		out = append(out, k+": "+str)
	}
	return out
}

// expressionsOf is every `when` and `${...}` the layers hold, asked in one
// evaluation.
func expressionsOf(layers []layer) []string {
	var out []string
	for _, l := range layers {
		for _, x := range l.decl.System {
			out = append(out, x.When)
			out = append(out, templateExprs(x.Text)...)
		}
		for _, x := range l.decl.Tools {
			out = append(out, x.When)
		}
		for _, x := range l.decl.Skills {
			out = append(out, x.When)
		}
		for _, x := range l.decl.Files {
			out = append(out, x.When)
		}
	}
	var kept []string
	for _, x := range out {
		if x = strings.TrimSpace(x); x != "" && x != "false" {
			kept = append(kept, x)
		}
	}
	return kept
}

// templateExprs are the `${...}` expressions of a text.
func templateExprs(s string) []string {
	var out []string
	for {
		i := strings.Index(s, "${")
		if i < 0 {
			return out
		}
		j := strings.Index(s[i+2:], "}")
		if j < 0 {
			return out
		}
		out = append(out, strings.TrimSpace(s[i+2:i+2+j]))
		s = s[i+2+j+1:]
	}
}

// resolution accumulates the output while the lists are resolved.
type resolution struct {
	out     api.AgentContextOutput
	subject cwmapi.FilterSubject
	ev      *evaluator
}

// withEdges gives the subject the object's edge kinds when a framing's filter
// selects by edge (`incoming:` / `outgoing:`) — read once, only then. Edges
// CWM cannot list leave the subject without any: such a filter does not admit.
func (r *resolution) withEdges(ctx context.Context, rd reader, id string, framings []api.AgentFraming) {
	if !slices.ContainsFunc(framings, func(f api.AgentFraming) bool { return f.Filter.ReadsEdges() }) {
		return
	}
	got, err := rd.cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: id})
	if err != nil {
		return
	}
	r.subject = r.subject.WithEdges(id, got.Edges)
}

// slot is one surviving entry of a list, with where it came from.
type slot[T any] struct {
	entry  T
	source string
	rank   int
	pos    int
}

// layered applies the override rules to one list: a closer entry with the
// same key replaces a farther one in its place; a closer "false" removes it.
func layered[T any](r *resolution, list string, layers []layer, entries func(cwmapi.AgentDecl) []T, key func(T) string, when func(T) string) []slot[T] {
	var order []string
	byKey := map[string]*slot[T]{}
	for _, l := range layers {
		for _, e := range entries(l.decl) {
			k := key(e)
			if when(e) == "false" {
				if prev, ok := byKey[k]; ok {
					r.note(list, k, prev.source, api.NoteRemoved, "removed by "+l.source)
					delete(byKey, k)
				}
				continue
			}
			if _, seen := byKey[k]; !seen {
				order = append(order, k)
			}
			byKey[k] = &slot[T]{entry: e, source: l.source, rank: l.rank}
		}
	}
	var out []slot[T]
	for i, k := range order {
		if sl, ok := byKey[k]; ok {
			sl.pos = i
			out = append(out, *sl)
		}
	}
	return out
}

func (r *resolution) note(list, key, source, kind, reason string) {
	r.out.Notes = append(r.out.Notes, api.Note{List: list, Key: key, Source: source, Kind: kind, Reason: reason})
}

// applies evaluates an entry's `when`; false or unreadable is noted.
func (r *resolution) applies(list, key, source, when string) bool {
	if when == "" {
		return true
	}
	ok, err := r.ev.predicate(strings.TrimSpace(when))
	switch {
	case err != nil:
		r.note(list, key, source, api.NoteError, "when: "+err.Error())
		return false
	case !ok:
		r.note(list, key, source, api.NoteSkipped, "when is false: "+when)
		return false
	}
	return true
}

// ranked is a kept section with what the budget and the sort need.
type ranked struct {
	api.Section
	rank, pos int
}

func (r *resolution) sections(layers []layer) []ranked {
	slots := layered(r, "system", layers,
		func(d cwmapi.AgentDecl) []cwmapi.AgentSection { return d.System },
		func(e cwmapi.AgentSection) string { return e.ID },
		func(e cwmapi.AgentSection) string { return e.When })
	var out []ranked
	for _, sl := range slots {
		e := sl.entry
		if e.ID == "" {
			r.note("system", "", sl.source, api.NoteError, "a section needs an id")
			continue
		}
		if !r.applies("system", e.ID, sl.source, e.When) {
			continue
		}
		text, err := r.interpolate(e.Text)
		if err != nil {
			r.note("system", e.ID, sl.source, api.NoteError, err.Error())
			continue
		}
		if strings.TrimSpace(text) == "" {
			r.note("system", e.ID, sl.source, api.NoteEmpty, "no text")
			continue
		}
		out = append(out, ranked{api.Section{ID: e.ID, Title: e.Title, Text: text, Order: e.Order, Source: sl.source}, sl.rank, sl.pos})
	}
	return out
}

// interpolate resolves every `${expr}` of a text. STRICT: a section the agent
// would read with a template in it is worse than no section, so an expression
// that does not resolve fails it, and the failure is reported.
func (r *resolution) interpolate(text string) (string, error) {
	if !strings.Contains(text, "${") {
		return text, nil
	}
	var b strings.Builder
	rest := text
	for {
		i := strings.Index(rest, "${")
		if i < 0 {
			b.WriteString(rest)
			return b.String(), nil
		}
		b.WriteString(rest[:i])
		rest = rest[i+2:]
		j := strings.Index(rest, "}")
		if j < 0 {
			return "", fmt.Errorf("unclosed ${ in text")
		}
		x := strings.TrimSpace(rest[:j])
		v, err := r.ev.value(x)
		if err != nil {
			return "", fmt.Errorf("${%s}: %v", x, err)
		}
		if v == nil {
			return "", fmt.Errorf("${%s}: no value", x)
		}
		b.WriteString(fmt.Sprint(v))
		rest = rest[j+1:]
	}
}

// EmptyFileText is the text of a declared file's section while the file is
// not written: the section is kept so the agent knows there is nothing to read.
const EmptyFileText = "(empty)"

// files reads each surviving named-file entry into a section at its declaring
// layer's rank, up to the budget — one file never exceeds it; a file not
// written yet is a section reading EmptyFileText.
func (r *resolution) files(ctx context.Context, rd reader, id string, layers []layer) []ranked {
	slots := layered(r, "files", layers,
		func(d cwmapi.AgentDecl) []cwmapi.AgentFile { return d.Files },
		func(e cwmapi.AgentFile) string { return e.Key() },
		func(e cwmapi.AgentFile) string { return e.When })
	var out []ranked
	for _, sl := range slots {
		e, key := sl.entry, sl.entry.Key()
		if e.Name == "" {
			r.note("files", key, sl.source, api.NoteError, "a file entry needs a name")
			continue
		}
		if !r.applies("files", key, sl.source, e.When) {
			continue
		}
		read, err := rd.cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: id, Capability: e.Capability, Name: e.Name, Limit: r.out.Budget})
		if err != nil {
			r.note("files", key, sl.source, api.NoteError, err.Error())
			continue
		}
		text := string(read.Content)
		if strings.TrimSpace(text) == "" {
			// Still a section: the agent sees the file is empty and does not
			// go looking for it.
			r.note("files", key, sl.source, api.NoteEmpty, "not written yet")
			text = EmptyFileText
		}
		if read.IsPartial {
			r.note("files", key, sl.source, api.NoteTrimmed, fmt.Sprintf("read up to the budget (%d bytes)", r.out.Budget))
		}
		title := e.Title
		if title == "" {
			title = strings.TrimPrefix(key, "/")
		}
		out = append(out, ranked{api.Section{ID: "file:" + key, Title: title, Text: text, Order: e.Order, Source: sl.source, File: key}, sl.rank, len(layers) + sl.pos})
	}
	return out
}

func (r *resolution) tools(layers []layer) {
	slots := layered(r, "tools", layers,
		func(d cwmapi.AgentDecl) []cwmapi.AgentTool { return d.Tools },
		func(e cwmapi.AgentTool) string { return e.Key() },
		func(e cwmapi.AgentTool) string { return e.When })
	for _, sl := range slots {
		e := sl.entry
		if (e.URI == "") == (e.Plugin == "") {
			r.note("tools", e.Key(), sl.source, api.NoteError, "exactly one of uri and plugin")
			continue
		}
		if r.applies("tools", e.Key(), sl.source, e.When) {
			r.out.Tools = append(r.out.Tools, api.Tool{URI: e.URI, Plugin: e.Plugin, Source: sl.source})
		}
	}
}

func (r *resolution) skills(layers []layer) {
	slots := layered(r, "skills", layers,
		func(d cwmapi.AgentDecl) []cwmapi.AgentSkill { return d.Skills },
		func(e cwmapi.AgentSkill) string { return e.Selector },
		func(e cwmapi.AgentSkill) string { return e.When })
	for _, sl := range slots {
		if sl.entry.Selector == "" {
			continue
		}
		if r.applies("skills", sl.entry.Selector, sl.source, sl.entry.When) {
			r.out.Skills = append(r.out.Skills, api.Skill{Selector: sl.entry.Selector, Source: sl.source})
		}
	}
}

// budget orders the sections (order, then layer, then place) and trims the
// farthest while they exceed the budget — the last of the farthest layer first.
func (r *resolution) budget(sections []ranked) {
	size := func(x ranked) int { return len(x.Title) + len(x.Text) }
	total := 0
	for _, x := range sections {
		total += size(x)
	}
	for total > r.out.Budget && len(sections) > 0 {
		victim := 0
		for i, x := range sections {
			v := sections[victim]
			if x.rank < v.rank || (x.rank == v.rank && x.pos > v.pos) {
				victim = i
			}
		}
		x := sections[victim]
		r.note("system", x.ID, x.Source, api.NoteTrimmed, fmt.Sprintf("%d bytes over the budget of %d", total-r.out.Budget, r.out.Budget))
		total -= size(x)
		sections = append(sections[:victim], sections[victim+1:]...)
	}
	sort.SliceStable(sections, func(i, j int) bool {
		a, b := sections[i], sections[j]
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		return a.pos < b.pos
	})
	for _, x := range sections {
		r.out.System = append(r.out.System, x.Section)
	}
	r.out.Bytes = total
}
