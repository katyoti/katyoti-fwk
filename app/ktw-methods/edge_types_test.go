// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	occontrib "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/contrib"
	panecontrib "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// TestATestCaseVerifiesAUseCase: the `verifies` edge this pack ships is
// writable as the method uses it — from a test-case to the use-case it
// exercises — on a real CWM holding the shipped types, validated as
// production validates (error mode). A source capability the test-case type
// does not carry would refuse every such edge.
func TestATestCaseVerifiesAUseCase(t *testing.T) {
	ctx, cwm := cwmapp.NewStrict(t,
		occontrib.Pack(""),
		panecontrib.Pack(""),
		cwmapp.Pack(os.DirFS(filepath.Join("..", "ktw-agents", "embed", "cwm"))),
		cwmapp.Pack(os.DirFS(filepath.Join("embed", "cwm"))),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	create := func(o cwmapi.Object) {
		t.Helper()
		o.Name = o.ID
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	create(cwmapi.Object{ID: "m.uc-1", Type: "use-case", Parent: "m"})
	create(cwmapi.Object{ID: "m.tc-1", Type: "test-case", Parent: "m",
		Edges: []cwmapi.ObjectEdge{{Kind: "verifies", To: "m.uc-1"}}})

	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.tc-1"})
	if err != nil {
		t.Fatalf("get test-case: %v", err)
	}
	if len(got.Object.Edges) != 1 || got.Object.Edges[0].Kind != "verifies" || got.Object.Edges[0].To != "m.uc-1" {
		t.Fatalf("test-case edges = %+v; want one verifies -> m.uc-1", got.Object.Edges)
	}
}
