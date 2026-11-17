// SPDX-License-Identifier: Apache-2.0

// Package extraction implements knowledge extractors and profile routing.
package extraction

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// PassthroughExtractor copies text files (md, txt, csv, json, xml, html) as-is into content.md.
type PassthroughExtractor struct{}

func (e *PassthroughExtractor) Name() string { return "text-passthrough" }

func (e *PassthroughExtractor) Formats() []string {
	return []string{"md", "txt", "csv", "json", "xml", "html", "htm"}
}

func (e *PassthroughExtractor) Extract(_ context.Context, inputPath string, outputDir string, _ map[string]any, _ *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	contentFile := "content.md"
	outPath := filepath.Join(outputDir, contentFile)
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("write content: %w", err)
	}

	return &kbapi.ExtractResult{
		ContentFiles: []kbapi.ContentFile{
			{File: contentFile, Part: "whole", Index: 0},
		},
		TotalParts: 1,
	}, nil
}
