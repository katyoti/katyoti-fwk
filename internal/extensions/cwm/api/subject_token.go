// SPDX-License-Identifier: Apache-2.0

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Object id → NATS subject token (decided 2026-09-17).
//
// A subject token must match [a-z0-9-]{1,63}, but an authored object id is
// module-qualified — `cwm.g1`, `compose.CMP-010` — so it carries a DOT, which
// is the NATS separator itself, and often uppercase. Such an id can never
// appear raw in a subject: the builders reject it, and a dot would not merely
// be invalid, it would split one segment into two and silently break every
// wildcard subscription.
//
// So a token is DERIVED. Two properties matter, in this order:
//
//  1. Injective. The 8-hex suffix is a hash of the FULL, untouched id, so ids
//     that flatten to the same slug — `cwm.g1` and `cwm-g1`, “ and
//     `tasks.uc1` — still produce different tokens. Stripping alone is not
//     enough: the separators that distinguish a module qualifier from a word
//     break are exactly the characters that get removed.
//  2. Readable. The slug prefix keeps a NATS trace legible, which matters in
//     the one moment anyone looks: working out why a suspended task did not
//     wake.
//
// THE RULE THAT MAKES IT WORK: publisher and subscriber must both call THIS
// function. They never exchange the token — each derives it from the id it
// already holds. Two implementations that merely agree today would drift, and
// the failure mode is a task that waits forever rather than an error.
const (
	subjectSlugMax = 40 // 40 + 1 + 8 = 49, comfortably inside the 63 limit
	subjectHashLen = 8
)

// SubjectToken derives the subject token for an object id. It is deterministic
// and always returns a valid token.
func SubjectToken(id string) string {
	sum := sha256.Sum256([]byte(id))
	h := hex.EncodeToString(sum[:])[:subjectHashLen]

	slug := subjectSlug(id)
	if slug == "" {
		return h
	}
	return slug + "-" + h
}

// subjectSlug lowercases and collapses everything outside [a-z0-9] into single
// hyphens, then caps the length. It is the READABLE half only — correctness
// comes from the hash, so this may lose information freely.
func subjectSlug(id string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(id) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
		if b.Len() >= subjectSlugMax {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}
