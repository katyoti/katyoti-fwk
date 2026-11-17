// SPDX-License-Identifier: Apache-2.0

// Package memberversion reads the version a universe member is pinned to from
// where it is wired: its path in the versioned store. A leaf: the marketplace
// core (installing a seed) and its packaging (authoring) both ask it, and
// neither pulls the other's dependencies into a binary that only needs this
// (tools/import-discipline, pure-sidecar-footprint).
package memberversion

import (
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// Derive pins a member to the version its store path resolves to
// (SidecarSpec.PinnedVersion, which the hub reads too). Empty when the member
// is not wired from the versioned store (a dev/PATH binary).
func Derive(sc orchapi.SidecarSpec) string {
	return sc.PinnedVersion()
}
