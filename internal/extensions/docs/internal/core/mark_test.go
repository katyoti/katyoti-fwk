// SPDX-License-Identifier: Apache-2.0

package core

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
)

// TestRenderCarriesMark renders a document to html and docx, and a markdown
// template to docx: each carries the production mark; a document that
// switches it off carries none.
func TestRenderCarriesMark(t *testing.T) {
	if _, err := exec.LookPath("pandoc"); err != nil {
		t.Skip("pandoc not on PATH")
	}
	d := &docServicesImpl{}
	ctx := context.Background()
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	on := write("on.md", "# Étude\n\nUn paragraphe.\n")
	offDoc := write("off.md", "---\nmark: off\n---\n# Étude\n\nUn paragraphe.\n")

	for _, format := range []string{"html", "docx"} {
		t.Run(format, func(t *testing.T) {
			out := filepath.Join(dir, "on."+format)
			if _, err := d.Render(ctx, api.RenderInput{Input: on, Output: out}); err != nil {
				t.Fatal(err)
			}
			if got := textOf(t, out); !strings.Contains(got, api.DefaultMarkText) {
				t.Fatalf("%s carries no mark:\n%s", format, got)
			}
			out = filepath.Join(dir, "off."+format)
			if _, err := d.Render(ctx, api.RenderInput{Input: offDoc, Output: out}); err != nil {
				t.Fatal(err)
			}
			if got := textOf(t, out); strings.Contains(got, api.DefaultMarkText) {
				t.Fatalf("%s switched off still carries the mark", format)
			}
		})
	}

	t.Run("md template", func(t *testing.T) {
		tpl := write("tpl.md", "# {{ title }}\n\nCorps.\n")
		out := filepath.Join(dir, "tpl.docx")
		if _, err := d.RenderTemplate(ctx, api.RenderTemplateInput{Template: tpl, Data: map[string]any{"title": "Étude"}, Output: out}); err != nil {
			t.Fatal(err)
		}
		if got := textOf(t, out); !strings.Contains(got, api.DefaultMarkText) {
			t.Fatalf("md template output carries no mark:\n%s", got)
		}
	})
}

// textOf is a rendered file's text: the html itself, or a docx's document.xml.
func textOf(t *testing.T, path string) string {
	t.Helper()
	if filepath.Ext(path) != ".docx" {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			b, _ := io.ReadAll(r)
			return string(b)
		}
	}
	t.Fatal("no word/document.xml")
	return ""
}
