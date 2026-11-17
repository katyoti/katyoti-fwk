// SPDX-License-Identifier: Apache-2.0

package commands_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	displayapi "github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/commands"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// renderDeck is a deck that holds one case per slide.
const renderDeck = `---
title: Render
---

# Render

---

<!-- slide: layout=content -->
## Costs
- alpha first item
- beta second item
- gamma third item

---

<!-- slide: layout=content steps=list -->
## Costs
- alpha first item
- beta second item
- gamma third item

---

<!-- slide: layout=blank -->
## A diagram
![Box](box.drawio)

---

<!-- slide: layout=timeline -->
## Road
::steps::
1. **Idea** — a need
2. **Study** – is it worth it
3. **Build**: the work
`

// One box filling most of the diagram, in a light blue fill.
const boxDrawio = `<mxfile host="test"><diagram id="d" name="d"><mxGraphModel dx="800" dy="400" grid="0" page="0"><root><mxCell id="0"/><mxCell id="1" parent="0"/><mxCell id="b" value="" style="rounded=0;whiteSpace=wrap;html=1;fillColor=#dae8fc;strokeColor=#6c8ebf;" vertex="1" parent="1"><mxGeometry x="0" y="0" width="800" height="400" as="geometry"/></mxCell></root></mxGraphModel></diagram></mxfile>`

// The exported slides show what the deck says, through the real display
// server and a headless browser: a steps=list slide exports with its whole
// list (picture and editable), a draw.io diagram in its file's colours, a
// timeline step without the separator its grammar writes and its line ending
// at its last step.
func TestExportedSlidesShowWhatTheDeckSays(t *testing.T) {
	long.Test(t)
	bin := displayBin(t)
	srv, err := displayapi.StartStandalone(context.Background(), bin)
	if err != nil {
		t.Skipf("no display server to run: %v", err)
	}
	defer srv.Stop()
	display := srv.Origin + "/?token=" + srv.Token

	dir := t.TempDir()
	deck := filepath.Join(dir, "render.deck.md")
	must(t, os.WriteFile(deck, []byte(renderDeck), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "box.drawio"), []byte(boxDrawio), 0o644))

	run := func(args ...string) string {
		t.Helper()
		ctx, _ := cwmapp.New(t, contrib.ContributeLayouts("ktw-test"))
		var out bytes.Buffer
		cmd := commands.NewDeckCommand()
		cmd.SetContext(ctx)
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"export", deck, "--display", display}, args...))
		if err := cmd.Execute(); err != nil {
			if strings.Contains(err.Error(), "Chromium") {
				t.Skipf("no browser: %v", err)
			}
			t.Fatalf("export %v: %v\n%s", args, err, out.String())
		}
		return out.String()
	}

	pics := filepath.Join(dir, "pics")
	run("--format", "png", "--out", pics)
	slide := func(n int) image.Image {
		f, err := os.Open(filepath.Join(pics, fmt.Sprintf("slide-%02d.png", n)))
		must(t, err)
		defer func() { _ = f.Close() }()
		img, err := png.Decode(f)
		must(t, err)
		return img
	}

	t.Run("a steps=list slide is pictured with its whole list", func(t *testing.T) {
		if d := differingPixels(slide(3), slide(2)); d > 0.001 {
			t.Errorf("the steps=list slide differs from the same slide without steps on %.1f %% of its pixels", d*100)
		}
	})

	t.Run("a draw.io diagram keeps its file's fill", func(t *testing.T) {
		img := slide(4)
		b := img.Bounds()
		r, g, bl, _ := img.At(b.Dx()/2, b.Dy()*3/5).RGBA()
		got := [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)}
		want := [3]uint8{0xda, 0xe8, 0xfc}
		for i := range got {
			if diff := int(got[i]) - int(want[i]); diff > 12 || diff < -12 {
				t.Fatalf("the diagram's box is drawn %02x%02x%02x, its file sets #dae8fc", got[0], got[1], got[2])
			}
		}
	})

	t.Run("a timeline's line ends at its last step", func(t *testing.T) {
		lineEnd, lastNumberEnd := timelineExtents(slide(5))
		if lineEnd < 0 || lastNumberEnd < 0 {
			t.Fatalf("no line (%d) or no step number (%d) found on the timeline", lineEnd, lastNumberEnd)
		}
		if lineEnd >= lastNumberEnd {
			t.Errorf("the line runs to x=%d, past the last step's number (ends at x=%d)", lineEnd, lastNumberEnd)
		}
	})

	editable := filepath.Join(dir, "render.pptx")
	run("--format", "pptx", "--out", editable)
	texts := slideTexts(t, editable)

	t.Run("a steps=list slide is editable with its whole list", func(t *testing.T) {
		for _, item := range []string{"alpha first item", "beta second item", "gamma third item"} {
			if !strings.Contains(texts[3], item) {
				t.Errorf("slide 3 lacks %q: %q", item, texts[3])
			}
		}
	})

	t.Run("a timeline step drops the separator its grammar writes", func(t *testing.T) {
		for _, sep := range []string{"— a need", "– is it", ": the work"} {
			if strings.Contains(texts[5], sep) {
				t.Errorf("the timeline shows its separator (%q): %q", sep, texts[5])
			}
		}
		for _, s := range []string{"Idea", "a need", "Study", "the work"} {
			if !strings.Contains(texts[5], s) {
				t.Errorf("the timeline lacks %q: %q", s, texts[5])
			}
		}
	})
}

// displayBin is bin/ktw-display(.exe) of this repository, or KTW_DISPLAY_BIN.
func displayBin(t *testing.T) string {
	if p := os.Getenv(displayapi.DisplayBinEnv); p != "" {
		return p
	}
	name := "ktw-display"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "bin", name))
	must(t, err)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("no %s: make ktw-display builds it", p)
	}
	return p
}

// timelineExtents finds, on a pictured timeline, the right end of its line
// (the row with the longest run of the pale grey line) and the right end of
// the step numbers along it (the accent colour, within the numbers' height).
func timelineExtents(img image.Image) (lineEnd, lastNumberEnd int) {
	b := img.Bounds()
	pale := func(x, y int) bool {
		r, g, bl, _ := img.At(x, y).RGBA()
		r, g, bl = r>>8, g>>8, bl>>8
		return r >= 200 && r <= 245 && g >= 200 && g <= 245 && bl >= 200 && bl <= 245
	}
	accent := func(x, y int) bool {
		r, g, bl, _ := img.At(x, y).RGBA()
		r, g, bl = r>>8, g>>8, bl>>8
		return bl > 180 && r < 140 && g < 140
	}
	lineY, best := -1, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		n := 0
		for x := b.Min.X; x < b.Max.X; x++ {
			if pale(x, y) {
				n++
			}
		}
		if n > best {
			lineY, best = y, n
		}
	}
	if best < b.Dx()/10 {
		return -1, -1
	}
	lineEnd = -1
	for x := b.Min.X; x < b.Max.X; x++ {
		if pale(x, lineY) {
			lineEnd = x
		}
	}
	lastNumberEnd = -1
	for y := lineY - b.Dy()/20; y <= lineY+b.Dy()/20; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if accent(x, y) && x > lastNumberEnd {
				lastNumberEnd = x
			}
		}
	}
	return lineEnd, lastNumberEnd
}

// differingPixels is the share of pixels that differ between two pictures.
func differingPixels(a, b image.Image) float64 {
	ab, bb := a.Bounds(), b.Bounds()
	if ab != bb {
		return 1
	}
	diff := 0
	for y := ab.Min.Y; y < ab.Max.Y; y++ {
		for x := ab.Min.X; x < ab.Max.X; x++ {
			r1, g1, b1, _ := a.At(x, y).RGBA()
			r2, g2, b2, _ := b.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 {
				diff++
			}
		}
	}
	return float64(diff) / float64(ab.Dx()*ab.Dy())
}

var textRun = regexp.MustCompile(`<a:t>([^<]*)</a:t>`)

// slideTexts reads each slide's text runs of a .pptx, by slide number.
func slideTexts(t *testing.T, path string) map[int]string {
	t.Helper()
	data, err := os.ReadFile(path)
	must(t, err)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	must(t, err)
	out := map[int]string{}
	slideXML := regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)
	for _, f := range zr.File {
		m := slideXML.FindStringSubmatch(f.Name)
		if m == nil {
			continue
		}
		rc, err := f.Open()
		must(t, err)
		raw, err := io.ReadAll(rc)
		_ = rc.Close()
		must(t, err)
		var n int
		_, _ = fmt.Sscan(m[1], &n)
		var parts []string
		for _, r := range textRun.FindAllStringSubmatch(string(raw), -1) {
			parts = append(parts, r[1])
		}
		out[n] = strings.Join(parts, " ")
	}
	return out
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
