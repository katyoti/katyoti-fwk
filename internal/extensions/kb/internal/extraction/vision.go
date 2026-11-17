// SPDX-License-Identifier: Apache-2.0

package extraction

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// VisionOCRExtractor uses a Vision API to describe/transcribe images.
type VisionOCRExtractor struct {
	VisionFn func(ctx context.Context, imagePath string, prompt string) (string, error)
}

func (e *VisionOCRExtractor) Name() string { return "vision-ocr" }

func (e *VisionOCRExtractor) Formats() []string {
	return []string{"png", "jpg", "jpeg", "gif", "bmp", "tiff", "webp", "heic", "heif"}
}

func (e *VisionOCRExtractor) Extract(ctx context.Context, inputPath string, outputDir string, config map[string]any, _ *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	if e.VisionFn == nil {
		return nil, fmt.Errorf("vision-ocr requires a VisionFn (not configured)")
	}

	prompt := "Describe this image thoroughly. Extract all visible text. Describe charts, diagrams, and visual elements. Output clean Markdown."
	if config != nil {
		if p, ok := config["prompt"].(string); ok {
			prompt = p
		}
	}

	md, err := e.VisionFn(ctx, inputPath, prompt)
	if err != nil {
		return nil, fmt.Errorf("vision API: %w", err)
	}

	// Copy image as attachment
	attachDir := filepath.Join(outputDir, "attachments")
	_ = os.MkdirAll(attachDir, 0o755)
	imgName := filepath.Base(inputPath)
	attachPath := filepath.Join(attachDir, imgName)

	imgData, err := os.ReadFile(inputPath)
	if err == nil {
		_ = os.WriteFile(attachPath, imgData, 0o644)
	}

	// Write content with image reference + description
	content := fmt.Sprintf("![%s](attachments/%s)\n\n%s\n", imgName, imgName, md)
	contentFile := "content.md"
	if err := os.WriteFile(filepath.Join(outputDir, contentFile), []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write content: %w", err)
	}

	return &kbapi.ExtractResult{
		ContentFiles: []kbapi.ContentFile{
			{File: contentFile, Part: "whole", Index: 0},
		},
		Attachments: []kbapi.Attachment{
			{File: "attachments/" + imgName, Type: "figure", ReferencedBy: contentFile},
		},
		TotalParts: 1,
	}, nil
}
