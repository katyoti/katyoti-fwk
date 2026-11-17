// SPDX-License-Identifier: Apache-2.0

package remote_test

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/busclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/remote/internal/genspec"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// TestGeneratedClientIsCurrent refuses a client generated from an older
// CWMService: a method added to the interface fails the build, but one removed
// or re-typed would linger here unnoticed. Fix: go generate ./internal/extensions/cwm/remote/
func TestGeneratedClientIsCurrent(t *testing.T) {
	want, err := busclient.Generate(genspec.Spec())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(genspec.FileName)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
		t.Fatalf("%s is stale — run: go generate ./internal/extensions/cwm/remote/", genspec.FileName)
	}
}

// TestClientRoundTripsToTheHost — a consumer's CWMService is the bus client; an
// object it creates lands in the host's CWM, and the host's object comes back.
func TestClientRoundTripsToTheHost(t *testing.T) {
	ctx, host := cwmapp.Remote(t)
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		t.Fatal(err)
	}

	obj := cwmapi.NewContainerObject("m", "m", "module", "", "m", "over the bus")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj}); err != nil {
		t.Fatalf("ObjectCreate over the bus: %v", err)
	}
	if _, err := host.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m"}); err != nil {
		t.Fatalf("the object is not on the host: %v", err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m"})
	if err != nil || got.Object.DisplayTitle() != "m" {
		t.Fatalf("ObjectGet over the bus: %v %+v", err, got.Object)
	}

	// A host error crosses the bus as an error.
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.nothing"}); err == nil {
		t.Fatal("ObjectGet of a missing object over the bus: no error")
	}

	// A stream is host-local: refused, not faked.
	if _, _, err := cwm.EventsSubscribe(ctx, cwmapi.EventsSubscribeInput{}); !errors.Is(err, busclient.ErrNotOverTheBus) {
		t.Fatalf("EventsSubscribe over the bus: err = %v", err)
	}
}
