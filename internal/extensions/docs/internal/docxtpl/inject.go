// SPDX-License-Identifier: Apache-2.0

package docxtpl

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/beevik/etree"
)

// RawXML marks a string as pre-built Open XML that should be injected
// directly into the document body, replacing the enclosing paragraph.
type RawXML string

// BodyExtraction holds the extracted body XML and any numbering definitions
// that need to be merged into the target document.
type BodyExtraction struct {
	BodyXML   RawXML
	Numbering []byte // raw word/numbering.xml from source (nil if none)
}

// ExtractBody reads a DOCX file and extracts all body elements
// (paragraphs, tables, etc.) as a raw XML string.
// Also extracts numbering.xml for merging into the target document.
func ExtractBody(docxPath string) (RawXML, error) {
	ext, err := ExtractBodyWithNumbering(docxPath)
	if err != nil {
		return "", err
	}
	return ext.BodyXML, nil
}

// ExtractBodyWithNumbering reads a DOCX and extracts body + numbering definitions.
func ExtractBodyWithNumbering(docxPath string) (*BodyExtraction, error) {
	data, err := os.ReadFile(docxPath)
	if err != nil {
		return nil, fmt.Errorf("read docx: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	result := &BodyExtraction{}

	for _, f := range reader.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		switch f.Name {
		case "word/document.xml":
			bodyXML, err := extractBodyXML(content)
			if err != nil {
				return nil, err
			}
			result.BodyXML = bodyXML

		case "word/numbering.xml":
			result.Numbering = content
		}
	}

	if result.BodyXML == "" {
		return nil, fmt.Errorf("word/document.xml not found")
	}

	return result, nil
}

func extractBodyXML(content []byte) (RawXML, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(content); err != nil {
		return "", fmt.Errorf("parse document.xml: %w", err)
	}

	body := findBody(doc.Root())
	if body == nil {
		return "", fmt.Errorf("no w:body found in document.xml")
	}

	var sb strings.Builder
	for _, child := range body.ChildElements() {
		if child.Tag == "sectPr" {
			continue
		}
		d := etree.NewDocument()
		d.SetRoot(child.Copy())
		d.WriteSettings.CanonicalEndTags = true
		s, _ := d.WriteToString()
		s = strings.TrimPrefix(s, `<?xml version="1.0" encoding="UTF-8"?>`)
		s = strings.TrimSpace(s)
		sb.WriteString(s)
	}

	return RawXML(sb.String()), nil
}

// MergeNumbering merges numbering definitions from a source numbering.xml
// into a target DOCX file's numbering.xml. This is needed when injecting
// body content from one DOCX (Pandoc output) into another (template),
// because list definitions (bullet/number) are in numbering.xml.
func MergeNumbering(targetDocxPath string, sourceNumbering []byte) error {
	if sourceNumbering == nil {
		return nil
	}

	// Parse source numbering
	srcDoc := etree.NewDocument()
	if err := srcDoc.ReadFromBytes(sourceNumbering); err != nil {
		return fmt.Errorf("parse source numbering: %w", err)
	}

	// Read target DOCX
	data, err := os.ReadFile(targetDocxPath)
	if err != nil {
		return fmt.Errorf("read target: %w", err)
	}
	srcReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open target zip: %w", err)
	}

	// Build new ZIP with merged numbering
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	merged := false
	for _, f := range srcReader.File {
		rc, _ := f.Open()
		content, _ := io.ReadAll(rc)
		rc.Close()

		if f.Name == "word/numbering.xml" {
			// Merge: add source abstractNum and num entries to target
			content, err = mergeNumberingXML(content, sourceNumbering)
			if err != nil {
				return err
			}
			merged = true
		}

		header := &zip.FileHeader{Name: f.Name, Method: f.Method}
		w, _ := writer.CreateHeader(header)
		w.Write(content)
	}

	// If target had no numbering.xml, add the source one
	if !merged {
		w, _ := writer.Create("word/numbering.xml")
		w.Write(sourceNumbering)
	}

	writer.Close()
	return os.WriteFile(targetDocxPath, buf.Bytes(), 0o644)
}

// mergeNumberingXML adds abstractNum and num entries from source into target.
func mergeNumberingXML(targetXML, sourceXML []byte) ([]byte, error) {
	tgtDoc := etree.NewDocument()
	if err := tgtDoc.ReadFromBytes(targetXML); err != nil {
		return nil, err
	}
	srcDoc := etree.NewDocument()
	if err := srcDoc.ReadFromBytes(sourceXML); err != nil {
		return nil, err
	}

	tgtRoot := tgtDoc.Root()
	srcRoot := srcDoc.Root()

	// Collect existing IDs to avoid duplicates
	existingAbstractNums := map[string]bool{}
	existingNums := map[string]bool{}
	for _, child := range tgtRoot.ChildElements() {
		if child.Tag == "abstractNum" {
			if id := child.SelectAttrValue("abstractNumId", ""); id != "" {
				existingAbstractNums[id] = true
			}
		}
		if child.Tag == "num" {
			if id := child.SelectAttrValue("numId", ""); id != "" {
				existingNums[id] = true
			}
		}
	}

	// Add missing abstractNum entries from source
	for _, child := range srcRoot.ChildElements() {
		if child.Tag == "abstractNum" {
			id := child.SelectAttrValue("abstractNumId", "")
			if id != "" && !existingAbstractNums[id] {
				tgtRoot.AddChild(child.Copy())
			}
		}
	}

	// Add missing num entries from source
	for _, child := range srcRoot.ChildElements() {
		if child.Tag == "num" {
			id := child.SelectAttrValue("numId", "")
			if id != "" && !existingNums[id] {
				tgtRoot.AddChild(child.Copy())
			}
		}
	}

	tgtDoc.WriteSettings.CanonicalEndTags = true
	result, err := tgtDoc.WriteToBytes()
	if err != nil {
		return nil, err
	}
	return result, nil
}

func findBody(el *etree.Element) *etree.Element {
	if el.Tag == "body" {
		return el
	}
	for _, child := range el.ChildElements() {
		if found := findBody(child); found != nil {
			return found
		}
	}
	return nil
}
