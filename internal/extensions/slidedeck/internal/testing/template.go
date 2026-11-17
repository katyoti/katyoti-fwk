// SPDX-License-Identifier: Apache-2.0

// Package testing builds a small PowerPoint template for tests: one master
// (theme colours, fonts, title and body styles, a logo, two rules, a footer),
// seven layouts (two covers, a chapter, an agenda, a content layout, a
// timeline on a coloured fill, an unknown layout carrying a logo drawn as
// vector shapes) and three example slides (a freeform pictogram, a chart) —
// the shapes an importer meets in a corporate master, without any brand's
// files.
package testing

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// The fixture is a small template written by the test: one master (theme
// colours, fonts, title and body styles, a logo, two rules, a footer), seven
// layouts and three example slides — the shapes an importer meets in a
// corporate master, without any brand's files.

const (
	nsP    = `xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
	nsA    = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"`
	nsR    = `xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	relPkg = `http://schemas.openxmlformats.org/package/2006/relationships`
	relDoc = `http://schemas.openxmlformats.org/officeDocument/2006/relationships/`
)

// emu turns slide pixels into EMU.
// emuPerPx is the importer's EMU-to-pixel ratio (pptx.EMUPerPx).
const emuPerPx = 9525.0

func emu(px float64) int { return int(px * emuPerPx) }

func xfrm(x, y, w, h float64) string {
	return fmt.Sprintf(`<a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`, emu(x), emu(y), emu(w), emu(h))
}

// ph is a placeholder: its type, its box (none = the master's), its size in
// pt (0 = inherited), its colour and its prompt.
func ph(typ, idx string, box []float64, sz int, clr, prompt string) string {
	attrs := ""
	if typ != "" {
		attrs += fmt.Sprintf(` type="%s"`, typ)
	}
	if idx != "" {
		attrs += fmt.Sprintf(` idx="%s"`, idx)
	}
	sp := ""
	if box != nil {
		sp = xfrm(box[0], box[1], box[2], box[3])
	}
	rpr := ""
	if sz > 0 || clr != "" {
		fill := ""
		if clr != "" {
			fill = fmt.Sprintf(`<a:solidFill><a:srgbClr val="%s"/></a:solidFill>`, clr)
		}
		size := ""
		if sz > 0 {
			size = fmt.Sprintf(` sz="%d"`, sz*100)
		}
		rpr = fmt.Sprintf(`<a:lstStyle><a:lvl1pPr><a:defRPr%s>%s</a:defRPr></a:lvl1pPr></a:lstStyle>`, size, fill)
	}
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="2" name="%s"/><p:cNvSpPr/><p:nvPr><p:ph%s/></p:nvPr></p:nvSpPr><p:spPr>%s</p:spPr><p:txBody><a:bodyPr/>%s<a:p><a:r><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>`,
		typ, attrs, sp, rpr, prompt)
}

func pic(rid string, x, y, w, h float64) string {
	return fmt.Sprintf(`<p:pic><p:nvPicPr><p:cNvPr id="3" name="Image"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:blipFill><a:blip r:embed="%s"/></p:blipFill><p:spPr>%s<a:prstGeom prst="rect"/></p:spPr></p:pic>`, rid, xfrm(x, y, w, h))
}

func rule(y float64) string {
	return fmt.Sprintf(`<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="4" name="Line"/><p:cNvCxnSpPr/><p:nvPr/></p:nvCxnSpPr><p:spPr>%s<a:prstGeom prst="line"/><a:ln w="9525"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill></a:ln></p:spPr></p:cxnSp>`, xfrm(60, y, 1160, 0))
}

func fill(clr string) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="5" name="Rect"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>%s<a:prstGeom prst="rect"/><a:solidFill><a:srgbClr val="%s"/></a:solidFill></p:spPr></p:sp>`, xfrm(0, 0, 1280, 720), clr)
}

func tree(shapes ...string) string {
	return `<p:cSld%s><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` + strings.Join(shapes, "") + `</p:spTree></p:cSld>`
}

func rels(targets ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="` + relPkg + `">`)
	for i := 0; i+1 < len(targets); i += 2 {
		fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="%s%s" Target="%s"/>`, i/2+1, relDoc, targets[i], targets[i+1])
	}
	return b.String() + `</Relationships>`
}

func solid(c color.Color, w, h int, asPNG bool) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if asPNG {
		_ = png.Encode(&buf, img)
	} else {
		_ = jpeg.Encode(&buf, img, nil)
	}
	return buf.Bytes()
}

// vectorLogo is a logo drawn as two freeforms in a group (a mark and a
// word), each with its own fill.
var vectorLogo = `<p:grpSp><p:nvGrpSpPr><p:cNvPr id="9" name="Logo"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr>` +
	`<a:xfrm><a:off x="` + fmt.Sprint(emu(900)) + `" y="` + fmt.Sprint(emu(600)) + `"/><a:ext cx="` + fmt.Sprint(emu(200)) + `" cy="` + fmt.Sprint(emu(50)) + `"/>` +
	`<a:chOff x="0" y="0"/><a:chExt cx="` + fmt.Sprint(emu(200)) + `" cy="` + fmt.Sprint(emu(50)) + `"/></a:xfrm></p:grpSpPr>` +
	freeform(0, 0, 50, 50, `<a:srgbClr val="E06666"/>`) + freeform(60, 10, 140, 30, `<a:schemeClr val="tx1"/>`) + `</p:grpSp>`

func freeform(x, y, w, h float64, clr string) string {
	return `<p:sp><p:nvSpPr><p:cNvPr id="10" name="Freeform"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>` + xfrm(x, y, w, h) +
		`<a:custGeom><a:pathLst><a:path w="100" h="100"><a:moveTo><a:pt x="0" y="0"/></a:moveTo><a:lnTo><a:pt x="100" y="100"/></a:lnTo><a:lnTo><a:pt x="0" y="100"/></a:lnTo><a:close/></a:path></a:pathLst></a:custGeom>` +
		`<a:solidFill>` + clr + `</a:solidFill></p:spPr></p:sp>`
}

type layoutDef struct {
	name, typ, showMaster, body, rels string
}

// Template is the fixture template, as the bytes of a .pptx.
func Template(t testing.TB) []byte {
	t.Helper()
	parts := map[string]string{}
	parts["ppt/presentation.xml"] = `<p:presentation ` + nsP + ` ` + nsR + `><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst><p:sldIdLst><p:sldId id="256" r:id="rId2"/><p:sldId id="257" r:id="rId3"/><p:sldId id="258" r:id="rId4"/></p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`
	parts["ppt/_rels/presentation.xml.rels"] = rels("slideMaster", "slideMasters/slideMaster1.xml", "slide", "slides/slide1.xml", "slide", "slides/slide2.xml", "slide", "slides/slide3.xml")

	parts["ppt/theme/theme1.xml"] = `<a:theme ` + nsA + ` name="Acme"><a:themeElements><a:clrScheme name="Acme">` +
		`<a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1><a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>` +
		`<a:dk2><a:srgbClr val="1F2A44"/></a:dk2><a:lt2><a:srgbClr val="EEECE1"/></a:lt2>` +
		`<a:accent1><a:srgbClr val="0B5394"/></a:accent1><a:accent2><a:srgbClr val="E06666"/></a:accent2><a:accent3><a:srgbClr val="D9D2C5"/></a:accent3>` +
		`<a:accent4><a:srgbClr val="F4F1EA"/></a:accent4><a:accent5><a:srgbClr val="333F50"/></a:accent5><a:accent6><a:srgbClr val="7F7F7F"/></a:accent6>` +
		`</a:clrScheme><a:fontScheme name="Acme"><a:majorFont><a:latin typeface="Acme Display"/></a:majorFont><a:minorFont><a:latin typeface="Acme Text"/></a:minorFont></a:fontScheme></a:themeElements></a:theme>`

	layouts := []layoutDef{
		{"Couverture titre", "title", "0", ph("ctrTitle", "", []float64{200, 300, 900, 220}, 72, "E06666", "TITRE") + ph("subTitle", "1", []float64{200, 100, 900, 180}, 20, "FFFFFF", "Sous-titre") + pic("rId2", 0, 0, 1280, 720) + pic("rId3", 80, 560, 300, 120), ""},
		{"1_Couverture titre", "title", "0", ph("ctrTitle", "", []float64{200, 300, 900, 220}, 72, "E06666", "TITRE") + ph("subTitle", "1", []float64{200, 100, 900, 180}, 20, "0B5394", "Sous-titre") + pic("rId2", 0, 0, 1280, 720) + pic("rId3", 80, 560, 300, 120), ""},
		{"Chapitre", "", "0", ph("title", "", []float64{60, 140, 1160, 140}, 50, "E06666", "00") + ph("body", "13", []float64{60, 300, 1160, 300}, 72, "E06666", "Titre du chapitre"), ""},
		{"Sommaire", "", "", ph("title", "", []float64{640, 100, 560, 500}, 72, "E06666", "Agenda") + ph("body", "13", []float64{200, 100, 420, 500}, 0, "", "Chapitre"), ""},
		{"Titre et contenu", "obj", "", ph("title", "", nil, 0, "", "Titre") + ph("body", "1", []float64{60, 260, 1160, 380}, 0, "", "Texte") + ph("ftr", "11", nil, 0, "", "Pied"), ""},
		{"Time line", "", "0", fill("D9D2C5") + ph("title", "", []float64{780, 130, 430, 130}, 24, "FFFFFF", "Texte") + ph("body", "20", []float64{120, 280, 94, 105}, 60, "FFFFFF", "1") + ph("body", "14", []float64{140, 400, 150, 220}, 12, "FFFFFF", "Légende") + pic("rId4", 30, 650, 220, 100), ""},
		{"Mystère", "", "", ph("title", "", nil, 0, "", "Titre") + ph("body", "13", []float64{60, 260, 560, 380}, 0, "", "Texte") + vectorLogo, ""},
	}
	media := map[string][]byte{
		"ppt/media/logo.png":     solid(color.NRGBA{11, 83, 148, 255}, 40, 20, true),
		"ppt/media/logo-w.png":   solid(color.NRGBA{255, 255, 255, 255}, 40, 20, true),
		"ppt/media/bordeaux.jpg": solid(color.NRGBA{120, 12, 40, 255}, 64, 36, false),
		"ppt/media/cream.jpg":    solid(color.NRGBA{242, 234, 222, 255}, 64, 36, false),
	}
	var ids strings.Builder
	masterRels := []string{"theme", "../theme/theme1.xml", "image", "../media/logo.png"}
	for i, l := range layouts {
		n := i + 1
		fmt.Fprintf(&ids, `<p:sldLayoutId id="%d" r:id="rId%d"/>`, 2147483649+i, n+2)
		masterRels = append(masterRels, "slideLayout", fmt.Sprintf("../slideLayouts/slideLayout%d.xml", n))
		attrs := ""
		if l.typ != "" {
			attrs += fmt.Sprintf(` type="%s"`, l.typ)
		}
		if l.showMaster != "" {
			attrs += fmt.Sprintf(` showMasterSp="%s"`, l.showMaster)
		}
		parts[fmt.Sprintf("ppt/slideLayouts/slideLayout%d.xml", n)] = `<p:sldLayout ` + nsP + ` ` + nsA + ` ` + nsR + attrs + `>` + strings.Replace(tree(l.body), "<p:cSld%s>", fmt.Sprintf(`<p:cSld name="%s">`, l.name), 1) + `</p:sldLayout>`
		bg := "../media/bordeaux.jpg"
		if i == 1 {
			bg = "../media/cream.jpg"
		}
		parts[fmt.Sprintf("ppt/slideLayouts/_rels/slideLayout%d.xml.rels", n)] = rels("slideMaster", "../slideMasters/slideMaster1.xml", "image", bg, "image", "../media/logo-w.png", "image", "../media/logo.png")
	}
	// The master: rId1 theme, rId2 logo, rId3… layouts.
	parts["ppt/slideMasters/slideMaster1.xml"] = `<p:sldMaster ` + nsP + ` ` + nsA + ` ` + nsR + `>` +
		strings.Replace(tree(
			ph("title", "", []float64{60, 100, 1160, 140}, 0, "", "Titre"),
			ph("body", "1", []float64{60, 260, 1160, 380}, 0, "", "Texte"),
			ph("dt", "10", []float64{0, 700, 20, 20}, 1, "", "date"),
			ph("ftr", "11", []float64{860, 665, 340, 40}, 10, "", "Pied"),
			ph("sldNum", "12", []float64{0, 700, 20, 20}, 1, "", "#"),
			pic("rId2", 30, 650, 220, 100), rule(70), rule(648),
		), "<p:cSld%s>", "<p:cSld>", 1) +
		`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="accent1" folHlink="accent2"/>` +
		`<p:sldLayoutIdLst>` + ids.String() + `</p:sldLayoutIdLst>` +
		`<p:txStyles><p:titleStyle><a:lvl1pPr><a:defRPr sz="4800" b="1"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill></a:defRPr></a:lvl1pPr></p:titleStyle>` +
		`<p:bodyStyle><a:lvl1pPr><a:defRPr sz="1800"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill></a:defRPr></a:lvl1pPr></p:bodyStyle></p:txStyles></p:sldMaster>`
	parts["ppt/slideMasters/_rels/slideMaster1.xml.rels"] = rels(masterRels...)

	// Example slides: two content slides keeping the footer, one with a
	// freeform pictogram and a chart.
	icon := `<p:sp><p:nvSpPr><p:cNvPr id="6" name="Freeform"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>` + xfrm(600, 300, 60, 60) +
		`<a:custGeom><a:pathLst><a:path w="100" h="100"><a:moveTo><a:pt x="0" y="0"/></a:moveTo><a:lnTo><a:pt x="100" y="0"/></a:lnTo><a:lnTo><a:pt x="50" y="100"/></a:lnTo><a:close/></a:path></a:pathLst></a:custGeom></p:spPr></p:sp>`
	chart := `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="7" name="Chart"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr><p:xfrm/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart"/></a:graphic></p:graphicFrame>`
	slides := []string{
		ph("title", "", nil, 0, "", "Bonjour") + ph("body", "1", nil, 0, "", "Texte") + ph("ftr", "11", nil, 0, "", "Pied"),
		ph("title", "", nil, 0, "", "Encore") + ph("ftr", "11", nil, 0, "", "Pied") + icon + chart,
		ph("title", "", nil, 0, "", "Fin") + ph("ftr", "11", nil, 0, "", "Pied"),
	}
	for i, s := range slides {
		parts[fmt.Sprintf("ppt/slides/slide%d.xml", i+1)] = `<p:sld ` + nsP + ` ` + nsA + ` ` + nsR + `>` + strings.Replace(tree(s), "<p:cSld%s>", "<p:cSld>", 1) + `</p:sld>`
		parts[fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", i+1)] = rels("slideLayout", "../slideLayouts/slideLayout5.xml")
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range parts {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	for name, body := range media {
		w, _ := zw.Create(name)
		_, _ = w.Write(body)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
