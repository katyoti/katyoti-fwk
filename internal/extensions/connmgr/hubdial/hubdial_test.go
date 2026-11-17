// SPDX-License-Identifier: Apache-2.0

package hubdial

import (
	"context"
	"testing"
)

// A universe whose hub does not run is not dialed: Dial answers no
// connection and no error, so its caller has nothing to restart.
func TestDialAnswersNilWhenNoHubRuns(t *testing.T) {
	c, err := Dial(context.Background(), t.TempDir())
	if err != nil || c != nil {
		t.Fatalf("Dial with no hub = (%v, %v); want (nil, nil)", c, err)
	}
}
