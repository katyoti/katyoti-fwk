// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// A package's kind is read from its path: the extension layout's role under
// an extension, the area elsewhere.
func TestPackageKind(t *testing.T) {
	for path, want := range map[string]string{
		"internal/extensions/reviews/api":                      KindAPI,
		"internal/extensions/reviews/local":                    KindLocal,
		"internal/extensions/reviews/contrib":                  KindContrib,
		"internal/extensions/manifest/lint":                    KindLint,
		"internal/extensions/code/commands":                    KindCommands,
		"internal/extensions/cwm/remote":                       KindRemote,
		"internal/extensions/reviews/internal/core":            KindInternal,
		"internal/extensions/agent/internal/plugin/compaction": KindInternal,
		"internal/extensions/tools/grep_files":                 KindInternal,
		"./internal/extensions/reviews/api/":                   KindAPI,
		"app/ktw-agents":                                       KindMain,
		"app/ktw-agents/cmd":                                   KindCmd,
		"app/ktw-gui/internal/orchestrator":                    KindInternal,
		"internal/test/arch":                                   KindTesting,
		"internal/core/render":                                 KindCore,
		"internal/kts/local":                                   KindCore,
		"tools/gen-data-type":                                  KindTool,
		"spikes/capability":                                    KindOther,
	} {
		if got := PackageKind(path); got != want {
			t.Errorf("PackageKind(%q) = %q, want %q", path, got, want)
		}
	}
	packageKindOverrides["internal/extensions/special"] = KindPlugin
	defer delete(packageKindOverrides, "internal/extensions/special")
	if got := PackageKind("internal/extensions/special/api"); got != KindPlugin {
		t.Errorf("an override wins over the path: %q", got)
	}
}
