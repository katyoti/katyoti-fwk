// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A DELIVERY LANE MAKES NO INCREMENT: an increment comes
// from a study's hand-over (k-study --decide) - the lane offers no « New
// increment », neither a title prompt nor an agent started.
func TestADeliveryLaneMakesNoIncrement(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(api.DeliveryDisplay["StreamLane"])
	s := string(raw)
	for _, no := range []string{"new-increment", "New increment", "increments/create", "Its short title"} {
		if strings.Contains(s, no) {
			t.Errorf("the lane still offers to make an increment (%s): %s", no, s)
		}
	}
}
