// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"errors"
	"testing"
)

// TestValidatePair covers both branches: pass-through for normal
// inputs and sentinel rejection for over-limit inputs.
func TestValidatePair(t *testing.T) {
	t.Parallel()

	t.Run("zero is valid", func(t *testing.T) {
		t.Parallel()
		if err := ValidatePair(0, 0); err != nil {
			t.Errorf("ValidatePair(0, 0) = %v, want nil", err)
		}
	})

	t.Run("typical values pass", func(t *testing.T) {
		t.Parallel()
		if err := ValidatePair(42, -17); err != nil {
			t.Errorf("ValidatePair(42, -17) = %v, want nil", err)
		}
	})

	t.Run("first operand over limit rejected", func(t *testing.T) {
		t.Parallel()
		err := ValidatePair(SentinelLimit+1, 0)
		if !errors.Is(err, ErrSentinelTooLarge) {
			t.Errorf("expected ErrSentinelTooLarge; got %v", err)
		}
	})

	t.Run("second operand over limit rejected", func(t *testing.T) {
		t.Parallel()
		err := ValidatePair(0, -SentinelLimit-1)
		if !errors.Is(err, ErrSentinelTooLarge) {
			t.Errorf("expected ErrSentinelTooLarge for negative over-limit; got %v", err)
		}
	})
}
