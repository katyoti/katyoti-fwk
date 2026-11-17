// SPDX-License-Identifier: Apache-2.0

package extraction

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// ExternalExtractor wraps an arbitrary CLI command that follows the extractor contract:
// reads an input file, writes content*.md + attachments/ to the output directory.
type ExternalExtractor struct {
	name    string
	formats []string
	command string
	args    []string
	timeout time.Duration
}

// NewExternalExtractor creates an ExternalExtractor from profile config.
// Config keys: command, args, formats, timeout
func NewExternalExtractor(config map[string]any) (*ExternalExtractor, error) {
	command, _ := config["command"].(string)
	if command == "" {
		return nil, fmt.Errorf("external extractor requires 'command' in config")
	}

	var formats []string
	if fmts, ok := config["formats"].([]any); ok {
		for _, f := range fmts {
			if s, ok := f.(string); ok {
				formats = append(formats, s)
			}
		}
	}
	if len(formats) == 0 {
		return nil, fmt.Errorf("external extractor requires 'formats' in config")
	}

	var args []string
	if a, ok := config["args"].([]any); ok {
		for _, v := range a {
			if s, ok := v.(string); ok {
				args = append(args, s)
			}
		}
	}

	timeout := 120 * time.Second
	if t, ok := config["timeout"].(string); ok {
		if d, err := time.ParseDuration(t); err == nil {
			timeout = d
		}
	}

	name := "external"
	if n, ok := config["name"].(string); ok {
		name = n
	}

	return &ExternalExtractor{
		name:    name,
		formats: formats,
		command: command,
		args:    args,
		timeout: timeout,
	}, nil
}

func (e *ExternalExtractor) Name() string      { return e.name }
func (e *ExternalExtractor) Formats() []string { return e.formats }

func (e *ExternalExtractor) Extract(ctx context.Context, inputPath string, outputDir string, config map[string]any, limits *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Expand placeholders in args
	expandedArgs := make([]string, len(e.args))
	for i, arg := range e.args {
		arg = strings.ReplaceAll(arg, "${input}", inputPath)
		arg = strings.ReplaceAll(arg, "${output_dir}", outputDir)
		expandedArgs[i] = arg
	}

	cmd := exec.CommandContext(ctx, e.command, expandedArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("external command %s: %w\n%s", e.command, err, string(output))
	}

	// Scan output dir for produced files
	var contentFiles []kbapi.ContentFile
	var attachments []kbapi.Attachment

	entries, _ := os.ReadDir(outputDir)
	for _, entry := range entries {
		name := entry.Name()
		if name == "metadata.yaml" {
			continue
		}
		if entry.IsDir() && name == "attachments" {
			_ = filepath.WalkDir(filepath.Join(outputDir, "attachments"), func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				relPath, _ := filepath.Rel(outputDir, path)
				attachments = append(attachments, kbapi.Attachment{
					File: filepath.ToSlash(relPath),
					Type: "external",
				})
				return nil
			})
			continue
		}
		if strings.HasPrefix(name, "content") && strings.HasSuffix(name, ".md") {
			contentFiles = append(contentFiles, kbapi.ContentFile{
				File:  name,
				Part:  "whole",
				Index: 0,
			})
		}
	}

	if len(contentFiles) == 0 {
		return nil, fmt.Errorf("external command produced no content*.md files in %s", outputDir)
	}

	return &kbapi.ExtractResult{
		ContentFiles: contentFiles,
		Attachments:  attachments,
		TotalParts:   len(contentFiles),
	}, nil
}
