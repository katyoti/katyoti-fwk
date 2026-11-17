// SPDX-License-Identifier: Apache-2.0

package permrules

import (
	"errors"
	"strings"
	"testing"
)

// ── ParseRule ───────────────────────────────────────────────────────────

func TestParseRule_Wildcard(t *testing.T) {
	r, err := ParseRule("*")
	if err != nil {
		t.Fatalf("ParseRule(*): %v", err)
	}
	if r.ToolName != "*" || r.Field != "" || r.ValueGlob != "**" {
		t.Errorf("got %+v, want ToolName=*, Field=\"\", ValueGlob=**", r)
	}
	if r.Raw != "*" {
		t.Errorf("Raw = %q, want %q", r.Raw, "*")
	}
}

func TestParseRule_ToolAnyInput(t *testing.T) {
	r, err := ParseRule("file-read:**")
	if err != nil {
		t.Fatalf("ParseRule: %v", err)
	}
	if r.ToolName != "file-read" || r.Field != "" || r.ValueGlob != "**" {
		t.Errorf("got %+v", r)
	}
}

func TestParseRule_ToolField(t *testing.T) {
	r, err := ParseRule("file-read.workspace_path:**")
	if err != nil {
		t.Fatalf("ParseRule: %v", err)
	}
	if r.ToolName != "file-read" || r.Field != "workspace_path" || r.ValueGlob != "**" {
		t.Errorf("got %+v", r)
	}
}

func TestParseRule_AbsolutePathGlob(t *testing.T) {
	r, err := ParseRule("file-read.absolute_path:/home/user/**")
	if err != nil {
		t.Fatalf("ParseRule: %v", err)
	}
	if r.ToolName != "file-read" || r.Field != "absolute_path" || r.ValueGlob != "/home/user/**" {
		t.Errorf("got %+v", r)
	}
}

func TestParseRule_AnyToolWithField(t *testing.T) {
	r, err := ParseRule("*.absolute_path:/etc/**")
	if err != nil {
		t.Fatalf("ParseRule: %v", err)
	}
	if r.ToolName != "*" || r.Field != "absolute_path" || r.ValueGlob != "/etc/**" {
		t.Errorf("got %+v", r)
	}
}

func TestParseRule_RejectsMissingColon(t *testing.T) {
	_, err := ParseRule("file-read")
	if !errors.Is(err, errInvalidRule) {
		t.Errorf("err = %v, want errInvalidRule", err)
	}
	if !strings.Contains(err.Error(), "missing ':'") {
		t.Errorf("error = %q, want hint about ':'", err.Error())
	}
}

func TestParseRule_RejectsBareValueWithoutField(t *testing.T) {
	// `file-read:/etc/passwd` looks like the user forgot the
	// `.absolute_path` part. We reject it with a helpful hint
	// rather than silently matching nothing.
	_, err := ParseRule("file-read:/etc/passwd")
	if !errors.Is(err, errInvalidRule) {
		t.Errorf("err = %v, want errInvalidRule", err)
	}
	if !strings.Contains(err.Error(), ".<field>") {
		t.Errorf("error = %q, want hint about adding '.<field>'", err.Error())
	}
}

func TestParseRule_RejectsEmptyToolName(t *testing.T) {
	_, err := ParseRule(":**")
	if !errors.Is(err, errInvalidRule) {
		t.Errorf("err = %v, want errInvalidRule", err)
	}
}

func TestParseRule_RejectsEmptyField(t *testing.T) {
	_, err := ParseRule("file_read.:**")
	if !errors.Is(err, errInvalidRule) {
		t.Errorf("err = %v, want errInvalidRule", err)
	}
}

func TestParseRule_RejectsEmptyValue(t *testing.T) {
	_, err := ParseRule("file-read:")
	if !errors.Is(err, errInvalidRule) {
		t.Errorf("err = %v, want errInvalidRule", err)
	}
}

func TestParseRule_RejectsEmpty(t *testing.T) {
	_, err := ParseRule("")
	if !errors.Is(err, errInvalidRule) {
		t.Errorf("err = %v, want errInvalidRule", err)
	}
}

func TestParseRule_TrimsWhitespace(t *testing.T) {
	r, err := ParseRule("  file-read.workspace_path:**  ")
	if err != nil {
		t.Fatalf("ParseRule: %v", err)
	}
	if r.ToolName != "file-read" {
		t.Errorf("ToolName = %q, want file-read (whitespace not trimmed)", r.ToolName)
	}
}

// ── String / canonical ──────────────────────────────────────────────────

func TestRule_String_RoundTrip(t *testing.T) {
	cases := []string{
		"*",
		"file-read:**",
		"file-read.workspace_path:**",
		"file-read.absolute_path:/home/user/**",
		"*.absolute_path:/etc/**",
	}
	for _, in := range cases {
		r := MustParseRule(in)
		if got := r.String(); got != in {
			t.Errorf("round trip: parsed %q got back %q", in, got)
		}
	}
}

func TestRule_Canonical_NoRaw(t *testing.T) {
	// A programmatically-constructed Rule (no Raw) reconstructs
	// from its fields.
	cases := []struct {
		name string
		in   Rule
		want string
	}{
		{"wildcard", Rule{ToolName: "*", ValueGlob: "**"}, "*"},
		{"tool only", Rule{ToolName: "file-read", ValueGlob: "**"}, "file-read:**"},
		{"tool+field", Rule{ToolName: "file-read", Field: "workspace_path", ValueGlob: "**"}, "file-read.workspace_path:**"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.in.canonical(); got != c.want {
				t.Errorf("canonical = %q, want %q", got, c.want)
			}
		})
	}
}

// ── Match ───────────────────────────────────────────────────────────────

func TestMatch_WildcardMatchesEverything(t *testing.T) {
	r := MustParseRule("*")
	if !r.Match("file-read", nil) {
		t.Error("* should match any tool")
	}
	if !r.Match("bash", map[string]string{"command": "rm -rf /"}) {
		t.Error("* should match any tool with any dimensions")
	}
}

func TestMatch_ToolAnyInput(t *testing.T) {
	r := MustParseRule("file-read:**")
	if !r.Match("file-read", nil) {
		t.Error("should match file-read with no dimensions")
	}
	if !r.Match("file-read", map[string]string{"workspace_path": "foo"}) {
		t.Error("should match file-read with any dimensions")
	}
	if r.Match("file-write", nil) {
		t.Error("should NOT match file-write")
	}
}

func TestMatch_FieldExact(t *testing.T) {
	r := MustParseRule("file-read.absolute_path:/etc/hosts")
	if !r.Match("file-read", map[string]string{"absolute_path": "/etc/hosts"}) {
		t.Error("exact path should match")
	}
	if r.Match("file-read", map[string]string{"absolute_path": "/etc/passwd"}) {
		t.Error("different path should NOT match")
	}
}

func TestMatch_FieldGlob(t *testing.T) {
	r := MustParseRule("file-read.absolute_path:/home/user/**")
	cases := []struct {
		path string
		want bool
	}{
		{"/home/user/file.txt", true},
		{"/home/user/sub/file.txt", true},
		{"/home/user/", true},
		{"/home/other/file.txt", false},
		{"/etc/passwd", false},
		// The doublestar /wsfoo trap: prefix-string match would
		// incorrectly include /home/userlands; the glob should not.
		{"/home/userlands/x", false},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			got := r.Match("file-read", map[string]string{"absolute_path": c.path})
			if got != c.want {
				t.Errorf("Match(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

func TestMatch_MissingDimension(t *testing.T) {
	// Rule wants `absolute_path` but the call only has `workspace_path`.
	// No match — the rule cannot apply.
	r := MustParseRule("file-read.absolute_path:**")
	if r.Match("file-read", map[string]string{"workspace_path": "foo"}) {
		t.Error("missing dimension should NOT match")
	}
	// Nil dimensions also no-match.
	if r.Match("file-read", nil) {
		t.Error("nil dimensions should NOT match")
	}
}

func TestMatch_AnyToolWithField(t *testing.T) {
	// `*.absolute_path:/etc/**` matches ANY tool that has the
	// absolute_path dimension under /etc.
	r := MustParseRule("*.absolute_path:/etc/**")
	if !r.Match("file-read", map[string]string{"absolute_path": "/etc/hosts"}) {
		t.Error("file-read should match")
	}
	if !r.Match("grep", map[string]string{"absolute_path": "/etc/passwd"}) {
		t.Error("grep should match")
	}
	if r.Match("file-read", map[string]string{"absolute_path": "/home/user/x"}) {
		t.Error("/home/user/x should NOT match /etc/**")
	}
	if r.Match("file-read", map[string]string{"workspace_path": "/etc/hosts"}) {
		t.Error("rule needs absolute_path; workspace_path should NOT match")
	}
}

func TestMatch_InvalidRule(t *testing.T) {
	var r Rule // zero value
	if r.Match("file-read", nil) {
		t.Error("zero-value rule should never match")
	}
}
