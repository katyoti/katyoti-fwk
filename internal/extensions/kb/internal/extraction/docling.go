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

// DoclingExtractor uses the docling CLI to convert PDFs to markdown.
// Layout-aware: handles tables, figures, multi-column. Figures extracted as images.
type DoclingExtractor struct{}

func (e *DoclingExtractor) Name() string      { return "docling-md" }
func (e *DoclingExtractor) Formats() []string { return []string{"pdf", "docx", "pptx", "html"} }

func (e *DoclingExtractor) Extract(ctx context.Context, inputPath string, outputDir string, _ map[string]any, _ *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	// docling converts to markdown and extracts images
	cmd := exec.CommandContext(ctx, "docling",
		"--from", inputPath,
		"--to", "md",
		"--output", outputDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docling: %w\n%s", err, string(output))
	}

	// Docling writes <basename>.md in the output dir — rename to content.md
	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	srcFile := filepath.Join(outputDir, baseName+".md")
	dstFile := filepath.Join(outputDir, "content.md")

	if _, err := os.Stat(srcFile); err == nil && srcFile != dstFile {
		if err := os.Rename(srcFile, dstFile); err != nil {
			return nil, fmt.Errorf("rename docling output: %w", err)
		}
	}

	// Collect extracted images as attachments
	var attachments []kbapi.Attachment
	attachDir := filepath.Join(outputDir, "attachments")
	_ = filepath.WalkDir(attachDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(outputDir, path)
		attachments = append(attachments, kbapi.Attachment{
			File:         filepath.ToSlash(relPath),
			Type:         "figure",
			ReferencedBy: "content.md",
		})
		return nil
	})

	return &kbapi.ExtractResult{
		ContentFiles: []kbapi.ContentFile{
			{File: "content.md", Part: "whole", Index: 0},
		},
		Attachments: attachments,
		TotalParts:  1,
	}, nil
}

// DoclingVisionExtractor uses docling for text/tables + Vision API for extracted figures.
type DoclingVisionExtractor struct {
	VisionFn func(ctx context.Context, imagePath string, prompt string) (string, error)
}

func (e *DoclingVisionExtractor) Name() string      { return "docling-vision" }
func (e *DoclingVisionExtractor) Formats() []string { return []string{"pdf"} }

func (e *DoclingVisionExtractor) Extract(ctx context.Context, inputPath string, outputDir string, config map[string]any, limits *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	if e.VisionFn == nil {
		return nil, fmt.Errorf("docling-vision requires a VisionFn (not configured)")
	}

	// First: run docling for text + tables
	base := &DoclingExtractor{}
	result, err := base.Extract(ctx, inputPath, outputDir, config, limits)
	if err != nil {
		return nil, err
	}

	// Then: describe each extracted image via Vision API
	prompt := "Describe this image from a document. Include all visible text, data, and visual elements. Output clean Markdown."
	if config != nil {
		if p, ok := config["prompt"].(string); ok {
			prompt = p
		}
	}

	contentPath := filepath.Join(outputDir, "content.md")
	content, err := os.ReadFile(contentPath)
	if err != nil {
		return result, nil // return docling result even if we can't read it
	}

	contentStr := string(content)
	for _, att := range result.Attachments {
		imgPath := filepath.Join(outputDir, att.File)
		description, err := e.VisionFn(ctx, imgPath, prompt)
		if err != nil {
			continue
		}
		// Replace image reference with description + image
		oldRef := fmt.Sprintf("![")
		if idx := strings.Index(contentStr, oldRef); idx >= 0 {
			// Insert description after the image reference
			contentStr = contentStr + "\n\n" + description
		}
	}

	_ = os.WriteFile(contentPath, []byte(contentStr), 0o644)

	return result, nil
}

// DoclingAvailable checks if the docling CLI is installed.
func DoclingAvailable() bool {
	_, err := exec.LookPath("docling")
	return err == nil
}
