// SPDX-License-Identifier: Apache-2.0

// Package toolproxy turns action URIs declared in a task's `tools:` list into
// native agent tools. Each resolved URI (`<provider>:<slot>/<method>`) becomes
// a ToolDef in the agent's catalog — its input schema + description come from
// the provider's manifest (the descriptor's slot-type contract) — and a call
// dispatches through the ActionService (KTSP method / task / script). A sidecar
// thus exposes its methods to the agent without shipping an agent plugin. This
// is the deferred "catalog-driven proxy"; it also serves node-type injected
// tools.
package toolproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

var strategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"tool", "Action-URI tools (sidecar methods/tasks as native tools)",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{FactoryName: "tool", CreateFunc: toolFactory}, nil
	},
)

// Extension contributes the tool-proxy plugin strategy. Declaring `tools:` on a
// task auto-adds a `{type:"tool", props:{tools:[...]}}` ref (see
// agentapi.DesugarPluginLists), which this factory consumes.
func Extension(init *application.ApplicationInit) {
	init.AddContribute("toolproxy", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, strategyDef)
	})
}

// permissiveSchema is the input schema used when the provider's manifest carries
// no typed schema for the method (e.g. an exact URI the descriptor doesn't list).
// The task body still names the arguments; a typed schema is strictly better
// when available.
var permissiveSchema = json.RawMessage(`{"type":"object"}`)

// minted carries everything needed to expose one resolved URI as a tool.
type minted struct {
	uri      mbapi.ToolURI
	desc     string
	schema   json.RawMessage
	timeoutS int // per-method request timeout (0 = transport default)
}

func toolFactory(ctx context.Context, _ agentapi.LoopContext, refs []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	var entries []string
	for _, r := range refs {
		entries = append(entries, agentapi.StringList(r.Props["tools"])...)
	}
	entries = trimmedUnique(entries)
	if len(entries) == 0 {
		return nil, nil
	}

	action, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil || action == nil {
		log.Printf("[toolproxy] no ActionService; tools %v dropped", entries)
		return nil, nil
	}
	// Catalog supplies typed schemas + descriptions and wildcard enumeration.
	// Optional: exact URIs still mint (with a permissive schema) without it.
	cat, _ := application.UseService[mbapi.Catalog](ctx)
	if cat == nil {
		log.Printf("[toolproxy] no Catalog service in context — sidecar tools %v will mint with permissive (untyped) schemas the LLM reads as 'no arguments'", entries)
	}

	resolved := resolveTools(ctx, cat, entries)
	if len(resolved) == 0 {
		log.Printf("[toolproxy] tools %v resolved to nothing", entries)
		return nil, nil
	}

	// Two-pass naming so distinct URIs NEVER lose a tool to a short-name
	// collision (e.g. ktw-example:example/query vs ktw-kb:kb/query both → "query").
	// Pass 1 counts short names; pass 2 keeps the short name when it is unique
	// (the common case — no churn to existing tasks / permission rules) and
	// qualifies only the colliders. See finalToolName.
	shortCount := map[string]int{}
	for _, m := range resolved {
		shortCount[toolName(m.uri)]++
	}
	slotQualCount := map[string]int{}
	for _, m := range resolved {
		if shortCount[toolName(m.uri)] > 1 {
			slotQualCount[slotQualifiedName(m.uri)]++
		}
	}

	byName := map[string]mbapi.ToolURI{}
	timeouts := map[string]int{}
	defs := make([]agentapi.ToolDef, 0, len(resolved))
	for _, m := range resolved {
		name := finalToolName(m.uri, shortCount, slotQualCount)
		if existing, dup := byName[name]; dup {
			// Same URI listed twice, or an unavoidable clash after qualifying —
			// keep first. (Distinct method collisions are resolved above.)
			log.Printf("[toolproxy] tool name %q still collides (%s vs %s); keeping first", name, existing.String(), m.uri.String())
			continue
		}
		byName[name] = m.uri
		if m.timeoutS > 0 {
			timeouts[name] = m.timeoutS
		}
		schema := m.schema
		if len(schema) == 0 {
			schema = permissiveSchema
			// Diagnostic: a contentless `{type:object}` schema means the
			// agent can't see the method's required args (e.g. weather's
			// `city`), so it calls with `{}` and the method rejects it.
			// This happens when the catalog has no typed manifest for the
			// method — rebuild the provider + ktw-hub to refresh it.
			log.Printf("[toolproxy] %s minted with PERMISSIVE schema (no typed schema in catalog manifest) — the LLM won't know its arguments", m.uri.String())
		}
		desc := m.desc
		if desc == "" {
			desc = fmt.Sprintf("Invoke %s. Sidecar action %s.", name, m.uri.String())
		}
		defs = append(defs, agentapi.ToolDef{Name: name, Description: desc, InputSchema: schema})
	}
	if len(defs) == 0 {
		return nil, nil
	}
	return []agentapi.AgentPlugin{&proxyPlugin{action: action, defs: defs, byName: byName, timeouts: timeouts}}, nil
}

// resolveTools turns `tools:` entries into concrete minted tools. Exact URIs
// mint one tool (schema/desc from the manifest if present); glob methods
// (`*`, `prefix-*`) enumerate the slot's advertised methods from the manifest.
func resolveTools(ctx context.Context, cat mbapi.Catalog, entries []string) []minted {
	manifests := map[string]*mbapi.Manifest{}
	getManifest := func(provider string) *mbapi.Manifest {
		if m, ok := manifests[provider]; ok {
			return m
		}
		var m *mbapi.Manifest
		if cat != nil {
			m, _ = cat.Get(ctx, provider)
		}
		manifests[provider] = m
		return m
	}

	seen := map[string]bool{}
	var out []minted
	add := func(m minted) {
		k := m.uri.String()
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, m)
	}

	for _, e := range entries {
		if strings.TrimSpace(e) == "*" {
			// Documented as "every advertised tool" (task-format.md) and, until
			// 2026-09-22, dropped as unparseable: ParseToolPattern wants a
			// provider and a slot. Expand it over every member the catalog holds.
			if cat == nil {
				log.Printf("[toolproxy] tools: [\"*\"] needs the catalog, which is unavailable")
				continue
			}
			all, lerr := cat.List(ctx)
			if lerr != nil {
				log.Printf("[toolproxy] tools: [\"*\"]: list catalog: %v", lerr)
				continue
			}
			for _, man := range all {
				if man == nil {
					continue
				}
				manifests[man.Provider] = man
				for i := range man.SlotTypes {
					st := &man.SlotTypes[i]
					for j := range st.Methods {
						sm := &st.Methods[j]
						if !methodAdvertised(sm) {
							continue
						}
						cu := mbapi.ToolURI{Provider: man.Provider, SlotType: corereflect.ToKebab(st.Type), Method: corereflect.ToKebab(sm.Name)}
						add(minted{uri: cu, desc: methodDesc(sm), schema: methodSchema(sm), timeoutS: methodTimeoutS(sm)})
					}
				}
			}
			continue
		}
		// ParseToolPattern (not ParseToolURI) so a glob method like `leanix/*`
		// survives parsing — ParseToolURI rejects `*` as a non-kebab char,
		// which would make the wildcard-enumeration branch below unreachable.
		u, err := mbapi.ParseToolPattern(e)
		if err != nil {
			log.Printf("[toolproxy] skip unparseable tool URI %q: %v", e, err)
			continue
		}
		man := getManifest(u.Provider)

		if !isGlob(u.Method) {
			// Exact: mint regardless (an exact name reaches even a method the
			// descriptor doesn't advertise); enrich with manifest schema/desc.
			sm := findMethod(man, u.SlotType, u.Method)
			if sm == nil {
				// Pinpoint why the typed schema/desc is missing — the tool
				// will mint with a permissive `{type:object}` schema the LLM
				// reads as "no arguments".
				switch {
				case man == nil:
					log.Printf("[toolproxy] %s: provider %q not in the agent's catalog (cat.Get returned no manifest) — is it a member of this universe + the catalog refreshed?", e, u.Provider)
				default:
					log.Printf("[toolproxy] %s: method %q/%q not found in %q's manifest slot-types — minting permissive schema", e, u.SlotType, u.Method, u.Provider)
				}
			}
			add(minted{uri: u, desc: methodDesc(sm), schema: methodSchema(sm), timeoutS: methodTimeoutS(sm)})
			continue
		}

		// Glob: enumerate the slot's advertised methods from the manifest.
		if man == nil {
			log.Printf("[toolproxy] wildcard %q needs the catalog, which is unavailable", e)
			continue
		}
		matched := 0
		for i := range man.SlotTypes {
			st := &man.SlotTypes[i]
			if corereflect.ToKebab(st.Type) != u.SlotType {
				continue
			}
			for j := range st.Methods {
				sm := &st.Methods[j]
				mk := corereflect.ToKebab(sm.Name)
				if !methodAdvertised(sm) {
					continue
				}
				if ok, _ := doublestar.Match(u.Method, mk); !ok {
					continue
				}
				cu := mbapi.ToolURI{Provider: u.Provider, SlotType: u.SlotType, Method: mk}
				add(minted{uri: cu, desc: methodDesc(sm), schema: methodSchema(sm), timeoutS: methodTimeoutS(sm)})
				matched++
			}
		}
		if matched == 0 {
			// Dump the manifest's actual slot inventory so the cause is obvious:
			// `slots=[]` ⇒ the catalog carries contributions (skills) but no KTSP
			// method registry for this provider; a non-empty list with the wrong
			// slot name ⇒ a slot/URI mismatch.
			inv := make([]string, 0, len(man.SlotTypes))
			for i := range man.SlotTypes {
				inv = append(inv, fmt.Sprintf("%s(%d)", man.SlotTypes[i].Type, len(man.SlotTypes[i].Methods)))
			}
			log.Printf("[toolproxy] wildcard %q matched no advertised methods — provider=%q manifest slots=%v contributions=%d", e, u.Provider, inv, len(man.Contributions))
		}
	}
	return out
}

// proxyPlugin is the AgentPlugin holding the minted tool defs and their URI map.
type proxyPlugin struct {
	action   actionsapi.ActionService
	defs     []agentapi.ToolDef
	byName   map[string]mbapi.ToolURI
	timeouts map[string]int // tool name → per-call request timeout in seconds (0/absent = default)
	// runs waits for a long method's run; nil resolves the TaskService from
	// the call's context.
	runs runWaiter
}

// runWaiter is what a tool call of a long method needs of the TaskService:
// the run's result (it blocks until the run ends) and its stop.
type runWaiter interface {
	Result(ctx context.Context, in taskapi.ResultRequest) (taskapi.ResultResponse, error)
	Stop(ctx context.Context, in taskapi.StopRequest) (taskapi.StopResponse, error)
}

func (p *proxyPlugin) Name() string                { return "tool" }
func (p *proxyPlugin) Hooks() agentapi.PluginHooks { return agentapi.PluginHooks{} }
func (p *proxyPlugin) Stable() bool                { return false }

// SystemMessage is empty — the tools are self-describing via their ToolDefs.
func (p *proxyPlugin) SystemMessage() string { return "" }

func (p *proxyPlugin) ToolDefs() []agentapi.ToolDef { return p.defs }

func (p *proxyPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	u, ok := p.byName[name]
	if !ok {
		return nil, fmt.Errorf("toolproxy: unknown tool %q", name)
	}
	// A method may advertise a longer-than-default request timeout (e.g. a
	// LeanIX GraphQL traversal). Set it as a ctx deadline — the NATS adapter
	// derives the bus request timeout from ctx.Deadline(), overriding its 5s
	// fallback. Only narrow the deadline; never extend one already set tighter.
	if secs := p.timeouts[name]; secs > 0 {
		want := time.Duration(secs) * time.Second
		if dl, has := ctx.Deadline(); !has || time.Until(dl) < want {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, want)
			defer cancel()
		}
	}
	out, err := p.action.Run(ctx, actionsapi.RunInput{URI: u.String(), Input: input})
	if err != nil {
		return nil, fmt.Errorf("toolproxy: run %s: %w", u.String(), err)
	}
	if len(out.Value) > 0 {
		return &agentapi.ToolCallResult{Data: json.RawMessage(out.Value)}, nil
	}
	// A method declared long answers its run and its panel at once
	// : the agent called a method, so its tool call waits for
	// that run and answers the method's value or its error -
	// the panel shows the work meanwhile.
	if out.TaskID != "" && out.Panel != "" {
		return p.awaitMethodRun(ctx, u.String(), out.TaskID)
	}
	if out.TaskID != "" {
		return agentapi.StringResult("task started: " + out.TaskID), nil
	}
	return agentapi.StringResult("ok"), nil
}

// awaitMethodRun waits for the method run runID started for the tool call of
// uri and answers the method's value; a failed or cancelled run is the call's
// error. The agent's call ended first (its context done): the run is stopped,
// never left working for no one.
func (p *proxyPlugin) awaitMethodRun(ctx context.Context, uri, runID string) (*agentapi.ToolCallResult, error) {
	runs := p.runs
	if runs == nil {
		svc, err := application.UseService[taskapi.TaskService](ctx)
		if err != nil || svc == nil {
			return nil, fmt.Errorf("toolproxy: run %s: its method run %s cannot be awaited: no TaskService", uri, runID)
		}
		runs = svc
	}
	res, err := runs.Result(ctx, taskapi.ResultRequest{RunID: runID})
	if err != nil {
		if ctx.Err() != nil {
			stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = runs.Stop(stopCtx, taskapi.StopRequest{RunID: runID})
		}
		return nil, fmt.Errorf("toolproxy: run %s: await its method run %s: %w", uri, runID, err)
	}
	if res.Error != "" {
		return nil, fmt.Errorf("toolproxy: run %s: %s", uri, res.Error)
	}
	if res.Status != "" && res.Status != string(taskapi.RunStatusCompleted) {
		return nil, fmt.Errorf("toolproxy: run %s: its method run %s ended %s", uri, runID, res.Status)
	}
	value, err := methodRunValue(res.Output)
	if err != nil {
		return nil, fmt.Errorf("toolproxy: run %s: %w", uri, err)
	}
	if len(value) == 0 {
		return agentapi.StringResult("ok"), nil
	}
	return &agentapi.ToolCallResult{Data: value}, nil
}

// methodRunValue is the method's value in a method run's output ({panel,
// value}), as the run answers it in process or as its kept result decodes.
func methodRunValue(output any) (json.RawMessage, error) {
	if output == nil {
		return nil, nil
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("a method run's output: %w", err)
	}
	var o struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, fmt.Errorf("a method run's output is no {panel, value}: %w", err)
	}
	if string(o.Value) == "null" {
		return nil, nil
	}
	return o.Value, nil
}

// --- manifest lookup helpers --------------------------------------------

// findMethod locates the SlotMethod for (slot, method) in the manifest,
// matching on the kebab form of both the slot-type name and the method name.
// Returns nil when the manifest is absent or doesn't list the method.
func findMethod(man *mbapi.Manifest, slot, method string) *mbapi.SlotMethod {
	if man == nil {
		return nil
	}
	for i := range man.SlotTypes {
		st := &man.SlotTypes[i]
		if corereflect.ToKebab(st.Type) != slot {
			continue
		}
		for j := range st.Methods {
			if corereflect.ToKebab(st.Methods[j].Name) == method {
				return &st.Methods[j]
			}
		}
	}
	return nil
}

func methodSchema(sm *mbapi.SlotMethod) json.RawMessage {
	if sm == nil {
		return nil
	}
	return sm.InputSchema
}

func methodDesc(sm *mbapi.SlotMethod) string {
	if sm == nil {
		return ""
	}
	return sm.Description
}

// methodTimeoutS returns the method's declared per-call request timeout in
// seconds (0 = transport default). Exact URIs the manifest doesn't list (sm
// == nil) get the default.
func methodTimeoutS(sm *mbapi.SlotMethod) int {
	if sm == nil {
		return 0
	}
	return sm.TimeoutS
}

// methodAdvertised reports whether a method is offered to agents for bulk
// (wildcard) selection. A method tagged "internal" is reachable only by an
// exact URI, never by `*` — this bounds what a broad wildcard mints.
func methodAdvertised(sm *mbapi.SlotMethod) bool {
	for _, t := range sm.Tags {
		// internal: never a tool. mcp=false: a method only a person calls
		// (meta.MethodDef.NoLLM — a gate confirmation) — no LLM surface
		// offers it, a wildcard selection included.
		if t == "internal" || t == "mcp=false" {
			return false
		}
	}
	return true
}

// --- small helpers ------------------------------------------------------

func toolName(u mbapi.ToolURI) string {
	m := u.Method
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	return m
}

// slotQualifiedName disambiguates a colliding short name by prefixing the slot
// (e.g. "kb-query" vs "leanix-query"). Both segments are already kebab.
func slotQualifiedName(u mbapi.ToolURI) string {
	return u.SlotType + "-" + toolName(u)
}

// providerQualifiedName is the last-resort form when even the slot-qualified
// name clashes (two providers, identical slot+method). The "ktw-" prefix is
// dropped for brevity (e.g. "kb-kb-query").
func providerQualifiedName(u mbapi.ToolURI) string {
	return strings.TrimPrefix(u.Provider, "ktw-") + "-" + slotQualifiedName(u)
}

// finalToolName returns the LLM-facing name for a proxied URI: the bare method
// when it is unique (common case — keeps names stable), else slot-qualified,
// else provider-qualified. Guarantees distinct URIs never share a name.
func finalToolName(u mbapi.ToolURI, shortCount, slotQualCount map[string]int) string {
	if shortCount[toolName(u)] <= 1 {
		return toolName(u)
	}
	if slotQualCount[slotQualifiedName(u)] <= 1 {
		return slotQualifiedName(u)
	}
	return providerQualifiedName(u)
}

func isGlob(s string) bool { return strings.ContainsAny(s, "*?[{") }

func trimmedUnique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
