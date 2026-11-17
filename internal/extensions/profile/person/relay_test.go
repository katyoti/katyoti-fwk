// SPDX-License-Identifier: Apache-2.0

package person

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
)

// The relayed act comes from KTW_RELAYED_ACT, only under an agent's session:
// a person's own command never carries one, and a malformed one is none.
func TestTheRelayedActIsReadUnderAnAgentSessionOnly(t *testing.T) {
	act := `{"person":"jdoe","confirmed":"prompt","at":"2026-10-11T16:00:00Z"}`
	env := map[string]string{"CLAUDECODE": "1", RelayedActEnv: act}
	getenv := func(k string) string { return env[k] }
	r := RelayedAct(getenv)
	if r == nil || r.Person != "jdoe" || r.Confirmed != cause.ConfirmedPrompt {
		t.Fatalf("relayed act: %+v; want jdoe confirmed at the prompt", r)
	}
	for _, bad := range []string{"", "{", `{"person":"jdoe"}`, `{"confirmed":"prompt"}`} {
		env[RelayedActEnv] = bad
		if r := RelayedAct(getenv); r != nil {
			t.Fatalf("%q read as %+v; want none", bad, r)
		}
	}
	t.Setenv("CLAUDECODE", "")
	t.Setenv(RelayedActEnv, act)
	if c, ok := cause.CauseFrom(CommandCause(context.Background())); ok && c.Relay != nil {
		t.Fatalf("a person's command carries %+v; the relay is an agent session's only", c)
	}
	t.Setenv("CLAUDECODE", "1")
	c, ok := cause.CauseFrom(CommandCause(context.Background()))
	if !ok || c.Kind != cause.KindAgent || c.Relay == nil || c.Relay.Person != "jdoe" {
		t.Fatalf("under Claude Code with a relayed act: %+v; want the agent relaying jdoe", c)
	}
}

// The one rule of the person's acts: their own call passes with nothing to
// record; an agent's passes only relaying that person's act, confirmed at the
// prompt, with relaying on; every other case is refused.
func TestAcceptRelayed(t *testing.T) {
	agent := func(r *cause.Relay) context.Context {
		return cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindAgent, ID: "claude-code", Relay: r})
	}
	prompt := &cause.Relay{Person: "jdoe", Confirmed: cause.ConfirmedPrompt, At: "2026-10-11T16:00:00Z"}
	on, off := func() bool { return true }, func() bool { return false }

	person := cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindPerson, ID: "jdoe"})
	for name, ctx := range map[string]context.Context{"no cause": context.Background(), "the person": person} {
		if r, err := acceptRelayed(ctx, "jdoe", "", "decide x", off); err != nil || r != nil {
			t.Fatalf("%s: %+v, %v; want accepted, nothing relayed", name, r, err)
		}
	}
	r, err := acceptRelayed(agent(prompt), "jdoe", "", "decide x", on)
	if err != nil || r == nil || r.Confirmed != cause.ConfirmedPrompt || r.Person != "jdoe" {
		t.Fatalf("relayed, confirmed, relaying on: %+v, %v; want the relay to record", r, err)
	}
	refused := []struct {
		name    string
		ctx     context.Context
		person  string
		enabled func() bool
		says    string
	}{
		{"no relay", agent(nil), "jdoe", on, "person's own act"},
		{"relaying off", agent(prompt), "jdoe", off, "relaying the person's acts is off"},
		{"another person's act", agent(prompt), "someone", on, "is jdoe's, not someone's"},
		{"a wrong code", agent(&cause.Relay{Person: "jdoe", Act: "decide:x", Confirmed: cause.ConfirmedTOTP, Proof: "000000"}), "jdoe", on, "does not prove this act"},
		{"no confirmation", agent(&cause.Relay{Person: "jdoe", Confirmed: "nod"}), "jdoe", on, "no confirmation"},
	}
	was := ProveCode
	ProveCode = func(person, act, code string) error {
		if person == "jdoe" && act == "decide:x" && code == "123456" {
			return nil
		}
		return errors.New("wrong or expired code")
	}
	wasPasskey := PasskeyCheck
	PasskeyCheck = func(_ context.Context, proof, act, person string) error {
		if proof == "h-1" && act == "decide:x" && person == "jdoe" {
			return nil
		}
		return errors.New("not approved on the phone yet")
	}
	t.Cleanup(func() { ProveCode, PasskeyCheck = was, wasPasskey })
	for _, c := range refused {
		r, err := acceptRelayed(c.ctx, c.person, "", "decide x", c.enabled)
		if err == nil || r != nil || !strings.Contains(err.Error(), c.says) {
			t.Fatalf("%s: %+v, %v; want refused saying %q", c.name, r, err, c.says)
		}
	}

	// A code that proves the act: accepted, the code itself never recorded.
	r, err = acceptRelayed(agent(&cause.Relay{Person: "jdoe", Act: "decide:x", Confirmed: cause.ConfirmedTOTP, Proof: "123456"}), "jdoe", "decide:x", "decide x", on)
	if err != nil || r == nil || r.Confirmed != cause.ConfirmedTOTP || r.Proof != "" || r.Act != "decide:x" {
		t.Fatalf("a code proving the act: %+v, %v; want accepted, the code not kept", r, err)
	}
	// The phone's one-tap approval, checked by the passkey service for the act.
	r, err = acceptRelayed(agent(&cause.Relay{Person: "jdoe", Act: "decide:x", Confirmed: cause.ConfirmedPasskey, Proof: "h-1"}), "jdoe", "decide:x", "decide x", on)
	if err != nil || r == nil || r.Confirmed != cause.ConfirmedPasskey || r.Proof != "" {
		t.Fatalf("a phone approval for the act: %+v, %v; want accepted, the handle not kept", r, err)
	}
	if r, err := acceptRelayed(agent(&cause.Relay{Person: "jdoe", Act: "decide:x", Confirmed: cause.ConfirmedPasskey, Proof: "h-2"}), "jdoe", "decide:x", "decide x", on); err == nil || r != nil || !strings.Contains(err.Error(), "phone approval does not prove") {
		t.Fatalf("a phone approval not given: %+v, %v; want refused", r, err)
	}
	// A confirmation covers its own act only.
	if r, err := acceptRelayed(agent(&cause.Relay{Person: "jdoe", Act: "answer:e1", Confirmed: cause.ConfirmedPrompt}), "jdoe", "decide:x", "decide x", on); err == nil || r != nil || !strings.Contains(err.Error(), "the confirmation is for") {
		t.Fatalf("another act's confirmation: %+v, %v; want refused", r, err)
	}
	// An act part of an accepted one (ForAct) passes without its proof again.
	inner := ForAct(agent(&cause.Relay{Person: "jdoe", Act: "answer:e1", Confirmed: cause.ConfirmedTOTP, Proof: "123456"}), "decide:x")
	if r, err := acceptRelayed(inner, "jdoe", "decide:x", "decide x", on); err != nil || r == nil || r.Via != "answer:e1" || r.Proof != "" {
		t.Fatalf("an act part of an accepted one: %+v, %v; want accepted via answer:e1", r, err)
	}
	// A session cannot claim one: via never comes from the environment.
	env := map[string]string{RelayedActEnv: `{"person":"jdoe","act":"decide:x","confirmed":"totp","via":"answer:e1"}`}
	if r := RelayedAct(func(k string) string { return env[k] }); r == nil || r.Via != "" {
		t.Fatalf("via read from the environment: %+v", r)
	}
}

// The relay travels with the cause: through JSON as the bus header carries it,
// and absent from every cause that relays nothing.
func TestTheRelayTravelsWithTheCause(t *testing.T) {
	c := cause.Cause{Kind: cause.KindAgent, ID: "claude-code", Relay: &cause.Relay{Person: "jdoe", Confirmed: cause.ConfirmedPrompt}}
	raw, _ := json.Marshal(c)
	var back cause.Cause
	if err := json.Unmarshal(raw, &back); err != nil || back.Relay == nil || *back.Relay != *c.Relay {
		t.Fatalf("round trip: %s -> %+v, %v", raw, back, err)
	}
	raw, _ = json.Marshal(cause.Cause{Kind: cause.KindAgent, ID: "run-1"})
	if strings.Contains(string(raw), "relay") {
		t.Fatalf("a cause relaying nothing writes %s; want no relay field", raw)
	}
	if s := c.String(); !strings.Contains(s, "relaying jdoe (prompt)") {
		t.Fatalf("String: %q; want the relay named", s)
	}
}
