// SPDX-License-Identifier: Apache-2.0

package solutionform

import (
	"errors"
	"testing"
)

// TestOSSBuildHasNoInteractiveForm: the published build's form answers
// ErrNotInBuild and Interactive is false, so compose-universe asks for --from-spec.
func TestOSSBuildHasNoInteractiveForm(t *testing.T) {
	if Interactive {
		t.Fatal("Interactive = true in the oss build")
	}
	if _, err := Collect([]Sidecar{{Name: "ktw-docs"}}); !errors.Is(err, ErrNotInBuild) {
		t.Fatalf("Collect = %v, want ErrNotInBuild", err)
	}
}
