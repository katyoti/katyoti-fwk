// SPDX-License-Identifier: Apache-2.0

package commands_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/commands"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
	"github.com/spf13/cobra"
)

func run(t *testing.T, ctx context.Context, cmd *cobra.Command, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("%s: %v", cmd.Use, err)
	}
	return out.String()
}

// The command line reads the method as the panels do: a `feature` serving a
// goal is counted, and method-act shapes a `feature` for an unserved goal.
func TestTheCommandsReadAndWriteFeatures(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":  {Data: []byte("name: module\ntree: node\n")},
		"object-types/goal.yaml":    {Data: []byte("name: goal\nfolder: goals\n")},
		"object-types/feature.yaml": {Data: []byte("name: feature\nfolder: features\n")},
	}))
	cwmapp.Module(t, ctx, cwm, "m")
	put := func(local, typ, title string, edges ...cwmapi.ObjectEdge) string {
		o := cwmapi.NewContainerObject("m", local, typ, "m", title, "")
		o.Edges = edges
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", local, err)
		}
		return o.ID
	}
	g := put("g1", "goal", "Served")
	put("f1", "feature", "Serve it", cwmapi.ObjectEdge{Kind: "serves", To: g})
	put("g2", "goal", "Unserved")

	out := run(t, ctx, commands.NewMethodOverviewCommand())
	if !strings.Contains(out, "features:  0 delivered / 1 total") || !strings.Contains(out, "served-by 1 features") {
		t.Errorf("the feature is not counted:\n%s", out)
	}
	if strings.Count(out, "[GAP") != 1 {
		t.Errorf("only the unserved goal is a GAP:\n%s", out)
	}

	run(t, ctx, commands.NewMethodActCommand(), "--kind", "shape-us")
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: "feature"})
	if err != nil || len(list.Objects) != 2 {
		t.Fatalf("method-act shapes a feature: %+v %v", list, err)
	}
	if out := run(t, ctx, commands.NewMethodOverviewCommand()); strings.Contains(out, "[GAP") {
		t.Errorf("after method-act no goal is a GAP:\n%s", out)
	}
}
