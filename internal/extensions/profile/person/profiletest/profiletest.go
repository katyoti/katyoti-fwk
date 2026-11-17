// SPDX-License-Identifier: Apache-2.0

// Package profiletest sets, for a test, the person's profile in a config
// folder - written by the profile service's own writer, as ktw-univers writes
// it on `ktw-univers:profile/update`.
package profiletest

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/internal/core"
)

// Set writes in's profile to dir's Profile.yaml.
func Set(tb testing.TB, dir string, in api.UpdateInput) {
	tb.Helper()
	if _, err := core.New(core.Options{Dir: dir}).Update(context.Background(), in); err != nil {
		tb.Fatalf("profiletest: set the profile in %s: %v", dir, err)
	}
}
