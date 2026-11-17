// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// asIsModel is what one source of the as-is study declares through the
// source model alone: who it is, which of the six operations
// it has (sync, test, browse, search, propose, fetch - the others declared
// absent by their empty field), its scopes, the data-types of its location
// and its items, what it reads from another source, and how it is picked.
type asIsModel struct {
	sidecar string // the namespace every operation and data-type it names lives in
	label   string
	icon    string
	color   string
	ops     []string
	scopes  *api.ConnectorScopes
	loc     bool
	items   []string
	from    *api.ConnectorFrom
	unit    string
	layout  string
}

var asIsSources = map[string]asIsModel{
	"example-application": {sidecar: "ktw-example", label: "Applications Example", icon: "building", color: "teal",
		ops: []string{"sync", "test", "browse", "search", "propose"}, loc: true, items: []string{"application", "interface"},
		unit: "application", layout: "list"},
	"sample-pages": {sidecar: "ktw-sample", label: "Pages Sample", icon: "file-text", color: "blue",
		ops:    []string{"sync", "test", "browse", "search", "propose", "fetch"},
		scopes: &api.ConnectorScopes{Unit: "espace", List: "ktw-sample:sample/source-scopes", Add: true},
		unit:   "page", layout: "tree"},
	"demo-entities": {sidecar: "ktw-demo", label: "Entités Demo", icon: "activity", color: "orange",
		ops: []string{"sync", "test", "browse"}, loc: true,
		from: &api.ConnectorFrom{Connector: "example-application", Attr: "code"}},
	"documents": {sidecar: "ktw-agents", label: "Documents", icon: "file-upload",
		ops: []string{"sync", "browse"}, unit: "document", layout: "list"},
}

func repoRootOf(t *testing.T) string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(here), "..", "..", "..", "..")
}

// operations answers the six operations of a declaration by name.
func operations(c api.ConnectorDecl) map[string]string {
	return map[string]string{"sync": c.Sync, "test": c.Test, "browse": c.Browse, "search": c.Search, "propose": c.Propose, "fetch": c.Fetch}
}

// : the four sources of a study - applications, pages in
// spaces, entities read from the applications (three made-up sidecars), and
// Documents - declare everything they need through the source model alone,
// each read from its sidecar's embedded entry through the source-connector
// kind:
// who it is, the six operations or their absence (each in its own sidecar),
// its scopes, its location and item data-types (each authored where it
// names), what it reads from (a connector of the study, an attribute its
// items carry), how it is picked; and the picker view carries that model.
func TestTheAsIsSourcesDeclareTheModel(t *testing.T) {
	ctx, cwm := cwmapp.New(t,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		exampleConnectors("ktw-sample"),
		exampleConnectors("ktw-example"),
		exampleConnectors("ktw-demo"),
		cwmapp.Pack(fstest.MapFS{"object-types/module.yaml": {Data: []byte("name: module\ntree: node\n")}}),
		sourceslocal.Extension(),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.KnowledgeCapability}); err != nil {
		t.Fatal(err)
	}
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	list, err := svc.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		t.Fatal(err)
	}
	decls := map[string]api.ConnectorDecl{}
	for _, c := range list.Connectors {
		decls[c.Name] = c
	}
	pk, err := svc.Pickers(ctx, api.PickersInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	views := map[string]api.ConnectorPickerView{}
	for _, v := range pk.Connectors {
		views[v.Name] = v
	}

	for name, want := range asIsSources {
		c, ok := decls[name]
		if !ok {
			t.Errorf("%s: not declared", name)
			continue
		}
		// Who it is.
		if c.Label != want.label || c.Icon != want.icon || c.Color != want.color || strings.TrimSpace(c.Description) == "" {
			t.Errorf("%s: identity label=%q icon=%q color=%q description=%q; want %q %q %q and a description", name, c.Label, c.Icon, c.Color, c.Description, want.label, want.icon, want.color)
		}
		// The six operations, each declared or declared absent, each its
		// own sidecar's method.
		has := map[string]bool{}
		for _, op := range want.ops {
			has[op] = true
		}
		for op, uri := range operations(c) {
			switch {
			case has[op] && uri == "":
				t.Errorf("%s: declares no %s", name, op)
			case !has[op] && uri != "":
				t.Errorf("%s: declares %s %q the as-is model says it has not", name, op, uri)
			case uri != "" && !strings.HasPrefix(uri, want.sidecar+":"):
				t.Errorf("%s: its %s %q is not its sidecar's (%s)", name, op, uri, want.sidecar)
			}
		}
		// Its scopes.
		if !reflect.DeepEqual(c.Scopes, want.scopes) {
			t.Errorf("%s: scopes %+v; want %+v", name, c.Scopes, want.scopes)
		}
		// Its location and item data-types, each authored where it names.
		if (c.Location != "") != want.loc {
			t.Errorf("%s: location %q; want one: %v", name, c.Location, want.loc)
		}
		var kinds []string
		for k := range c.Items {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds) // a map's order is random; want.items is sorted
		if !reflect.DeepEqual(kinds, want.items) && !(len(kinds) == 0 && len(want.items) == 0) {
			t.Errorf("%s: item kinds %v; want %v", name, kinds, want.items)
		}
		dts := []string{}
		if c.Location != "" {
			dts = append(dts, c.Location)
		}
		for _, dt := range c.Items {
			dts = append(dts, dt)
		}
		for _, dt := range dts {
			if !strings.HasPrefix(dt, want.sidecar+":/data-type/") {
				t.Errorf("%s: data-type %q is not its sidecar's", name, dt)
			} else if _, ok := exampleDataType(dt); !ok {
				t.Errorf("%s: data-type %q is authored nowhere", name, dt)
			}
		}
		// What it reads from: a source of the study, by an attribute its
		// items' data-type declares.
		if !reflect.DeepEqual(c.From, want.from) {
			t.Errorf("%s: from %+v; want %+v", name, c.From, want.from)
		} else if c.From != nil {
			followed, ok := decls[c.From.Connector]
			if _, ofStudy := asIsSources[c.From.Connector]; !ok || !ofStudy {
				t.Errorf("%s: reads from %q, no source of the study", name, c.From.Connector)
			}
			attr := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(c.From.Attr) + `:\s*$`)
			found := false
			for _, dt := range followed.Items {
				raw, ok := exampleDataType(dt)
				if ok && attr.Match(raw) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: reads %s's %q, which no item data-type of theirs declares", name, c.From.Connector, c.From.Attr)
			}
		}
		// How it is picked: by a search or a browse, or from another source.
		if c.Search == "" && c.Browse == "" && c.From == nil {
			t.Errorf("%s: can be picked neither by search, browse nor from another source", name)
		}
		var unit, layout string
		if c.Picker != nil {
			unit, layout = c.Picker.Unit, c.Picker.Layout
		}
		if unit != want.unit || layout != want.layout {
			t.Errorf("%s: picker unit=%q layout=%q; want %q %q", name, unit, layout, want.unit, want.layout)
		}
		// The picker view carries the model - every view reads it, never
		// the connector's name.
		v, ok := views[name]
		if !ok {
			t.Errorf("%s: no picker view", name)
			continue
		}
		if v.Label != c.Label || v.Icon != c.Icon || v.Color != c.Color || !reflect.DeepEqual(v.Scopes, c.Scopes) ||
			!reflect.DeepEqual(v.From, c.From) || v.Fetch != (c.Fetch != "") || v.Propose != (c.Propose != "") {
			t.Errorf("%s: the picker view does not carry the source model: %+v", name, v)
		}
	}
}

// goCode blanks a Go source's comments and raw strings (struct tags, whose
// jsonschema descriptions give examples), line numbers unchanged.
func goCode(src string) string {
	var b strings.Builder
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				b.WriteByte('\n')
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*', c == '`':
			end, skip := "*/", 2
			if c == '`' {
				end, skip = "`", 1
			}
			j := strings.Index(src[i+skip:], end)
			if j < 0 {
				j = len(src) - i - skip
			}
			b.WriteString(strings.Repeat("\n", strings.Count(src[i:i+skip+j], "\n")))
			i += skip + j + len(end) - 1
		case c == '"' || c == '\'':
			b.WriteByte(c)
			for i++; i < len(src) && src[i] != c && src[i] != '\n'; i++ {
				if src[i] == '\\' && i+1 < len(src) {
					b.WriteByte(src[i])
					i++
				}
				b.WriteByte(src[i])
			}
			if i < len(src) {
				b.WriteByte(src[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// : no view of the sources pack names a source of the study -
// its code (comments and struct-tag descriptions aside) and its object
// panels read the connector's declared model, never Confluence, LeanIX or
// Dynatrace. The GUI and the display library are held so by
// TestGUINamesNoPack.
func TestNoSourcesViewNamesASource(t *testing.T) {
	repo := repoRootOf(t)
	// The as-is study's sources: its connectors and the products they read.
	words := []string{"leanix-application", "dynatrace-entities", "confluence", "leanix", "dynatrace"}
	named := regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_])(` + strings.Join(words, "|") + `)(?:$|[^\p{L}\p{N}_-])`)
	root := filepath.Join(repo, "internal", "extensions", "sources")
	checked := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
		var code string
		switch {
		case strings.HasSuffix(rel, "_test.go"):
			return nil
		case strings.HasSuffix(rel, ".go"):
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			code = goCode(string(raw))
		case strings.HasPrefix(rel, "contrib/embed/cwm/object-panels/"):
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			lines := strings.Split(string(raw), "\n")
			for i, l := range lines {
				if strings.HasPrefix(strings.TrimSpace(l), "#") {
					lines[i] = ""
				}
			}
			code = strings.Join(lines, "\n")
		default:
			return nil
		}
		checked++
		for i, line := range strings.Split(code, "\n") {
			if m := named.FindStringSubmatch(line); m != nil {
				t.Errorf("%s:%d names the source %q - a view reads the connector's declared model: %s", rel, i+1, m[1], strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 20 {
		t.Fatalf("only %d files checked: the roots are wrong", checked)
	}
}
