// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
)

// epoch.go — a provider's identity as an epoch.
//
// An IMMUTABLE provider's source is fixed for the process lifetime, so the
// enumeration it serves has one version, and that version has a name: the
// digest of the enumeration. Two binaries serving the same bytes under the same
// URIs carry the same digest; a rebuild that changes one byte of one layer
// carries a different one. A MUTABLE provider carries no digest: its source can
// change under it, so there is no single version to name — its consumers
// re-read it by the stat and content digests the tier-1 cache keeps.
//
// The rule the census and providertest hold: immutable ⇒ digest present,
// mutable ⇒ digest absent.

// DigestOf returns the enumeration digest of items: "sha256:" + hex(sha256)
// over the sorted lines `<ResolvedURI> NUL <layer name> NUL <layer hash>` of
// every layer of every entry. Metadata is not part of it: it is derived from
// the layers or authored beside them, and the layers are the content.
//
// An empty enumeration has a digest too (the digest of nothing): an immutable
// provider that publishes nothing still names the version it serves.
func DigestOf(items []Contribution) string {
	lines := make([]string, 0, len(items))
	for _, c := range items {
		uri := c.ResolvedURI()
		if len(c.Layers) == 0 {
			lines = append(lines, uri)
			continue
		}
		for name, l := range c.Layers {
			lines = append(lines, uri+"\x00"+name+"\x00"+l.Hash)
		}
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// EnumerationDigest enumerates p and returns the digest of what it served.
func EnumerationDigest(ctx context.Context, p ContributionProvider) (string, error) {
	items, err := p.Enumerate(ctx)
	if err != nil {
		return "", fmt.Errorf("digest %s/%s: %w", p.Provider(), p.Type(), err)
	}
	return DigestOf(items), nil
}

// ImmutableEpoch memoises an immutable provider's digest. Embed one (or hold
// one) per provider; the zero value is ready.
//
// Two ways in, because providers differ in what their Enumerate needs:
//
//   - Record: the provider's Enumerate stamps the digest of what it just
//     produced. Required for a provider whose Enumerate reads the application
//     context (Identity has no context to hand it). Every enumeration of an
//     immutable source is the same, so the latest record is the answer.
//   - Identity: answers from the record, else computes EnumerationDigest once
//     with a background context — for a provider whose Enumerate needs none.
//
// The manifest builder reads Identity AFTER Enumerate, so a recording provider
// is always stamped when it is asked.
type ImmutableEpoch struct {
	mu     sync.Mutex
	digest string
}

// Record stamps the digest of items and returns items, so an Enumerate can end
// with `return p.epoch.Record(out), nil`.
func (e *ImmutableEpoch) Record(items []Contribution) []Contribution {
	d := DigestOf(items)
	e.mu.Lock()
	e.digest = d
	e.mu.Unlock()
	return items
}

// Identity returns Identity{Digest: <the epoch>} for the immutable provider p.
// An enumeration error leaves the digest empty (and unmemoised), which the
// census refuses: a provider that cannot enumerate cannot name its version.
func (e *ImmutableEpoch) Identity(p ContributionProvider) Identity {
	e.mu.Lock()
	d := e.digest
	e.mu.Unlock()
	if d != "" {
		return Identity{Digest: d}
	}
	d, err := EnumerationDigest(context.Background(), p)
	if err != nil {
		return Identity{}
	}
	e.mu.Lock()
	if e.digest == "" {
		e.digest = d
	}
	d = e.digest
	e.mu.Unlock()
	return Identity{Digest: d}
}

// IdentityRule reports how id breaks the identity rule, or "" when it holds:
// an immutable source names its version (a digest), a mutable one does not
// (there is no single version to name).
func IdentityRule(id Identity) string {
	switch {
	case !id.Mutable && id.Digest == "":
		return "immutable provider carries no digest (its enumeration epoch)"
	case id.Mutable && id.Digest != "":
		return "mutable provider carries a digest (a mutable source has no single version)"
	default:
		return ""
	}
}
