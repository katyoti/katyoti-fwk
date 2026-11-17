// SPDX-License-Identifier: Apache-2.0

package extraction

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// PandocExtractor uses pandoc to convert documents (DOCX, ODT, RTF, HTML, EPUB) to markdown.
type PandocExtractor struct{}

func (e *PandocExtractor) Name() string { return "pandoc-md" }

func (e *PandocExtractor) Formats() []string {
	return []string{"docx", "pptx", "odt", "rtf", "epub"}
}

func (e *PandocExtractor) Extract(ctx context.Context, inputPath string, outputDir string, config map[string]any, limits *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	contentFile := "content.md"
	outPath := filepath.Join(outputDir, contentFile)

	// Build pandoc args
	args := []string{
		inputPath,
		"--to", "markdown",
		"--wrap=none",
		"--markdown-headings=atx",
		"-o", outPath,
	}

	// Extract media (images) to attachments/
	mediaDir := filepath.Join(outputDir, "attachments")
	args = append(args, fmt.Sprintf("--extract-media=%s", mediaDir))

	// Apply config overrides
	if config != nil {
		if wrap, ok := config["wrap"].(string); ok {
			// Replace default wrap
			for i, a := range args {
				if a == "--wrap=none" {
					args[i] = "--wrap=" + wrap
				}
			}
		}
	}

	cmd := exec.CommandContext(ctx, "pandoc", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pandoc: %w\n%s", err, string(output))
	}

	// Collect attachments
	var attachments []kbapi.Attachment
	_ = filepath.WalkDir(mediaDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(outputDir, path)
		relPath = filepath.ToSlash(relPath)
		attachments = append(attachments, kbapi.Attachment{
			File:         relPath,
			Type:         "figure",
			ReferencedBy: contentFile,
		})
		return nil
	})

	return &kbapi.ExtractResult{
		ContentFiles: []kbapi.ContentFile{
			{File: contentFile, Part: "whole", Index: 0},
		},
		Attachments: attachments,
		TotalParts:  1,
	}, nil
}

// PandocAvailable checks if pandoc is installed.
func PandocAvailable() bool {
	_, err := exec.LookPath("pandoc")
	return err == nil
}
