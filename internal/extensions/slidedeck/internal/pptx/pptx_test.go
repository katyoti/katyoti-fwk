// SPDX-License-Identifier: Apache-2.0

package pptx

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	slidedeckcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib"
	decktesting "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/internal/testing"
	decklint "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/lint"
)

func TestReadInventory(t *testing.T) {
	inv, err := Read(decktesting.Template(t))
	if err != nil {
		t.Fatal(err)
	}
	if inv.Aspect != "16:9" || inv.Width != 1280 || inv.Height != 720 {
		t.Fatalf("page = %s %vx%v", inv.Aspect, inv.Width, inv.Height)
	}
	if inv.Theme.Name != "Acme" || inv.Theme.Major != "Acme Display" || inv.Theme.Minor != "Acme Text" || inv.Theme.Colours["accent1"] != "#0B5394" || inv.Theme.Colours["dk1"] != "#000000" {
		t.Fatalf("theme = %+v", inv.Theme)
	}
	m := inv.Masters[0]
	if m.TitleSize != 48 || !m.TitleBold || m.TitleColour != "#000000" || m.BodySize != 18 {
		t.Fatalf("master styles = %v %v %q %v", m.TitleSize, m.TitleBold, m.TitleColour, m.BodySize)
	}
	if len(m.Rules) != 2 || m.Rules[0].Y != 70 || m.Rules[0].Colour != "#000000" || len(m.Pictures) != 1 {
		t.Fatalf("master chrome = %+v %+v", m.Rules, m.Pictures)
	}
	if len(inv.Layouts) != 7 || inv.Layouts[0].Name != "Couverture titre" || inv.Layouts[0].ShowMasterShapes {
		t.Fatalf("layouts = %d %+v", len(inv.Layouts), inv.Layouts[0])
	}
	// A placeholder the layout leaves out is its master's.
	content := inv.Layouts[4]
	if p := titleOf(content); p == nil || p.Box.Y != 100 || p.Size != 48 || !p.Bold {
		t.Fatalf("inherited title = %+v", p)
	}
	if len(inv.Slides) != 3 || len(inv.Icons) != 1 || len(inv.Dropped) != 1 || !strings.Contains(inv.Dropped[0], "chart") {
		t.Fatalf("slides %d icons %d dropped %v", len(inv.Slides), len(inv.Icons), inv.Dropped)
	}
	if !strings.Contains(inv.Icons[0].SVG, `d="M0 0L60 0L30 60Z"`) {
		t.Fatalf("icon svg = %s", inv.Icons[0].SVG)
	}
	// A group of freeforms is a picture: one SVG keeping each shape's colour.
	pics := inv.Layouts[6].Pictures
	if len(pics) != 1 || pics[0].Box != (Box{900, 600, 200, 50}) || !strings.HasSuffix(pics[0].Media, ".svg") {
		t.Fatalf("vector logo = %+v", pics)
	}
	svg := string(inv.Media(pics[0].Media))
	for _, w := range []string{`viewBox="0 0 200 50"`, `fill="#E06666"`, `fill="#000000"`, `translate(60 10)`} {
		if !strings.Contains(svg, w) {
			t.Errorf("vector logo misses %s: %s", w, svg)
		}
	}
}

func propose(t *testing.T) (*Inventory, *Proposal) {
	t.Helper()
	inv, err := Read(decktesting.Template(t))
	if err != nil {
		t.Fatal(err)
	}
	layouts, err := slidedeckcontrib.BuiltinLayouts()
	if err != nil {
		t.Fatal(err)
	}
	p, err := Propose(inv, Options{Name: "acme", Layouts: layouts})
	if err != nil {
		t.Fatal(err)
	}
	return inv, p
}

func TestProposeTheme(t *testing.T) {
	_, p := propose(t)
	th := p.Theme
	want := map[string]string{
		"bg": "#FFFFFF", "fg": "#000000", "heading": "#000000",
		"accent": "#E06666", "muted": "#1F2A44", "surface": "#F4F1EA", "border": "#EEECE1",
		"size": "24px", "h2": "2.67em", "h1": "4em", "number-size": "2.78em",
		"font": "'Acme Text', Arial, sans-serif", "office-heading-font": "Acme Display",
		"cover-direction": "column-reverse", "cover-transform": "uppercase",
	}
	for k, v := range want {
		if th.Tokens[k] != v {
			t.Errorf("token %s = %q, want %q", k, th.Tokens[k], v)
		}
	}
	c := th.Chrome
	if c == nil || c.Logo == nil || c.Logo.Asset != "logo.png" || len(c.Rules) != 2 || c.Rules[0].Token != "fg" {
		t.Fatalf("chrome = %+v", c)
	}
	if c.Footer == nil || c.Footer.Template != "{title}" || c.Footer.Box.X != 860 || c.SlideNumber != nil {
		t.Fatalf("footer %+v, number %+v (the number is 1 pt: hidden)", c.Footer, c.SlideNumber)
	}
	cover := th.Layouts["cover"]
	if cover.Chrome == nil || *cover.Chrome || cover.Default == nil || cover.Default.Background != "bg/bordeaux.jpg" || cover.Default.Logo.Asset != "logo-2.png" {
		t.Fatalf("cover = %+v %+v", cover, cover.Default)
	}
	if v, ok := cover.Variants["cream"]; !ok || v.Background != "bg/cream.jpg" || v.Tokens["fg"] != "#0B5394" {
		t.Fatalf("cover variants = %+v", cover.Variants)
	}
	if s := th.Layouts["section"]; s.Chrome == nil || *s.Chrome || s.Default.Tokens["heading"] != "#E06666" {
		t.Fatalf("section = %+v", s)
	}
	// The timeline draws the master's logo again: its chrome stays.
	if tl := th.Layouts["timeline"]; tl.Chrome != nil || tl.Default.Background != "#D9D2C5" || tl.Default.Tokens["fg"] != "#FFFFFF" {
		t.Fatalf("timeline = %+v %+v", tl, tl.Default)
	}
	if len(th.Icons) != 1 {
		t.Fatalf("icons = %v", th.Icons)
	}
	by := map[string]Mapping{}
	for _, m := range p.Mapping {
		by[m.Layout] = m
	}
	if m := by["Titre et contenu"]; m.Deck != "content" || m.By != "name" {
		t.Errorf("content mapping = %+v", m)
	}
	if m := by["Mystère"]; m.Deck != "" || m.By != "none" {
		t.Errorf("unmapped layout = %+v", m)
	}
	notes := strings.Join(p.Notes, "\n")
	for _, w := range []string{"Acme Display", "chart", "charter"} {
		if !strings.Contains(notes, w) {
			t.Errorf("notes miss %q:\n%s", w, notes)
		}
	}
}

// TestProposalIsAThemeFolder: the pack written (deck-themes/<name>.yaml and
// deck-assets/<name>/ under its root) passes, as written, the checks a
// sidecar's pack runs on its themes (declaration, every file named and
// shipped, the sample deck lint-clean), and the same template gives the same
// files.
func TestProposalIsAThemeFolder(t *testing.T) {
	inv, p := propose(t)
	dir := t.TempDir()
	if err := p.Write(filepath.Join(dir, "pack")); err != nil {
		t.Fatal(err)
	}
	for _, e := range decklint.CheckPack(os.DirFS(dir), "pack") {
		t.Error(e)
	}
	if err := p.Write(filepath.Join(dir, "pack")); err == nil {
		t.Error("Write overwrote a theme already there")
	}
	y1, _ := p.YAML()
	_, again := propose(t)
	y2, _ := again.YAML()
	if !bytes.Equal(y1, y2) {
		t.Error("two proposals from one template differ")
	}
	written, _ := os.ReadFile(filepath.Join(dir, "pack", decklint.ThemesFolder, "acme.yaml"))
	if !bytes.Equal(written, y1) {
		t.Error("the written declaration is not YAML()")
	}
	r := p.Report(inv, "acme.pptx")
	for _, w := range []string{"| Mystère |", "| Couverture titre | slideLayout1.xml | cover |", "acme/bg/bordeaux.jpg"} {
		if !strings.Contains(r, w) {
			t.Errorf("report misses %q:\n%s", w, r)
		}
	}
	var lint []deckapi.Finding
	sample, _ := os.ReadFile(filepath.Join(dir, "pack", decklint.AssetsFolder, "acme", "sample.deck.md"))
	layouts, _ := slidedeckcontrib.BuiltinLayouts()
	lint = deckapi.Lint(string(sample), deckapi.Grammar{Layouts: layouts, Themes: []deckapi.ThemeBody{p.Theme}})
	if deckapi.HasErrors(lint) {
		t.Errorf("sample deck: %+v", lint)
	}
}

// TestDecisions: what an agent decides is applied over the proposal, and a
// decision the importer cannot apply is refused with what it knows.
func TestDecisions(t *testing.T) {
	inv, err := Read(decktesting.Template(t))
	if err != nil {
		t.Fatal(err)
	}
	layouts, _ := slidedeckcontrib.BuiltinLayouts()
	p, err := Propose(inv, Options{Name: "acme", Layouts: layouts, Decisions: deckapi.ImportDecisions{
		Layouts:  map[string]string{"Mystère": "comparison", "slideLayout4.xml": "drop"},
		Variants: map[string]string{"cover/cream": "creme"},
		Guidance: "Ouvrez par une couverture.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Mapping{}
	for _, m := range p.Mapping {
		by[m.Layout] = m
	}
	if m := by["Mystère"]; m.Deck != "comparison" || m.By != "decision" {
		t.Errorf("Mystère = %+v", m)
	}
	// A layout a decision adds to a deck layout does not displace the one
	// matched by name: « Mystère » → content stays behind « Titre et contenu ».
	p2, err := Propose(inv, Options{Name: "acme", Layouts: layouts, Decisions: deckapi.ImportDecisions{Layouts: map[string]string{"Mystère": "content"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range p2.Mapping {
		if m.Layout == "Mystère" && !strings.Contains(m.Note, "not kept") {
			t.Errorf("Mystère displaced the content layout: %+v", m)
		}
	}
	if m := by["Sommaire"]; m.Deck != "" || m.By != "decision" {
		t.Errorf("dropped Sommaire = %+v", m)
	}
	if _, ok := p.Theme.Layouts["agenda"]; ok {
		t.Error("a dropped layout still styles agenda")
	}
	if m := by["1_Couverture titre"]; m.Variant != "creme" {
		t.Errorf("renamed variant = %+v", m)
	}
	if _, ok := p.Theme.Layouts["cover"].Variants["creme"]; !ok {
		t.Errorf("cover variants = %v", p.Theme.VariantNames("cover"))
	}
	if p.Theme.Guidance != "Ouvrez par une couverture.\n" {
		t.Errorf("guidance = %q", p.Theme.Guidance)
	}
	if !strings.Contains(string(p.Files["sample.deck.md"]), "variant=creme") {
		t.Error("the sample deck does not use the renamed variant")
	}
	for _, d := range []deckapi.ImportDecisions{
		{Layouts: map[string]string{"Absent": "content"}},
		{Layouts: map[string]string{"Mystère": "carousel"}},
		{Variants: map[string]string{"cover/cream": "Crème"}},
		{Variants: map[string]string{"cover/violet": "violet"}},
		{Variants: map[string]string{"cream": "creme"}},
	} {
		if _, err := Propose(inv, Options{Name: "acme", Layouts: layouts, Decisions: d}); err == nil {
			t.Errorf("decision %+v accepted", d)
		}
	}
}

// TestImport: a dry run writes nothing and returns the report and the
// declaration; with out, the theme folder is written and listed.
func TestImport(t *testing.T) {
	layouts, _ := slidedeckcontrib.BuiltinLayouts()
	dry, err := Import(decktesting.Template(t), Options{Name: "acme", Layouts: layouts}, "acme.pptx", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Written) != 0 || !strings.Contains(dry.Report, "## Example slides") || !strings.HasPrefix(dry.Declaration, "# Imported") {
		t.Fatalf("dry run = written %v, report %d bytes, declaration %.40q", dry.Written, len(dry.Report), dry.Declaration)
	}
	out := filepath.Join(t.TempDir(), "embed")
	res, err := Import(decktesting.Template(t), Options{Name: "acme", Layouts: layouts}, "acme.pptx", out)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Written {
		if _, err := os.Stat(f); err != nil {
			t.Error(err)
		}
	}
	if res.Written[0] != filepath.Join(out, decklint.ThemesFolder, "acme.yaml") || !strings.HasPrefix(res.Written[1], filepath.Join(out, decklint.AssetsFolder, "acme")+string(filepath.Separator)) {
		t.Errorf("written = %v", res.Written)
	}
}

// A malformed template is an error, never a crash: a master without its
// colour map or shape tree, a slide without a shape tree, an empty zip.
func TestReadRefusesMalformedTemplates(t *testing.T) {
	zipOf := func(parts map[string]string) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for n, b := range parts {
			w, _ := zw.Create(n)
			_, _ = w.Write([]byte(b))
		}
		_ = zw.Close()
		return buf.Bytes()
	}
	ns := `xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	bare := map[string]string{
		"ppt/presentation.xml":              `<p:presentation ` + ns + `><p:sldMasterIdLst><p:sldMasterId r:id="rId1"/></p:sldMasterIdLst><p:sldIdLst><p:sldId r:id="rId2"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels":   `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="slideMasters/slideMaster1.xml"/><Relationship Id="rId2" Target="slides/slide1.xml"/></Relationships>`,
		"ppt/slideMasters/slideMaster1.xml": `<p:sldMaster ` + ns + `/>`,
		"ppt/slides/slide1.xml":             `<p:sld ` + ns + `/>`,
	}
	for name, data := range map[string][]byte{
		"empty zip":             zipOf(nil),
		"not a zip":             []byte("plain text"),
		"bare master and slide": zipOf(bare),
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panicked: %v", name, r)
				}
			}()
			_, _ = Read(data)
		}()
	}
}
