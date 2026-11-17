// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// ktw-display answers no `sidecar` verb (a web server: run with one, it starts
// serving): its config is never driven, so doctor, configure and a solution's
// install do not wait on it.
func TestAHostBinaryHasNoConfigSurface(t *testing.T) {
	_, _, err := configSurface(mktapi.Installed{Name: "ktw-display", Kind: "sidecar", BinaryPath: "C:/store/ktw-display/v1/ktw-display.exe"})
	if err == nil || !strings.Contains(err.Error(), "no configuration") {
		t.Fatalf("configSurface(ktw-display) = %v; want it refused", err)
	}
	if bin, _, err := configSurface(mktapi.Installed{Name: "ktw-weather", Kind: "sidecar", BinaryPath: "w.exe"}); err != nil || bin != "w.exe" {
		t.Fatalf("a sidecar's surface: %q, %v", bin, err)
	}
}
