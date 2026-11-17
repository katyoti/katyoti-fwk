// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// memberWriting is a member whose sidecar-info states it writes `hook` in
// format f, publishing one hook and one object type, each carrying a key this
// host does not know (`colour`).
func memberWriting(f string) *mbapi.Manifest {
	info := contribapi.Contribution{Provider: "ktw-new", Type: contribapi.SidecarInfoKind, ID: "default"}
	info.SetData([]byte(`{"name":"ktw-new","version":"9","description":"d"}`))
	if f != "" {
		if err := contribapi.SetFormats(&info, map[string]string{"hook": f}); err != nil {
			panic(err)
		}
	}
	return &mbapi.Manifest{Provider: "ktw-new", Contributions: []contribapi.Contribution{
		info,
		{Provider: "ktw-new", Type: "hook", ID: "h", Layers: contribapi.InlineData([]byte(`{"name":"h","type":"lint","uri":"x:y/z","colour":"red"}`))},
		{Provider: "ktw-new", Type: "object-type", ID: "o", Layers: contribapi.InlineData([]byte(`{"name":"o","colour":"red"}`))},
	}}
}

// A kind's format decides how a member's entries of it are read: the host's
// own or an older one strictly (an unknown key refuses the declaration), a
// newer minor leniently (the unknown keys ignored, named in a finding), a
// newer major not at all (refused, saying to update the hub). The object type,
// whose format the member does not state, stays strict throughout.
func TestDeclarationsReadByFormat(t *testing.T) {
	host := builtin.Formats()["hook"]
	hf, ok := contribapi.ParseFormat(host)
	if !ok {
		t.Fatalf("the built-in kind reader states no format for hook (%q)", host)
	}
	newerMinor := contribapi.Format{Major: hf.Major, Minor: hf.Minor + 1}.String()
	newerMajor := contribapi.Format{Major: hf.Major + 1}.String()

	rules := func(d cwmapi.Declarations) map[string]string {
		out := map[string]string{}
		for _, f := range d.Findings {
			out[f.Kind] = f.Rule + ": " + f.Message
		}
		return out
	}

	t.Run("same format: strict", func(t *testing.T) {
		d := cwmapi.DeclarationsFromManifests([]*mbapi.Manifest{memberWriting(host)}, nil)
		if len(d.Hooks) != 0 {
			t.Errorf("a hook with an unknown key in the host's format was read: %+v", d.Hooks)
		}
		if r := rules(d)["hook"]; !strings.HasPrefix(r, cwmapi.FindingDecode) || !strings.Contains(r, "colour") {
			t.Errorf("hook finding = %q, want a decode finding naming colour", r)
		}
	})
	t.Run("no format stated: strict", func(t *testing.T) {
		d := cwmapi.DeclarationsFromManifests([]*mbapi.Manifest{memberWriting("")}, nil)
		if len(d.Hooks) != 0 {
			t.Errorf("a member stating no format was read leniently")
		}
	})
	t.Run("newer minor: unknown keys ignored, with a finding", func(t *testing.T) {
		d := cwmapi.DeclarationsFromManifests([]*mbapi.Manifest{memberWriting(newerMinor)}, nil)
		if len(d.Hooks) != 1 || d.Hooks[0].Provider != "ktw-new" {
			t.Fatalf("the hook in a newer minor was not read: %+v (findings %v)", d.Hooks, d.Findings)
		}
		r := rules(d)["hook"]
		if !strings.HasPrefix(r, cwmapi.FindingFormatNewerMinor) || !strings.Contains(r, "colour") || !strings.Contains(r, newerMinor) {
			t.Errorf("hook finding = %q, want a newer-minor finding naming colour and %s", r, newerMinor)
		}
		if len(d.Objects) != 0 {
			t.Errorf("the object type, whose format is not stated, must stay strict")
		}
	})
	t.Run("newer major: refused, update the hub", func(t *testing.T) {
		d := cwmapi.DeclarationsFromManifests([]*mbapi.Manifest{memberWriting(newerMajor)}, nil)
		if len(d.Hooks) != 0 {
			t.Errorf("a hook in a newer major was read")
		}
		r := rules(d)["hook"]
		if !strings.HasPrefix(r, cwmapi.FindingFormatNewerMajor) || !strings.Contains(r, "update the hub") {
			t.Errorf("hook finding = %q, want a newer-major refusal saying to update the hub", r)
		}
	})
}

// The host reads the formats of every kind compiled into it from the built-in
// kind reader, never from the system set alone: a kind that leaves the system
// set for its pack (skill, task, persona) keeps its format here
// . With no pack leaf both readers agree, so the
// guard is on the source.
func TestDeclarationsHostFormatsFromBuiltin(t *testing.T) {
	src, err := os.ReadFile("declarations.go")
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`contribution\.SystemFormats\(\)`).Match(src) {
		t.Error("declarations.go reads the host formats from contribution.SystemFormats(): a pack-owned kind's format would vanish; read builtin.Formats()")
	}
	if !regexp.MustCompile(`host := builtin\.Formats\(\)`).Match(src) {
		t.Error("declarations.go does not read the host formats from builtin.Formats()")
	}
}
