// SPDX-License-Identifier: Apache-2.0

package extraction

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// MarkerExtractor uses the marker CLI (marker-pdf) to convert PDFs to markdown.
// Optimized for academic papers and complex layouts.
type MarkerExtractor struct{}

func (e *MarkerExtractor) Name() string      { return "marker-md" }
func (e *MarkerExtractor) Formats() []string { return []string{"pdf"} }

func (e *MarkerExtractor) Extract(ctx context.Context, inputPath string, outputDir string, _ map[string]any, _ *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	// marker_single <input> <output_dir>
	cmd := exec.CommandContext(ctx, "marker_single", inputPath, outputDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("marker: %w\n%s", err, string(output))
	}

	// Marker writes <basename>/<basename>.md — find and rename to content.md
	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	possiblePaths := []string{
		filepath.Join(outputDir, baseName, baseName+".md"),
		filepath.Join(outputDir, baseName+".md"),
	}

	var srcFile string
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			srcFile = p
			break
		}
	}

	dstFile := filepath.Join(outputDir, "content.md")
	if srcFile != "" && srcFile != dstFile {
		data, err := os.ReadFile(srcFile)
		if err != nil {
			return nil, fmt.Errorf("read marker output: %w", err)
		}
		if err := os.WriteFile(dstFile, data, 0o644); err != nil {
			return nil, fmt.Errorf("write content: %w", err)
		}
	}

	// Move any images to attachments/
	var attachments []kbapi.Attachment
	markerDir := filepath.Join(outputDir, baseName)
	if info, err := os.Stat(markerDir); err == nil && info.IsDir() {
		attachDir := filepath.Join(outputDir, "attachments")
		_ = os.MkdirAll(attachDir, 0o755)
		_ = filepath.WalkDir(markerDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if strings.HasSuffix(d.Name(), ".md") {
				return nil // skip the md file
			}
			dest := filepath.Join(attachDir, d.Name())
			_ = os.Rename(path, dest)
			attachments = append(attachments, kbapi.Attachment{
				File:         "attachments/" + d.Name(),
				Type:         "figure",
				ReferencedBy: "content.md",
			})
			return nil
		})
		_ = os.RemoveAll(markerDir) // clean up marker subdir
	}

	return &kbapi.ExtractResult{
		ContentFiles: []kbapi.ContentFile{
			{File: "content.md", Part: "whole", Index: 0},
		},
		Attachments: attachments,
		TotalParts:  1,
	}, nil
}

// MarkerAvailable checks if the marker CLI is installed.
func MarkerAvailable() bool {
	_, err := exec.LookPath("marker_single")
	return err == nil
}
