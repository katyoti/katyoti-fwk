// SPDX-License-Identifier: Apache-2.0

package mdrender

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/docxtpl"
	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/pandoc"
)

// RenderMarkdownTemplate renders a Jinja2 markdown template to DOCX via Pandoc,
// closed by the production mark's line when markLine is not empty.
func RenderMarkdownTemplate(templatePath string, data map[string]any, output, markLine string) error {
	raw, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}

	rendered, err := RenderJinja2(string(raw), data)
	if err != nil {
		return fmt.Errorf("render jinja2: %w", err)
	}

	tmpMd, err := os.CreateTemp("", "ktw-md-*.md")
	if err != nil {
		return err
	}
	defer os.Remove(tmpMd.Name())
	tmpMd.WriteString(rendered)
	tmpMd.Close()

	luaFilter, err := WriteLuaFilter()
	if err != nil {
		return err
	}
	defer os.Remove(luaFilter)

	outFmt := "docx"
	switch strings.ToLower(filepath.Ext(output)) {
	case ".pdf":
		outFmt = "pdf"
	case ".html":
		outFmt = "html"
	}

	executor := pandoc.NewPandocExecutor()
	return executor.Render(pandoc.RenderOpts{
		InputPath:    tmpMd.Name(),
		OutputPath:   output,
		OutputFormat: outFmt,
		ResourcePath: filepath.Dir(templatePath),
		ExtraArgs:    []string{"--lua-filter=" + luaFilter},
		MarkLine:     markLine,
	})
}

// LoadMarkdownData parses a .md file: frontmatter → data vars, body → Pandoc → RawXML at {{content}}.
func LoadMarkdownData(mdPath, templatePath string, data map[string]any) error {
	raw, err := os.ReadFile(mdPath)
	if err != nil {
		return fmt.Errorf("read markdown: %w", err)
	}
	metadata, body, err := ParseFrontmatter(raw)
	if err != nil {
		return err
	}

	for k, v := range metadata {
		data[k] = v
	}

	tmpMd, err := os.CreateTemp("", "ktw-md-*.md")
	if err != nil {
		return err
	}
	defer os.Remove(tmpMd.Name())
	tmpMd.WriteString(body)
	tmpMd.Close()

	tmpDocx, err := os.CreateTemp("", "ktw-pandoc-*.docx")
	if err != nil {
		return err
	}
	tmpDocx.Close()
	defer os.Remove(tmpDocx.Name())

	luaFilter, err := WriteLuaFilter()
	if err != nil {
		return err
	}
	defer os.Remove(luaFilter)

	absTemplate, _ := filepath.Abs(templatePath)
	absMdDir, _ := filepath.Abs(filepath.Dir(mdPath))

	executor := pandoc.NewPandocExecutor()
	if err := executor.Render(pandoc.RenderOpts{
		InputPath:    tmpMd.Name(),
		OutputPath:   tmpDocx.Name(),
		OutputFormat: "docx",
		ReferenceDoc: absTemplate,
		ExtraArgs:    []string{"--lua-filter=" + luaFilter},
		ResourcePath: absMdDir,
	}); err != nil {
		return fmt.Errorf("pandoc render: %w", err)
	}

	extraction, err := docxtpl.ExtractBodyWithNumbering(tmpDocx.Name())
	if err != nil {
		return fmt.Errorf("extract body: %w", err)
	}

	data["content"] = extraction.BodyXML
	data["__numbering"] = extraction.Numbering
	return nil
}

// ParseFrontmatter splits a markdown file into YAML frontmatter metadata and body.
func ParseFrontmatter(raw []byte) (map[string]any, string, error) {
	content := string(raw)
	if !strings.HasPrefix(content, "---") {
		return map[string]any{}, content, nil
	}
	end := strings.Index(content[3:], "\n---")
	if end < 0 {
		return map[string]any{}, content, nil
	}
	end += 3

	fmText := content[3:end]
	body := strings.TrimPrefix(content[end+4:], "\n")

	var metadata map[string]any
	if err := yaml.Unmarshal([]byte(fmText), &metadata); err != nil {
		return nil, "", fmt.Errorf("parse YAML frontmatter: %w", err)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	return metadata, body, nil
}

// fixPandocStyles replaces Pandoc's hardcoded paragraph styles with
// standard Word built-in styles that exist in most reference docs.
// IsMarkdownFile returns true if the file extension indicates markdown.
func IsMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".mdx" || ext == ".markdown"
}
