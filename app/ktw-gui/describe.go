// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
)

// version is stamped at build time (`-X main.version=…`, see
// scripts/build-cross.sh) so the marketplace catalog records the same version as
// every other shipped binary. Unstamped builds fall back to the VCS-derived
// build info via appcore.ResolveVersion.
var version = "dev"

// describeRequested reports whether this process was invoked as `describe`.
//
// ktw-gui has no cobra root — main() goes straight to wails.Run — so this is a
// deliberate exact match on the FIRST argument, mirroring how the marketplace
// indexer invokes it (`ktw-gui describe --out json`). Matching only position 0
// avoids treating e.g. `--universe describe` as a describe call.
func describeRequested(args []string) bool {
	return len(args) > 0 && args[0] == "describe"
}

// emitDescribe prints this binary's marketplace metadata as JSON to stdout.
//
// This is the ONLY thing ktw-gui may do for `describe`: it must never reach
// wails.Run, which would open a WebView window on whatever machine is building
// the catalog. Being describable headlessly is what lets ktw-gui be a
// first-class marketplace item — the control plane installs it via
// `katyoti install-infra`, which needs a catalog entry to install from.
//
// Always JSON: the indexer passes `--out json`, and a desktop binary has no
// useful human console surface. The indexer unmarshals stdout directly into
// appcore.SidecarDescription, so this must be the bare object.
func emitDescribe() {
	d := appcore.SidecarDescription{
		Name:        "ktw-gui",
		DisplayName: "ktw-gui",
		Icon:        "🖥",
		Description: "katyoti desktop client — universes, chat, workspace, marketplace",
		Version:     appcore.ResolveVersion(version),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(d)
}
