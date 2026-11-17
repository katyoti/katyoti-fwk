// SPDX-License-Identifier: Apache-2.0

// Package internal holds the gocode-acceptance module's private
// implementation. Importable only from gocode-acceptance/public
// + gocode-acceptance/local (via the standard Go internal/ rule);
// other modules cannot reach in.
package internal

import "errors"

// ErrSentinelTooLarge is the canonical rejection for "comically
// large" inputs the validator refuses. Exists so the test suite
// has something other than ErrOverflow to assert against.
var ErrSentinelTooLarge = errors.New("calc/internal: input exceeds sentinel limit")

// SentinelLimit caps either operand to a value chosen well below
// int64 overflow so we have a clean rejection path before the math
// gets near MaxInt64.
const SentinelLimit = int64(1_000_000_000_000)

// ValidatePair rejects pairs where either operand exceeds the
// sentinel limit. Returns nil for normal inputs (including 0 +
// negatives within range).
func ValidatePair(a, b int64) error {
	if a > SentinelLimit || a < -SentinelLimit {
		return ErrSentinelTooLarge
	}
	if b > SentinelLimit || b < -SentinelLimit {
		return ErrSentinelTooLarge
	}
	return nil
}
