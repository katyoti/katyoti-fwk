// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A feature specified by an acceptance criterion under it is ready to plan:
// the criterion says what done is, with no use case.
func TestAFeatureWithACriterionIsSpecified(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := worldWith(t, fstest.MapFS{
		"object-types/acceptance-criterion.yaml": {Data: []byte("name: acceptance-criterion\nfolder: criteria\nid_prefix: AC\ncapabilities: [want, checkable]\n")},
	})
	f := want(t, ctx, cwm, "feature", "Send by mail")
	accepted(t, ctx, cwm, f)
	if err := move(ctx, cwm, f, "specified", intake); !errors.Is(err, cwmapi.ErrTransitionRefused) {
		t.Fatalf("specified with no criterion: %v, want refused", err)
	}
	ac := cwmapi.Object{Type: "acceptance-criterion", Edges: []cwmapi.ObjectEdge{{Kind: "serves", To: f}}}
	ac.Parent = f
	ac.Name = "A mail is sent for each invoice"
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: ac}); err != nil {
		t.Fatal(err)
	}
	if err := move(ctx, cwm, f, "specified", intake); err != nil {
		t.Fatalf("specified once a criterion is under it: %v", err)
	}
}
