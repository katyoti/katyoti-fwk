// SPDX-License-Identifier: Apache-2.0

package pandoc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/docs/internal/diagram"
)

// fileDiagrams stands for the draw.io renderer: it draws a .drawio file it
// can read and fails on one it cannot, as the draw.io CLI does.
type fileDiagrams struct{}

func (fileDiagrams) MatchesLang(string) bool { return false }
func (fileDiagrams) MatchesFile(p string) bool {
	return strings.HasSuffix(p, ".drawio")
}
func (fileDiagrams) RenderSource(string, string, string) ([]byte, error) {
	return nil, fmt.Errorf("no source rendering")
}
func (fileDiagrams) RenderFile(p, _ string) ([]byte, error) {
	if _, err := os.Stat(p); err != nil {
		return nil, err
	}
	return []byte("png"), nil
}

// sectionWithDiagram writes a made-up study section that embeds a draw.io
// diagram from its media folder (a name with spaces, as the scaffold writes
// them), and the copy of its text an export renders from another folder.
func sectionWithDiagram(t *testing.T) (srcDir, copyPath string) {
	t.Helper()
	root := t.TempDir()
	srcDir = filepath.Join(root, "as-is")
	if err := os.MkdirAll(filepath.Join(srcDir, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "media", "Example - overview.drawio"), []byte("<mxfile/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	text := "# Views\n\n![Overview](media/Example - overview.drawio)\n\nMore text.\n"
	exportDir := filepath.Join(root, "export")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyPath = filepath.Join(exportDir, "document.md")
	if err := os.WriteFile(copyPath, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return srcDir, copyPath
}

// A diagram that cannot be drawn never stays an image reference to its
// source: Pandoc fails the whole document on it (a pptx refuses the missing
// file). It is named in the text instead.
func TestUndrawableDiagramIsNamedNotLeftAsImage(t *testing.T) {
	_, copyPath := sectionWithDiagram(t)
	p := &PandocExecutor{Diagrams: diagram.NewDispatcher(fileDiagrams{})}
	out, _, err := p.preprocessInput(RenderOpts{
		InputPath:    copyPath,
		OutputPath:   filepath.Join(filepath.Dir(copyPath), "export.pptx"),
		OutputFormat: "pptx",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "](media/Example - overview.drawio)") {
		t.Fatalf("the undrawable diagram is still an image reference:\n%s", got)
	}
	if !strings.Contains(got, "Overview — diagram not rendered: media/Example - overview.drawio") {
		t.Fatalf("the undrawable diagram is not named in the text:\n%s", got)
	}
}

// A copy rendered with the source's folder as BaseDir draws the diagram its
// source points at.
func TestCopyDrawsDiagramsFromBaseDir(t *testing.T) {
	srcDir, copyPath := sectionWithDiagram(t)
	p := &PandocExecutor{Diagrams: diagram.NewDispatcher(fileDiagrams{})}
	out, _, err := p.preprocessInput(RenderOpts{
		InputPath:    copyPath,
		BaseDir:      srcDir,
		OutputPath:   filepath.Join(filepath.Dir(copyPath), "export.docx"),
		OutputFormat: "docx",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "![Overview](export-kroki-images/Example - overview.png)") {
		t.Fatalf("the diagram is not drawn from the source folder:\n%s", got)
	}
}
