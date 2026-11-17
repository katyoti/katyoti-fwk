// SPDX-License-Identifier: Apache-2.0

// Package testing holds draw.io file fixtures shared by the tests of the
// drawio engine and of the extensions that read draw.io files.
package testing

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/binary"
	"net/url"
)

// BuildMxfileXML creates a draw.io XML with compressed diagram content.
func BuildMxfileXML(compressed bool) []byte {
	inner := `<mxGraphModel><root><mxCell id="0"/></root></mxGraphModel>`
	if !compressed {
		return []byte(`<mxfile><diagram name="Page-1">` + inner + `</diagram></mxfile>`)
	}

	// Compression chain: URL-encode → deflate → base64
	urlEncoded := url.QueryEscape(inner)

	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.DefaultCompression)
	w.Write([]byte(urlEncoded))
	w.Close()

	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	return []byte(`<mxfile><diagram name="Page-1">` + b64 + `</diagram></mxfile>`)
}

// BuildTestPNG creates a minimal valid PNG with a tEXt chunk containing
// draw.io diagram data under the "mxfile" keyword.
func BuildTestPNG(mxfileXML []byte) []byte {
	var buf bytes.Buffer

	// PNG signature
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})

	// IHDR chunk (minimal, 13 bytes of data)
	ihdrData := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdrData[0:4], 1) // width
	binary.BigEndian.PutUint32(ihdrData[4:8], 1) // height
	ihdrData[8] = 8                              // bit depth
	ihdrData[9] = 2                              // color type (RGB)
	WriteChunk(&buf, "IHDR", ihdrData)

	// tEXt chunk with mxfile keyword
	textData := append([]byte("mxfile\x00"), []byte(url.QueryEscape(string(mxfileXML)))...)
	WriteChunk(&buf, "tEXt", textData)

	// IEND chunk
	WriteChunk(&buf, "IEND", nil)

	return buf.Bytes()
}

// WriteChunk writes a PNG chunk (length + type + data + CRC placeholder).
func WriteChunk(buf *bytes.Buffer, chunkType string, data []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	buf.Write(length[:])
	buf.WriteString(chunkType)
	buf.Write(data)
	// CRC (simplified — not actually computing CRC, just 4 zero bytes)
	buf.Write([]byte{0, 0, 0, 0})
}

// EscapeXMLAttr escapes special characters for XML attribute values in test data.
func EscapeXMLAttr(s string) string {
	var buf bytes.Buffer
	for _, c := range s {
		switch c {
		case '"':
			buf.WriteString("&quot;")
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		default:
			buf.WriteRune(c)
		}
	}
	return buf.String()
}
