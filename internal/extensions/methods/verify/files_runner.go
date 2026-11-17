// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// FilesProbeRunner is the FILES-ONLY ProbeRunner — the seam implementation
// for compose / da / any files-only pack. It runs deterministic, token-free
// probes selected by the `test_ref`'s prefix:
//
//	exists:<path>      → the file/dir exists under Root
//	resolves:<uri>     → the URI resolves (via Resolve, e.g. catalog lookup)
//	manual:<note>      → ALWAYS harness-error: a human-only check has no
//	                     automatable probe, so it can never auto-pass
//
// An unrecognized prefix is a `harness-error`, never a silent pass — a probe
// that cannot run is not evidence. `go test`-style probes belong in a
// separate code ProbeRunner; this one stays files-only + offline.
type FilesProbeRunner struct {
	// Root is the base dir for `exists:` paths (the repo / workspace root).
	Root string
	// Resolve answers `resolves:<uri>` — does the URI resolve in the catalog?
	// Optional; nil means every `resolves:` probe is a harness-error.
	Resolve func(uri string) bool
}

func (r FilesProbeRunner) RunProbe(_ context.Context, testRef string) ProbeResult {
	ref := strings.TrimSpace(testRef)
	switch {
	case strings.HasPrefix(ref, "exists:"):
		rel := strings.TrimSpace(strings.TrimPrefix(ref, "exists:"))
		p := rel
		if !filepath.IsAbs(rel) {
			p = filepath.Join(r.Root, rel)
		}
		if _, err := os.Stat(p); err == nil {
			return ProbeResult{Status: "pass", Evidence: "exists: " + p}
		}
		return ProbeResult{Status: "fail", Evidence: "missing: " + p}

	case strings.HasPrefix(ref, "resolves:"):
		uri := strings.TrimSpace(strings.TrimPrefix(ref, "resolves:"))
		if r.Resolve == nil {
			return ProbeResult{Status: "harness-error", Evidence: "no resolver wired for: " + uri}
		}
		if r.Resolve(uri) {
			return ProbeResult{Status: "pass", Evidence: "resolves: " + uri}
		}
		return ProbeResult{Status: "fail", Evidence: "does not resolve: " + uri}

	case strings.HasPrefix(ref, "manual:"):
		return ProbeResult{Status: "harness-error", Evidence: "manual check (no automatable probe): " + strings.TrimPrefix(ref, "manual:")}

	default:
		return ProbeResult{Status: "harness-error", Evidence: "no files-only probe for test_ref: " + ref}
	}
}
