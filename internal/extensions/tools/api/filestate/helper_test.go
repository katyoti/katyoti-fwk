// SPDX-License-Identifier: Apache-2.0

package filestate

import (
	"context"
	"testing"
)

func TestFrom_NoStoreRegistered_ReturnsNil(t *testing.T) {
	// Bare context with no application wired in: From must return nil
	// rather than panicking. This is the documented test-tolerance behavior.
	got := From(context.Background())
	if got != nil {
		t.Errorf("From(bare ctx) = %v, want nil", got)
	}
}
