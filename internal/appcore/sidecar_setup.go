// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// SidecarBinaryDef describes a ktw-* sidecar binary discovered on PATH.
type SidecarBinaryDef struct {
	Name string // binary name (e.g. "ktw-docs")
	Path string // resolved filesystem path from exec.LookPath
}

// runSidecarCheck executes `{binary} check --schema` and parses the JSON output.
func runSidecarCheck(binaryPath string) (*CheckReport, error) {
	cmd := exec.Command(binaryPath, "check", "--schema")
	publish.NoConsoleWindow(cmd) // a probe opens no window, whoever runs it
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exec: %w", err)
	}
	var report CheckReport
	if err := json.Unmarshal(out, &report); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	return &report, nil
}

// RunEndpointExtensions executes `{binary} extensions --json` and parses the endpoint manifest.
// Returns nil manifest (not an error) if the command is not supported.
func RunEndpointExtensions(binaryPath string) (*EndpointManifest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// `sc extensions` (relocated from root 2026-05-23, G3 follow-up).
	cmd := exec.CommandContext(ctx, binaryPath, "sc", "extensions", "--json")
	publish.NoConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exec: %w", err)
	}
	var endpoint EndpointManifest
	if err := json.Unmarshal(out, &endpoint); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	return &endpoint, nil
}

// RunResourcesList executes `{binary} sc resources manifest` and
// parses the full `mbapi.Manifest` — provider + version + hash + slotTypes
// + slots + contributions. Used by ktw-hub's cache hydration and
// `catalog refresh` to fill the tier-1 manifest cache without keeping any
// sidecar long-running.
//
// 30-second timeout: ktw-agents in particular has a heavy bootstrap (its
// own sidecarDiscoveryExt runs at boot, recursively probing other
// sidecars — see follow-up todo `bus-agents-catalog-via-hub` to remove
// that recursion). 5 seconds was too tight; bumping to 30s covers
// pathological cold-disk + antivirus-scanning scenarios.
// includeSystem controls whether the manifest carries the binary's
// embedded `system` meta-model. The hub passes TRUE for exactly one
// canonical sidecar per universe and FALSE for the rest — every binary
// embeds the identical system set, so ingesting it from all of them
// would duplicate the 28 entries once per sidecar in the catalog.
//
// The manifest is always asked for WITHOUT the member's `local:` entries
// (MemberManifestRequest): a local contribution is binary-private and never
// crosses a process boundary, and this probe IS the boundary. Before
// 2026-09-26 the flag was left at its CLI default (true), so every member's
// `local:/service/*` reached the aggregate and duplicated across members
// (blind audit M3).
func RunResourcesList(binaryPath, contentRoot string, includeSystem bool) (*mbapi.Manifest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// `sc resources manifest` — returns the full Manifest
	// (provider/version/hash/slotTypes/contributions with bodies). The
	// hub catalog hydration needs the manifest in one round-trip,
	// including slot-type signatures and full bodies for schema +
	// edge extraction. The metadata-only `sc resources list` is
	// the operator-friendly default; this is the bulk-cache path.
	cmd := exec.CommandContext(ctx, binaryPath, memberManifestArgs(MemberManifestRequest(includeSystem))...)
	publish.NoConsoleWindow(cmd) // the hub probes every member after each build: no window each
	// A generic host has no contributions of its own — the folder it serves
	// has them. Probing it without saying which folder returns the HOST's
	// manifest, which is empty, so the member would appear to publish nothing
	// at all. Passing the declared root through the environment is what makes
	// the probe ask the right question.
	if contentRoot != "" {
		cmd.Env = append(os.Environ(), "KTW_CONTENT_ROOT="+contentRoot)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exec: %w", err)
	}
	var m mbapi.Manifest
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	return &m, nil
}

// MemberManifestRequest is the ONE request the aggregator makes of a member's
// manifest — what RunResourcesList renders as CLI flags and what the census
// asks the member's ResourceService in process, so the two cannot drift.
//
// IncludeLocal is always false: the member's binary-private entries stay in
// the member. IncludeSystem is the aggregator's choice (false for every
// probed member; the hub's own record is the one carrier of the system set).
func MemberManifestRequest(includeSystem bool) mbapi.ContributionGetManifestInput {
	no := false
	incl := includeSystem
	return mbapi.ContributionGetManifestInput{IncludeSystem: &incl, IncludeLocal: &no}
}

// memberManifestVerb is the command the member-manifest probe runs: what
// memberManifestArgs renders and what IsMemberManifestProbe recognises.
var memberManifestVerb = []string{"sc", "resources", "manifest"}

// IsMemberManifestProbe reports whether a process's arguments (os.Args[1:])
// run the member-manifest probe, whatever global flags surround the verb.
func IsMemberManifestProbe(args []string) bool {
	for i := 0; i+len(memberManifestVerb) <= len(args); i++ {
		match := true
		for j, w := range memberManifestVerb {
			if args[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

var (
	manifestProbeOnce sync.Once
	manifestProbe     bool
)

// ManifestProbeProcess reports whether THIS process was started to describe
// itself — the hub's probe, `<binary> sc resources manifest`. Such a process
// builds its contributions and slot signatures and exits: a service that
// holds an exclusive resource (the CWM workspace and its lock) must not open
// it there. The application boots before the command line is parsed, so the
// answer is read from os.Args, once.
func ManifestProbeProcess() bool {
	manifestProbeOnce.Do(func() {
		if len(os.Args) > 1 {
			manifestProbe = IsMemberManifestProbe(os.Args[1:])
		}
	})
	return manifestProbe
}

// memberManifestArgs renders MemberManifestRequest as the `sc resources
// manifest` invocation. Every toggle is passed explicitly — the CLI defaults
// both to true (a binary's SELF view is complete), and the aggregator must
// never depend on a default it does not own.
func memberManifestArgs(in mbapi.ContributionGetManifestInput) []string {
	args := append([]string(nil), memberManifestVerb...)
	if in.IncludeSystem != nil {
		args = append(args, fmt.Sprintf("--include-system=%t", *in.IncludeSystem))
	}
	if in.IncludeLocal != nil {
		args = append(args, fmt.Sprintf("--include-local=%t", *in.IncludeLocal))
	}
	return args
}

// GenerateUniverseSidecars runs `extensions --json` on all discovered
// sidecars and builds an UniverseSidecars. Used by `ktw-agents config`
// to seed/refresh the `sidecars:` block of Univers.yaml. The GUI's per-universe
// activation flow (UpdateUniverse) bypasses this — it writes spec entries
// directly without manifests; sidecarDiscoveryExt fetches them at boot.
func GenerateUniverseSidecars(sidecars []SidecarBinaryDef) *UniverseSidecars {
	reg := &UniverseSidecars{Version: 1}

	for _, sc := range sidecars {
		spec := SidecarSpec{
			Name:      sc.Name,
			Transport: "stdio",
			Binary:    sc.Path,
			Args:      []string{"sc", "ktsp", "stdio"},
		}
		endpoint, err := RunEndpointExtensions(sc.Path)
		if err != nil {
			fmt.Printf("  [skip] %s: extensions not supported (%v) — adding without manifest\n", sc.Name, err)
		} else {
			spec.Manifest = endpoint
			fmt.Printf("  [ok] %s v%s (%d slots)\n", sc.Name, endpoint.Version, len(endpoint.KTSP.Slots))
		}
		reg.Sidecars = append(reg.Sidecars, spec)
	}

	return reg
}
