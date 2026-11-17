// SPDX-License-Identifier: Apache-2.0

package cwmapp

import (
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Two applications in one process keep their contributions apart: each
// resolves the type its own pack declares, and the second does not resolve
// a type only the first one's pack declares. (Every cwmapp application
// publishes as ktw-test, so state keyed by the provider name across
// applications would leak it.)
func TestTwoApplicationsKeepTheirContributionsApart(t *testing.T) {
	ctx1, cwm1 := New(t, Pack(fstest.MapFS{"object-types/verdict.yaml": {Data: []byte("name: verdict\n")}}))
	if _, err := cwm1.ObjectTypeGet(ctx1, cwmapi.ObjectTypeGetInput{Name: "verdict"}); err != nil {
		t.Fatalf("the first application does not resolve its own type: %v", err)
	}
	ctx2, cwm2 := New(t, Pack(fstest.MapFS{"object-types/memo.yaml": {Data: []byte("name: memo\n")}}))
	if _, err := cwm2.ObjectTypeGet(ctx2, cwmapi.ObjectTypeGetInput{Name: "memo"}); err != nil {
		t.Fatalf("the second application does not resolve its own type: %v", err)
	}
	if _, err := cwm2.ObjectTypeGet(ctx2, cwmapi.ObjectTypeGetInput{Name: "verdict"}); err == nil {
		t.Fatal("the second application resolves a type only the first one's pack declares")
	}
}
