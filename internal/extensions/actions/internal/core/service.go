// SPDX-License-Identifier: Apache-2.0

// Package core implements the ActionService dispatcher.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	schemaparser "github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	scriptsapi "github.com/katyoti/katyoti-fwk/internal/extensions/scripts/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// Config holds wire-format configuration. Empty for now — the
// dispatcher resolves its runner dependencies lazily via the captured
// `*Application` reference.
type Config struct{}

// Service implements actionsapi.ActionService as a thin dispatcher.
type Service struct {
	// app is captured at factory construction time. KTSP handlers
	// receive a bare `context.Background()` (per the MessageBus
	// adapter contract), so the per-request ctx can't be used with
	// `application.UseService`. Holding the `*Application`
	// reference lets us resolve runners via `AppUseService` at
	// handler time regardless of how the request arrived.
	// cwm-v2-providers US-15 smoke fix (2026-06-08).
	app *application.Application

	// Lazily-built bus sender for the KTSP method-call slot dispatch
	// (any URI that isn't tasks/scripts). Connected on first use via
	// hubclient ModeReuse; held for the Service's (singleton) lifetime —
	// the bus connection is shared infra, not a per-call resource.
	mu     sync.Mutex
	sender ktsp.Sender

	// dial builds the bus sender; nil = connect to the running hub. A test
	// sets it to prove a call never reaches the bus.
	dial func(context.Context) (ktsp.Sender, error)
}

// New constructs the dispatcher. The factory ctx carries the
// Application (PublishService wires it that way); we capture the
// `*Application` reference for use at handler time. cfg is reserved
// for future tuning (per-slot timeouts, default human-gate policy) —
// unused in v1.
//
// A nil app is tolerated at construction (some test contexts don't
// wire one) — Run / Describe surface a clean error if invoked
// without an Application to resolve runners from.
func New(ctx context.Context, _ *Config) (actionsapi.ActionService, error) {
	return &Service{app: application.AppFromContext(ctx)}, nil
}

// NewDialing is New with the bus sender built by dial instead of a connection
// to the running hub: a process that already holds a bus (a test's in-memory
// one) — every other step of a dispatch, the method's declared timeout among
// them, is the same.
func NewDialing(ctx context.Context, dial func(context.Context) (ktsp.Sender, error)) actionsapi.ActionService {
	return &Service{app: application.AppFromContext(ctx), dial: dial}
}

// Describe parses the action URI and forwards to the slot-appropriate
// inspector. Errors out for unknown slots.
func (s *Service) Describe(ctx context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	uri, err := mbapi.ParseToolURI(in.URI)
	if err != nil {
		return actionsapi.DescribeOutput{}, usermsg.Wrap(fmt.Errorf("ActionService.Describe: %w", err), "actions.invalidURI", map[string]any{"uri": in.URI})
	}
	var out actionsapi.DescribeOutput
	switch uri.SlotType {
	case "tasks":
		out, err = s.describeTask(ctx, uri)
	case "scripts":
		out, err = s.describeScript(ctx, uri)
	case "mutations":
		out, err = s.describeMutation(ctx, uri)
	default:
		out, err = s.describeMethod(ctx, uri)
	}
	if err == nil && out.InputSchemaAST == nil && len(out.InputSchema) > 0 {
		// An input that travels only as JSON Schema (a method's, a
		// mutation's) is read back into the AST a form renders.
		if ast, aerr := schemaapi.FromJSONSchema(out.InputSchema, inputTypeName(uri.Method)); aerr == nil {
			out.InputSchemaAST, _ = json.Marshal(ast)
		}
	}
	return out, err
}

// inputTypeName names the root type of an action's input: `file-request` →
// FileRequestInput.
func inputTypeName(method string) string {
	var b strings.Builder
	for _, part := range strings.FieldsFunc(method, func(r rune) bool { return r == '-' || r == '/' || r == '_' }) {
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String() + "Input"
}

// Run parses the action URI and forwards to the slot-appropriate
// runner.
func (s *Service) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	uri, err := mbapi.ParseToolURI(in.URI)
	if err != nil {
		return actionsapi.RunOutput{}, usermsg.Wrap(fmt.Errorf("ActionService.Run: %w", err), "actions.invalidURI", map[string]any{"uri": in.URI})
	}
	var out actionsapi.RunOutput
	var runErr error
	switch uri.SlotType {
	case "tasks":
		out, runErr = s.runTask(ctx, uri, in.Input, in.Context)
	case "scripts":
		out, runErr = s.runScript(ctx, uri, in.Input)
	case "mutations":
		out, runErr = s.runMutation(ctx, uri, in.Input)
	default:
		// A method declared long runs as a method run with its panel,
		// answering both at once - unless this IS that run
		// calling it (its context names the panel).
		if s.long(ctx, uri) && in.Context["panel"] == nil {
			out, runErr = s.startMethodRun(ctx, uri, in.Input, in.Context)
			break
		}
		out, runErr = s.runMethod(ctx, uri, in.Input)
		// A method that only reads changes nothing worth recording: checking
		// (a guide recipe's check) must not grow an object's history.
		if s.readOnly(ctx, uri) {
			return out, runErr
		}
	}
	s.recordInvocation(ctx, uri, in.Input, out, runErr)
	return out, runErr
}

// --- tasks slot ---------------------------------------------------

func (s *Service) describeTask(ctx context.Context, uri mbapi.ToolURI) (actionsapi.DescribeOutput, error) {
	if s.app == nil {
		return actionsapi.DescribeOutput{}, fmt.Errorf("ActionService.Describe: no Application available (factory was constructed without one)")
	}
	svc, err := application.AppUseService[taskapi.TaskService](s.app)
	if err != nil {
		return actionsapi.DescribeOutput{}, fmt.Errorf("resolve TaskService: %w", err)
	}
	// Task name = the URI's method segment as-is. Multi-segment
	// methods carry the `/` separator through to TaskService.Get,
	// which expects a single canonical name string today. If/when
	// TaskService gains a hierarchical lookup, ActionService passes
	// it the same string.
	info, err := svc.Get(ctx, taskapi.GetTaskRequest{Name: uri.Method})
	if err != nil {
		return actionsapi.DescribeOutput{}, fmt.Errorf("TaskService.Get(%q): %w", uri.Method, err)
	}
	// A task declares its I/O in the DSL; `DescribeOutput` promises JSON
	// Schema, because its readers are launch-form renderers and agents. The
	// projection happens here, at the edge, so the stored contract stays in
	// one language. A GUI that wants the DSL's `ui-*` extensions reads the
	// data-type URI instead, where the AST survives intact.
	input, inputAST := s.launchFormSchema(ctx, info.Name, info.InputSchema)
	return actionsapi.DescribeOutput{
		URI:            uri.String(),
		Label:          info.Name,
		Description:    info.Description,
		Details:        info.Body,
		InputSchema:    input,
		InputSchemaAST: inputAST,
		OutputSchema:   schemaparser.JSONSchemaOrNil(ctx, info.OutputSchema),
		IsAsync:        true, // tasks always async (return a task_id)
	}, nil
}

// launchFormSchema is a task's input schema as the launch form renders it:
// the fields declaring an `options_from` get their choices now, from the
// catalog the universe holds (this binary's own when it holds no universe),
// and reach the form as `enum` — in the JSON Schema and in the AST alike.
// Nil when the DSL does not compile.
func (s *Service) launchFormSchema(ctx context.Context, task string, dsl []byte) (json.RawMessage, json.RawMessage) {
	if len(dsl) == 0 {
		return nil, nil
	}
	actx := application.WithApplication(ctx, s.app)
	parsed, err := schemaparser.Parse(actx, schemaapi.ParseInput{Body: dsl})
	if err != nil || parsed.Schema == nil {
		return nil, nil
	}
	for _, w := range schemaapi.ResolveOptions(actx, parsed.Schema, s.optionSources(actx)) {
		slog.Warn("action launch form: a field keeps free text", "task", task, "error", w)
	}
	compiled, err := parsed.Schema.ToJSONSchema()
	if err != nil {
		return nil, nil
	}
	ast, _ := json.Marshal(parsed.Schema)
	return compiled, ast
}

// optionSources are the `options_from` sources a launch form draws its
// choices from: the catalog, and the project's objects where this binary
// holds a CWM service.
func (s *Service) optionSources(ctx context.Context) map[string]schemaapi.OptionSource {
	cwm, _ := application.UseService[cwmapi.CWMService](ctx)
	return map[string]schemaapi.OptionSource{
		cwmapi.OptionSourceObjects: cwmapi.ObjectOptions(cwm),
		mbapi.OptionSourceCatalog: schemaapi.OptionSourceFunc(func(ctx context.Context, params map[string]any) ([]map[string]any, error) {
			records, err := mbapi.CatalogOptions(mbapi.Q{Reach: mbapi.ReachUniverse}).Records(ctx, params)
			if err != nil {
				return mbapi.CatalogOptions(mbapi.Q{Reach: mbapi.ReachSelf}).Records(ctx, params)
			}
			return records, nil
		}),
	}
}

func (s *Service) runTask(ctx context.Context, uri mbapi.ToolURI, input json.RawMessage, pluginCtx map[string]any) (actionsapi.RunOutput, error) {
	if s.app == nil {
		return actionsapi.RunOutput{}, fmt.Errorf("ActionService.Run: no Application available (factory was constructed without one)")
	}
	svc, err := application.AppUseService[taskapi.TaskService](s.app)
	if err != nil {
		return actionsapi.RunOutput{}, fmt.Errorf("resolve TaskService: %w", err)
	}
	// Decode the JSON input into a generic map. TaskService.Run
	// expects map[string]any for the input field.
	var inputMap map[string]any
	if len(input) > 0 {
		if err := json.Unmarshal(input, &inputMap); err != nil {
			return actionsapi.RunOutput{}, fmt.Errorf("decode input: %w", err)
		}
	}
	req := taskapi.TaskRunRequest{
		TaskName: uri.Method,
		Input:    inputMap,
	}
	// Thread the plugin-context envelope (e.g. {_target: <id>}) the same
	// way the GUI terminal path does: ConfigOverride["context"] is merged into
	// the agent task's config and decoded onto AgentRunRequest.Context, which
	// WithPluginContext exposes to the cwm plugin.
	if len(pluginCtx) > 0 {
		req.ConfigOverride = map[string]any{"context": pluginCtx}
		if t, ok := pluginCtx["_target"].(string); ok {
			req.Target = t // the run explorer shows what it works on
		}
	}
	resp, err := svc.Run(ctx, req)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	return actionsapi.RunOutput{
		URI:    uri.String(),
		TaskID: resp.RunID,
	}, nil
}

// --- scripts slot -------------------------------------------------

func (s *Service) describeScript(ctx context.Context, uri mbapi.ToolURI) (actionsapi.DescribeOutput, error) {
	// ScriptService has no per-script introspection today (no Get /
	// Describe method on the interface). Surface what we know from
	// the URI itself + flag the gap.
	_ = ctx
	return actionsapi.DescribeOutput{
		URI:         uri.String(),
		Label:       uri.Method,
		Description: fmt.Sprintf("Script %s (ScriptService does not yet expose per-script metadata)", uri.Method),
		IsAsync:     false,
	}, nil
}

func (s *Service) runScript(ctx context.Context, uri mbapi.ToolURI, input json.RawMessage) (actionsapi.RunOutput, error) {
	if s.app == nil {
		return actionsapi.RunOutput{}, fmt.Errorf("ActionService.Run: no Application available (factory was constructed without one)")
	}
	svc, err := application.AppUseService[scriptsapi.ScriptService](s.app)
	if err != nil {
		return actionsapi.RunOutput{}, fmt.Errorf("resolve ScriptService: %w", err)
	}
	// Decode the input as a generic map, then stringify each value
	// for ScriptRunInput.Params (which is `map[string]string` — scripts
	// receive params as env-like string pairs). Non-string values
	// (numbers, bools, objects) get JSON-encoded into the string slot
	// so the script side can re-parse if needed.
	params := map[string]string{}
	if len(input) > 0 {
		var raw map[string]any
		if err := json.Unmarshal(input, &raw); err != nil {
			return actionsapi.RunOutput{}, fmt.Errorf("decode input: %w", err)
		}
		for k, v := range raw {
			if s, ok := v.(string); ok {
				params[k] = s
				continue
			}
			b, err := json.Marshal(v)
			if err != nil {
				return actionsapi.RunOutput{}, fmt.Errorf("encode param %q: %w", k, err)
			}
			params[k] = string(b)
		}
	}
	out, err := svc.Run(ctx, scriptsapi.ScriptRunInput{
		Name:   uri.Method,
		Params: params,
	})
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	// Combine stdout + stderr + exit_code into a single JSON value
	// for the sync return shape.
	value, err := json.Marshal(map[string]any{
		"stdout":    out.Stdout,
		"stderr":    out.Stderr,
		"exit_code": out.ExitCode,
	})
	if err != nil {
		return actionsapi.RunOutput{}, fmt.Errorf("encode script output: %w", err)
	}
	return actionsapi.RunOutput{
		URI:   uri.String(),
		Value: value,
	}, nil
}

// --- KTSP method-call slot (any non-tasks/scripts URI) -------------

// describeMethod returns the method's contract from the provider's manifest
// (the descriptor's slot-type methods): label, description, and the JSON-Schema
// input/output documents. Falls back to a minimal shape when no Application /
// catalog is available, or the method isn't in the descriptor.
func (s *Service) describeMethod(ctx context.Context, uri mbapi.ToolURI) (actionsapi.DescribeOutput, error) {
	out := actionsapi.DescribeOutput{
		URI:     uri.String(),
		Label:   uri.Method,
		IsAsync: false, // method calls are synchronous request/reply
	}
	if sm := s.lookupSlotMethod(ctx, uri); sm != nil {
		out.Description = sm.Description
		out.InputSchema = sm.InputSchema
		out.OutputSchema = sm.OutputSchema
	} else if def := localMethod(ctx, uri); def != nil {
		// A method this binary publishes, unknown to the catalog (a lone
		// process, or the provider asking about itself): its own declaration.
		opts := def.Opts()
		out.Description = opts.Desc
		out.InputSchema = rawSchema(opts.InputSchema)
		out.OutputSchema = rawSchema(opts.OutputSchema)
	}
	out.ReadOnly = s.readOnly(ctx, uri)
	// A long method answers its run and its panel at once: asynchronous.
	out.IsAsync = s.long(ctx, uri)
	if out.Description == "" {
		out.Description = fmt.Sprintf("Invoke %s on %s (%s).", uri.Method, uri.Provider, uri.SlotType)
	}
	return out, nil
}

// lookupSlotMethod finds the SlotMethod for uri in the provider's cached
// manifest, matching slot-type + method on their kebab forms. Returns nil when
// no Application / catalog / manifest / method is available.
func (s *Service) lookupSlotMethod(ctx context.Context, uri mbapi.ToolURI) *mbapi.SlotMethod {
	if s.app == nil {
		return nil
	}
	cat, err := application.AppUseService[mbapi.Catalog](s.app)
	if err != nil || cat == nil {
		return nil
	}
	return mbapi.FindSlotMethod(ctx, cat.Get, uri)
}

// runMethod dispatches a slot-method URI as one synchronous KTSP
// request/reply over the bus (the "next consumer" the v1 stub was
// waiting for — the workflow `call` primitive, 2026-06-10). The
// sender path grammar kebab-normalizes method names, so kebab URIs
// reach PascalCase Go methods without a special case.
func (s *Service) runMethod(ctx context.Context, uri mbapi.ToolURI, input json.RawMessage) (actionsapi.RunOutput, error) {
	// In-process first: a method this binary publishes is called on its own
	// KTSP endpoint — the same slot type, instance and method the bus would
	// deliver — so a lone process (CLI, CI, a live run with no hub) can run
	// the hooks and actions its own services answer. Anything else goes to
	// the hub.
	if out, ok, err := runLocal(ctx, uri, input); ok {
		return out, err
	}
	// A binary never sends its OWN slots over the bus: the hub would answer by
	// spawning another member of this binary — which takes the workspace lock
	// and locks this process out — or, with no hub, the call fails. Its own
	// slot not being on its endpoint means it is not published (yet), and
	// that is the error.
	if self, found := application.SelfProviderFromContext(ctx); found && uri.Provider == self {
		return actionsapi.RunOutput{}, fmt.Errorf("ActionService.Run %s: %s does not publish this slot on its own endpoint (not served, or called before the binary finished booting); its own slots never go to the bus", senderPath(uri), self)
	}
	sender, err := s.busSender(ctx)
	if err != nil {
		return actionsapi.RunOutput{}, fmt.Errorf("ActionService.Run: %w", err)
	}
	// Honor the method's advertised per-call timeout. The bus default is 5s
	// (timeoutFromCtx fallback), which is too short for long methods like
	// build-marketplace (describe over many binaries). A method advertises more
	// via MethodDef.TimeoutS → manifest SlotMethod.TimeoutS; setting the ctx
	// deadline makes the bus honor it (timeoutFromCtx prefers the ctx deadline).
	if sm := s.lookupSlotMethod(ctx, uri); sm != nil && sm.TimeoutS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(sm.TimeoutS)*time.Second)
		defer cancel()
	}
	path := senderPath(uri)
	raw, err := sender.Call(ctx, path, input)
	if err != nil {
		return actionsapi.RunOutput{}, fmt.Errorf("ActionService.Run %s: %w", path, err)
	}
	return actionsapi.RunOutput{
		URI:   uri.String(),
		Value: raw,
	}, nil
}

// readOnly says whether a method only reads: as the catalog declares it, or —
// for a method this binary publishes itself (a lone process has no catalog) —
// as its own endpoint's slot declares it. A guard's expression calls only
// read-only methods, so a lone process must know its own.
func (s *Service) readOnly(ctx context.Context, uri mbapi.ToolURI) bool {
	if sm := s.lookupSlotMethod(ctx, uri); sm != nil {
		return sm.IsReadOnly()
	}
	if def := localMethod(ctx, uri); def != nil {
		return def.Opts().Tags[ktsp.TagMethodReadOnly] == "true"
	}
	return false
}

// long says whether a method declares that it runs long: as the catalog
// declares it, or as this binary's own endpoint does.
func (s *Service) long(ctx context.Context, uri mbapi.ToolURI) bool {
	if sm := s.lookupSlotMethod(ctx, uri); sm != nil {
		return sm.IsLong()
	}
	if def := localMethod(ctx, uri); def != nil {
		return def.Opts().Tags[ktsp.TagMethodLong] == "true"
	}
	return false
}

// startMethodRun runs a long method as a method run: it mints
// the run's panel id, starts the inline method-run task with the method's
// input, and answers the run and the panel at once - the screen that ran it
// shows the panel while the method reports into it. The object the action is
// run on (pluginCtx's _target) is the run's target and its panel's object, so
// a kept outcome lands on it.
func (s *Service) startMethodRun(ctx context.Context, uri mbapi.ToolURI, input json.RawMessage, pluginCtx map[string]any) (actionsapi.RunOutput, error) {
	if s.app == nil {
		return actionsapi.RunOutput{}, fmt.Errorf("ActionService.Run: no Application available (factory was constructed without one)")
	}
	svc, err := application.AppUseService[taskapi.TaskService](s.app)
	if err != nil {
		return actionsapi.RunOutput{}, fmt.Errorf("resolve TaskService: %w", err)
	}
	var inputMap map[string]any
	if len(input) > 0 {
		if err := json.Unmarshal(input, &inputMap); err != nil {
			return actionsapi.RunOutput{}, fmt.Errorf("decode input: %w", err)
		}
	}
	panelID := panel.NewID()
	target, _ := pluginCtx["_target"].(string)
	resp, err := svc.Run(ctx, taskapi.TaskRunRequest{
		TaskDef: taskapi.MethodRunDef(uri.String(), panelID, uri.Method, target),
		Input:   inputMap,
		Target:  target,
	})
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	return actionsapi.RunOutput{URI: uri.String(), TaskID: resp.RunID, Panel: panelID}, nil
}

// localMethod is the declaration of a method this binary publishes on its own
// endpoint; nil when the URI is another provider's or the slot is not here.
func localMethod(ctx context.Context, uri mbapi.ToolURI) *ktsp.MethodDef {
	self, found := application.SelfProviderFromContext(ctx)
	if !found || uri.Provider != self {
		return nil
	}
	endpoint, err := application.UseService[*ktsp.Endpoint](ctx)
	if err != nil || endpoint == nil {
		return nil
	}
	instance := uri.Instance
	if instance == "" {
		instance = ktsp.DefaultInstance
	}
	slot := endpoint.Slot(uri.SlotType, instance)
	if slot == nil {
		return nil
	}
	for _, def := range slot.Defs() {
		if corereflect.ToKebab(def.Name()) == uri.Method {
			return def
		}
	}
	return nil
}

// rawSchema is a method's declared schema as JSON; nil when it has none.
func rawSchema(schema any) json.RawMessage {
	switch v := schema.(type) {
	case nil:
		return nil
	case json.RawMessage:
		return v
	case []byte:
		return v
	}
	raw, err := json.Marshal(schema)
	if err != nil || string(raw) == "null" {
		return nil
	}
	return raw
}

// runLocal calls a method on this binary's own KTSP endpoint when the URI's
// provider is this binary and the endpoint holds the slot; ok is false when it
// does not (the caller goes to the hub).
func runLocal(ctx context.Context, uri mbapi.ToolURI, input json.RawMessage) (actionsapi.RunOutput, bool, error) {
	self, found := application.SelfProviderFromContext(ctx)
	if !found || uri.Provider != self {
		return actionsapi.RunOutput{}, false, nil
	}
	endpoint, err := application.UseService[*ktsp.Endpoint](ctx)
	if err != nil || endpoint == nil {
		return actionsapi.RunOutput{}, false, nil
	}
	instance := uri.Instance
	if instance == "" {
		instance = ktsp.DefaultInstance
	}
	if endpoint.Slot(uri.SlotType, instance) == nil {
		return actionsapi.RunOutput{}, false, nil
	}
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	raw, err := endpoint.Handle(ctx, uri.SlotType, instance, uri.Method, input)
	if err != nil {
		return actionsapi.RunOutput{}, true, fmt.Errorf("ActionService.Run %s (in-process): %w", senderPath(uri), err)
	}
	return actionsapi.RunOutput{URI: uri.String(), Value: raw}, true, nil
}

// senderPath converts a ToolURI into the ktsp.Sender path grammar
// `[PROVIDER]':/'TYPE['('INSTANCE')']'/'METHOD`.
func senderPath(uri mbapi.ToolURI) string {
	instance := ""
	if uri.Instance != "" && uri.Instance != "default" {
		instance = "(" + uri.Instance + ")"
	}
	return fmt.Sprintf("%s:/%s%s/%s", uri.Provider, uri.SlotType, instance, uri.Method)
}

// busSender lazily connects to the running hub (ModeReuse — never
// spawns one) and builds the scope-bound messagebus sender. Held for
// the Service's singleton lifetime; the bus connection is shared
// infra, closed with the process.
func (s *Service) busSender(ctx context.Context) (ktsp.Sender, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sender != nil {
		return s.sender, nil
	}
	if s.dial != nil {
		sender, err := s.dial(ctx)
		if err != nil {
			return nil, err
		}
		s.sender = sender
		return sender, nil
	}
	env := application.Env()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{
		DataDir: env.DataDir(),
		Mode:    hubclient.ModeReuse,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to ktw-hub for method dispatch: %w", err)
	}
	tenantID := os.Getenv("KTW_TENANT_ID")
	if tenantID == "" {
		tenantID = "local"
	}
	universeID := "default"
	if m, mErr := cenv.LoadUniverseManifest(env.UniverseDir()); mErr == nil && m != nil && m.Name != "" {
		universeID = m.Name
	}
	// Wrap with the A-RETRY lazy-spawn sender: a cold target sidecar (not
	// yet running) answers the first call with ErrSlotNotFound; RetrySender
	// then calls ktw-hub ConnMgrService.EnsureSpawned and retries once. This
	// is the same pattern every other cross-binary caller uses — without it,
	// dispatching to an unspawned sidecar fails permanently.
	inner := messagebus.NewSender(hc.MessageBus(), tenantID, universeID)
	s.sender = messagebus.NewRetrySender(inner, "")
	return s.sender, nil
}

// --- helper -------------------------------------------------------

// SlotPrefix returns the first segment of the method path. Used by
// callers that want to inspect routing without parsing the URI
// themselves. Today it just returns the SlotType for compatibility,
// but is kept as a method so callers don't reach into ToolURI's
// internals.
func SlotPrefix(uri mbapi.ToolURI) string {
	return strings.TrimSpace(uri.SlotType)
}
