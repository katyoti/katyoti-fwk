// SPDX-License-Identifier: Apache-2.0

package soldocs

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	cwmfolder "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/folder"
)

// A solution's documents go into a project no hub serves through CWM's verbs:
// read back through CWM, each is a document of the first module with its
// title, language and body; installed again, each is updated in place.
func TestPlaceWritesDocumentsThroughCWM(t *testing.T) {
	ctx := context.Background()
	project := t.TempDir()
	for _, m := range []string{"zeta", "demo"} {
		if err := os.MkdirAll(filepath.Join(project, "modules", m), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	place := func(lang, name, md string) string {
		t.Helper()
		p, err := Open(ctx, project)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		id, err := p.Place(ctx, lang, name, []byte(md))
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	id := place("fr", "Lisez-moi.md", "# Lisez-moi\r\n\r\nBonjour.\r\n")
	if id != "demo.lisez-moi-fr" {
		t.Fatalf("id = %q, want demo.lisez-moi-fr", id)
	}
	read := func() cwmapi.Object {
		t.Helper()
		c, err := cwmfolder.Open(ctx, project)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		got, err := c.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "demo", ID: id})
		if err != nil {
			t.Fatalf("the document should be read back through CWM: %v", err)
		}
		return got.Object
	}
	o := read()
	if o.Type != "document" || o.Name != "Lisez-moi" || o.Parent != "demo" || o.Fields["title"] != "Lisez-moi" || o.Fields["lang"] != "fr" {
		t.Fatalf("document read back = %+v", o)
	}
	if o.Body != "# Lisez-moi\n\nBonjour.\n" {
		t.Fatalf("body = %q", o.Body)
	}
	// The same solution installed again: the document is updated, not refused.
	place("fr", "Lisez-moi.md", "# Lisez-moi\n\nBonjour encore.\n")
	if o := read(); o.Body != "# Lisez-moi\n\nBonjour encore.\n" {
		t.Fatalf("a re-install should update the document; body = %q", o.Body)
	}
}

// soldocs writes no file itself: every write goes through CWM.
func TestSoldocsWritesNoFileItself(t *testing.T) {
	src, err := os.ReadFile("soldocs.go")
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`os\.(WriteFile|MkdirAll|Create|OpenFile|Mkdir)\(`).Find(src); m != nil {
		t.Fatalf("soldocs writes a file itself (%s): place a document through CWM's verbs", m)
	}
}
