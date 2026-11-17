// SPDX-License-Identifier: Apache-2.0

package extraction

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// An item a source brings is data, text or both:
// its data is the file <stem>.data.json, its markdown text <stem>.md beside
// it. The knowledge base reads the text; an item with data only is read as
// its data rendered as text. A data file beside its item's text is never read
// on its own, so the item is indexed once.

// ItemDataSuffix ends the name of an item's data file.
const ItemDataSuffix = ".data.json"

// IsItemData says whether a file is an item's data.
func IsItemData(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ItemDataSuffix)
}

// ItemTextOf is the text file of the item whose data file is given.
func ItemTextOf(dataPath string) string {
	return dataPath[:len(dataPath)-len(ItemDataSuffix)] + ".md"
}

// HasItemText says whether the item whose data file is given has its text
// beside it - then the text carries the item.
func HasItemText(dataPath string) bool {
	if !IsItemData(dataPath) {
		return false
	}
	info, err := os.Stat(ItemTextOf(dataPath))
	return err == nil && !info.IsDir()
}

// RenderData renders an item's data as markdown text: one line per field,
// a nested object or a list indented under its field, the fields in name
// order so the text is the same at every read.
func RenderData(raw []byte) (string, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("the item's data is not JSON: %w", err)
	}
	var b strings.Builder
	renderValue(&b, v, 0)
	return b.String(), nil
}

func renderValue(b *strings.Builder, v any, depth int) {
	indent := strings.Repeat("  ", depth)
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if scalar, ok := scalarText(t[k]); ok {
				fmt.Fprintf(b, "%s- **%s**: %s\n", indent, k, scalar)
				continue
			}
			fmt.Fprintf(b, "%s- **%s**:\n", indent, k)
			renderValue(b, t[k], depth+1)
		}
	case []any:
		for _, e := range t {
			if scalar, ok := scalarText(e); ok {
				fmt.Fprintf(b, "%s- %s\n", indent, scalar)
				continue
			}
			fmt.Fprintf(b, "%s-\n", indent)
			renderValue(b, e, depth+1)
		}
	default:
		s, _ := scalarText(t)
		fmt.Fprintf(b, "%s%s\n", indent, s)
	}
}

// scalarText is a scalar's text; false for an object or a list.
func scalarText(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", true
	case string:
		return t, true
	case bool, float64:
		return fmt.Sprint(t), true
	}
	return "", false
}

// ItemDataExtractor reads an item's data file as its data rendered as text.
type ItemDataExtractor struct{}

// ItemDataExtractorName names it in the router.
const ItemDataExtractorName = "item-data-text"

func (e *ItemDataExtractor) Name() string { return ItemDataExtractorName }

// Formats is empty: the router gives it an item's data file by its name
// (ItemDataSuffix), never a plain .json document.
func (e *ItemDataExtractor) Formats() []string { return nil }

func (e *ItemDataExtractor) Extract(_ context.Context, inputPath string, outputDir string, _ map[string]any, _ *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	text, err := RenderData(raw)
	if err != nil {
		return nil, err
	}
	const contentFile = "content.md"
	if err := os.WriteFile(filepath.Join(outputDir, contentFile), []byte(text), 0o644); err != nil {
		return nil, fmt.Errorf("write content: %w", err)
	}
	return &kbapi.ExtractResult{
		ContentFiles: []kbapi.ContentFile{{File: contentFile, Part: "whole", Index: 0}},
		TotalParts:   1,
	}, nil
}
