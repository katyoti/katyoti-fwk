// SPDX-License-Identifier: Apache-2.0

package remote_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/cli"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
	"github.com/spf13/cobra"
)

const askRevision = "read the object first and pass its revision (or --force to overwrite)"

// TestUpdateOverTheBusNamesItsRevision: through the KTSP method (a sidecar's
// client), an update without a revision is refused, saying what to do, while
// the same update made in process on the host still writes; force writes; a
// stale revision is a conflict; the fresh one writes.
func TestUpdateOverTheBusNamesItsRevision(t *testing.T) {
	ctx, host := cwmapp.Remote(t)
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	obj := cwmapi.NewContainerObject("m", "m", "module", "", "m", "over the bus")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj}); err != nil {
		t.Fatalf("ObjectCreate over the bus: %v", err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m"})
	if err != nil {
		t.Fatal(err)
	}

	o := got.Object
	o.Name = "blind"
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err == nil || !strings.Contains(err.Error(), askRevision) {
		t.Fatalf("an update without a revision over the bus: err = %v, want the refusal naming what to do", err)
	}
	// In process, a Go caller states its own condition: unchanged.
	if _, err := host.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatalf("the same update in process on the host: %v", err)
	}
	o.Name = "forced"
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o, Force: true}); err != nil {
		t.Fatalf("a forced update over the bus: %v", err)
	}

	got, _ = cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m"})
	stale := got.Revision
	o = got.Object
	o.Name = "written since"
	if _, err := host.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	o.Name = "over it"
	_, err = cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o, Revision: stale})
	if !errors.Is(cwmapi.Restore(err), cwmapi.ErrConflict) {
		t.Fatalf("an update with a stale revision over the bus: err = %v, want a conflict", err)
	}
	got, _ = cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m"})
	o = got.Object
	o.Name = "after reading again"
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o, Revision: got.Revision}); err != nil {
		t.Fatalf("an update with the fresh revision over the bus: %v", err)
	}
}

// TestObjectUpdateVerbNamesItsRevision: the generated `cwm object-update` verb
// refuses an update without --revision, writes with --force, and with the
// revision read.
func TestObjectUpdateVerbNamesItsRevision(t *testing.T) {
	ctx, host := cwmapp.New(t)
	obj := cwmapi.NewContainerObject("m", "m", "module", "", "m", "by the verb")
	if _, err := host.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj}); err != nil {
		t.Fatal(err)
	}
	// The container the binary generates its verbs from, holding the host's
	// service; no app name, so the verb dispatches here, never to a server.
	def := cwmapi.CWMServiceDef
	endpoint := ktsp.NewEndpoint("", "CWM", "test")
	endpoint.AddSlot(ktslocal.BuildKTSPSlotWithTags(def.ResolvedBusName(), ktsp.DefaultInstance, def.Short, host, nil, def.Methods))
	verb := func(extra ...string) (string, error) {
		got, err := host.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m"})
		if err != nil {
			t.Fatal(err)
		}
		o := got.Object
		o.Name = "renamed by the verb"
		b, _ := json.Marshal(o)
		root := &cobra.Command{Use: "ktw-test", SilenceUsage: true, SilenceErrors: true}
		cli.GenerateCLI(root, endpoint)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		args := append([]string{def.ResolvedBusName(), "object-update", "--module", "m", "--object", string(b), "--out", "json"}, extra...)
		for i, a := range args {
			if a == "{rev}" {
				args[i] = got.Revision
			}
		}
		root.SetArgs(args)
		root.SetContext(ctx)
		err = root.Execute()
		return out.String(), err
	}

	if out, err := verb(); err == nil || !strings.Contains(err.Error()+out, askRevision) {
		t.Fatalf("object-update without --revision: err = %v, out = %s; want the refusal naming what to do", err, out)
	}
	if out, err := verb("--force"); err != nil {
		t.Fatalf("object-update --force: %v %s", err, out)
	}
	if got, _ := host.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m"}); got.Object.Name != "renamed by the verb" {
		t.Fatalf("object-update --force did not write: name = %q", got.Object.Name)
	}
	if out, err := verb("--revision", "{rev}"); err != nil {
		t.Fatalf("object-update --revision <read>: %v %s", err, out)
	}
}
