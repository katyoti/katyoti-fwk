// SPDX-License-Identifier: Apache-2.0

package pandoc

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/diagram"
)

// PandocExecutor runs Pandoc commands for document conversion.
type PandocExecutor struct {
	PandocPath string              // path to pandoc binary (default: "pandoc")
	TypstPath  string              // path to typst binary (default: "typst")
	Diagrams   *diagram.Dispatcher // optional — diagram rendering dispatcher
}

// NewPandocExecutor creates an executor that looks up pandoc/typst on PATH.
func NewPandocExecutor() *PandocExecutor {
	return &PandocExecutor{
		PandocPath: "pandoc",
		TypstPath:  "typst",
	}
}

// RenderOpts describes a document rendering request.
type RenderOpts struct {
	InputPath    string   // path to input markdown file
	OutputPath   string   // path to output file
	OutputFormat string   // pdf, docx, pptx, html, typst
	Template     string   // template name or path (optional)
	ReferenceDoc string   // --reference-doc path for DOCX/PPTX (optional)
	FilterPath   string   // path to Pandoc filter binary (optional — ktw-docs filter)
	ResourcePath string   // --resource-path for Pandoc (where to find images)
	ExtraArgs    []string // additional pandoc arguments
	WorkDir      string   // working directory for the command
	BaseDir      string   // folder relative references resolve against (default: the input's folder)
	MarkLine     string   // the production mark's line, closing the document (none when empty)
}

// Render converts a document using Pandoc (and Typst for PDF).
// Preprocessing (text-level transforms + diagram rendering) always runs.
func (p *PandocExecutor) Render(opts RenderOpts) error {
	inputPath := opts.InputPath

	// Pre-process markdown: text-level transforms (admonitions, badges, emoji,
	// highlights) always run; diagram rendering runs when dispatcher is available.
	{
		processed, tmpFile, err := p.preprocessInput(opts)
		if err != nil {
			return fmt.Errorf("preprocess: %w", err)
		}
		if tmpFile != "" {
			defer os.Remove(tmpFile)
			inputPath = tmpFile
		}
		_ = processed
	}

	renderOpts := opts
	renderOpts.InputPath = inputPath

	// Make paths absolute so we can set cmd.Dir to the output directory.
	absInput, err := filepath.Abs(renderOpts.InputPath)
	if err == nil {
		renderOpts.InputPath = absInput
	}
	absOutput, err := filepath.Abs(renderOpts.OutputPath)
	if err == nil {
		renderOpts.OutputPath = absOutput
	}

	outputDir := filepath.Dir(absOutput)

	if renderOpts.ResourcePath == "" {
		renderOpts.ResourcePath = outputDir
		if renderOpts.BaseDir != "" {
			renderOpts.ResourcePath += string(os.PathListSeparator) + renderOpts.BaseDir
		}
	}

	args := p.buildArgs(renderOpts)

	cmd := exec.Command(p.PandocPath, args...)
	if renderOpts.WorkDir != "" {
		cmd.Dir = renderOpts.WorkDir
	} else {
		cmd.Dir = outputDir
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pandoc: %w\nstderr: %s", err, stderr.String())
	}

	return nil
}

// preprocessInput reads the input markdown, applies text-level preprocessing
// and renders diagram code blocks, writing a temp file. Returns the temp path.
func (p *PandocExecutor) preprocessInput(opts RenderOpts) ([]byte, string, error) {
	markdown, err := os.ReadFile(opts.InputPath)
	if err != nil {
		return nil, "", fmt.Errorf("read input: %w", err)
	}

	processed := []byte(PreprocessMarkdown(string(markdown)))
	processed = AppendMark(processed, opts.MarkLine)

	if p.Diagrams != nil {
		imgDir := OutputDirFromInputFile(opts.OutputPath)
		baseDir := opts.BaseDir
		if baseDir == "" {
			baseDir = filepath.Dir(opts.InputPath)
		}
		outputDir := filepath.Dir(opts.OutputPath)
		processed, err = diagram.PreprocessDiagrams(processed, p.Diagrams, imgDir, opts.OutputFormat, baseDir, outputDir)
		if err != nil {
			return nil, "", err
		}
	}

	if bytes.Equal(markdown, processed) {
		return processed, "", nil
	}

	dir := filepath.Dir(opts.InputPath)
	tmp, err := os.CreateTemp(dir, "ktw-docs-*.md")
	if err != nil {
		return nil, "", fmt.Errorf("create temp file: %w", err)
	}
	if _, err := tmp.Write(processed); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, "", fmt.Errorf("write temp file: %w", err)
	}
	tmp.Close()

	return processed, tmp.Name(), nil
}

// AppendMark closes a markdown document with the production mark's line, a
// line in a div every format can style (the docx custom style
// "Production Mark", the html class production-mark).
func AppendMark(markdown []byte, line string) []byte {
	if strings.TrimSpace(line) == "" {
		return markdown
	}
	out := append(bytes.TrimRight(markdown, "\r\n "), "\n\n::: {.production-mark custom-style=\"Production Mark\"}\n"...)
	out = append(out, line...)
	return append(out, "\n:::\n"...)
}

// buildArgs constructs the pandoc command-line arguments.
func (p *PandocExecutor) buildArgs(opts RenderOpts) []string {
	var args []string

	args = append(args, opts.InputPath)
	args = append(args, "-o", opts.OutputPath)

	switch strings.ToLower(opts.OutputFormat) {
	case "pdf":
		args = append(args, "--pdf-engine="+p.TypstPath)
		args = append(args,
			"--variable", "mainfont=Arial",
			"--variable", "sansfont=Arial",
			"--variable", "monofont=Courier New",
		)
	case "docx":
		args = append(args, "-t", "docx")
	case "pptx":
		args = append(args, "-t", "pptx")
	case "html":
		args = append(args, "-t", "html5", "--standalone")
	}

	if opts.Template != "" {
		args = append(args, "--template="+opts.Template)
	}

	if opts.ReferenceDoc != "" {
		args = append(args, "--reference-doc="+opts.ReferenceDoc)
	}

	if opts.FilterPath != "" {
		args = append(args, "--filter="+opts.FilterPath)
	}

	if opts.ResourcePath != "" {
		args = append(args, "--resource-path="+opts.ResourcePath)
	}

	args = append(args, opts.ExtraArgs...)

	return args
}

// CheckPandoc returns the Pandoc version or an error if not found.
func (p *PandocExecutor) CheckPandoc() (string, error) {
	return checkToolVersion(p.PandocPath, "--version")
}

// CheckTypst returns the Typst version or an error if not found.
func (p *PandocExecutor) CheckTypst() (string, error) {
	return checkToolVersion(p.TypstPath, "--version")
}

func checkToolVersion(path, flag string) (string, error) {
	cmd := exec.Command(path, flag)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s not found: %w", path, err)
	}
	line := strings.SplitN(string(out), "\n", 2)[0]
	return strings.TrimSpace(line), nil
}

// BuildDiagramDispatcher creates a Dispatcher with all available renderers.
func BuildDiagramDispatcher(kroki *diagram.KrokiRenderer) *diagram.Dispatcher {
	var renderers []diagram.DiagramRenderer

	if drawio := diagram.NewDrawioRenderer(); drawio != nil {
		renderers = append(renderers, drawio)
	}

	if kroki != nil {
		renderers = append(renderers, kroki)
	}

	return diagram.NewDispatcher(renderers...)
}
