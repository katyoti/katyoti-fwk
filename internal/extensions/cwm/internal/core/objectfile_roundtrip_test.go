// SPDX-License-Identifier: Apache-2.0

package core

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"gopkg.in/yaml.v3"
)

// roundTripTexts are the strings a literal block cannot carry as yaml.v3 writes
// it (a leading newline, a leading-space first line) beside the ones it
// carries — every one must read back to itself.
var roundTripTexts = []string{
	"\nab",
	"\n\nab",
	"ab\n",
	"ab\n\n",
	"a\n\nb",
	"  indented\nx",
	"x\n  indented",
	"\n漢字",
	"漢字\nかな",
	"trailing  \nspaces  ",
	"a\n \nb",
	"\t tab\nx",
	"one line",
	"ordinary\nmulti-line text\n",
}

// TestObjectFileTextRoundTrip writes each text as a capability field (and as
// the name) in both spellings, fresh and over a prior file, and reads it back.
func TestObjectFileTextRoundTrip(t *testing.T) {
	for _, suffix := range []string{api.ObjectFileSuffix, api.ObjectFileSuffixMD} {
		for i, text := range roundTripTexts {
			path := filepath.Join(t.TempDir(), "o"+suffix)
			obj := api.Object{ID: "m.o", Type: "note", Name: "n",
				Capabilities: map[string]map[string]any{"want": {"statement": "first"}}}
			if suffix == api.ObjectFileSuffixMD {
				obj.Body = "prose\n"
			}
			// fresh write
			fresh := obj
			fresh.Capabilities = map[string]map[string]any{"want": {"statement": text}}
			fresh.Name = text
			check(t, path, fresh, text, "fresh", i, suffix)
			// written over a prior file (the splice / merge path)
			if err := WriteObjectFile(path, obj); err != nil {
				t.Fatal(err)
			}
			check(t, path, fresh, text, "over", i, suffix)
		}
	}
}

func check(t *testing.T, path string, obj api.Object, text, how string, i int, suffix string) {
	t.Helper()
	if err := WriteObjectFile(path, obj); err != nil {
		t.Fatalf("%s %s #%d write: %v", suffix, how, i, err)
	}
	got, err := ReadObjectFile(path)
	if err != nil {
		t.Errorf("%s %s %q read: %v", suffix, how, text, err)
		return
	}
	if s, _ := got.Capabilities["want"]["statement"].(string); s != text {
		t.Errorf("%s %s field %q read back %q", suffix, how, text, s)
	}
	if got.Name != text {
		t.Errorf("%s %s name %q read back %q", suffix, how, text, got.Name)
	}
}

// TestObjectFileTextStyle keeps committed files readable: only the text a
// block would lose is quoted; ordinary multi-line text stays a block.
func TestObjectFileTextStyle(t *testing.T) {
	for text, want := range map[string]string{
		"ordinary\nmulti-line text\n": "statement: |\n",
		"\nab":                        `statement: "\nab"`,
	} {
		obj := api.Object{ID: "m.o", Type: "note", Capabilities: map[string]map[string]any{"want": {"statement": text}}}
		out, err := marshalObjectYAML(obj)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), want) {
			t.Errorf("%q encoded as\n%s\nwant it to contain %q", text, out, want)
		}
	}
}

// TestKeptNodeTextsWhole: a node the merge keeps from the file is re-emitted;
// an authored literal block that opens with a blank line keeps it.
func TestKeptNodeTextsWhole(t *testing.T) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("k: |\n\n  ab\n"), &doc); err != nil {
		t.Fatal(err)
	}
	keepTextsWhole(&doc)
	out, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]string
	if err := yaml.Unmarshal(out, &back); err != nil || back["k"] != "\nab\n" {
		t.Errorf("re-emitted %q read back %q (%v)", out, back["k"], err)
	}
}
