// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// A request with no step yet draws an empty stepper, never "steps": null —
// the GUI maps over it and a null breaks the whole Requests tab.
func TestStepsOfARequestWithoutStepsAreAnEmptyList(t *testing.T) {
	for name, n := range map[string]node{"dots": stepDots(nil)} {
		b, err := json.Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "null") {
			t.Errorf("%s: %s carries a null", name, b)
		}
	}
}
