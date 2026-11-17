// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// TestPackURIsNameTheirHost: a pack writes the URIs its host serves with the
// empty provider; the published declaration names whichever provider hosts the
// pack. A URI naming another binary, and a string that is no URI, are kept.
func TestPackURIsNameTheirHost(t *testing.T) {
	pack := fstest.MapFS{"capabilities/probe.yaml": &fstest.MapFile{Data: []byte(
		"name: probe\n" +
			"type_schema: :/data-type/type-probe\n" +
			"functions:\n" +
			"  - name: here\n" +
			"    description: ':not a uri'\n" +
			"    action: :probe/here-function\n" +
			"  - name: there\n" +
			"    description: d\n" +
			"    action: ktw-elsewhere:probe/there-function\n")}}

	app, err := runApp(t, manifestcontrib.ContributePackPart("other-host", "probe", "cwm", pack))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var caps *packProvider
	for _, p := range cwmProviders(app) {
		if p.kind == "capability" {
			caps = p
		}
	}
	got, err := caps.Enumerate(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatalf("Enumerate = %v, %v; want one capability", got, err)
	}
	var c api.Capability
	if err := json.Unmarshal(got[0].Data(), &c); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c.TypeSchema != "other-host:/data-type/type-probe" {
		t.Errorf("type_schema = %q, want the host's", c.TypeSchema)
	}
	if len(c.Functions) != 2 {
		t.Fatalf("functions = %+v", c.Functions)
	}
	if c.Functions[0].Action != "other-host:probe/here-function" {
		t.Errorf("host-relative action = %q, want the host's", c.Functions[0].Action)
	}
	if c.Functions[0].Description != ":not a uri" {
		t.Errorf("a non-URI string changed: %q", c.Functions[0].Description)
	}
	if c.Functions[1].Action != "ktw-elsewhere:probe/there-function" {
		t.Errorf("another binary's action = %q, want it kept", c.Functions[1].Action)
	}
}
