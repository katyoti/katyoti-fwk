// SPDX-License-Identifier: Apache-2.0

// Package public is the exported API surface of the gocode-acceptance
// throwaway module. Lives only as test fixture for ktw-gocode v1
// acceptance; never imported by production code.
//
// Two functions are exported (Add, Subtract). Both delegate input
// validation to the internal/ package — illustrating the
// public/internal split that the gocode-module-decompose skill
// references.
package public

import (
	"errors"

	"github.com/katyoti/katyoti-fwk/internal/test-modules/gocode-acceptance/internal"
)

// ErrOverflow is returned when Add / Subtract result would exceed
// the int64 range. Surface via errors.Is for branching.
var ErrOverflow = errors.New("calc: overflow")

// Add returns a + b. Returns ErrOverflow when the result would
// exceed int64's range in either direction. Inputs are validated
// via the internal/ checker — a NaN-shaped input like the zero
// value is treated as valid (zero is a legitimate operand).
func Add(a, b int64) (int64, error) {
	if err := internal.ValidatePair(a, b); err != nil {
		return 0, err
	}
	if (b > 0 && a > maxInt64-b) || (b < 0 && a < minInt64-b) {
		return 0, ErrOverflow
	}
	return a + b, nil
}

// Subtract returns a - b. Returns ErrOverflow when the result
// would exceed int64's range in either direction.
func Subtract(a, b int64) (int64, error) {
	if err := internal.ValidatePair(a, b); err != nil {
		return 0, err
	}
	if (b < 0 && a > maxInt64+b) || (b > 0 && a < minInt64+b) {
		return 0, ErrOverflow
	}
	return a - b, nil
}

const (
	maxInt64 = 1<<63 - 1
	minInt64 = -1 << 63
)
