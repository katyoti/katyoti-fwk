// SPDX-License-Identifier: Apache-2.0

// Package source implements CWM's source-document anchor protocol
// per concepts.md § 5.2: items derived from an external document
// snapshot the anchored region at ingest, store its SHA256, and
// can later be re-checked via `Reanchor` to detect drift.
//
// Three responsibilities split across files:
//
//   - `normalize.go` — whitespace canonicalization + hashing.
//   - `fetcher.go`   — the URI fetchers: file:// is built in and
//     reads only inside the project root the context carries;
//     others register through Service.RegisterFetcher.
//   - `section.go`   — heading-path section extractor for
//     markdown sources (other formats deferred).
//   - `source.go`    — Service: Ingest and Reanchor over one anchor
//     computation (anchorSection).
package anchor

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// normalizeText canonicalizes a chunk of source content for
// stable hashing. Two changes:
//
//  1. Collapse every run of Unicode whitespace (spaces, tabs,
//     newlines) into a single ASCII space.
//  2. Trim leading + trailing whitespace.
//
// Case is preserved (specs care about "MUST" vs "must").
func normalizeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inWS := false
	written := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !written {
				continue // skip leading whitespace
			}
			if !inWS {
				b.WriteByte(' ')
				inWS = true
			}
			continue
		}
		b.WriteRune(r)
		inWS = false
		written = true
	}
	out := b.String()
	return strings.TrimRight(out, " ")
}

// hashOf returns `sha256:<hex>` of the given text: the content hash a
// source-document carries (modules/sources/concepts.md), of the form
// `^sha256:[a-f0-9]{64}$`.
//
// Callers normally pass the result of `normalizeText` so the hash
// survives cosmetic whitespace drift in the source.
func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}
