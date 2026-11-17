// SPDX-License-Identifier: Apache-2.0

package template

import (
	"encoding/json"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// testLibrary: an Application template (bound fill, a label with an
// optional description and link, an XLS badge kept only with the xls
// marker), a Flow edge template (a binding composing links), a default
// template, and a page template with a title and two slots.
const testDrawio = `<mxfile><diagram name="templates"><mxGraphModel><root>
<mxCell id="0"/><mxCell id="1" parent="0"/>
<UserObject label="&lt;b&gt;%name%&lt;/b&gt;&lt;br&gt;&lt;i&gt;%description%&lt;/i&gt;&lt;br&gt;&lt;a href=&quot;%external_url%&quot;&gt;%external_url_label%&lt;/a&gt;" placeholders="1" name="Argos" description="Sample" ktw-template="Application" ktw-style="fillColor=fill;strokeColor=stroke" id="app">
  <mxCell style="rounded=1;html=1;fillColor=#d5e8d4;strokeColor=#82B366;" vertex="1" parent="1"><mxGeometry x="40" y="40" width="180" height="70" as="geometry"/></mxCell>
</UserObject>
<UserObject label="XLS" ktw-when="'xls' in (attributes.markers ?? [])" id="app-xls">
  <mxCell style="rounded=1;fillColor=#107C41;" vertex="1" parent="app"><mxGeometry x="166" y="-8" width="28" height="16" as="geometry"/></mxCell>
</UserObject>
<UserObject label="%flLinks%" ktw-template="edge:Flow" ktw-style="strokeColor=flowColor" id="flow">
  <mxCell style="endArrow=block;html=1;strokeColor=#444444;" edge="1" parent="1"><mxGeometry relative="1" as="geometry"><mxPoint x="0" y="0" as="sourcePoint"/><mxPoint x="100" y="0" as="targetPoint"/></mxGeometry></mxCell>
</UserObject>
<mxCell id="dflt" value="%name%" style="rounded=0;" vertex="1" parent="1" ktw-template="*"><mxGeometry x="0" y="200" width="120" height="50" as="geometry"/></mxCell>
</root></mxGraphModel></diagram>
<diagram name="page:default"><mxGraphModel><root>
<mxCell id="0"/><mxCell id="1" parent="0"/>
<mxCell id="title" value="%title% — %date%" style="text;" vertex="1" parent="1"><mxGeometry x="10" y="10" width="400" height="30" as="geometry"/></mxCell>
<mxCell id="c" value="" ktw-slot="content" vertex="1" parent="1"><mxGeometry x="20" y="60" width="800" height="600" as="geometry"/></mxCell>
<mxCell id="l" value="" ktw-slot="legend" vertex="1" parent="1"><mxGeometry x="900" y="60" width="200" height="300" as="geometry"/></mxCell>
</root></mxGraphModel></diagram></mxfile>`

func testLibrary(t *testing.T) *Library {
	t.Helper()
	var bindings map[string]drawioapi.Resolver
	if err := json.Unmarshal([]byte(`{
		"fill":      {"lookup": "attributes.domain", "table": "palette", "field": "fill", "default": "#FFFFFF"},
		"stroke":    {"expr": "'xls' in (attributes.markers ?? []) ? '#107C41' : (constants.palette[attributes.domain ?? '']?.stroke ?? '#000000')"},
		"flLinks":   {"expr": "join(map(attributes.fl_codes ?? [], {'<a href=\"' + #.url + '\">' + #.code + '</a>'}), ' / ')"},
		"flowColor": {"lookup": "attributes.flow_type", "table": "flows", "default": "#444444"}
	}`), &bindings); err != nil {
		t.Fatal(err)
	}
	lib, err := Parse(&drawioapi.TemplateLibraryBody{
		ID: "t", Drawio: testDrawio, Bindings: bindings,
		Constants: map[string]any{
			"palette": map[string]any{"reference": map[string]any{"fill": "#d5e8d4", "stroke": "#82B366"}},
			"flows":   map[string]any{"invoice": "#CC6600"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func render(in *Instance) string {
	var b strings.Builder
	in.Write(&b, 0)
	return b.String()
}

func TestParse_FindsTemplatesAndPages(t *testing.T) {
	lib := testLibrary(t)
	app := lib.Element("Application")
	if app == nil || app.Width != 180 || app.Height != 70 || len(app.parts) != 1 {
		t.Fatalf("Application template: %+v", app)
	}
	if lib.Element("Unknown") == nil || lib.Element("Unknown").Key != "*" {
		t.Error("an unknown kind falls back to the default template")
	}
	if lib.Relationship("Flow") == nil || !lib.Relationship("Flow").Edge {
		t.Error("Flow edge template missing")
	}
	pg := lib.Page("default")
	if pg == nil || pg.Content == nil || pg.Content.X != 20 || pg.Legend == nil || len(pg.cells) != 1 {
		t.Fatalf("page template: %+v", pg)
	}
}

func TestInstantiate_Element(t *testing.T) {
	lib := testLibrary(t)
	scope := NewScope("argos", "Argos <prod>", "Application", map[string]any{
		"domain": "reference", "markers": []any{"xls"}, "external_url": "https://x.test/1", "external_url_label": "LeanIX",
	}, lib.Body.Constants)
	in := lib.Instantiate(lib.Element("Application"), scope, Placement{
		CellID: "n-argos", ParentID: "1", X: 5, Y: 6, Width: 180, Height: 70,
		Identity: []Attr{{"ktw-role", "node"}}, PartRole: []Attr{{"ktw-role", "decoration"}},
	}, nil)
	if len(in.Problems) > 0 {
		t.Fatalf("problems: %v", in.Problems)
	}
	out := render(in)
	for _, want := range []string{
		`id="n-argos"`, `fillColor=#d5e8d4`, `strokeColor=#107C41`, `x="5"`, `y="6"`,
		`&lt;b&gt;Argos &amp;lt;prod&amp;gt;&lt;/b&gt;`, // the model label is escaped inside the HTML label
		`href=&#34;https://x.test/1&#34;`, `id="n-argos-app-xls"`, `parent="n-argos"`, `ktw-role="decoration"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	for _, not := range []string{`ktw-template`, `ktw-style`, `ktw-when`, `description="Sample"`, `placeholders`, `&lt;i&gt;&lt;/i&gt;`} {
		if strings.Contains(out, not) {
			t.Errorf("must not contain %s:\n%s", not, out)
		}
	}
}

func TestInstantiate_ConditionalPartDropped(t *testing.T) {
	lib := testLibrary(t)
	scope := NewScope("a", "A", "Application", map[string]any{"domain": "reference"}, lib.Body.Constants)
	out := render(lib.Instantiate(lib.Element("Application"), scope, Placement{CellID: "n-a", ParentID: "1"}, nil))
	if strings.Contains(out, "XLS") {
		t.Errorf("the badge needs the xls marker:\n%s", out)
	}
	if !strings.Contains(out, "strokeColor=#82B366") {
		t.Errorf("stroke from the palette without the marker:\n%s", out)
	}
}

func TestInstantiate_Edge(t *testing.T) {
	lib := testLibrary(t)
	scope := NewScope("e1", "", "Flow", map[string]any{
		"flow_type": "invoice",
		"fl_codes":  []any{map[string]any{"code": "FL1", "url": "u1"}, map[string]any{"code": "FL2", "url": "u2"}},
	}, lib.Body.Constants)
	out := render(lib.Instantiate(lib.Relationship("Flow"), scope, Placement{
		CellID: "e-e1", ParentID: "1", SourceID: "n-a", TargetID: "n-b", Points: []drawioapi.Point{{X: 1, Y: 2}},
	}, nil))
	for _, want := range []string{`source="n-a"`, `target="n-b"`, `strokeColor=#CC6600`, `FL1&lt;/a&gt; / &lt;a`, `<mxPoint x="1" y="2"/>`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	if strings.Contains(out, "sourcePoint") {
		t.Error("the template's sample end points must not survive")
	}
}

func TestInstantiatePage(t *testing.T) {
	lib := testLibrary(t)
	out := render(lib.InstantiatePage(lib.Page("default"), map[string]string{"title": "Landscape", "date": "2026-09-28"}, []Attr{{"ktw-role", "page"}}))
	if !strings.Contains(out, "Landscape — 2026-09-28") || strings.Contains(out, "ktw-slot") {
		t.Errorf("page frame:\n%s", out)
	}
}

func TestCleanHTML(t *testing.T) {
	for in, want := range map[string]string{
		`<b>A</b><br><i></i><br><a href=""></a>`: `<b>A</b>`,
		`<br><b>A</b><br><br><br>B<br>`:          `<b>A</b><br>B`,
		`<font style="x"><i></i></font>C`:        `C`,
	} {
		if got := cleanHTML(in); got != want {
			t.Errorf("cleanHTML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSwatchStyle(t *testing.T) {
	lib := testLibrary(t)
	if s := lib.SwatchStyle(lib.Element("Application")); !strings.Contains(s, "fillColor=#FFFFFF") {
		t.Errorf("swatch uses the binding default for an empty element: %s", s)
	}
}

// TestSize_Autosize grows a ktw-autosize template to fit its label.
func TestSize_Autosize(t *testing.T) {
	lib := testLibrary(t)
	app := lib.Element("Application")
	if w, h := lib.Size(app, NewScope("a", "A", "Application", nil, nil)); w != 180 || h != 70 {
		t.Errorf("without ktw-autosize the template size stands, got %vx%v", w, h)
	}
	app.root.el.set(attrAutosize, "1")
	scope := NewScope("a", "A", "Application", map[string]any{"description": "d", "external_url": "u", "external_url_label": "L"}, nil)
	if _, h := lib.Size(app, scope); h <= 70-1 && h != 70 {
		t.Errorf("unexpected height %v", h)
	}
	app.root.el.set("label", "%name%<br>1<br>2<br>3<br>4<br>5")
	if _, h := lib.Size(app, scope); h != 6*lineHeight+labelPadding {
		t.Errorf("six lines: want %v, got %v", 6*lineHeight+labelPadding, h)
	}
}
