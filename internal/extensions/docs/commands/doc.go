// SPDX-License-Identifier: Apache-2.0

// Package commands provides sidecar-specific CLI commands for ktw-docs.
package commands

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	"github.com/spf13/cobra"
)

// NewDocCommand builds the doc reference command.
func NewDocCommand(efs embed.FS) *cobra.Command {
	var (
		docOutput string
		docFormat string
	)

	cmd := &cobra.Command{
		Use:   "doc",
		Short: "Show the ktw-docs Markdown reference",
		Long: `Renders the built-in Markdown reference document and opens it in
the default browser (HTML) or writes it to a file.

Examples:
  ktw-docs doc                          # render HTML and open in browser
  ktw-docs doc -o reference.pdf         # export as PDF (format inferred)
  ktw-docs doc -o reference.docx        # export as DOCX (format inferred)
  ktw-docs doc -t pdf -o reference.pdf  # explicit format`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Format: explicit -t flag, or empty (service infers from -o extension).
			// Default to "html" only when no output is specified (browser preview).
			format := docFormat
			if format == "" && docOutput == "" {
				format = "html"
			}

			// Extract embedded docs to a temp dir (relative references need it).
			tmpDir, err := os.MkdirTemp("", "ktw-docs-ref-*")
			if err != nil {
				return fmt.Errorf("create temp dir: %w", err)
			}
			defer os.RemoveAll(tmpDir)

			entries, err := efs.ReadDir("embed/docs")
			if err != nil {
				return fmt.Errorf("read embedded docs dir: %w", err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				data, err := efs.ReadFile("embed/docs/" + entry.Name())
				if err != nil {
					return fmt.Errorf("read embedded asset %s: %w", entry.Name(), err)
				}
				if err := os.WriteFile(filepath.Join(tmpDir, entry.Name()), data, 0o644); err != nil {
					return fmt.Errorf("write asset %s: %w", entry.Name(), err)
				}
			}

			inputPath := filepath.Join(tmpDir, "markdown-reference.md")

			// Determine output path.
			output := docOutput
			openBrowser := false
			if output == "" {
				tmp, err := os.CreateTemp("", "ktw-docs-ref-*."+format)
				if err != nil {
					return fmt.Errorf("create temp output: %w", err)
				}
				output = tmp.Name()
				tmp.Close()
				if format == "html" {
					openBrowser = true
				} else {
					defer os.Remove(output)
				}
			}

			// Use the DocService (same pipeline as render CLI + KTSP + agent tools).
			svc, err := application.UseService[docsapi.DocService](ctx)
			if err != nil {
				return fmt.Errorf("docs service: %w", err)
			}

			result, err := svc.Render(ctx, docsapi.RenderInput{
				Input:  inputPath,
				Output: output,
				Format: format,
			})
			if err != nil {
				return err
			}

			if openBrowser {
				fmt.Printf("Opening %s in browser...\n", result.OutputPath)
				return openInBrowser(result.OutputPath)
			}

			fmt.Printf("Rendered: %s\n", result.OutputPath)
			return nil
		},
	}
	cmd.Flags().StringVarP(&docOutput, "output", "o", "", "Output file path (default: temp file)")
	cmd.Flags().StringVarP(&docFormat, "format", "t", "", "Output format: html, pdf, docx (inferred from -o extension if omitted)")
	return cmd
}

func openInBrowser(target string) error {
	abs, err := filepath.Abs(target)
	if err == nil {
		target = abs
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}
