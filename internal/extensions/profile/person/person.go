// SPDX-License-Identifier: Apache-2.0

// Package person makes the person using a command the CAUSE of what it
// writes: the one place a CLI's root context is stamped `{person, <profile
// handle>, 0}` (internal/core/cause), so the object events and journal
// records of the writes a command makes — in process, or over the bus
// through the ktw.meta.cause header — say who made them.
//
// The handle is read from Profile.yaml in the global config layer, the file
// the admin universe's ProfileService writes (read here, never written: a
// CLI start cannot wait on the admin hub). No profile set — the placeholder —
// is no person: the context is left as it is, and the writes carry no cause,
// as before.
//
// A command an agent's session runs is not the person's: under an agent
// (AgentSessionCause — Claude Code sets CLAUDECODE in the processes it runs),
// CommandCause makes the agent the cause, whatever the profile says, so a
// service can refuse what only the person answers (a person's to-do report,
// a person's verification). The one exception is a relayed act: the person's
// own act an agent's session relays once the person confirmed it
// (RelayedActEnv, set by the KaTyoti MCP server's act tools), accepted by
// AcceptRelayed only while the person has relaying on.
package person

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/internal/proof"
)

// Cause returns ctx carrying the person using this machine as its cause, or
// ctx as it is when no profile is set or it cannot be read.
func Cause(ctx context.Context) context.Context {
	return CauseFromDir(ctx, filepath.Join(application.Env().GlobalHomeDir(), "config"))
}

// CauseFromDir is Cause reading Profile.yaml in dir.
func CauseFromDir(ctx context.Context, dir string) context.Context {
	h := core.ReadHandle(dir)
	if h == "" {
		return ctx
	}
	return cause.WithCause(ctx, cause.Cause{Kind: cause.KindPerson, ID: h, Depth: 0})
}

// HandleIn is the handle of the person set in Profile.yaml in dir (the
// global config layer), "" when none is set (the placeholder is nobody) or
// it cannot be read.
func HandleIn(dir string) string {
	return core.ReadHandle(dir)
}

// agentSessions are the environment variables an agent's session sets in the
// processes it runs, the agent each one names and the variable holding that
// session's id.
var agentSessions = []struct{ env, agent, session string }{
	{"CLAUDECODE", "claude-code", "CLAUDE_CODE_SESSION_ID"},
}

// AgentSessionCause is the agent session a process runs under, read through
// getenv: {agent, <the agent>, 0} with the session's id when it gives one, or
// false when no agent's session runs it.
func AgentSessionCause(getenv func(string) string) (cause.Cause, bool) {
	for _, s := range agentSessions {
		if getenv(s.env) != "" {
			return cause.Cause{Kind: cause.KindAgent, ID: s.agent, Depth: 0, Session: strings.TrimSpace(getenv(s.session))}, true
		}
	}
	return cause.Cause{}, false
}

// CommandCause returns ctx carrying the cause of a command line: the agent
// session it runs under — with the person's act it relays (RelayedActEnv),
// if any — else the person using this machine (Cause). Only a command a
// person or a session types takes it — never the process serving a universe,
// whose environment says nothing of who calls it.
func CommandCause(ctx context.Context) context.Context {
	if c, ok := AgentSessionCause(os.Getenv); ok {
		c.Relay = RelayedAct(os.Getenv)
		return cause.WithCause(ctx, c)
	}
	return Cause(ctx)
}

// RelayedActEnv carries, into a command an agent's session runs, the person's
// act it relays and how the person confirmed it (JSON: cause.Relay). The
// KaTyoti MCP server's act tools set it on the program they start; its plain
// tools never pass it on.
const RelayedActEnv = "KTW_RELAYED_ACT"

// RelayedAct is the relayed act getenv carries (RelayedActEnv), nil when none
// or when it does not name a person and a confirmation.
func RelayedAct(getenv func(string) string) *cause.Relay {
	raw := strings.TrimSpace(getenv(RelayedActEnv))
	if raw == "" {
		return nil
	}
	var r cause.Relay
	if err := json.Unmarshal([]byte(raw), &r); err != nil || r.Person == "" || r.Confirmed == "" {
		return nil
	}
	r.Via = "" // only the serving code chains an act to an accepted one
	return &r
}

// AcceptRelayed is the one rule of the person's own acts (deciding, reporting
// their to-do, answering or dismissing an Inbox entry, testing or verifying by
// hand, holding, waiving): a call whose cause is not an agent is the person's
// own (nil, nil); one an agent's session makes passes only when it relays an
// act of that person, confirmed, and the person turned relaying on — then the
// relay is answered for the verb to record. Everything else is refused,
// naming the act (what).
func AcceptRelayed(ctx context.Context, person, act, what string) (*cause.Relay, error) {
	return acceptRelayed(ctx, person, act, what, RelaySwitch)
}

// ForAct is ctx for an act that is part of one accepted on it (an Inbox
// choice deciding its decision, a passing hand test verifying its
// deliverable): its relay names act, via the accepted one, and carries no
// proof - the proof was spent there. ctx as it is when it relays nothing.
func ForAct(ctx context.Context, act string) context.Context {
	c, ok := cause.CauseFrom(ctx)
	if !ok || c.Kind != cause.KindAgent || c.Relay == nil {
		return ctx
	}
	r := *c.Relay
	if r.Via == "" {
		r.Via = r.Act
	}
	r.Act, r.Proof = act, ""
	c.Relay = &r
	return cause.WithCause(ctx, c)
}

// RelaySwitch reports whether the person has relaying on: the relay section of
// Profile.yaml in the global config, read at each call. Replaced in tests
// (person/relaytest).
var RelaySwitch = func() bool {
	return core.ReadRelay(filepath.Join(application.Env().GlobalHomeDir(), "config")).Enabled
}

func acceptRelayed(ctx context.Context, person, act, what string, enabled func() bool) (*cause.Relay, error) {
	c, ok := cause.CauseFrom(ctx)
	if !ok || c.Kind != cause.KindAgent {
		return nil, nil
	}
	r := c.Relay
	switch {
	case r == nil:
		return nil, fmt.Errorf("%s is the person's own act (the Inbox, the request view, or their own command) - not an agent's session's (%s): only a relayed act the person confirmed passes, through the KaTyoti MCP server's act tools", what, c.ID)
	case person != "" && r.Person != person:
		return nil, fmt.Errorf("%s: the relayed act is %s's, not %s's (%s)", what, r.Person, person, c.ID)
	case !enabled():
		return nil, fmt.Errorf("%s: relaying the person's acts is off - the person turns it on (katyoti profile relay on) before a session relays one (%s)", what, c.ID)
	case act != "" && r.Act != act:
		return nil, fmt.Errorf("%s: the confirmation is for %q, not for %s (%s)", what, r.Act, act, c.ID)
	case r.Via != "":
		return r, nil // part of an act accepted where it was asked
	case r.Confirmed == cause.ConfirmedPrompt:
		return r, nil
	case r.Confirmed == cause.ConfirmedTOTP:
		if err := ProveCode(r.Person, r.Act, r.Proof); err != nil {
			return nil, fmt.Errorf("%s: the authenticator code does not prove this act - %v (%s)", what, err, c.ID)
		}
		proven := *r
		proven.Proof = "" // spent: the code is never recorded
		return &proven, nil
	case r.Confirmed == cause.ConfirmedPasskey:
		if err := PasskeyCheck(ctx, r.Proof, r.Act, r.Person); err != nil {
			return nil, fmt.Errorf("%s: the phone approval does not prove this act - %v (%s)", what, err, c.ID)
		}
		proven := *r
		proven.Proof = "" // spent where the page verified it
		return &proven, nil
	default:
		p, ok := proverNamed(r.Confirmed)
		if !ok {
			return nil, fmt.Errorf("%s: %q is no confirmation (%s) (%s)", what, r.Confirmed, strings.Join(confirmations(), ", "), c.ID)
		}
		if err := p.check(ctx, r.Proof, r.Act, r.Person); err != nil {
			return nil, fmt.Errorf("%s: %s does not prove this act - %v (%s)", what, p.by, err, c.ID)
		}
		proven := *r
		proven.Proof = "" // spent where the prover checked it
		return &proven, nil
	}
}

// prover is a confirmation of a relayed act served by another program,
// beyond the prompt, a code and the phone's approval: a build adds its
// provers at init (provers). check asks that program whether proof proves
// act for person, spending it - never the session's word.
type prover struct {
	confirmed string // the cause.Relay confirmation it proves
	by        string // who proves it, in a refusal
	check     func(ctx context.Context, proof, act, person string) error
}

// provers are the confirmations added to prompt, totp and passkey; none in
// the open source build.
var provers []prover

// proverNamed returns the prover of a confirmation.
func proverNamed(confirmed string) (prover, bool) {
	for _, p := range provers {
		if p.confirmed == confirmed {
			return p, true
		}
	}
	return prover{}, false
}

// confirmations are the confirmations a relayed act is accepted with.
func confirmations() []string {
	out := []string{cause.ConfirmedPrompt, cause.ConfirmedTOTP, cause.ConfirmedPasskey}
	for _, p := range provers {
		out = append(out, p.confirmed)
	}
	return out
}

// ProveCode checks an authenticator code as the person's proof of act (its
// key): once, within ±1 step, fail-closed with nothing enrolled - the store
// beside Profile.yaml in the global config, sealed for this user
// (profile/internal/proof). Replaced in tests.
var ProveCode = func(person, act, code string) error {
	return proof.New(filepath.Join(application.Env().GlobalHomeDir(), "config")).Prove(person, act, code)
}

// CodeStore is the person's authenticator enrolment, for the person's own
// commands (katyoti profile code): enrolling, confirming and revoking are
// refused from an agent's session by their callers.
func CodeStore() *proof.Store {
	return proof.New(filepath.Join(application.Env().GlobalHomeDir(), "config"))
}

// passkeyCheckURI is the passkey's check, served by the process serving the
// approval page (ktw-agents): valid once, for that act and person.
const passkeyCheckURI = "ktw-agents:passkey/check"

// PasskeyCheck asks the passkey service whether proof is an approval the
// person gave on their phone for act, spending it - never the session's word.
// Replaced in tests.
var PasskeyCheck = func(ctx context.Context, proof, act, person string) error {
	if proof == "" {
		return fmt.Errorf("no approval given")
	}
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil || actions == nil {
		return fmt.Errorf("the phone approval cannot be checked here: %v", err)
	}
	in, _ := json.Marshal(map[string]string{"proof": proof, "act": act, "person": person})
	out, err := actions.Run(ctx, actionsapi.RunInput{URI: passkeyCheckURI, Input: in})
	if err != nil {
		return fmt.Errorf("the phone approval cannot be checked: %v", err)
	}
	var got struct {
		Valid  bool   `json:"valid"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(out.Value, &got); err != nil {
		return fmt.Errorf("the passkey service's answer is unreadable: %v", err)
	}
	if !got.Valid {
		return fmt.Errorf("%s", got.Reason)
	}
	return nil
}
