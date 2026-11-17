// SPDX-License-Identifier: Apache-2.0

package public

import (
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/test-modules/gocode-acceptance/internal"
)

// TestAdd_HappyPath covers the documented Add behavior — including
// the zero / negative / typical cases.
func TestAdd_HappyPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a, b int64
		want int64
	}{
		{"two positives", 3, 4, 7},
		{"positive + negative", 10, -3, 7},
		{"two negatives", -5, -2, -7},
		{"zero + value", 0, 42, 42},
		{"large within sentinel", 999_999_999_999, 1, 1_000_000_000_000},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := Add(c.a, c.b)
			if err != nil {
				t.Fatalf("Add(%d, %d) unexpected error: %v", c.a, c.b, err)
			}
			if got != c.want {
				t.Errorf("Add(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

// TestAdd_SentinelRejection asserts the public surface delegates
// to internal.ValidatePair and surfaces ErrSentinelTooLarge for
// operands exceeding the sentinel limit.
func TestAdd_SentinelRejection(t *testing.T) {
	t.Parallel()
	_, err := Add(internal.SentinelLimit+1, 0)
	if !errors.Is(err, internal.ErrSentinelTooLarge) {
		t.Errorf("expected ErrSentinelTooLarge for over-limit input; got %v", err)
	}
}

// TestSubtract_HappyPath mirrors Add's happy-path table for the
// subtraction surface.
func TestSubtract_HappyPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a, b int64
		want int64
	}{
		{"positive - positive", 10, 3, 7},
		{"positive - negative", 5, -2, 7},
		{"zero - value", 0, 42, -42},
		{"value - zero", 42, 0, 42},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := Subtract(c.a, c.b)
			if err != nil {
				t.Fatalf("Subtract(%d, %d) unexpected error: %v", c.a, c.b, err)
			}
			if got != c.want {
				t.Errorf("Subtract(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}
