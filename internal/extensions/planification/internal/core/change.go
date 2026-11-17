// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// Changes implements api.ChangeService over the Object API: an entry is a
// `change` object in sys-planification whose parent is its release.
type Changes struct{}

// numbering serialises Add: an entry's number is the highest of every entry
// plus one, so two writers in the process serving the workspace never take
// the same one.
var numbering sync.Mutex

// NewChanges returns the change-log service.
func NewChanges() *Changes { return &Changes{} }

var _ api.ChangeService = (*Changes)(nil)

// findRelease reads a release by its id or by its version (the project's
// release of that version first, else a module's).
func findRelease(ctx context.Context, cwm cwmapi.CWMService, ref string) (cwmapi.Object, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return cwmapi.Object{}, invalid("a release is required: its id or its version")
	}
	if strings.Contains(ref, ".RL-") {
		o, err := loadRelease(ctx, cwm, ref)
		if err == nil && o.Type != api.ReleaseType {
			return cwmapi.Object{}, invalid("%s is an increment: a change ships in a release", ref)
		}
		return o, err
	}
	all, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: api.ReleaseType})
	if err != nil {
		return cwmapi.Object{}, err
	}
	var found []cwmapi.Object
	for _, o := range all.Objects {
		if releaseFields(o).Version == ref {
			found = append(found, o)
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		mi, _ := cwmapi.ModuleOfObjectID(found[i].ID)
		return mi == api.SystemModule
	})
	if len(found) == 0 {
		return cwmapi.Object{}, invalid("no release of version %q", ref)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: found[0].ID})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return got.Object, nil
}

// entries reads every change, in full.
func (c *Changes) entries(ctx context.Context, cwm cwmapi.CWMService) ([]cwmapi.Object, error) {
	all, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: api.SystemModule, Type: api.ChangeType})
	if err != nil {
		return nil, err
	}
	out := make([]cwmapi.Object, 0, len(all.Objects))
	for _, o := range all.Objects {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: api.SystemModule, ID: o.ID})
		if err != nil {
			return nil, err
		}
		out = append(out, got.Object)
	}
	return out, nil
}

func changeFields(o cwmapi.Object) api.ChangeFields {
	f, _, _ := cwmapi.DecodeCapability[api.ChangeFields](o, api.CapChange)
	return f
}

// toChange reads an entry; rels caches its releases.
func toChange(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, rels map[string]cwmapi.Object) api.Change {
	ch := api.Change{ID: o.ID, Release: o.ParentID(), Title: o.Name, Text: o.Body, ChangeFields: changeFields(o)}
	rel, ok := rels[ch.Release]
	if !ok && ch.Release != "" {
		if r, err := loadRelease(ctx, cwm, ch.Release); err == nil {
			rel, ok = r, true
		}
		rels[ch.Release] = rel
	}
	if ok {
		ch.Version, ch.Stream = releaseFields(rel).Version, edgeTo(rel, api.EdgeInStream)
	}
	return ch
}

// changeModules is the modules the named requests and features belong to.
func changeModules(ids []string) []string {
	var out []string
	for _, id := range ids {
		if m, err := cwmapi.ModuleOfObjectID(id); err == nil && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

func checkValue(verb, v string) error {
	if !slices.Contains(api.ValueOrder, v) {
		return invalid("%s: value %q is none of major, notable, improvement or fix", verb, v)
	}
	return nil
}

// Add records a change under its release, numbered after every entry.
func (c *Changes) Add(ctx context.Context, in api.AddChangeInput) (api.ChangeOutput, error) {
	title, text, domain := strings.TrimSpace(in.Title), strings.TrimSpace(in.Text), strings.TrimSpace(in.Domain)
	if title == "" || text == "" || domain == "" {
		return api.ChangeOutput{}, invalid("Add: title, domain and text are required")
	}
	if err := checkValue("Add", in.Value); err != nil {
		return api.ChangeOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ChangeOutput{}, err
	}
	rel, err := findRelease(ctx, cwm, in.Release)
	if err != nil {
		return api.ChangeOutput{}, err
	}
	f := api.ChangeFields{
		Domain: domain, Value: in.Value, Maturity: strings.TrimSpace(in.Maturity), ForWhom: strings.TrimSpace(in.ForWhom),
		Requests: in.Requests, Modules: in.Modules, Date: strings.TrimSpace(in.Date), Update: strings.TrimSpace(in.Update),
	}
	if f.Maturity == "" {
		f.Maturity = "Construit et testé"
	}
	if f.ForWhom == "" {
		f.ForWhom = "tous"
	}
	if len(f.Modules) == 0 {
		f.Modules = changeModules(in.Requests)
	}
	if f.Date == "" {
		f.Date = time.Now().Format("2006-01-02")
	}
	numbering.Lock()
	defer numbering.Unlock()
	all, err := c.entriesResident(ctx, cwm)
	if err != nil {
		return api.ChangeOutput{}, err
	}
	for _, o := range all {
		if n := changeFields(o).Number; n >= f.Number {
			f.Number = n
		}
	}
	f.Number++
	out, err := c.create(ctx, cwm, rel.ID, title, text, f)
	if err != nil {
		return api.ChangeOutput{}, err
	}
	return api.ChangeOutput{Change: toChange(ctx, cwm, out, map[string]cwmapi.Object{rel.ID: rel})}, nil
}

// entriesResident lists the entries without reading their bodies.
func (c *Changes) entriesResident(ctx context.Context, cwm cwmapi.CWMService) ([]cwmapi.Object, error) {
	all, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: api.SystemModule, Type: api.ChangeType})
	if err != nil {
		return nil, err
	}
	return all.Objects, nil
}

func (c *Changes) create(ctx context.Context, cwm cwmapi.CWMService, release, title, text string, f api.ChangeFields) (cwmapi.Object, error) {
	// A datum lives in its field or in the prose, never both (CWM refuses
	// a body repeating a field): who it is for or its maturity said again in
	// its text stays in its badge line, kept as written.
	for _, v := range []string{f.ForWhom, f.Maturity} {
		if len(strings.TrimSpace(v)) >= 20 && strings.Contains(text, strings.TrimSpace(v)) {
			f.Badge = badgeOf(pageEntry{Badge: f.Badge, Value: f.Value, Maturity: f.Maturity, ForWhom: f.ForWhom})
			f.Maturity, f.ForWhom = "", ""
			break
		}
	}
	if u := strings.TrimSpace(f.Update); len(u) >= 20 && strings.Contains(text, u) {
		f.Update = ""
	}
	o := cwmapi.Object{Type: api.ChangeType}
	o.Parent = release
	o.Name = title
	o.Body = text
	if err := o.SetCapability(api.CapChange, f); err != nil {
		return cwmapi.Object{}, err
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: api.SystemModule, Object: o})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return out.Object, nil
}

// Update changes an entry; only what is given changes.
func (c *Changes) Update(ctx context.Context, in api.UpdateChangeInput) (api.ChangeOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ChangeOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: api.SystemModule, ID: in.ID})
	if err != nil {
		return api.ChangeOutput{}, err
	}
	o := got.Object
	if o.Type != api.ChangeType {
		return api.ChangeOutput{}, invalid("Update: %s is a %q, not a change", in.ID, o.Type)
	}
	f := changeFields(o)
	set := func(dst *string, v string) {
		if v = strings.TrimSpace(v); v != "" {
			*dst = v
		}
	}
	set(&o.Name, in.Title)
	set(&o.Body, in.Text)
	set(&f.Domain, in.Domain)
	set(&f.Maturity, in.Maturity)
	set(&f.ForWhom, in.ForWhom)
	set(&f.Update, in.Update)
	if in.Value != "" {
		if err := checkValue("Update", in.Value); err != nil {
			return api.ChangeOutput{}, err
		}
		f.Value, f.Badge = in.Value, ""
	}
	if in.Maturity != "" || in.ForWhom != "" {
		f.Badge = ""
	}
	if len(in.Requests) > 0 {
		f.Requests, f.Modules = in.Requests, changeModules(in.Requests)
	}
	if strings.TrimSpace(in.Release) != "" {
		to, err := moveTo(ctx, cwm, o.ParentID(), in.Release)
		if err != nil {
			return api.ChangeOutput{}, err
		}
		o.Parent = to
	}
	if err := o.SetCapability(api.CapChange, f); err != nil {
		return api.ChangeOutput{}, err
	}
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: api.SystemModule, Object: o, Revision: got.Revision})
	if err != nil {
		return api.ChangeOutput{}, err
	}
	return api.ChangeOutput{Change: toChange(ctx, cwm, out.Object, map[string]cwmapi.Object{})}, nil
}

// moveTo is the release an entry moves to: a release
// already released keeps what it shipped, and nothing ships in a released or
// dropped one. Its number is kept: numbers run across the releases.
func moveTo(ctx context.Context, cwm cwmapi.CWMService, from, ref string) (string, error) {
	to, err := findRelease(ctx, cwm, ref)
	if err != nil {
		return "", err
	}
	if s := releaseState(to); s == api.ReleaseReleased || s == api.ReleaseDropped {
		return "", invalid("Update: %s is %s - an entry moves only to a release still to ship", to.ID, s)
	}
	if from != "" && from != to.ID {
		if rel, err := loadRelease(ctx, cwm, from); err == nil && releaseState(rel) == api.ReleaseReleased {
			return "", invalid("Update: %s is released - what it shipped stays in its change log", from)
		}
	}
	return to.ID, nil
}

// Get reads one entry.
func (c *Changes) Get(ctx context.Context, in api.GetInput) (api.ChangeOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ChangeOutput{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: api.SystemModule, ID: in.ID})
	if err != nil {
		return api.ChangeOutput{}, err
	}
	if got.Object.Type != api.ChangeType {
		return api.ChangeOutput{}, invalid("Get: %s is a %q, not a change", in.ID, got.Object.Type)
	}
	return api.ChangeOutput{Change: toChange(ctx, cwm, got.Object, map[string]cwmapi.Object{})}, nil
}

// List reads the entries a filter keeps, by number then id.
func (c *Changes) List(ctx context.Context, in api.ListChangesInput) (api.ListChangesOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ListChangesOutput{}, err
	}
	release := ""
	if in.Release != "" {
		rel, err := findRelease(ctx, cwm, in.Release)
		if err != nil {
			return api.ListChangesOutput{}, err
		}
		release = rel.ID
	}
	all, err := c.entries(ctx, cwm)
	if err != nil {
		return api.ListChangesOutput{}, err
	}
	words := strings.Fields(strings.ToLower(in.Text))
	rels := map[string]cwmapi.Object{}
	out := api.ListChangesOutput{Changes: []api.Change{}}
	for _, o := range all {
		ch := toChange(ctx, cwm, o, rels)
		switch {
		case release != "" && ch.Release != release,
			in.Stream != "" && ch.Stream != in.Stream,
			in.Module != "" && !slices.Contains(ch.Modules, in.Module),
			in.From != "" && (ch.Date == "" || ch.Date < in.From),
			in.To != "" && (ch.Date == "" || ch.Date > in.To):
			continue
		}
		hay := strings.ToLower(ch.Title + "\n" + ch.Text)
		if !slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(hay, w) }) {
			out.Changes = append(out.Changes, ch)
		}
	}
	sort.SliceStable(out.Changes, func(i, j int) bool {
		if out.Changes[i].Number != out.Changes[j].Number {
			return out.Changes[i].Number < out.Changes[j].Number
		}
		return out.Changes[i].ID < out.Changes[j].ID
	})
	return out, nil
}

func pagePath(version string) string { return api.ChangeLogDir + "/" + version + ".md" }

// pageRoot is the project root a release's change-log page lives under: the
// root of the project CWM serves the release from, never the process-wide
// one when the release resolves - a world of its own writes its own page.
func pageRoot(ctx context.Context, cwm cwmapi.CWMService, release string) string {
	if r, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: release}); err == nil && r.ProjectRoot != "" {
		return r.ProjectRoot
	}
	return application.ProjectRoot()
}

// Render writes a release's page from its head, its entries and its tail.
func (c *Changes) Render(ctx context.Context, in api.RenderChangeLogInput) (api.RenderChangeLogOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RenderChangeLogOutput{}, err
	}
	rel, err := findRelease(ctx, cwm, in.Release)
	if err != nil {
		return api.RenderChangeLogOutput{}, err
	}
	list, err := c.List(ctx, api.ListChangesInput{Release: rel.ID})
	if err != nil {
		return api.RenderChangeLogOutput{}, err
	}
	entries := make([]pageEntry, 0, len(list.Changes))
	for _, ch := range list.Changes {
		entries = append(entries, pageEntry{
			Number: ch.Number, Label: ch.Label, Title: ch.Title, Domain: ch.Domain, Badge: ch.Badge, Value: ch.Value,
			Maturity: ch.Maturity, ForWhom: ch.ForWhom, Text: ch.Text, Date: ch.Date, Update: ch.Update,
		})
	}
	f := releaseFields(rel)
	if blankPageEnds(f) {
		if f, err = adoptPageHead(ctx, cwm, rel, f, entries, in.Write); err != nil {
			return api.RenderChangeLogOutput{}, err
		}
	}
	out := api.RenderChangeLogOutput{
		Release: rel.ID, Path: pagePath(f.Version), Changes: len(entries),
		Markdown: renderPage(f.ChangeLogHead, f.ChangeLogTail, entries),
	}
	if in.Write {
		path := filepath.Join(pageRoot(ctx, cwm, rel.ID), filepath.FromSlash(out.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return out, err
		}
		if err := os.WriteFile(path, []byte(out.Markdown), 0o644); err != nil {
			return out, err
		}
		out.Written = true
	}
	return out, nil
}

// adoptPageHead takes the head and tail of a release's existing page when the
// release holds none (its page was never imported), so a render keeps the
// page's frontmatter, info box and closing sections instead of writing them
// away; written, they go on the release. A page listing a change the release
// does not hold is refused: rendering would drop it - import the page first.
func adoptPageHead(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, f api.ReleaseFields, entries []pageEntry, write bool) (api.ReleaseFields, error) {
	raw, err := os.ReadFile(filepath.Join(pageRoot(ctx, cwm, rel.ID), filepath.FromSlash(pagePath(f.Version))))
	if err != nil {
		return f, nil // no page yet: rendered from the entries alone
	}
	page := parsePage(string(raw))
	held := map[int]bool{}
	for _, e := range entries {
		held[e.Number] = true
	}
	for _, e := range page.Entries {
		if !held[e.Number] {
			return f, invalid("Render: the page %s lists change %d %q that %s does not hold, and %s holds no page head: move its entries out, run changes import, move them back",
				pagePath(f.Version), e.Number, e.Title, rel.ID, rel.ID)
		}
	}
	f.ChangeLogHead, f.ChangeLogTail = page.Head, page.Tail
	if !write || blankPageEnds(f) {
		return f, nil
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: rel.ID})
	if err != nil {
		return f, err
	}
	r := got.Object
	stored := releaseFields(r)
	stored.ChangeLogHead, stored.ChangeLogTail = page.Head, page.Tail
	if err := r.SetCapability(api.CapRelease, stored); err != nil {
		return f, err
	}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Object: r, Revision: got.Revision}); err != nil {
		return f, err
	}
	return f, nil
}

// blankPageEnds says a release holds no page head nor tail: none, or only
// blank lines - a page rendered with no head leaves a
// newline, which must not keep a head written on the page later from being adopted.
func blankPageEnds(f api.ReleaseFields) bool {
	return strings.TrimSpace(f.ChangeLogHead) == "" && strings.TrimSpace(f.ChangeLogTail) == ""
}

// Import reads a release's page into entries, once; the page's head and tail
// go on the release.
func (c *Changes) Import(ctx context.Context, in api.ImportChangeLogInput) (api.ImportChangeLogOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ImportChangeLogOutput{}, err
	}
	rel, err := findRelease(ctx, cwm, in.Release)
	if err != nil {
		return api.ImportChangeLogOutput{}, err
	}
	have, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: api.SystemModule, Type: api.ChangeType, Parent: rel.ID})
	if err != nil {
		return api.ImportChangeLogOutput{}, err
	}
	if len(have.Objects) > 0 {
		return api.ImportChangeLogOutput{}, invalid("Import: %s already has %d entries", rel.ID, len(have.Objects))
	}
	fields := releaseFields(rel)
	md := in.Markdown
	if md == "" {
		raw, err := os.ReadFile(filepath.Join(pageRoot(ctx, cwm, rel.ID), filepath.FromSlash(pagePath(fields.Version))))
		if err != nil {
			return api.ImportChangeLogOutput{}, fmt.Errorf("Import: the page of %s: %w", rel.ID, err)
		}
		md = string(raw)
	}
	page := parsePage(md)
	out := api.ImportChangeLogOutput{Release: rel.ID, Changes: []string{}}
	for _, e := range page.Entries {
		f := api.ChangeFields{
			Number: e.Number, Label: e.Label, Domain: e.Domain, Value: e.Value, Maturity: e.Maturity, ForWhom: e.ForWhom,
			Badge: e.Badge, Date: e.Date, Update: e.Update,
		}
		f.Requests = requestsNamed(e.Text)
		f.Modules = changeModules(f.Requests)
		o, err := c.create(ctx, cwm, rel.ID, e.Title, e.Text, f)
		if err != nil {
			return out, fmt.Errorf("Import: entry %d %q: %w", e.Number, e.Title, err)
		}
		out.Changes = append(out.Changes, o.ID)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: rel.ID})
	if err != nil {
		return out, err
	}
	r := got.Object
	fields = releaseFields(r)
	fields.ChangeLogHead, fields.ChangeLogTail = page.Head, page.Tail
	if err := r.SetCapability(api.CapRelease, fields); err != nil {
		return out, err
	}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Object: r, Revision: got.Revision}); err != nil {
		return out, err
	}
	return out, nil
}

// requestsNamed is the ids an entry's text names in brackets ([
func requestsNamed(text string) []string {
	var out []string
	for _, m := range bracketIDs.FindAllStringSubmatch(text, -1) {
		for _, id := range strings.Split(m[1], ",") {
			id = strings.TrimSpace(id)
			if objectRef.MatchString(id) && !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}
