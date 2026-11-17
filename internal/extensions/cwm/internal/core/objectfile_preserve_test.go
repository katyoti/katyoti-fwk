// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The echo module object as authored — header note, a folded description, an
// inline remark. The GUI run of 2026-09-25 erased all three with one click.
const authoredEcho = `# The module object — this folder DECLARES itself a module.
# Written from the folder's own narrative.
id: echo
type: module
name: echo
description: >-
  Echo — the module's concepts and design notes under modules/echo/.
capabilities:
  module:
    decisions: none   # m.UC-012: no open decision on this module today
`

// TestObjectWriteKeepsWhatDidNotChange — a write changes what changed and
// nothing else: add an aspect, and the comments, the folding and the inline
// remark survive; take it away again, and the file is what it was.
func TestObjectWriteKeepsWhatDidNotChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "echo.cwm.yaml")
	if err := os.WriteFile(path, []byte(authoredEcho), 0o644); err != nil {
		t.Fatal(err)
	}
	obj, err := ReadObjectFile(path)
	if err != nil {
		t.Fatal(err)
	}

	obj.Aspects = []string{"isa"}
	if err := WriteObjectFile(path, obj); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	for _, keep := range []string{
		"# The module object — this folder DECLARES itself a module.",
		"description: >-",
		"# m.UC-012: no open decision on this module today",
	} {
		if !strings.Contains(string(after), keep) {
			t.Errorf("the write lost %q:\n%s", keep, after)
		}
	}
	if !strings.Contains(string(after), "aspects:") {
		t.Errorf("the change itself is missing:\n%s", after)
	}

	obj.Aspects = nil
	if err := WriteObjectFile(path, obj); err != nil {
		t.Fatal(err)
	}
	back, _ := os.ReadFile(path)
	// The one thing yaml.v3 does not keep is the PADDING before an inline
	// comment: it writes exactly one space. The comment itself survives.
	pad := regexp.MustCompile(`[ 	]+#`)
	if pad.ReplaceAllString(string(back), " #") != pad.ReplaceAllString(authoredEcho, " #") {
		t.Errorf("add-then-remove must give the authored file back.\nwant:\n%s\ngot:\n%s", authoredEcho, back)
	}
}

// TestObjectWriteKeepsTheCommentOfAChangedValue — a changed scalar takes its
// new value and keeps the remark that explained it.
func TestObjectWriteKeepsTheCommentOfAChangedValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "echo.cwm.yaml")
	_ = os.WriteFile(path, []byte(authoredEcho), 0o644)
	obj, _ := ReadObjectFile(path)
	obj.Capabilities["module"]["decisions"] = "open"
	if err := WriteObjectFile(path, obj); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), "decisions: open") || !strings.Contains(string(after), "# m.UC-012") {
		t.Errorf("changed value or its comment wrong:\n%s", after)
	}
}

// A file written by a newer binary: a top-level key this binary's api.Object
// does not know (lifecycle), and an edge key api.ObjectEdge does not know (since).
const writtenByANewerBinary = `id: m.thing
type: note
name: Old
lifecycle:
  state: proposed
  history: [created]
edges:
  - kind: serves
    to: m.goal
    since: "2026-10-10"
  - kind: realizes
    to: m.feature
`

// TestObjectWriteKeepsWhatThisBinaryDoesNotKnow — an older binary updating an
// object a newer one wrote keeps the keys it cannot see, at the top level and
// in an edge, in both spellings; a key it knows still goes when cleared.
func TestObjectWriteKeepsWhatThisBinaryDoesNotKnow(t *testing.T) {
	for _, tc := range []struct{ file, text string }{
		{"thing.cwm.yaml", writtenByANewerBinary},
		{"thing.cwm.md", "---\n" + writtenByANewerBinary + "---\nThe prose body.\n"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.file)
			if err := os.WriteFile(path, []byte(tc.text), 0o644); err != nil {
				t.Fatal(err)
			}
			obj, err := ReadObjectFile(path)
			if err != nil {
				t.Fatal(err)
			}
			obj.Name = "New"
			obj.Tags = []string{"t"}
			if err := WriteObjectFile(path, obj); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(path)
			for _, want := range []string{"name: New", "lifecycle:", "state: proposed", "history: [created]", `since: "2026-10-10"`, "to: m.feature"} {
				if !strings.Contains(string(after), want) {
					t.Errorf("the write lost %q:\n%s", want, after)
				}
			}

			// A key this binary knows still goes when the update clears it.
			obj.Tags = nil
			if err := WriteObjectFile(path, obj); err != nil {
				t.Fatal(err)
			}
			back, _ := os.ReadFile(path)
			if strings.Contains(string(back), "tags:") {
				t.Errorf("a cleared known key must go:\n%s", back)
			}
			if !strings.Contains(string(back), "lifecycle:") {
				t.Errorf("the unknown key must survive a second write:\n%s", back)
			}
		})
	}
}

// TestObjectWriteKeepsFrontmatterComments — the .cwm.md spelling writes its
// frontmatter the same way, and the prose body is untouched.
func TestObjectWriteKeepsFrontmatterComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.cwm.md")
	src := "---\n# authored remark\nid: m.note\ntype: note\nname: Old\n---\nThe prose body.\n"
	_ = os.WriteFile(path, []byte(src), 0o644)
	obj, err := ReadObjectFile(path)
	if err != nil {
		t.Fatal(err)
	}
	obj.Name = "New"
	if err := WriteObjectFile(path, obj); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	for _, want := range []string{"# authored remark", "name: New", "The prose body."} {
		if !strings.Contains(string(after), want) {
			t.Errorf("missing %q:\n%s", want, after)
		}
	}
}
