// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

type readOnlyProbe struct{}

type probeOut struct {
	OK bool `json:"ok"`
}

func (readOnlyProbe) Look(context.Context, struct{}) (probeOut, error) {
	return probeOut{OK: true}, nil
}
func (readOnlyProbe) Touch(context.Context, struct{}) (probeOut, error) { return probeOut{}, nil }

// MethodDef.ReadOnly reaches the manifest as the `read-only` tag, and only on
// the method that declares it — the path a recipe lint and ActionService read.
func TestReadOnlyMethodDefReachesTheManifestAsATag(t *testing.T) {
	slot := ktslocal.BuildKTSPSlotWithTags("probe", ktsp.DefaultInstance, "", readOnlyProbe{}, nil,
		map[string]meta.MethodDef{"Look": {ReadOnly: true}})
	methods := mapMethods(slot.Info().Methods)
	got := map[string]bool{}
	for _, m := range methods {
		got[m.Name] = m.IsReadOnly()
	}
	if len(got) != 2 {
		t.Fatalf("methods = %v, want Look and Touch", got)
	}
	for name, want := range map[string]bool{"look": true, "touch": false} {
		ro, ok := got[name]
		if !ok {
			for n := range got {
				if strings.EqualFold(n, name) {
					ro, ok = got[n], true
				}
			}
		}
		if !ok {
			t.Fatalf("method %s missing from %v", name, got)
		}
		if ro != want {
			t.Errorf("%s: IsReadOnly = %v, want %v", name, ro, want)
		}
	}
}

// A SlotMethod decodes its tags from the manifest's list AND from the KTSP
// descriptor's map — the descriptor is what `gen-ktsp-client` reads, and a
// map there once broke the generator on the first tagged method.
func TestSlotMethodDecodesTagsFromListAndMap(t *testing.T) {
	for _, raw := range []string{
		`{"name":"guide-get","tags":["cli.name=x","read-only"]}`,
		`{"name":"guide-get","tags":{"read-only":"true","cli.name":"x"}}`,
	} {
		var m mbapi.SlotMethod
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if !m.IsReadOnly() || len(m.Tags) != 2 || m.Tags[0] != "cli.name=x" || m.Name != "guide-get" {
			t.Errorf("%s: decoded %+v", raw, m)
		}
	}
	var m mbapi.SlotMethod
	if err := json.Unmarshal([]byte(`{"name":"x","tags":7}`), &m); err == nil {
		t.Error("tags that are neither list nor map decoded")
	}
}
