// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestNoVerdictTypeRefusesRatherThanMisfiles: where a verdict is filed is its
// type's to say; in a universe that declares no verdict type, Verify refuses,
// naming it, and writes nothing under the criterion. On a real application:
// two applications of one process keep their packs apart, so the verdict
// type the other tests declare is not seen here
// (cwmapp.TestTwoApplicationsKeepTheirContributionsApart).
func TestNoVerdictTypeRefusesRatherThanMisfiles(t *testing.T) {
	ctx, cwm := world(t, false)
	svc, err := application.UseService[api.AcceptanceService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: "m.ac-1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Verify(ctx, api.VerifyInput{ID: "m.ac-1"})
	if err == nil || !errors.Is(err, cwmapi.ErrNotFound) || !strings.Contains(err.Error(), "verdict") {
		t.Fatalf("err = %v, want a refusal naming the missing verdict type", err)
	}
	after, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: "m.ac-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Objects) != len(before.Objects) {
		t.Fatalf("%d object(s) written under the criterion without the verdict type", len(after.Objects)-len(before.Objects))
	}
}
