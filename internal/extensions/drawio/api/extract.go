// SPDX-License-Identifier: Apache-2.0

// extract.go — reading the draw.io XML back out of the files draw.io
// writes: a .drawio.png carries it in a tEXt chunk, a .drawio.svg in the
// root's content attribute, and a <diagram> may be deflate-compressed.

package api

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// DrawioError indicates a problem with a draw.io file or rendering.
type DrawioError struct {
	Message string
}

func (e *DrawioError) Error() string {
	return "drawio: " + e.Message
}

// pngSignature is the magic bytes at the start of every valid PNG file.
var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// ExtractXMLFromPNG reads PNG tEXt chunks and extracts the embedded draw.io
// diagram XML. Draw.io stores the diagram in a tEXt chunk with keyword "mxfile".
// The text value is URL-encoded XML which may contain compressed diagram data.
func ExtractXMLFromPNG(pngData []byte) ([]byte, error) {
	if len(pngData) < len(pngSignature) || !bytes.Equal(pngData[:len(pngSignature)], pngSignature) {
		return nil, &DrawioError{"not a valid PNG file"}
	}

	offset := len(pngSignature)
	for offset < len(pngData) {
		if offset+8 > len(pngData) {
			return nil, &DrawioError{"corrupted PNG: incomplete chunk header"}
		}

		length := int(binary.BigEndian.Uint32(pngData[offset : offset+4]))
		chunkType := string(pngData[offset+4 : offset+8])
		offset += 8

		if offset+length+4 > len(pngData) {
			return nil, &DrawioError{fmt.Sprintf("corrupted PNG: incomplete data for chunk %s", chunkType)}
		}

		chunkData := pngData[offset : offset+length]
		offset += length + 4 // data + CRC

		if chunkType != "tEXt" {
			continue
		}

		// tEXt format: keyword\0text
		nullPos := bytes.IndexByte(chunkData, 0)
		if nullPos < 0 {
			return nil, &DrawioError{"corrupted PNG: tEXt chunk missing keyword or data"}
		}

		keyword := string(chunkData[:nullPos])
		if keyword != "mxfile" {
			continue
		}

		textData := chunkData[nullPos+1:]

		// URL-decode the text data to get the outer XML
		decoded, err := url.QueryUnescape(string(textData))
		if err != nil {
			return nil, &DrawioError{"data in tEXt chunk is not properly URL-encoded"}
		}

		// Decompress the diagram content within the XML
		return DecompressDiagram([]byte(decoded))
	}

	return nil, &DrawioError{"not a PNG file made with draw.io"}
}

// ExtractXMLFromSVG reads an SVG file's root "content" attribute, which
// contains the embedded draw.io diagram XML (possibly compressed).
func ExtractXMLFromSVG(svgData []byte) ([]byte, error) {
	type svgRoot struct {
		XMLName xml.Name `xml:"svg"`
		Content string   `xml:"content,attr"`
	}

	var root svgRoot
	if err := xml.Unmarshal(svgData, &root); err != nil {
		return nil, &DrawioError{"invalid SVG XML"}
	}

	if root.Content == "" {
		return nil, &DrawioError{"SVG root element has no attribute content"}
	}

	return DecompressDiagram([]byte(root.Content))
}

// mxfile is the outer XML structure of a draw.io file.
type mxfile struct {
	XMLName  xml.Name   `xml:"mxfile"`
	Diagrams []mxDiag   `xml:"diagram"`
	Attrs    []xml.Attr `xml:",any,attr"`
}

type mxDiag struct {
	XMLName  xml.Name   `xml:"diagram"`
	Name     string     `xml:"name,attr,omitempty"`
	ID       string     `xml:"id,attr,omitempty"`
	Content  string     `xml:",chardata"`
	Children []byte     `xml:",innerxml"`
	Attrs    []xml.Attr `xml:",any,attr"`
}

// DecompressDiagram takes draw.io XML and decompresses the <diagram> text content
// if it is compressed. The compression chain is: base64-decode → inflate → URL-decode.
// Returns the full XML with decompressed content.
func DecompressDiagram(xmlData []byte) ([]byte, error) {
	var file mxfile
	if err := xml.Unmarshal(xmlData, &file); err != nil {
		return nil, &DrawioError{"invalid outer XML"}
	}

	if file.XMLName.Local != "mxfile" {
		return nil, &DrawioError{"root element is not <mxfile>"}
	}

	if len(file.Diagrams) == 0 {
		return nil, &DrawioError{"<diagram> element not found"}
	}

	// Check if already decompressed (has child elements).
	for i := range file.Diagrams {
		content := strings.TrimSpace(file.Diagrams[i].Content)
		if content == "" {
			// Already has child elements or is empty, skip.
			continue
		}

		// Base64 decode
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, &DrawioError{"<diagram> element text is not properly Base64-encoded"}
		}

		// Inflate (raw DEFLATE)
		reader := flate.NewReader(bytes.NewReader(decoded))
		inflated, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return nil, &DrawioError{"<diagram> element text cannot be decompressed using INFLATE"}
		}

		// URL decode
		urlDecoded, err := url.QueryUnescape(string(inflated))
		if err != nil {
			return nil, &DrawioError{"decompressed data in <diagram> element is not properly URL-encoded"}
		}

		// Replace compressed content with decompressed XML.
		file.Diagrams[i].Content = ""
		file.Diagrams[i].Children = []byte(urlDecoded)
	}

	return xml.Marshal(&file)
}

// ExtractDrawioXML returns the draw.io XML of a file by what it holds: a
// .drawio.png's embedded diagram, a .drawio.svg's content attribute, or
// the document itself (plain or compressed .drawio).
func ExtractDrawioXML(data []byte) ([]byte, error) {
	if bytes.HasPrefix(data, pngSignature) {
		return ExtractXMLFromPNG(data)
	}
	head := data[:min(len(data), 1024)]
	if bytes.Contains(head, []byte("<svg")) && !bytes.Contains(head, []byte("<mxfile")) && !bytes.Contains(head, []byte("<mxGraphModel")) {
		return ExtractXMLFromSVG(data)
	}
	return data, nil
}
