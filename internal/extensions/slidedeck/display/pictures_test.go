// SPDX-License-Identifier: Apache-2.0

package display

import (
	"archive/zip"
	"bytes"
	"fmt"
	"testing"
)

// A pptx-pictures file as PptxGenJS writes it: each slide names its picture
// in its relationships. SlidePictures answers them in slide order — slide 10
// after slide 2 — whatever their media names.
func TestSlidePicturesAreInSlideOrder(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	put := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	for _, n := range []int{10, 2, 1} {
		put(fmt.Sprintf("ppt/slides/slide%d.xml", n), "<p:sld/>")
		put(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", n), fmt.Sprintf(`<?xml version="1.0"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="../notesSlides/notesSlide%d.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image-%d-1.png"/>
</Relationships>`, n, n))
		put(fmt.Sprintf("ppt/media/image-%d-1.png", n), fmt.Sprintf("png of %d", n))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	pics, err := SlidePictures(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range pics {
		got = append(got, string(p))
	}
	if want := []string{"png of 1", "png of 2", "png of 10"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("pictures = %q, want %q", got, want)
	}
}

func TestSlidePicturesRefusesAnEditableExport(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("ppt/slides/slide1.xml")
	_, _ = w.Write([]byte("<p:sld/>"))
	w, _ = zw.Create("ppt/slides/_rels/slide1.xml.rels")
	_, _ = w.Write([]byte(`<Relationships/>`))
	_ = zw.Close()
	if _, err := SlidePictures(buf.Bytes()); err == nil {
		t.Fatal("a slide with no picture was accepted")
	}
}
