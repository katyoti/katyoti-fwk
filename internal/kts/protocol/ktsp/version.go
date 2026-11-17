// SPDX-License-Identifier: Apache-2.0

package ktsp

import "fmt"

// KTSPProtocolVersion is the KTSP wire-protocol MAJOR this build speaks. It is
// stamped into every ContainerInfo (a sidecar's descriptor) and negotiated at
// the hub's first contact with a remote container.
//
// Additive-only within a major: the JSON-RPC 2.0 envelope is frozen, and the
// descriptor schema + subject grammar only GAIN fields within a major (unknown
// fields are ignored). A breaking change to any of those bumps this. The stable
// control plane (latest katyoti + ktw-hub + ktw-gui driving any universe) relies
// on this — see modules/katyoti/layout.md §5.
const KTSPProtocolVersion = 1

// KTSPProtocolMinSupported is the oldest protocol major this build accepts from
// a peer. The control plane supports the range [KTSPProtocolMinSupported ..
// KTSPProtocolVersion]; anything outside is refused (fail-closed).
const KTSPProtocolMinSupported = 1

// NegotiateProtocol checks a peer's advertised protocol version against this
// build's supported range. A zero version (a pre-versioning sidecar whose
// descriptor carries no stamp) is treated as the baseline major (1), which
// today's sidecars are compatible with. It returns a clear error when the peer
// is outside [KTSPProtocolMinSupported .. KTSPProtocolVersion] — fail-closed, so
// a mismatch surfaces at connect time, never as a silent runtime break.
func NegotiateProtocol(peer int) error {
	if peer == 0 {
		peer = 1 // baseline for descriptors without a stamped protocol version
	}
	if peer < KTSPProtocolMinSupported || peer > KTSPProtocolVersion {
		return fmt.Errorf("KTSP protocol v%d is out of range: this build supports [%d..%d] — update the out-of-range component (hub/sidecar)",
			peer, KTSPProtocolMinSupported, KTSPProtocolVersion)
	}
	return nil
}
