// SPDX-License-Identifier: Apache-2.0

package contribapi

// provider_scope.go — visibility, expressed as a provider name.
//
// Two provider names are reserved, and each states a fact about how far a
// contribution travels. Visibility is therefore already carried by the
// envelope: nothing extra is stamped on a contribution to say how far it goes,
// and a query can filter on it without resolving the kind first.
//
//	system — the bootstrap metadata every binary embeds identically
//	         (kind declarations, well-known data-types, system service
//	         interfaces). Aggregation dedups these: they appear once in the
//	         universe view although every binary carries them.
//	local  — binary-private. Visible to the binary's own CLI surface,
//	         never aggregated, never over the wire.
//
// Everything else is a sidecar-named provider: shared, universe-wide.

const (
	// SystemProvider is the provider name for system-scope contributions.
	// When emitting one, always set `Provider = SystemProvider`.
	SystemProvider = "system"

	// LocalProvider is the provider name for binary-private contributions.
	//
	// Naming caveat: `local:/<kind>/<id>` URIs are NOT globally unique —
	// every binary's `local:` namespace is its own. Anywhere such a URI is
	// logged or traced across binaries, attach the binary identity separately.
	LocalProvider = "local"
)

// IsBusVisible reports whether a contribution with this provider name may
// cross a process boundary. False for `LocalProvider`, true for everything
// else — system and sidecar-named providers alike.
func IsBusVisible(provider string) bool {
	return provider != LocalProvider
}
