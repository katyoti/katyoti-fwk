// SPDX-License-Identifier: Apache-2.0

package solutionform

import api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"

// Interactive is false: the published build has no interactive form.
const Interactive = false

// Collect answers ErrNotInBuild: the solution is described with --from-spec.
func Collect([]Sidecar) (api.SolutionSpec, error) {
	return api.SolutionSpec{}, ErrNotInBuild
}
