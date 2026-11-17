// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/el"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// Detail composes the pane in one call: the object as CWM read it (with its
// errors), its type, its effective capabilities, the tab strip, its aspects,
// badges, policies, links and children by type. The id is bare or qualified
// (<key>:<module>.<local>); when the universe serves several projects the
// answer names the one holding the object (Project), and everything it reads
// of the object - capabilities, edges, children, link targets - is that
// project's.
func (s *Service) Detail(ctx context.Context, in api.DetailInput) (api.DetailOutput, error) {
	if in.ID == "" {
		return api.DetailOutput{}, fmt.Errorf("%w: ID required", cwmapi.ErrInvalidInput)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.DetailOutput{}, fmt.Errorf("object-pane: resolve CWMService: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(in.ID)
	if err != nil {
		return api.DetailOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return api.DetailOutput{}, err
	}
	full := got.Object
	decl := s.declarations(ctx)
	p := pane{ctx: ctx, cwm: cwm, decl: decl, types: map[string]typeAnswer{}, project: got.Project}
	obj := full
	if !in.Body {
		obj = p.resident(full)
	}
	// The object's own id, bare (edges and lists carry bare ids) and as every
	// by-object call names it.
	id, self := full.ID, p.ref(full.ID)

	out := api.DetailOutput{
		Object:       obj,
		Project:      got.Project,
		Revision:     got.Revision,
		Effective:    []cwmapi.CapabilityRef{},
		QuickActions: defaultQuickActions(),
		Diagnostics:  got.Errors,
	}
	if t, ok := p.typeOf(obj.Type); ok {
		out.ObjectType = t
	}

	eff, err := cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: self})
	if err != nil {
		return api.DetailOutput{}, err
	}
	caps := decl.capabilities()
	// The actions its type and its capabilities declare quick: small buttons
	// on its first page, beside the defaults.
	out.QuickActions = append(out.QuickActions, decl.quickActions(obj.Type, eff.Capabilities)...)
	// What the header shows: every capability in force but the type-level
	// ones — set by a section of the type, they carry no group on the object.
	// The tab strip still reads them all.
	for _, a := range eff.Capabilities {
		if caps[a.ID].IsTypeLevel() {
			continue
		}
		out.Effective = append(out.Effective, a)
	}
	for _, a := range out.Effective {
		out.Capabilities = append(out.Capabilities, a.ID)
		c := caps[a.ID]
		out.CapabilityInfo = append(out.CapabilityInfo, api.CapabilityInfo{
			ID: a.ID, Label: c.Label, Description: c.Description, Icon: c.Icon, Source: a.Source,
		})
	}
	// The incident edges, read once: the Links section, and what an
	// `incoming:` / `outgoing:` filter reads.
	edges := p.edges(self)
	var replaced map[string]bool
	out.Panels, replaced = p.panels(obj, eff.Capabilities, edges)

	for _, name := range obj.Aspects {
		ref := api.AspectRef{Name: name}
		if a, ok := decl.aspect(name); ok {
			ref.Declared = true
			ref.Label, ref.Description, ref.Capabilities = a.Label, a.Description, a.Capabilities
		}
		out.Aspects = append(out.Aspects, ref)
	}

	surf, err := cwm.ObjectEffectiveSurface(ctx, cwmapi.ObjectEffectiveSurfaceInput{ID: self})
	if err != nil {
		return api.DetailOutput{}, err
	}
	// The type's own badges (its `pane` section) first, then the
	// capabilities' — each over its own group.
	defs := surf.Badges
	if t, ok := p.typeOf(obj.Type); ok {
		if ps, err := api.PaneOf(t); err == nil && len(ps.Badges) > 0 {
			defs = append(append([]cwmapi.BadgeDef(nil), ps.Badges...), surf.Badges...)
		}
	}
	out.Badges = badges(full, defs)
	out.Policies = decl.policiesNamed(surf.Policies)

	// The journal reads a qualified id in its own project's journal only.
	out.CreatedAt, out.UpdatedAt = timesOf(ctx, self)
	out.Links = p.links(id, edges)
	out.Children = p.children(id)
	for _, g := range out.Children {
		// A drawn type shows in the tree; a type a shown panel stands for
		// (its `replaces_children`) shows in that panel — never twice.
		if g.Drawn || replaced[g.Type] {
			continue
		}
		name := g.Plural
		if name == "" {
			name = g.Type
		}
		out.Panels = append(out.Panels, api.ObjectPanel{
			ID: ChildrenTab(g.Type), Name: name, Order: api.ChildrenPanelOrder,
			Kind: api.PanelKindChildren, ChildType: g.Type, Origin: "children",
		})
	}
	holds := p.variantHolds(self, decl.variantWhens(obj.Type, obj.Aspects))
	out.Bare = !decl.showsHeader(obj.Type)
	if list, ok := decl.onlyTabs(obj.Type, holds); ok {
		out.Panels = onlyListed(out.Panels, list)
		return out, nil
	}
	main, more := decl.tabList(obj.Type, obj.Aspects, holds)
	out.Panels = applyTabList(out.Panels, main, more)
	return out, nil
}

// showsHeader is whether the type's pane draws the standard object header -
// its own option (`pane.header`), unset: shown except under
// `pane.only`. A type with no pane section, or one that does not read, shows it.
func (d decl) showsHeader(typ string) bool {
	for _, t := range d.Objects {
		if t.Name != typ {
			continue
		}
		tp, err := api.PaneOf(t)
		if err != nil {
			return true
		}
		return tp.ShowsHeader()
	}
	return true
}

// variantWhens are the expressions the type's and its aspects' pane
// variants name.
func (d decl) variantWhens(typ string, aspects []string) []string {
	var out []string
	for _, t := range d.Objects {
		if t.Name == typ {
			if tp, err := api.PaneOf(t); err == nil {
				for _, v := range tp.Variants {
					out = append(out, v.When)
				}
			}
			break
		}
	}
	for _, name := range aspects {
		if a, ok := d.aspect(name); ok {
			if ap, err := api.AspectPaneOf(a); err == nil {
				for _, v := range ap.Variants {
					out = append(out, v.When)
				}
			}
		}
	}
	return out
}

// variantHolds evaluates the variants' expressions on the object in one
// evaluation and answers whether one holds: true, never on an error.
func (p *pane) variantHolds(id string, whens []string) func(string) bool {
	if len(whens) == 0 {
		return nil
	}
	answers := p.evaluate(id, whens)
	return func(expr string) bool {
		a, ok := answers[expr]
		if !ok || a.err != "" {
			return false
		}
		b, _ := a.value.(bool)
		return b
	}
}

// onlyTabs is the tabs a type lists as its whole pane (`pane.only` with
// `pane.tabs` - or the first of its variants that holds), and whether it
// does: its aspects add none then. A variant whose when fails does not hold:
// the plain tabs show.
func (d decl) onlyTabs(typ string, holds func(string) bool) ([]string, bool) {
	for _, t := range d.Objects {
		if t.Name != typ {
			continue
		}
		tp, err := api.PaneOf(t)
		if err != nil || !tp.Only || len(tp.Tabs) == 0 {
			return nil, false
		}
		return api.VariantTabs(tp.Tabs, tp.Variants, holds), true
	}
	return nil, false
}

// onlyListed is the listed tabs alone, in list order: every other entry -
// Information and its sections, the generic panels, the children tabs, the
// header panels - is left out. A name matches
// `panel:<name>` or a full strip id; one the strip does not hold is skipped.
func onlyListed(panels []api.ObjectPanel, list []string) []api.ObjectPanel {
	at := make(map[string]int, len(panels))
	for i, p := range panels {
		at[p.ID] = i
	}
	out := make([]api.ObjectPanel, 0, len(list))
	taken := map[int]bool{}
	for _, name := range list {
		i, ok := at[PanelTab(name)]
		if !ok {
			i, ok = at[name]
		}
		if !ok || taken[i] {
			continue
		}
		taken[i] = true
		out = append(out, panels[i])
	}
	return out
}

// An object's tabs and sections have element ids: a declared
// panel `tab:<stem>`, a children group `tab:children/<type>`, the type's
// fields `section:fields`, a capability's fields `section:cap/<capability>`.

// InformationPanel is the strip id of the Information tab: always a main tab.
const InformationPanel = "tab:information"

// PanelTab is a declared panel's tab id.
func PanelTab(stem string) string { return "tab:" + stem }

// ChildrenTab is the tab id of the children of one type.
func ChildrenTab(childType string) string { return "tab:children/" + childType }

// FieldsSection is the section id of the type's own fields.
const FieldsSection = "section:fields"

// CapabilitySection is the section id of a capability's fields.
func CapabilitySection(capability string) string { return "section:cap/" + capability }

// tabList is the main tabs the object's type lists (`pane.tabs`, or the
// first of its variants that holds), then its aspects', in aspect order then
// list order, each name once - and the « More… » tabs they name (`more`,
// ), joined the same way, a name a main list holds left
// out. A type or an aspect that is undeclared, lists none or whose pane does
// not decode adds nothing.
func (d decl) tabList(typ string, aspects []string, holds func(string) bool) (main, more []string) {
	seen := map[string]bool{}
	var moreAll []string
	add := func(tabs, m []string) {
		for _, tab := range tabs {
			if tab != "" && !seen[tab] {
				seen[tab] = true
				main = append(main, tab)
			}
		}
		moreAll = append(moreAll, m...)
	}
	for _, t := range d.Objects {
		if t.Name != typ {
			continue
		}
		tp, err := api.PaneOf(t)
		if err != nil {
			slog.Warn("object-pane: type pane not read", "type", typ, "error", err)
			break
		}
		add(api.VariantPane(tp.Tabs, tp.More, tp.Variants, holds))
		break
	}
	for _, name := range aspects {
		a, ok := d.aspect(name)
		if !ok {
			continue
		}
		ap, err := api.AspectPaneOf(a)
		if err != nil {
			slog.Warn("object-pane: aspect pane not read", "aspect", name, "error", err)
			continue
		}
		add(api.VariantPane(ap.Tabs, ap.More, ap.Variants, holds))
	}
	for _, tab := range moreAll {
		if tab != "" && !seen[tab] {
			seen[tab] = true
			more = append(more, tab)
		}
	}
	return main, more
}

// applyTabList puts the listed tabs first, in list order, and flags every other
// tab More, in its own order. A name matches `panel:<name>` or a full strip id;
// a name the strip does not hold is skipped. Information stays first and main
// unless a list places it; sections (of the Information tab) and header
// panels are never More. With a More list the More tabs
// are exactly those it names, in its order - Information may be one - and
// every other tab is hidden - once at least one of its names is a tab of the
// strip: a More list matching none is logged and leaves the unlisted tabs
// behind More. No list: the strip is unchanged.
func applyTabList(panels []api.ObjectPanel, list, moreList []string) []api.ObjectPanel {
	if len(list) == 0 && len(moreList) == 0 {
		return panels
	}
	at := make(map[string]int, len(panels))
	for i, p := range panels {
		at[p.ID] = i
	}
	taken := make([]bool, len(panels))
	// A section or a header panel may be placed by the main list, never sent
	// behind More.
	pick := func(names []string, more bool) []api.ObjectPanel {
		var got []api.ObjectPanel
		for _, name := range names {
			i, ok := at[PanelTab(name)]
			if !ok {
				i, ok = at[name]
			}
			if !ok || taken[i] || (more && (panels[i].Section || panels[i].Header)) {
				continue
			}
			taken[i] = true
			got = append(got, panels[i])
		}
		return got
	}
	listed := pick(list, false)
	behind := pick(moreList, true)
	// A More list none of whose names the strip holds (a typo, a renamed
	// panel) would hide every unlisted tab: say so and keep them behind More.
	hide := len(behind) > 0
	if len(moreList) > 0 && !hide {
		slog.Warn("object-pane: no name of the More list is a tab of the strip; unlisted tabs stay behind More", "more", moreList)
	}
	out := make([]api.ObjectPanel, 0, len(panels))
	if i, ok := at[InformationPanel]; ok && !taken[i] {
		taken[i] = true
		out = append(out, panels[i])
	}
	out = append(out, listed...)
	for _, p := range behind {
		p.More = true
		out = append(out, p)
	}
	for i, p := range panels {
		if taken[i] {
			continue
		}
		if p.Section || p.Header {
			out = append(out, p)
			continue
		}
		if hide {
			continue // neither main nor More: hidden for this type
		}
		p.More = true
		out = append(out, p)
	}
	return out
}

// resident is the object as its index record holds it: the row CWM lists under
// its parent in the object's project, without the body and the fields read
// from the file.
func (p *pane) resident(full cwmapi.Object) cwmapi.Object {
	module, _ := cwmapi.ModuleOfObjectID(full.ID)
	rows, err := p.cwm.ObjectList(p.ctx, cwmapi.ObjectListInput{Module: module, Type: full.Type, Parent: full.Parent})
	if err == nil {
		for i, r := range rows.Objects {
			if r.ID == full.ID && p.ours(rows.Projects, i) {
				return r
			}
		}
	}
	full.Body, full.Fields = "", nil
	return full
}

func defaultQuickActions() []cwmapi.Action {
	return []cwmapi.Action{{URI: cwmapi.SystemTaskChatURI, Label: "Start chat", Icon: "message", Kind: "sync", Source: "quick"}}
}

// quickActions are the actions an object's type and its capabilities declare
// quick, each once, in declaration order (the type's first).
func (d decl) quickActions(objType string, eff []cwmapi.CapabilityRef) []cwmapi.Action {
	var refs []cwmapi.ActionRef
	for _, t := range d.Objects {
		if t.Name == objType {
			refs = append(refs, t.Actions...)
		}
	}
	caps := d.capabilities()
	for _, a := range eff {
		refs = append(refs, caps[a.ID].Actions...)
	}
	var out []cwmapi.Action
	seen := map[string]bool{}
	for _, r := range refs {
		if !r.Quick || seen[r.URI+"\x00"+r.Label] {
			continue
		}
		seen[r.URI+"\x00"+r.Label] = true
		label, kind := r.Label, r.Kind
		if label == "" {
			label = r.URI
		}
		if kind == "" {
			kind = "sync"
		}
		out = append(out, cwmapi.Action{URI: r.URI, Label: label, Description: r.Description, Icon: r.Icon,
			Kind: kind, Source: "quick", Confirm: r.Confirm, Preview: r.Preview, Quick: true, BoundInput: r.Input})
	}
	return out
}

// decl is the catalog's declarations as CWM reads them (DeclarationsFromCatalog),
// and the pane's own: the `object-panel` contributions.
type decl struct {
	cwmapi.Declarations
	Panels []api.PanelDecl
}

func (s *Service) declarations(ctx context.Context) decl {
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		return decl{}
	}
	return decl{Declarations: cwmapi.DeclarationsFromCatalog(ctx, cat), Panels: panelsOf(ctx, cat)}
}

// panelsOf reads every declared panel, sorted by id. The id is the
// CONTRIBUTION's (the file's basename), never the body's: two places for one
// id is how they disagree. Each is decoded on its own: one that does not decode
// (a filter key no field takes) is skipped with the reason, never all of them.
// A stem is one tab id (`tab:<stem>`): a second pack declaring the same stem
// is skipped with both named (a repo test refuses it before it ships).
func panelsOf(ctx context.Context, cat mbapi.Catalog) []api.PanelDecl {
	got, err := mbapi.QueryTyped[json.RawMessage](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: api.ObjectPanelKind, Catalog: cat})
	if err != nil {
		slog.Warn("object-pane: object-panel declarations not read", "error", err)
		return nil
	}
	out := make([]api.PanelDecl, 0, len(got))
	sort.SliceStable(got, func(i, j int) bool { return got[i].URI < got[j].URI })
	first := map[string]string{}
	for _, g := range got {
		u, err := contribapi.ParseResourceURI(g.URI)
		if err != nil || u.ID == "" {
			continue
		}
		if prior, ok := first[u.ID]; ok {
			slog.Warn("object-pane: object-panel skipped, its stem is another pack's", "uri", g.URI, "kept", prior)
			continue
		}
		first[u.ID] = g.URI
		var d api.PanelDecl
		if len(g.Body) > 0 {
			if err := json.Unmarshal(g.Body, &d); err != nil {
				slog.Warn("object-pane: object-panel skipped", "uri", g.URI, "error", err)
				continue
			}
		}
		d.ID = u.ID
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (d decl) capabilities() map[string]cwmapi.Capability {
	out := make(map[string]cwmapi.Capability, len(d.Capabilities))
	for _, c := range d.Capabilities {
		out[c.Name] = c
	}
	return out
}

func (d decl) aspect(name string) (cwmapi.Aspect, bool) {
	for _, a := range d.Aspects {
		if a.Name == name {
			return a, true
		}
	}
	return cwmapi.Aspect{}, false
}

// policiesNamed is the declared policies CWM says are in force (the surface's
// Policies), in its order.
func (d decl) policiesNamed(names []string) []cwmapi.Policy {
	var out []cwmapi.Policy
	for _, n := range names {
		for _, p := range d.Policies {
			if p.Name == n {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// pane is one Detail's reads, with the type lookups it already made.
type pane struct {
	ctx   context.Context
	cwm   cwmapi.CWMService
	decl  decl
	types map[string]typeAnswer
	// project is the key of the project holding the object; "" when the
	// universe serves one project.
	project string
}

// ref is an id as a by-object call names it: qualified with the object's
// project when the universe serves several (two projects may hold one bare
// id); as written with one project, or when already qualified.
func (p *pane) ref(id string) string { return refIn(p.project, id) }

// refIn is id as a by-object call names it for an object of project ("" when
// the universe serves one): qualified with it, or as written.
func refIn(project, id string) string {
	if project == "" || strings.Contains(id, ":") {
		return id
	}
	return cwmapi.QualifiedID(project, id)
}

// ours reports whether the i-th row of a list joined over the projects (its
// Projects, in the order of its Objects) is of the object's project; every
// row is with one project.
func (p *pane) ours(projects []string, i int) bool {
	return p.project == "" || i >= len(projects) || projects[i] == p.project
}

type typeAnswer struct {
	t  cwmapi.ObjectType
	ok bool
}

// typeOf is CWM's own type lookup (ObjectTypeGet), asked once per type.
func (p *pane) typeOf(name string) (cwmapi.ObjectType, bool) {
	if a, done := p.types[name]; done {
		return a.t, a.ok
	}
	var a typeAnswer
	if name != "" {
		if got, err := p.cwm.ObjectTypeGet(p.ctx, cwmapi.ObjectTypeGetInput{Name: name}); err == nil {
			a = typeAnswer{got.ObjectType, true}
		}
	}
	p.types[name] = a
	return a.t, a.ok
}

// drawn reports whether objects of the type appear as nodes of the tree: the
// type's `load:` when set, else its `tree:`; an UNDECLARED type is drawn, so it
// is noticed rather than vanish.
func (p *pane) drawn(typ string) bool {
	t, ok := p.typeOf(typ)
	if !ok {
		return true
	}
	if t.Load != "" {
		return t.Load == loadWithTree
	}
	return t.Drawn()
}

const loadWithTree = "with-tree"

// links is the Links section: every incident edge, named from THIS object's
// side (id is bare, as edges carry it). An end that is not an object of the
// project comes back unresolved, never hidden. Targets are read as resident
// rows, one list per module.
func (p *pane) links(id string, edges []cwmapi.Edge) []api.ObjectLink {
	if len(edges) == 0 {
		return nil
	}
	out := make([]api.ObjectLink, 0, len(edges))
	for _, e := range edges {
		// Named as a person reads it: the declaration's label from this side,
		// else the kind's identifier (its inverse name from the target).
		link := api.ObjectLink{Kind: string(e.Kind), Direction: "out", TargetID: e.To, Label: firstOf(e.Label, string(e.Kind)), Ended: e.Ended}
		if e.To == id {
			link.Direction, link.TargetID, link.Ended = "in", e.From, false
			link.Label = firstOf(e.InverseLabel, e.Inverse, string(e.Kind))
		}
		out = append(out, link)
	}
	rows := p.residentRows(out)
	for i := range out {
		if target, ok := rows[p.ref(out[i].TargetID)]; ok {
			out[i].Resolved = true
			out[i].TargetType = target.Type
			out[i].TargetTitle = target.DisplayTitle()
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Direction != out[j].Direction {
			return out[i].Direction == "out"
		}
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].TargetID < out[j].TargetID
	})
	return out
}

// edges is every edge incident to the object, both directions; none when CWM
// cannot list them.
func (p *pane) edges(id string) []cwmapi.Edge {
	got, err := p.cwm.EdgeList(p.ctx, cwmapi.EdgeListInput{ObjectID: id})
	if err != nil {
		return nil
	}
	return got.Edges
}

// residentRows reads the link targets' modules once each, keyed as ref names
// a target: a bare target is of the object's project, a qualified one of the
// project its key names - a homonym of another project never resolves it.
func (p *pane) residentRows(links []api.ObjectLink) map[string]cwmapi.Object {
	modules := map[string]bool{}
	for _, l := range links {
		_, bare, err := cwmapi.SplitReference(l.TargetID)
		if err != nil {
			continue
		}
		if m, err := cwmapi.ModuleOfObjectID(bare); err == nil {
			modules[m] = true
		}
	}
	rows := map[string]cwmapi.Object{}
	for m := range modules {
		got, err := p.cwm.ObjectList(p.ctx, cwmapi.ObjectListInput{Module: m})
		if err != nil {
			continue
		}
		for i, o := range got.Objects {
			key := o.ID
			if i < len(got.Projects) && got.Projects[i] != "" {
				key = cwmapi.QualifiedID(got.Projects[i], o.ID)
			}
			rows[key] = o
		}
	}
	return rows
}

// children is the lists under an object, one per child type, as resident rows.
func (p *pane) children(id string) []api.ObjectChildGroup {
	got, err := p.cwm.ObjectList(p.ctx, cwmapi.ObjectListInput{Parent: id})
	if err != nil || len(got.Objects) == 0 {
		return nil
	}
	// The list is joined over every project: a homonym parent's children are
	// another project's.
	byType := map[string][]cwmapi.Object{}
	for i, k := range got.Objects {
		if p.ours(got.Projects, i) {
			byType[k.Type] = append(byType[k.Type], k)
		}
	}
	if len(byType) == 0 {
		return nil
	}
	out := make([]api.ObjectChildGroup, 0, len(byType))
	for typeName, objs := range byType {
		sort.Slice(objs, func(i, j int) bool { return objs[i].ID < objs[j].ID })
		g := api.ObjectChildGroup{Type: typeName, Label: typeName, Count: len(objs), Objects: objs}
		t, typeOK := p.typeOf(typeName)
		if typeOK {
			if t.Label != "" {
				g.Label = t.Label
			}
			g.Icon = t.Icon
			if g.Icon == "" {
				g.Icon = t.IconName
			}
		}
		g.Plural = g.Label
		if t.LabelPlural != "" {
			g.Plural = t.LabelPlural
		}
		g.Drawn = p.drawn(typeName)
		if !g.Drawn {
			g.Rows = p.childRows(t, objs)
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// childRows is what a children tab shows of each object beside its name: its
// state and its key fields — the badges its type (`pane.badges`) and its
// effective capabilities declare, resolved over the resident record as the
// header resolves them.
func (p *pane) childRows(t cwmapi.ObjectType, objs []cwmapi.Object) []api.ObjectChildRow {
	var typeBadges []cwmapi.BadgeDef
	if ps, err := api.PaneOf(t); err == nil {
		typeBadges = ps.Badges
	}
	stateID := cwmapi.CapLifecycle + "." + lifecycleStateField
	rows := make([]api.ObjectChildRow, 0, len(objs))
	for _, o := range objs {
		row := api.ObjectChildRow{ID: o.ID}
		defs := append([]cwmapi.BadgeDef(nil), typeBadges...)
		carriesLifecycle := false
		if surf, err := p.cwm.ObjectEffectiveSurface(p.ctx, cwmapi.ObjectEffectiveSurfaceInput{ID: p.ref(o.ID)}); err == nil {
			defs = append(defs, surf.Badges...)
		}
		if eff, err := p.cwm.ObjectEffectiveCapabilities(p.ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: p.ref(o.ID)}); err == nil {
			for _, c := range eff.Capabilities {
				carriesLifecycle = carriesLifecycle || c.ID == cwmapi.CapLifecycle
			}
		}
		var stateDef *cwmapi.BadgeDef
		for _, b := range badges(withoutBadges(o), defs) {
			if b.ID == stateID {
				continue
			}
			row.Badges = append(row.Badges, b)
		}
		for i := range defs {
			if defs[i].Capability == cwmapi.CapLifecycle && defs[i].Field == lifecycleStateField {
				stateDef = &defs[i]
			}
		}
		if carriesLifecycle {
			if state := p.stateOf(t, o); state != "" {
				b := cwmapi.Badge{ID: stateID, Label: state, Capability: cwmapi.CapLifecycle}
				if stateDef != nil {
					b.Icon, b.Tooltip = stateDef.Icon, stateDef.Tooltip
					if b.Color = stateDef.Colors[state]; b.Color == "" {
						b.Color = stateDef.Colors["*"]
					}
				}
				row.State = &b
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// withoutBadges is the object without its own set badges: a list row shows
// the declared key fields, not the header's free chips.
func withoutBadges(o cwmapi.Object) cwmapi.Object {
	o.Badges = nil
	return o
}

// lifecycleStateField is the field of the lifecycle group holding the state.
const lifecycleStateField = "state"

// stateOf is an object's lifecycle state: the stored one, else the initial
// state of its type's workflow — an object that never moved is in it.
func (p *pane) stateOf(t cwmapi.ObjectType, o cwmapi.Object) string {
	lc, _ := o.Lifecycle()
	if lc.State != "" {
		return lc.State
	}
	if wf, ok := p.decl.workflow(t.Workflow); ok {
		return wf.Initial
	}
	return ""
}

// panels builds the tab strip. Every declared `object-panel` whose filter
// admits the object is one entry, once; then the type's own fields section,
// then each effective capability with a schema no matched panel names. A
// panel whose `when` cannot be read is NOT shown. Every expression — the
// filters' `when` and the contents' `${...}` — is evaluated by CWM in the
// object's vocabulary (ObjectEvaluate), so a function is asked once. It also
// answers the child types the shown panels stand for (`replaces_children`).
func (p *pane) panels(obj cwmapi.Object, effective []cwmapi.CapabilityRef, edges []cwmapi.Edge) ([]api.ObjectPanel, map[string]bool) {
	capIDs := make([]string, 0, len(effective))
	for _, c := range effective {
		capIDs = append(capIDs, c.ID)
	}
	t, typeOK := p.typeOf(obj.Type)
	if !typeOK && obj.Type != "" {
		t = cwmapi.ObjectType{Name: obj.Type} // undeclared: its body still shows
	}

	// The candidates — admitted by type, capabilities and edges — and every
	// expression they need.
	var cands []api.PanelDecl
	seen := map[string]bool{}
	var exprs []string
	for _, d := range p.decl.Panels {
		if d.ID == "" || seen[d.ID] || !d.OnObjects() {
			continue
		}
		if ok, _ := d.Filter.Matches(cwmapi.FilterSubject{Type: obj.Type, Capabilities: capIDs, Eval: func(string) (bool, error) { return true, nil }}.WithEdges(obj.ID, edges)); !ok {
			continue
		}
		seen[d.ID] = true
		cands = append(cands, d)
		if d.Filter.When != "" {
			exprs = append(exprs, d.Filter.When)
		}
		if d.Content != nil {
			exprs = append(exprs, templatesOf(*d.Content)...)
		}
		exprs = append(exprs, el.TemplateExpressions(d.Name)...)
	}
	// The type's `pane` section: its view of the object, if it declares one.
	ps, _ := api.PaneOf(t)
	if ps.View != nil {
		exprs = append(exprs, templatesOf(*ps.View)...)
	}
	answers := p.evaluate(p.ref(obj.ID), exprs)

	subject := cwmapi.FilterSubject{Type: obj.Type, Capabilities: capIDs, Eval: func(expr string) (bool, error) {
		a, ok := answers[expr]
		if !ok || a.err != "" {
			return false, fmt.Errorf("%s", a.err)
		}
		b, isBool := a.value.(bool)
		if !isBool {
			return false, fmt.Errorf("%w (got %T)", el.ErrNotBool, a.value)
		}
		return b, nil
	}}.WithEdges(obj.ID, edges)
	resolve := func(d render.Def) *render.Def { return resolveRenderDef(d, answers) }

	var out []api.ObjectPanel
	covered := map[string]bool{}
	replaced := map[string]bool{}
	for _, d := range cands {
		if ok, _ := d.Filter.Matches(subject); !ok {
			continue
		}
		e := api.ObjectPanel{
			ID: PanelTab(d.ID), Name: panelName(d.Name, answers), Description: d.Description, Icon: d.Icon,
			Order: d.EffectiveOrder(), Capabilities: d.Filter.Capabilities, Origin: "panel", Input: d.Input, Empty: d.Empty,
			Header: d.Header, Count: d.Count,
		}
		r := d.EffectiveRefresh()
		e.Refresh = &r
		if e.Name == "" {
			e.Name = d.ID
		}
		if d.Badge != nil {
			e.Badge = tabBadge(obj, *d.Badge)
		}
		switch {
		case d.Source != "":
			e.Kind, e.Source = api.PanelKindSource, d.Source
			if d.Content != nil {
				e.Content = resolve(*d.Content) // Source answers the data; Content is the display it fills
			}
		case d.Content != nil:
			e.Kind, e.Content = api.PanelKindContent, resolve(*d.Content)
		default:
			continue // declares nothing to show; the census refuses it upstream
		}
		for _, c := range d.Filter.Capabilities {
			covered[c] = true
		}
		for _, t := range d.ReplacesChildren {
			replaced[t] = true
		}
		out = append(out, e)
	}

	// The type's own fields: its `pane.view` when it declares one, else a form from
	// its `fields:` data-type — and the body, as the type declares it.
	if !seen[cwmapi.FieldsPanel] && (ps.View != nil || t.Fields != "" || t.Body != nil || obj.Body != "") {
		name := t.Label
		if name == "" {
			name = t.Name
		}
		e := api.ObjectPanel{
			ID: FieldsSection, Name: name, Order: 3, Origin: "type", Section: true,
			Capabilities: []string{cwmapi.FieldsPanel}, Icon: &contribapi.ContributionIcon{Tabler: "list-details", Emoji: "📋"},
		}
		if ps.View != nil {
			e.Kind, e.Content = api.PanelKindContent, resolve(*ps.View)
		} else {
			e.Kind, e.Schema = api.PanelKindFields, t.Fields
		}
		if t.Body != nil || obj.Body != "" {
			body := t.BodyOf()
			e.Body = &body
		}
		out = append(out, e)
	}

	caps := p.decl.capabilities()
	for _, c := range effective {
		// An UNDECLARED capability has no schema, so it draws no entry; it is
		// still listed in the pane's capabilities.
		cp := caps[c.ID]
		if cp.Schema == "" || covered[c.ID] {
			continue
		}
		origin := "type"
		if _, onInstance := obj.Capabilities[c.ID]; onInstance {
			origin = "object"
		}
		name := cp.Label
		if name == "" {
			name = c.ID
		}
		out = append(out, api.ObjectPanel{
			ID: CapabilitySection(c.ID), Name: name, Kind: api.PanelKindFields, Order: api.DefaultPanelOrder,
			Capabilities: []string{c.ID}, Schema: cp.Schema, Verify: c.Verify, Origin: origin, Section: true,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out, replaced
}

type answer struct {
	value any
	err   string
}

// evaluate asks CWM for every expression in one evaluation.
func (p *pane) evaluate(id string, exprs []string) map[string]answer {
	out := map[string]answer{}
	var uniq []string
	for _, e := range exprs {
		if _, dup := out[e]; dup || e == "" {
			continue
		}
		out[e] = answer{}
		uniq = append(uniq, e)
	}
	if len(uniq) == 0 {
		return out
	}
	got, err := p.cwm.ObjectEvaluate(p.ctx, cwmapi.ObjectEvaluateInput{ID: id, Expressions: uniq, NoCostly: true})
	if err != nil {
		for _, e := range uniq {
			out[e] = answer{err: err.Error()}
		}
		return out
	}
	for _, r := range got.Results {
		out[r.Expression] = answer{value: r.Value, err: r.Error}
	}
	return out
}

// panelName is a declared panel's tab name: as written, or - a name carrying
// `${...}` - evaluated over the object with the panel's other expressions (a
// source's « what it takes » tab named by its connector). An answer that is
// empty or not a string leaves no name, so the tab falls back to its id.
func panelName(name string, answers map[string]answer) string {
	if len(el.TemplateExpressions(name)) == 0 {
		return name
	}
	s, ok := interpolateTree(name, answers).(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// templatesOf lists the `${...}` expressions a render definition carries.
func templatesOf(d render.Def) []string {
	var out []string
	walkStrings(defTree(d), func(s string) {
		if !holdsHostTemplate(s) {
			out = append(out, el.TemplateExpressions(s)...)
		}
	})
	return out
}

// holdsHostTemplate reports whether a string carries a template only the page
// host can fill (`${item.id}` in a row action, render.HostTemplates): the pane
// keeps it as written.
func holdsHostTemplate(s string) bool {
	for _, expr := range el.TemplateExpressions(s) {
		if render.IsHostTemplate(expr) {
			return true
		}
	}
	return false
}

func defTree(d render.Def) any {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil
	}
	var tree any
	if json.Unmarshal(raw, &tree) != nil {
		return nil
	}
	return tree
}

func walkStrings(v any, fn func(string)) {
	switch t := v.(type) {
	case map[string]any:
		for _, x := range t {
			walkStrings(x, fn)
		}
	case []any:
		for _, x := range t {
			walkStrings(x, fn)
		}
	case string:
		fn(t)
	}
}

// resolveRenderDef resolves every `${...}` string in a panel's content. An
// expression that cannot be resolved becomes EMPTY, never the literal
// template. An extension's input is passed verbatim: it is the extension's own
// contract and may carry templates only it can resolve.
func resolveRenderDef(d render.Def, answers map[string]answer) *render.Def {
	tree := defTree(d)
	if tree == nil {
		return &d
	}
	raw, err := json.Marshal(interpolateTree(tree, answers))
	if err != nil {
		return &d
	}
	var out render.Def
	if json.Unmarshal(raw, &out) != nil {
		return &d
	}
	if d.Kind == "extension" {
		out.Data = d.Data
	}
	return &out
}

func interpolateTree(v any, answers map[string]answer) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = interpolateTree(x, answers)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = interpolateTree(x, answers)
		}
		return out
	case string:
		if holdsHostTemplate(t) {
			return t // the host fills it when the action runs
		}
		val := el.Interpolate(t, func(expr string) (any, bool) {
			a := answers[expr]
			return a.value, a.err == "" && a.value != nil
		})
		if s, ok := val.(string); ok && strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
			return ""
		}
		return val
	}
	return v
}

// badges resolves the header badges: the object's own, then each declared one
// whose field has a value.
func badges(obj cwmapi.Object, defs []cwmapi.BadgeDef) []cwmapi.Badge {
	out := append([]cwmapi.Badge(nil), obj.Badges...)
	for _, d := range defs {
		v := badgeValue(obj, d)
		if v == "" {
			continue
		}
		label := v
		if d.Label != "" {
			label = d.Label + ": " + v
		}
		color := d.Colors[v]
		if color == "" {
			color = d.Colors["*"]
		}
		id := d.Field
		if d.Capability != "" && !strings.Contains(d.Field, ".") {
			id = d.Capability + "." + d.Field
		}
		out = append(out, cwmapi.Badge{ID: id, Label: label, Icon: d.Icon, Color: color, Tooltip: d.Tooltip, Capability: d.Capability})
	}
	return out
}

// tabBadge is a declared panel's tab badge: its field's
// value read from the object as a header chip's, in its tone - `colors` by
// value, `*` for any -; none when the value is empty or 0.
func tabBadge(obj cwmapi.Object, d cwmapi.BadgeDef) *cwmapi.Badge {
	v := badgeValue(obj, d)
	if v == "" || v == "0" {
		return nil
	}
	b := badges(withoutBadges(obj), []cwmapi.BadgeDef{d})
	if len(b) == 0 {
		return nil
	}
	return &b[0]
}

// badgeValue reads the field a badge shows: a capability's own field, or a
// path — `<capability>.<field>`, `fields.<key>` (the type's own), or a core
// field.
func badgeValue(obj cwmapi.Object, d cwmapi.BadgeDef) string {
	field := d.Field
	if d.Capability != "" && !strings.Contains(field, ".") {
		return scalarOf(obj.Capabilities[d.Capability][field])
	}
	head, rest, _ := strings.Cut(field, ".")
	switch head {
	case "name":
		return obj.Name
	case "description":
		return obj.Description
	case "slug":
		return obj.Slug
	case "icon":
		return obj.Icon
	case "parent":
		return obj.Parent
	case "fields":
		if rest != "" {
			return scalarOf(obj.Fields[rest])
		}
		return ""
	}
	if rest == "" {
		return ""
	}
	return scalarOf(obj.Capabilities[head][rest])
}

func scalarOf(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool, int, int64, float64:
		return fmt.Sprint(t)
	}
	return ""
}

// firstOf is the first non-empty string.
func firstOf(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
