// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// The GUI starts on an address: `--open <address>`
// `--open=<address>` or a katyoti:// link handed over as a bare argument;
// the address's first segment is its universe.
func TestTheGUIReadsTheAddressItIsLaunchedOn(t *testing.T) {
	for args, want := range map[string]string{
		"--open dev/object/m.F-001/links":                "dev/object/m.F-001/links",
		"--open=archi/screen/marketplace/sidecars":       "archi/screen/marketplace/sidecars",
		"katyoti://open/dev/home/mes-etudes":             "katyoti://open/dev/home/mes-etudes",
		"--universe dev":                                 "",
		"--universe dev --open dev/object/m.F-001/links": "dev/object/m.F-001/links",
	} {
		if got := openArg(strings.Fields(args)); got != want {
			t.Errorf("openArg(%q) = %q; want %q", args, got, want)
		}
	}
	for addr, want := range map[string]string{
		"dev/object/m.F-001/links":                "dev",
		"katyoti://open/archi/screen/marketplace": "archi",
		"/archi/home/":                            "archi",
		"":                                        "",
	} {
		if got := universeOfAddress(addr); got != want {
			t.Errorf("universeOfAddress(%q) = %q; want %q", addr, got, want)
		}
	}
	if a, b := singleInstanceID("dev", "1.0"), singleInstanceID("archi", "1.0"); a == b {
		t.Errorf("two universes share one window lock: %s", a)
	}
	if a, b := singleInstanceID("dev", "1.0"), singleInstanceID("dev", "1.1"); a == b {
		t.Errorf("a relaunched version meets its predecessor's lock: %s", a)
	}
}
