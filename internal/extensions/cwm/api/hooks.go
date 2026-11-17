// SPDX-License-Identifier: Apache-2.0

package api

import "encoding/json"

// HOOKS AT NAMED POINTS. A point is a `hook-type` contribution (CWM ships its
// own; an extension may open more); what runs at a point is an action URI — a
// service an extension brings (a linter, a verifier, a bootstrap). Entries come
// from `hook` contributions, a capability, an object type, one object.

// HookPoint names a moment in an object's life at which declared behaviour runs.
type HookPoint string

const (
	// HookBeforeCreate / HookBeforeUpdate run SYNCHRONOUSLY before the write and
	// may refuse it: the action returns {accept, reason}.
	HookBeforeCreate HookPoint = "before_create"
	HookBeforeUpdate HookPoint = "before_update"
	// HookBeforeDelete runs SYNCHRONOUSLY before a delete and may refuse it.
	// A move is not a delete: it does not run.
	HookBeforeDelete HookPoint = "before_delete"
	// HookOnCreate / HookOnUpdate / HookOnDelete fire after the write. A call is
	// journaled until it ran (replayed at the next start), a failure retried
	// then shown on the object (DiagnosticHookFailed); it never undoes the write.
	HookOnCreate HookPoint = "on_create"
	HookOnUpdate HookPoint = "on_update"
	HookOnDelete HookPoint = "on_delete"
	// HookLint returns FINDINGS (rule, message, severity) and runs whenever
	// asked. A finding never counts as a verdict.
	HookLint HookPoint = "lint"
	// HookVerify returns a VERDICT per criterion when acceptance is asked. Only a
	// run produces a verdict.
	HookVerify HookPoint = "verify"
	// HookOnTransition runs when a lifecycle state changes; it may refuse.
	HookOnTransition HookPoint = "on_transition"
	// HookBootstrap runs when a project is opened, so an extension can create the
	// objects it needs (the inbox extension creates sys-inbox). It runs on the
	// project objects: an extension declares it as a `hook` contribution whose
	// filter admits the `project` type.
	HookBootstrap HookPoint = "bootstrap"
)

// HookPoints lists CWM's own points, in lifecycle order — the points CWM
// dispatches itself. They are also shipped as `hook-type` contributions; an
// extension opens a point of its own by shipping one (HookType).
var HookPoints = []HookPoint{
	HookBeforeCreate, HookBeforeUpdate, HookBeforeDelete, HookOnCreate, HookOnUpdate, HookOnDelete,
	HookLint, HookVerify, HookOnTransition, HookBootstrap,
}

// HookType is a `hook-type` contribution: a POINT at which declared behaviour
// runs — when it fires and what an answer to it is. CWM's own points are
// shipped as hook-types; an extension opens a new one (a `review`, a
// `publish`) by contributing one, and runs it with ObjectRunHooks.
type HookType struct {
	Name        HookPoint `json:"name"                  yaml:"name"                  jsonschema:"The point's name (snake_case); unique"`
	Label       string    `json:"label,omitempty"       yaml:"label,omitempty"       jsonschema:"The point as a person reads it; empty = the name"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"When it fires and what it is for"`
	Timing      string    `json:"timing"                yaml:"timing"                jsonschema:"before (synchronous; may refuse the write) | after (after the write; reported never undone) | on-demand (run when asked)"`
	Answer      string    `json:"answer"                yaml:"answer"                jsonschema:"accept ({accept reason}) | findings ({status findings}) | verdict ({status evidence verdict}) | none"`
}

// Hook timings and answers.
const (
	HookTimingBefore   = "before"
	HookTimingAfter    = "after"
	HookTimingOnDemand = "on-demand"

	HookAnswerAccept   = "accept"
	HookAnswerFindings = "findings"
	HookAnswerVerdict  = "verdict"
	HookAnswerNone     = "none"
)

// Hook is a `hook` contribution: one hook AT a point, from a pack — the filter
// says which objects it applies to (as an object-panel's does). It is the
// farthest layer of an object's hook table: hook contributions, then the
// capabilities in force, then the policies that apply (`policy:<name>`, whose
// entries are never replaced), then the type, then the object itself.
type Hook struct {
	Name        string         `json:"name"                  yaml:"name"                  jsonschema:"Unique within the provider; defaults to the file name"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"What it does"`
	Type        HookPoint      `json:"type"                  yaml:"type"                  jsonschema:"The hook-type (point) it runs at"`
	URI         string         `json:"uri"                   yaml:"uri"                   jsonschema:"Action URI run at the point"`
	When        string         `json:"when,omitempty"        yaml:"when,omitempty"        jsonschema:"el predicate over the object vocabulary"`
	Filter      ObjectFilter   `json:"filter,omitempty"      yaml:"filter,omitempty"      jsonschema:"The objects it applies to; empty applies to every object"`
	Input       map[string]any `json:"input,omitempty"       yaml:"input,omitempty"       jsonschema:"The action's input; ${...} over the object. Default: {point object prior}"`
	Async       *AsyncSpec     `json:"async,omitempty"       yaml:"async,omitempty"       jsonschema:"Makes the entry an asynchronous subscription: matched at the write and handed to the scheduler; never run by CWM. Only at a point an event type names"`
	// Provider is who contributed it, set when the catalog is read — never
	// authored, never on the wire.
	Provider string `json:"-" yaml:"-"`
}

// Source is the hook's name in a hook table.
func (h Hook) Source() string {
	if h.Provider == "" {
		return "hook:" + h.Name
	}
	return "hook:" + h.Provider + "/" + h.Name
}

// BuiltinHookTypes are CWM's own points as declarations — the floor of the
// declared set, and what the shipped `hook-types/*.yaml` files state (a test
// holds the two together).
var BuiltinHookTypes = []HookType{
	{Name: HookBeforeCreate, Timing: HookTimingBefore, Answer: HookAnswerAccept},
	{Name: HookBeforeUpdate, Timing: HookTimingBefore, Answer: HookAnswerAccept},
	{Name: HookBeforeDelete, Timing: HookTimingBefore, Answer: HookAnswerAccept},
	{Name: HookOnCreate, Timing: HookTimingAfter, Answer: HookAnswerNone},
	{Name: HookOnUpdate, Timing: HookTimingAfter, Answer: HookAnswerNone},
	{Name: HookOnDelete, Timing: HookTimingAfter, Answer: HookAnswerNone},
	{Name: HookLint, Timing: HookTimingOnDemand, Answer: HookAnswerFindings},
	{Name: HookVerify, Timing: HookTimingOnDemand, Answer: HookAnswerVerdict},
	{Name: HookOnTransition, Timing: HookTimingBefore, Answer: HookAnswerAccept},
	{Name: HookBootstrap, Timing: HookTimingOnDemand, Answer: HookAnswerNone},
}

// IsHookPoint reports one of CWM's own points. Whether a point is DECLARED
// (CWM's or an extension's hook-type) is the service's to answer.
func IsHookPoint(p HookPoint) bool {
	for _, q := range HookPoints {
		if q == p {
			return true
		}
	}
	return false
}

// HookEntry is one declaration at one point: the action to run, and when it
// applies. The same shape everywhere a contributor declares.
type HookEntry struct {
	// URI is the action run at the point (a method, a script, a task).
	URI string `json:"uri" yaml:"uri" jsonschema:"Action URI run at the point; required"`
	// When is an el predicate over the object vocabulary. The literal "false"
	// on a CLOSER declaration removes a farther entry with the same URI.
	When string `json:"when,omitempty" yaml:"when,omitempty" jsonschema:"el predicate over the object vocabulary; \"false\" removes a farther entry of the same uri"`
	// Filter narrows the objects the entry applies to — the ObjectFilter panels,
	// `hook` contributions and agent-contexts use.
	Filter *ObjectFilter `json:"filter,omitempty" yaml:"filter,omitempty" jsonschema:"Objects the entry applies to"`
	// Input, when declared, IS what the action receives — each value's ${...}
	// resolved over the object vocabulary — instead of {point, object, prior}.
	// That is how an existing service (a linter taking {path}) serves a point
	// with no adapter: `input: {path: "${current.file}"}`.
	Input map[string]any `json:"input,omitempty" yaml:"input,omitempty" jsonschema:"The action's input; ${...} over the object. Default: {point, object, prior}"`
	// Async, when set, makes the entry an asynchronous SUBSCRIPTION
	// (subscriptions.go): at the write CWM records a match and hands it over;
	// it never dispatches the entry itself. Accepted only at a point an event
	// type names (on_create / on_update / on_delete, or a pack's own).
	Async *AsyncSpec `json:"async,omitempty" yaml:"async,omitempty" jsonschema:"Makes the entry an asynchronous subscription: matched at the write and handed to the scheduler"`
	// On, on a MODULE object's entry, makes it a CROSS-MODULE subscription
	// (module_subscriptions.go): the subscriber is the module object that
	// declares it, the events are those of type On.Event raised or written in
	// module On.Module. It sits at the point its event type names and is
	// async; another module's event type must be published. Ignored anywhere
	// but on a module object.
	On *EventSource `json:"on,omitempty" yaml:"on,omitempty" jsonschema:"On a module object: the module and the event type subscribed to (a published one unless the module is the subscriber)"`
	// Writes, beside On, is the modules the subscription's reactions may
	// write; empty = the subscriber's only. CWM refuses a reaction's write
	// into any other module.
	Writes []string `json:"writes,omitempty" yaml:"writes,omitempty" jsonschema:"With on: the modules the reactions may write; default the subscriber's own"`
}

// EventSource names the events a cross-module subscription listens to: the
// module that emits them and their event type.
type EventSource struct {
	Module string `json:"module" yaml:"module" jsonschema:"The emitting module; required"`
	Event  string `json:"event"  yaml:"event"  jsonschema:"The event type (an event-type name); required"`
}

// Hooks is a contributor's declarations, by point.
type Hooks map[HookPoint][]HookEntry

// HookCall is what a hook's action receives when its entry declares no
// `input`: the point, the object as it is (or is about to be) written and,
// for an update, a delete or a transition, the object as it was; at a point
// that fires on an event (an on_* write, a raised event), the event. A service
// serving a point takes it as its method's input.
type HookCall struct {
	Point  HookPoint     `json:"point"            jsonschema:"The point the hook runs at"`
	Object *Object       `json:"object,omitempty" jsonschema:"The object as it is or is about to be written"`
	Prior  *Object       `json:"prior,omitempty"  jsonschema:"The object as it was (update delete transition)"`
	Event  *MatchedEvent `json:"event,omitempty"  jsonschema:"The event the point fires on (a write's or a raised one)"`
}

// HookAccept is the answer at a point whose answer is `accept` (the before_*
// points, on_transition): Accept must be true for the write to go ahead — an
// action that answers nothing has not accepted. A gate's `uri:` requirement
// answers the same shape.
type HookAccept struct {
	Accept bool   `json:"accept"           jsonschema:"True lets the write go ahead"`
	Reason string `json:"reason,omitempty" jsonschema:"Why it refuses"`
}

// ResolvedHook is one entry as it applies to one object.
type ResolvedHook struct {
	Point  HookPoint `json:"point"`
	URI    string    `json:"uri"`
	When   string    `json:"when,omitempty"`
	Source string    `json:"source" jsonschema:"Who declared it: hook:<provider>/<name> | capability:<id> | policy:<name> | type:<name> | object"`
	// Provider is the binary that contributed the entry (empty for the
	// object's own): a refusal names it, and a gate entry applies only where
	// it owns the object (ScopeOf).
	Provider string `json:"provider,omitempty" jsonschema:"The binary that contributed the entry; empty for the object's own"`
	// Input is the entry's declared input as authored; ResolvedInput is the
	// same input resolved for THIS object — what the action receives — so a
	// caller running the hook itself sends it as is.
	Input         map[string]any `json:"input,omitempty"`
	ResolvedInput map[string]any `json:"resolved_input,omitempty"`
	// Applies is false when the entry's filter or when does not hold for this
	// object, or when it is a gate entry out of its provider's scope; Reason
	// says why (and, out of scope, the grant to add). A removed entry is not
	// listed at all.
	Applies bool   `json:"applies"`
	Reason  string `json:"reason,omitempty"`
	// Async is the entry's subscription spec, its defaults filled; Subscription
	// its id (<source>#<point>#<uri>). Absent on an entry CWM runs itself.
	Async        *AsyncSpec `json:"async,omitempty"`
	Subscription string     `json:"subscription,omitempty"`
	// On and Writes are a cross-module subscription's (source
	// module:<subscriber>): what it listens to, and the modules its
	// reactions may write (defaults filled: the subscriber's).
	On     *EventSource `json:"on,omitempty"`
	Writes []string     `json:"writes,omitempty"`
}

// ObjectHooksInput asks which hooks apply to one object.
type ObjectHooksInput struct {
	ID string `json:"id" jsonschema:"Object id; required"`
	// Point narrows to one point; empty = every point.
	Point HookPoint `json:"point,omitempty" jsonschema:"One point; empty = every point"`
}

// ObjectHooksOutput is the resolved table, farthest declarations first.
type ObjectHooksOutput struct {
	Hooks []ResolvedHook `json:"hooks"`
}

// ObjectRunHooksInput runs one on-demand point — CWM's (lint, verify,
// bootstrap) or one an extension declared — over an object, its subtree, or a
// module's objects. Exactly one of ID and Module. A before / after point runs
// on the writes and is refused here.
type ObjectRunHooksInput struct {
	Point   HookPoint `json:"point"             jsonschema:"The hook-type to run; required"`
	ID      string    `json:"id,omitempty"      jsonschema:"One object (with Subtree: and everything under it)"`
	Module  string    `json:"module,omitempty"  jsonschema:"Every object of a module"`
	Subtree bool      `json:"subtree,omitempty" jsonschema:"With id: the object and every descendant"`
}

// HookRun is one hook's answer about one object, raw: interpreting it is the
// point's business (the verification service reads lint findings and verify
// statuses).
type HookRun struct {
	ObjectID string          `json:"object_id"`
	URI      string          `json:"uri"`
	Source   string          `json:"source" jsonschema:"Who declared the hook"`
	Answer   json.RawMessage `json:"answer,omitempty" jsonschema:"What the action answered"`
	Error    string          `json:"error,omitempty"  jsonschema:"Why it could not run (no action service; the action failed)"`
}

// ObjectRunHooksOutput is every run, in object then hook order.
type ObjectRunHooksOutput struct {
	Point   HookPoint `json:"point"`
	Timing  string    `json:"timing"`
	Answer  string    `json:"answer" jsonschema:"What an answer at this point is: accept | findings | verdict | none"`
	Objects int       `json:"objects" jsonschema:"Objects that had a hook at the point"`
	Runs    []HookRun `json:"runs"`
}
