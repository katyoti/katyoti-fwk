// SPDX-License-Identifier: Apache-2.0

package signing

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// provenance.go — producing and checking a contribution's origin assertion.
//
// The mechanics live here because this package already owns key handling and
// the signature format used for distributed artifacts. Provenance over a
// contribution is that same facility applied at a finer grain: the pack asserts
// "these bytes, from this publisher", and this asserts the same for each
// contribution inside it.
//
// What is signed is decided by contribapi.IdentityDigest, not here. This
// package knows how to sign; the envelope package knows what identity means.
// Keeping that split is why the signature algorithm can change without the
// envelope changing, and why the envelope can gain a layer without this package
// knowing.

// SignContribution returns an assertion of origin over c's identity.
//
// The digest is signed, not the content — so signing a contribution whose
// layers total gigabytes costs exactly what signing a small one costs.
func SignContribution(c contribapi.Contribution, sk SecretKey, publisher string) contribapi.Provenance {
	digest := c.IdentityDigest()
	return contribapi.Provenance{
		Signature: Sign([]byte(digest), sk, "contribution "+c.ResolvedURI()),
		KeyID:     hex.EncodeToString(sk.KeyID[:]),
		Publisher: publisher,
	}
}

// ErrNoProvenance is returned by VerifyContribution for a contribution that
// carries no assertion.
//
// A distinct error, not a verification failure: unsigned and
// signed-but-invalid are different facts, and a consumer applying a policy
// needs to tell them apart. Collapsing them would make "we do not know" look
// identical to "this is forged".
var ErrNoProvenance = errors.New("contribution carries no provenance")

// VerifyContribution checks c's assertion against pk.
//
// Returns nil when the assertion is present and valid for this exact identity.
// Because identity includes every layer hash, tampering with any layer's
// content changes its hash, changes the identity, and fails this check —
// without the verifier ever reading the content.
//
// Cost is one signature check over a few hundred bytes. The hash comparison
// that ties identity to actual content is the one a consumer already performs
// when it reads a layer.
func VerifyContribution(c contribapi.Contribution, pk PublicKey) error {
	if !c.Signed() {
		return ErrNoProvenance
	}
	if err := Verify([]byte(c.IdentityDigest()), []byte(c.Provenance.Signature), pk); err != nil {
		return fmt.Errorf("provenance for %s: %w", c.ResolvedURI(), err)
	}
	return nil
}

// Assertions maps a contribution URI to its provenance, and is what a packaging
// step writes beside a pack so a host can stamp each contribution as it serves
// it.
//
// Keyed by URI rather than carried inside each content file because the
// contributions of a files-only pack do not exist as files: a host DERIVES them
// from the folder. There is nothing to stamp until they are derived, so the
// assertions travel alongside and are applied at that moment.
type Assertions map[string]contribapi.Provenance

// SignAll returns an assertion for every contribution, skipping any whose kind
// is in `skip`.
//
// Ordering is deterministic so a rebuild of unchanged content produces an
// identical result — a packaging step that produced different bytes each run
// would make every rebuild look like a change.
func SignAll(cs []contribapi.Contribution, sk SecretKey, publisher string, skip ...string) Assertions {
	skipped := make(map[string]bool, len(skip))
	for _, k := range skip {
		skipped[k] = true
	}
	out := make(Assertions, len(cs))
	uris := make([]string, 0, len(cs))
	byURI := make(map[string]contribapi.Contribution, len(cs))
	for _, c := range cs {
		if skipped[c.Type] {
			continue
		}
		uri := c.ResolvedURI()
		if _, seen := byURI[uri]; seen {
			continue
		}
		byURI[uri] = c
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	for _, uri := range uris {
		out[uri] = SignContribution(byURI[uri], sk, publisher)
	}
	return out
}

// VerifyAll checks every contribution in `cs` against pk, returning the URIs
// whose assertion failed and the URIs that carried none.
//
// Reports rather than refuses. What to DO about an unsigned or invalid
// contribution is policy — this returns the facts a policy needs.
func VerifyAll(cs []contribapi.Contribution, pk PublicKey) (invalid, unsigned []string) {
	for _, c := range cs {
		switch err := VerifyContribution(c, pk); {
		case err == nil:
		case errors.Is(err, ErrNoProvenance):
			unsigned = append(unsigned, c.ResolvedURI())
		default:
			invalid = append(invalid, c.ResolvedURI())
		}
	}
	sort.Strings(invalid)
	sort.Strings(unsigned)
	return invalid, unsigned
}
