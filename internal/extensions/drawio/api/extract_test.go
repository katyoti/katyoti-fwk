// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"

	"bytes"
	"testing"
)

func TestExtractXMLFromPNG_Valid(t *testing.T) {
	mxXML := drawiotesting.BuildMxfileXML(true)
	pngData := drawiotesting.BuildTestPNG(mxXML)

	result, err := drawioapi.ExtractXMLFromPNG(pngData)
	if err != nil {
		t.Fatalf("drawioapi.ExtractXMLFromPNG() error: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}

	// The result should be valid XML containing mxfile.
	if !bytes.Contains(result, []byte("mxfile")) {
		t.Error("result should contain mxfile element")
	}
	if !bytes.Contains(result, []byte("mxGraphModel")) {
		t.Error("result should contain decompressed mxGraphModel")
	}
}

func TestExtractXMLFromPNG_InvalidSignature(t *testing.T) {
	_, err := drawioapi.ExtractXMLFromPNG([]byte("not a png"))
	if err == nil {
		t.Fatal("expected error for invalid PNG")
	}
	de, ok := err.(*drawioapi.DrawioError)
	if !ok {
		t.Fatalf("expected drawioapi.DrawioError, got %T", err)
	}
	if de.Message != "not a valid PNG file" {
		t.Errorf("message = %q, want 'not a valid PNG file'", de.Message)
	}
}

func TestExtractXMLFromPNG_NoMxfileChunk(t *testing.T) {
	var buf bytes.Buffer
	// Valid PNG signature
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})

	// IHDR chunk
	ihdrData := make([]byte, 13)
	drawiotesting.WriteChunk(&buf, "IHDR", ihdrData)

	// tEXt chunk without mxfile keyword
	textData := append([]byte("other\x00"), []byte("some data")...)
	drawiotesting.WriteChunk(&buf, "tEXt", textData)

	// IEND
	drawiotesting.WriteChunk(&buf, "IEND", nil)

	_, err := drawioapi.ExtractXMLFromPNG(buf.Bytes())
	if err == nil {
		t.Fatal("expected error for PNG without mxfile chunk")
	}
	de := err.(*drawioapi.DrawioError)
	if de.Message != "not a PNG file made with draw.io" {
		t.Errorf("message = %q", de.Message)
	}
}

func TestExtractXMLFromSVG_Valid(t *testing.T) {
	mxXML := drawiotesting.BuildMxfileXML(true)
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" content="` +
		drawiotesting.EscapeXMLAttr(string(mxXML)) + `"></svg>`)

	result, err := drawioapi.ExtractXMLFromSVG(svgData)
	if err != nil {
		t.Fatalf("drawioapi.ExtractXMLFromSVG() error: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}
	if !bytes.Contains(result, []byte("mxfile")) {
		t.Error("result should contain mxfile element")
	}
}

func TestExtractXMLFromSVG_NoContent(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)

	_, err := drawioapi.ExtractXMLFromSVG(svgData)
	if err == nil {
		t.Fatal("expected error for SVG without content attribute")
	}
}

func TestExtractXMLFromSVG_InvalidXML(t *testing.T) {
	_, err := drawioapi.ExtractXMLFromSVG([]byte("not xml"))
	if err == nil {
		t.Fatal("expected error for invalid SVG")
	}
}

func TestDecompressDiagram_Uncompressed(t *testing.T) {
	xml := drawiotesting.BuildMxfileXML(false) // not compressed
	result, err := drawioapi.DecompressDiagram(xml)
	if err != nil {
		t.Fatalf("drawioapi.DecompressDiagram() error: %v", err)
	}
	if !bytes.Contains(result, []byte("mxGraphModel")) {
		t.Error("result should contain mxGraphModel")
	}
}

func TestDecompressDiagram_Compressed(t *testing.T) {
	xml := drawiotesting.BuildMxfileXML(true)
	result, err := drawioapi.DecompressDiagram(xml)
	if err != nil {
		t.Fatalf("drawioapi.DecompressDiagram() error: %v", err)
	}
	if !bytes.Contains(result, []byte("mxGraphModel")) {
		t.Error("result should contain decompressed mxGraphModel")
	}
}

func TestDecompressDiagram_InvalidXML(t *testing.T) {
	_, err := drawioapi.DecompressDiagram([]byte("not xml"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecompressDiagram_NotMxfile(t *testing.T) {
	_, err := drawioapi.DecompressDiagram([]byte(`<other/>`))
	if err == nil {
		t.Fatal("expected error for non-mxfile root")
	}
}
