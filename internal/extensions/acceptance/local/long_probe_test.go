// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/api"
	acceptancecore "github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/internal/core"
	actionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/actions/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	verificationlocal "github.com/katyoti/katyoti-fwk/internal/extensions/verification/local"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A LONG PROBE, OVER THE BUS. The verify hook is another binary's method
// (ktw-methods:acceptance/verify): the real ActionService sends it over the bus
// — here an in-memory one whose default request timeout is cut to
// busDefault, as the NATS adapter's 5s is in production — and must wait for the
// time the method DECLARES (its TimeoutS, read from the provider's manifest),
// not the default, so a probe that runs longer than the default is still
// verified and its verdict recorded.

const (
	busTenant, busUniverse = "local", "test"
	methodsHost            = "ktw-methods"
	busDefault             = 300 * time.Millisecond
)

// overTheBus is CWM, verification and the real ActionService in one process,
// and the acceptance service published on the bus as ktw-methods publishes it
// (its def's slot and methods), its probes bounded at probeTimeout. The
// catalog holds ktw-methods' manifest as the hub's does: what the ActionService
// reads a method's declared timeout from. The criterion m.ac-slow carries probe.
func overTheBus(t *testing.T, probeTimeout time.Duration, probe string) (context.Context, cwmapi.CWMService) {
	t.Helper()
	bus := teststub.New()
	bus.SetRequestTimeout(busDefault)
	t.Cleanup(func() { _ = bus.Drain(context.Background()) })

	ctx, cwm := cwmapp.New(t,
		verificationlocal.Extension(),
		cwmapp.Pack(pack(true)),
		actionslocal.ExtensionDialing(func(context.Context) (ktsapi.Sender, error) {
			return messagebus.NewSender(bus, busTenant, busUniverse), nil
		}),
	)

	def := api.AcceptanceServiceDef
	var svc api.AcceptanceService = acceptancecore.New().WithProbeTimeout(probeTimeout)
	slot := ktslocal.BuildKTSPSlotWithTags(def.ResolvedBusName(), ktsp.DefaultInstance, def.Short, svc, nil, def.Methods)
	endpoint := ktsp.NewEndpoint(methodsHost, "acceptance host", "test")
	endpoint.AddSlot(slot)
	group, err := messagebus.Subscribe(ctx, bus, busTenant, busUniverse, methodsHost, endpoint)
	if err != nil {
		t.Fatalf("publish acceptance on the bus: %v", err)
	}
	t.Cleanup(func() { _ = group.Unsubscribe() })

	st := mbapi.SlotType{Type: def.ResolvedBusName()}
	for _, d := range slot.Defs() {
		st.Methods = append(st.Methods, mbapi.SlotMethod{Name: d.Name(), TimeoutS: d.Opts().TimeoutS})
	}
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil {
		t.Fatal(err)
	}
	mem, ok := cat.(*manifestlocal.MemoryCatalog)
	if !ok {
		t.Fatalf("the test catalog is a %T, not a memory catalog", cat)
	}
	mem.Put(methodsHost, &mbapi.Manifest{Provider: methodsHost, Version: "test", SlotTypes: []mbapi.SlotType{st}})

	for _, o := range []cwmapi.Object{
		{ID: "m", Type: "module"},
		{ID: "m.uc", Type: "use-case", Parent: "m"},
		{ID: "m.ac-slow", Type: "criterion", Parent: "m.uc"},
	} {
		o.Name = o.ID
		if o.Type == "criterion" {
			_ = o.SetCapability(cwmapi.CapCheckable, map[string]any{"kind": "command", "probe": probe})
		}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	return ctx, cwm
}

// sleepProbe is a command probe that runs about d (whole seconds) and passes —
// POSIX shell, as every probe is.
func sleepProbe(d time.Duration) string {
	return "sleep " + itoa(int(d/time.Second)) + " && echo slow probe done"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// verdictOf reads the criterion's verdict group.
func verdictOf(t *testing.T, ctx context.Context, cwm cwmapi.CWMService) map[string]any {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.verdict-ac-slow"})
	if err != nil {
		t.Fatalf("no verdict recorded for m.ac-slow: %v", err)
	}
	grp, _ := got.Object.Capability(api.CapVerdict)
	return grp
}

// TestVerifyLongProbe: a criterion whose probe runs longer than the bus's
// default request timeout is verified through the bus — the ActionService waits
// the time acceptance/verify declares — and its passing verdict is recorded.
func TestVerifyLongProbe(t *testing.T) {
	ctx, cwm := overTheBus(t, api.ProbeTimeout, sleepProbe(time.Second))
	start := time.Now()
	out, err := verificationOf(t, ctx).Verify(ctx, verificationapi.VerifyInput{ID: "m.uc"})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if took := time.Since(start); took <= busDefault {
		t.Fatalf("the probe ran %s, not longer than the bus default %s: the test proves nothing", took, busDefault)
	}
	if out.Runs != 1 || out.Passed != 1 {
		t.Fatalf("runs %d passed %d (failed %+v, unproven %+v), want the slow probe passed", out.Runs, out.Passed, out.Failed, out.Unproven)
	}
	if grp := verdictOf(t, ctx, cwm); grp["status"] != "pass" {
		t.Fatalf("verdict = %+v, want pass", grp)
	}
}

// TestVerifyLongProbeOverrunRecordsATimedOutVerdict: a probe that overruns its
// bound is stopped — and not waited for even while a process its shell started
// keeps the output open (sleep, forked by bash before the &&) — and the
// criterion FAILS with a verdict saying it timed out, not a hook error with no
// verdict.
func TestVerifyLongProbeOverrunRecordsATimedOutVerdict(t *testing.T) {
	probe := "sleep 60 && echo late"
	ctx, cwm := overTheBus(t, 2*time.Second, probe)
	start := time.Now()
	out, err := verificationOf(t, ctx).Verify(ctx, verificationapi.VerifyInput{ID: "m.uc"})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if took := time.Since(start); took > 30*time.Second {
		t.Fatalf("Verify took %s: the overrunning probe was not stopped at its 2s bound", took)
	}
	if len(out.Failed) != 1 || !strings.Contains(out.Failed[0].Evidence, "timed out after 2s") {
		t.Fatalf("failed %+v, unproven %+v; want m.ac-slow failed, timed out after 2s", out.Failed, out.Unproven)
	}
	grp := verdictOf(t, ctx, cwm)
	if ev, _ := grp["evidence"].(string); grp["status"] != "fail" || !strings.Contains(ev, "timed out after 2s") {
		t.Fatalf("verdict = %+v, want fail, timed out after 2s", grp)
	}
}
