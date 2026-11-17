// SPDX-License-Identifier: Apache-2.0

// Package solutionform is compose-universe's interactive form: the person picks
// the solution's sidecars and, optionally, a new files-only sidecar to scaffold.
// The form (huh, over Bubbletea) is private (form.go, tag !oss); the published
// build (tag oss) keeps the package's API in oss.go, where Interactive is false
// and Collect answers ErrNotInBuild, so compose-universe runs from --from-spec.
package solutionform

import "errors"

// ErrNotInBuild is what Collect answers in a build that does not carry the
// interactive form (the published build, tag oss).
var ErrNotInBuild = errors.New("the interactive form is not part of this build: run compose-universe with --from-spec <spec.yaml>")

// Sidecar is one installed sidecar the form offers: its name and, when known,
// its binary.
type Sidecar struct {
	Name   string
	Binary string
}
