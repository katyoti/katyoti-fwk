// SPDX-License-Identifier: Apache-2.0

package messagebus

// The bus checks keys: against a keyed embedded
// server - the hub's - a connection without a key is refused, a member with
// its own key is served and stamped, a tenant header that disagrees with the
// caller's key is refused, and a person's cause no person's client attested
// is refused while one relayed with its attestation is served.

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/busauth"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
	natsgo "github.com/nats-io/nats.go"
)

func natsMsg(subject string, h map[string][]string) *natsgo.Msg {
	return &natsgo.Msg{Subject: subject, Header: natsgo.Header(h), Data: nil}
}

// keyedHub is a keyed embedded server over a fresh data folder, its hub,
// CLI and member keys minted in testTenant.
type keyedHub struct {
	h       *natsembedded.EmbeddedHandle
	dataDir string
}

func startKeyedHub(t *testing.T) *keyedHub {
	t.Helper()
	dataDir := t.TempDir()
	hubKey, err := busauth.Ensure(dataDir, busauth.KindHub, busauth.KindHub, testTenant)
	if err != nil {
		t.Fatal(err)
	}
	opts := natsembedded.EmbeddedOptions(filepath.Join(dataDir, "js"), true)
	h, err := natsembedded.StartEmbeddedKeyed(opts, hubKey.Registry(), hubKey)
	if err != nil {
		t.Fatalf("StartEmbeddedKeyed: %v", err)
	}
	t.Cleanup(h.Stop)
	return &keyedHub{h: h, dataDir: dataDir}
}

// connect dials the hub as (kind, name), its key minted; kind "" dials
// without a key.
func (k *keyedHub) connect(t *testing.T, kind, name string) (*natsxport.ClientHandle, error) {
	t.Helper()
	var cred *busauth.Credential
	if kind != "" {
		var err error
		if cred, err = busauth.Ensure(k.dataDir, kind, name, testTenant); err != nil {
			t.Fatal(err)
		}
	}
	h, err := natsxport.Connect(context.Background(), k.h.Server.ClientURL(), natsxport.ConnectOptions{
		Name: "test-" + kind, Credential: cred, MaxReconnects: 0,
	})
	if err == nil {
		t.Cleanup(h.Close)
	}
	return h, err
}

// callServed calls path, again while no member has subscribed it yet (a
// subscription made on another connection reaches the server a moment
// later): the answer is the member's, never the race's.
func callServed(ctx context.Context, bus mbapi.MessageBus, tenant, path string) (json.RawMessage, error) {
	for {
		raw, err := NewSender(bus, tenant, testUniverse).Call(ctx, path, nil)
		if err == nil || !strings.Contains(err.Error(), "Slot not found") || ctx.Err() != nil {
			return raw, err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// whoEndpoint answers, for each call, its stamped caller and its cause kind.
func whoEndpoint() *ktsp.Endpoint {
	ep := ktsp.NewEndpoint("test", "", "")
	slot := ktsp.NewSlot("who", ktsp.DefaultInstance, "", nil)
	slot.Add(ktsp.RawMethod("am-i", ktsp.MethodOpts{}, func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		c, _ := busauth.CallerFrom(ctx)
		k, _ := cause.CauseFrom(ctx)
		return json.Marshal(map[string]string{"caller": c.String(), "cause": k.Kind})
	}))
	ep.AddSlot(slot)
	return ep
}

func callWho(ctx context.Context, bus mbapi.MessageBus, tenant string) (map[string]string, error) {
	raw, err := callServed(ctx, bus, tenant, testProvider+":/who/am-i")
	if err != nil {
		return nil, err
	}
	var out map[string]string
	err = json.Unmarshal(raw, &out)
	return out, err
}

func TestKeyedBus_RefusesAConnectionWithoutKey(t *testing.T) {
	k := startKeyedHub(t)
	if _, err := k.connect(t, "", ""); err == nil {
		t.Fatal("a connection without a key was accepted by the keyed hub")
	}
	if _, err := k.connect(t, busauth.KindCLI, busauth.KindCLI); err != nil {
		t.Fatalf("the CLI's key was refused: %v", err)
	}
}

func TestKeyedBus_ARefusedKeyNotMintedByTheHub(t *testing.T) {
	k := startKeyedHub(t)
	// A key minted in another data folder: not in this hub's registry.
	other, err := busauth.Ensure(t.TempDir(), busauth.KindCLI, busauth.KindCLI, testTenant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := natsxport.Connect(context.Background(), k.h.Server.ClientURL(), natsxport.ConnectOptions{
		Name: "stranger", Credential: other, MaxReconnects: 0,
	}); err == nil {
		t.Fatal("a key the hub never minted was accepted")
	}
}

// serve subscribes the who endpoint as a member with its own key.
func (k *keyedHub) serve(t *testing.T) {
	t.Helper()
	member, err := k.connect(t, busauth.KindSidecar, testProvider)
	if err != nil {
		t.Fatalf("the member's own key was refused: %v", err)
	}
	g, err := Subscribe(context.Background(), member.MessageBus(), testTenant, testUniverse, testProvider, whoEndpoint())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Unsubscribe() })
}

func TestKeyedBus_StampsTheCallerAndServesAMember(t *testing.T) {
	k := startKeyedHub(t)
	k.serve(t)
	cli, err := k.connect(t, busauth.KindCLI, busauth.KindCLI)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := callWho(ctx, cli.MessageBus(), testTenant)
	if err != nil {
		t.Fatalf("the CLI's call was not served: %v", err)
	}
	if got["caller"] != "cli:cli" {
		t.Errorf("stamped caller = %q, want cli:cli", got["caller"])
	}
}

func TestKeyedBus_RefusesADisagreeingTenantHeader(t *testing.T) {
	k := startKeyedHub(t)
	cli, err := k.connect(t, busauth.KindCLI, busauth.KindCLI)
	if err != nil {
		t.Fatal(err)
	}
	// Served on the forged tenant's subject, so the call reaches the member
	// and the refusal is the stamp's, not a missing responder.
	member, err := k.connect(t, busauth.KindSidecar, testProvider)
	if err != nil {
		t.Fatal(err)
	}
	g, err := Subscribe(context.Background(), member.MessageBus(), "forged", testUniverse, testProvider, whoEndpoint())
	if err != nil {
		t.Fatal(err)
	}
	defer g.Unsubscribe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = callWho(ctx, cli.MessageBus(), "forged")
	if err == nil || !strings.Contains(err.Error(), "tenant header") {
		t.Fatalf("a call whose tenant header disagrees with its key was not refused: %v", err)
	}
}

func TestKeyedBus_APersonsCauseNeedsAPersonsClient(t *testing.T) {
	k := startKeyedHub(t)
	k.serve(t)
	person := cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindPerson, ID: "jdoe"})
	ctx, cancel := context.WithTimeout(person, 5*time.Second)
	defer cancel()

	// A member (an agent's process holding a sidecar's key) makes up the
	// person's act: refused.
	other, err := k.connect(t, busauth.KindSidecar, "ktw-other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := callWho(ctx, other.MessageBus(), testTenant); err == nil || !strings.Contains(err.Error(), "person") {
		t.Fatalf("a member's call claiming the person's act was not refused: %v", err)
	}

	// The CLI (a person's client) says it: served, the cause the person's.
	cli, err := k.connect(t, busauth.KindCLI, busauth.KindCLI)
	if err != nil {
		t.Fatal(err)
	}
	got, err := callWho(ctx, cli.MessageBus(), testTenant)
	if err != nil {
		t.Fatalf("the CLI's person call was refused: %v", err)
	}
	if got["cause"] != cause.KindPerson {
		t.Errorf("served cause = %q, want person", got["cause"])
	}
}

func TestKeyedBus_ARelayedPersonsCauseKeepsItsAttestation(t *testing.T) {
	k := startKeyedHub(t)
	k.serve(t)
	// A relay member: serving the CLI's call, it calls the who member in
	// turn with the context it was served - its own key signs, the person's
	// attestation rides along.
	relay, err := k.connect(t, busauth.KindSidecar, "ktw-relay")
	if err != nil {
		t.Fatal(err)
	}
	ep := ktsp.NewEndpoint("relay", "", "")
	slot := ktsp.NewSlot("relay", ktsp.DefaultInstance, "", nil)
	slot.Add(ktsp.RawMethod("on", ktsp.MethodOpts{}, func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		got, err := callWho(ctx, relay.MessageBus(), testTenant)
		if err != nil {
			return nil, err
		}
		return json.Marshal(got)
	}))
	ep.AddSlot(slot)
	g, err := Subscribe(context.Background(), relay.MessageBus(), testTenant, testUniverse, "ktw-relay", ep)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Unsubscribe()

	cli, err := k.connect(t, busauth.KindCLI, busauth.KindCLI)
	if err != nil {
		t.Fatal(err)
	}
	person := cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindPerson, ID: "jdoe"})
	ctx, cancel := context.WithTimeout(person, 5*time.Second)
	defer cancel()
	raw, err := callServed(ctx, cli.MessageBus(), testTenant, "ktw-relay:/relay/on")
	if err != nil {
		t.Fatalf("the relayed person's call was refused: %v", err)
	}
	var got map[string]string
	_ = json.Unmarshal(raw, &got)
	if got["caller"] != "sidecar:ktw-relay" || got["cause"] != cause.KindPerson {
		t.Errorf("relayed call = %v, want caller sidecar:ktw-relay with the person's cause", got)
	}
}

// A message whose signed content was changed after signing is refused: the
// receiver checks the signature, not only that one is there.
func TestKeyedBus_RefusesATamperedMessage(t *testing.T) {
	k := startKeyedHub(t)
	k.serve(t)
	cred, err := busauth.Ensure(k.dataDir, busauth.KindCLI, busauth.KindCLI, testTenant)
	if err != nil {
		t.Fatal(err)
	}
	// Signed by the CLI's key, then its tenant header rewritten: sent raw
	// through the hub's own connection (its adapter would sign afresh, so the
	// bare client is used).
	subject := subjects.SlotSubject(testTenant, testUniverse, testProvider, "who", ktsp.DefaultInstance, "am-i")
	h, err := cred.Sign(subject, map[string][]string{HeaderTenantID: {testTenant}, HeaderUniverseID: {testUniverse}}, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	h[HeaderUniverseID] = []string{"other"}
	var reply *natsgo.Msg
	for deadline := time.Now().Add(5 * time.Second); ; {
		reply, err = k.h.Client.RequestMsg(natsMsg(subject, h), 5*time.Second)
		if err == nil || !errors.Is(err, natsgo.ErrNoResponders) || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond) // the member's subscription not there yet
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reply.Data), "signature does not match") {
		t.Fatalf("a tampered message was served: %s", reply.Data)
	}
}

// A connection presenting a public key of the registry without its secret
// (its nonce signed by another key) is refused.
func TestKeyedBus_RefusesAKeyPresentedWithoutItsSecret(t *testing.T) {
	k := startKeyedHub(t)
	cli, err := busauth.Ensure(k.dataDir, busauth.KindCLI, busauth.KindCLI, testTenant)
	if err != nil {
		t.Fatal(err)
	}
	thief, err := busauth.Ensure(t.TempDir(), busauth.KindCLI, busauth.KindCLI, testTenant)
	if err != nil {
		t.Fatal(err)
	}
	nc, err := natsgo.Connect(k.h.Server.ClientURL(), natsgo.Nkey(cli.PublicKey(), thief.SignNonce), natsgo.NoReconnect())
	if err == nil {
		nc.Close()
		t.Fatal("the CLI's public key signed by another key was accepted")
	}
}

// A member serving the bus from a context that carries a person's cause - as
// every binary's start gives it from the machine's profile - serves a call
// that carries none, and calls another member in turn: the onward call is
// caused by no one and served. Inheriting the serving process's person cause,
// unattested, it was refused, and the hub's ready-ping of a member it spawned
// on first use (conn-mgr ensure-spawned) never succeeded.
func TestKeyedBus_AServedCallWithoutCauseTakesNoneFromTheServer(t *testing.T) {
	k := startKeyedHub(t)
	k.serve(t)
	relay, err := k.connect(t, busauth.KindSidecar, "ktw-relay")
	if err != nil {
		t.Fatal(err)
	}
	ep := ktsp.NewEndpoint("relay", "", "")
	slot := ktsp.NewSlot("relay", ktsp.DefaultInstance, "", nil)
	slot.Add(ktsp.RawMethod("on", ktsp.MethodOpts{}, func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		got, err := callWho(ctx, relay.MessageBus(), testTenant)
		if err != nil {
			return nil, err
		}
		return json.Marshal(got)
	}))
	ep.AddSlot(slot)
	serving := cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindPerson, ID: "jdoe"})
	g, err := Subscribe(serving, relay.MessageBus(), testTenant, testUniverse, "ktw-relay", ep)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Unsubscribe()

	cli, err := k.connect(t, busauth.KindCLI, busauth.KindCLI)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := callServed(ctx, cli.MessageBus(), testTenant, "ktw-relay:/relay/on")
	if err != nil {
		t.Fatalf("a call without cause, relayed by a member serving under a person's cause, was refused: %v", err)
	}
	var got map[string]string
	_ = json.Unmarshal(raw, &got)
	if got["caller"] != "sidecar:ktw-relay" || got["cause"] != "" {
		t.Errorf("onward call = %v, want caller sidecar:ktw-relay and no cause", got)
	}
}
