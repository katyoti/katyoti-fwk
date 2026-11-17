// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	objectpaneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// defaultMethodTimeout applies when a method declares no per-call TimeoutS —
// a sane UI ceiling (not the bus's ~5s, which is too tight for the odd slow
// call, nor a blanket minutes). Methods that need longer declare TimeoutS
// (MethodDef.TimeoutS → manifest SlotMethod.TimeoutS), which we honor exactly.
const defaultMethodTimeout = 30 * time.Second

// WorkspaceCapabilityPanelResult carries a capability-panel descriptor (raw JSON)
// or an error. The GUI renders a generic node-tab panel from the descriptor.
type WorkspaceCapabilityPanelResult struct {
	URI        string `json:"uri,omitempty"`
	Descriptor string `json:"descriptor,omitempty"`
	Error      string `json:"error,omitempty"`
}

// WorkspaceCapabilityPanel fetches the `capability-panel` Contribution whose ID
// matches capabilityID (published by ANY sidecar) and returns its raw JSON
// descriptor. The GUI thus renders the panel WITHOUT importing or naming the
// publishing sidecar — pure contribution consumption.
//
// Wails-bound: window.go.main.App.WorkspaceCapabilityPanel(universeName, capabilityID).
func (a *App) WorkspaceCapabilityPanel(universeName, capabilityID string) WorkspaceCapabilityPanelResult {
	if capabilityID == "" {
		return WorkspaceCapabilityPanelResult{Error: "capability id is required"}
	}
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		return WorkspaceCapabilityPanelResult{Error: fmt.Sprintf("cannot reach universe %q: %v", universeName, err)}
	}

	// The descriptor is the method-panel extension's INPUT on the
	// `object-panel` of the same id (p7p): the panel kind that used to hold it
	// alone is gone, and one declaration serves both the node pane (this
	// binding) and the object pane (the panel itself).
	entries, err := mbapi.Query(a.ctx,
		mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "object-panel", Catalog: cat})
	if err != nil {
		return WorkspaceCapabilityPanelResult{Error: fmt.Sprintf("catalog list failed: %v", err)}
	}
	var c contribapi.Contribution
	for _, e := range entries {
		if e.ID == capabilityID {
			c = e
			break
		}
	}
	if c.ID == "" {
		return WorkspaceCapabilityPanelResult{Error: fmt.Sprintf("no object-panel %q in catalog", capabilityID)}
	}
	uri := c.ResolvedURI()
	body := c.Data()
	if len(body) == 0 {
		b, gErr := cat.GetBody(a.ctx, uri)
		if gErr != nil {
			return WorkspaceCapabilityPanelResult{URI: uri, Error: fmt.Sprintf("read %s: %v", uri, gErr)}
		}
		body = b
	}
	var panel objectpaneapi.PanelDecl
	if err := json.Unmarshal(body, &panel); err != nil {
		return WorkspaceCapabilityPanelResult{URI: uri, Error: fmt.Sprintf("decode %s: %v", uri, err)}
	}
	if panel.Content == nil || panel.Content.Extension != "method-panel" || panel.Content.Data == nil {
		return WorkspaceCapabilityPanelResult{URI: uri, Error: "not a method-panel: its content names no descriptor"}
	}
	raw, err := json.Marshal(panel.Content.Data)
	if err != nil {
		return WorkspaceCapabilityPanelResult{URI: uri, Error: err.Error()}
	}
	return WorkspaceCapabilityPanelResult{URI: uri, Descriptor: string(raw)}
}

// WorkspaceCallMethodResult carries a method call's raw JSON result or an error.
type WorkspaceCallMethodResult struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// tmplRe matches an arg value that is EXACTLY a `${node.field}` / `${opt.field}`
// placeholder — replaced with the TYPED source value (not string-substituted).
var tmplRe = regexp.MustCompile(`^\$\{(node|opt)\.([A-Za-z0-9_]+)\}$`)

// tmplEmbedRe matches a `${node.field}` / `${opt.field}` placeholder ANYWHERE
// inside a larger string (for suffixes like `${node.module_dir}/confluence`).
// These are string-substituted (values stringified), whereas an exact-match
// placeholder (tmplRe) keeps its source type — so a bare `${opt.dryRun}` stays a
// bool while a composed path becomes a string.
var tmplEmbedRe = regexp.MustCompile(`\$\{(node|opt)\.([A-Za-z0-9_]+)\}`)

// WorkspaceCallMethod dispatches an addressable method URI
// (`provider:slot/method`) with resolved args — the generic primitive behind
// capability-panel state/actions. `argsJSON` is the descriptor's arg template;
// `optionsJSON` is the panel's current option values. Each arg value equal to
// `${node.X}` is replaced (typed) from the node's type_specific, and `${opt.X}`
// from the options. NO sidecar-specific knowledge: the URI + args come from the
// contributed descriptor.
//
// Wails-bound: window.go.main.App.WorkspaceCallMethod(universeName, nodeID, methodURI, argsJSON, optionsJSON).
func (a *App) WorkspaceCallMethod(universeName, nodeID, methodURI, argsJSON, optionsJSON string) WorkspaceCallMethodResult {
	if methodURI == "" {
		return WorkspaceCallMethodResult{Error: "method uri is required"}
	}
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceCallMethodResult{Error: fmt.Sprintf("cannot reach universe %q: %v", universeName, err)}
	}

	args, err := a.methodArgs(sender, universeName, nodeID, argsJSON, optionsJSON)
	if err != nil {
		return WorkspaceCallMethodResult{Error: err.Error()}
	}

	// The GUI's direct bus call bypasses the toolproxy, so it does NOT inherit
	// the method's TimeoutS automatically — resolve it from the manifest (the
	// SAME source the toolproxy reads: MethodDef.TimeoutS → SlotMethod.TimeoutS)
	// and honor it. Undeclared → defaultMethodTimeout (not the bus's tight ~5s,
	// not a blanket minutes).
	deadline := defaultMethodTimeout
	if provider, slot, method, ok := splitMethodURI(methodURI); ok {
		if cat, cErr := a.universeCatalog(universeName); cErr == nil {
			if m, mErr := cat.Manifest(a.ctx, provider); mErr == nil {
				if ts := slotMethodTimeoutS(m, slot, method); ts > 0 {
					deadline = time.Duration(ts) * time.Second
				}
			}
		}
	}

	ctx, cancel := context.WithTimeout(a.ctx, deadline)
	defer cancel()
	raw, err := sender.Call(ctx, methodURI, args)
	if err != nil {
		return WorkspaceCallMethodResult{Error: err.Error()}
	}
	return WorkspaceCallMethodResult{Result: string(raw)}
}

// WorkspaceRunMethodResult is a method-panel action run through the action
// surface: a method declared long answers its run and the display panel it
// reports into (the panel drawn in place); any other its value (Result).
type WorkspaceRunMethodResult struct {
	Result string `json:"result,omitempty"`
	TaskID string `json:"taskId,omitempty"`
	Panel  string `json:"panel,omitempty"`
	Error  string `json:"error,omitempty"`
}

// WorkspaceRunMethod runs a method-panel action through ActionService
// : its args resolved as WorkspaceCallMethod resolves them, the
// object it runs on as the run's target - so a long method's run, and the
// panel it reports into, are the object's (a kept outcome lands on it). A
// method declared long answers at once with its run and panel; any other
// answers its value. Nothing names a sidecar: the URI and args are the
// contributed descriptor's.
//
// Wails-bound: window.go.main.App.WorkspaceRunMethod(universeName, nodeID, methodURI, argsJSON, optionsJSON).
func (a *App) WorkspaceRunMethod(universeName, nodeID, methodURI, argsJSON, optionsJSON string) WorkspaceRunMethodResult {
	if methodURI == "" {
		return WorkspaceRunMethodResult{Error: "method uri is required"}
	}
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceRunMethodResult{Error: fmt.Sprintf("cannot reach universe %q: %v", universeName, err)}
	}
	args, err := a.methodArgs(sender, universeName, nodeID, argsJSON, optionsJSON)
	if err != nil {
		return WorkspaceRunMethodResult{Error: err.Error()}
	}
	in, err := methodRunInput(methodURI, nodeID, args)
	if err != nil {
		return WorkspaceRunMethodResult{Error: err.Error()}
	}
	ctx, cancel := a.actionRunContext(universeName, methodURI)
	defer cancel()
	return methodRunResult(runActionCall(ctx, sender, methodURI, in))
}

// methodRunInput is the action-surface run of a method-panel action: the
// method's URI, its resolved args as the input, the object as `_target`.
func methodRunInput(methodURI, nodeID string, args map[string]any) (actionsapi.RunInput, error) {
	raw, err := json.Marshal(args)
	if err != nil {
		return actionsapi.RunInput{}, fmt.Errorf("encode args: %w", err)
	}
	in := actionsapi.RunInput{URI: methodURI, Input: raw}
	if nodeID != "" {
		in.Context = map[string]any{"_target": nodeID}
	}
	return in, nil
}

// methodRunResult reads an action run back for the method-panel: an error,
// a run with its panel, or a value.
func methodRunResult(r WorkspaceRunActionResult) WorkspaceRunMethodResult {
	if r.Error != "" {
		return WorkspaceRunMethodResult{Error: r.Error}
	}
	return WorkspaceRunMethodResult{Result: r.Value, TaskID: r.TaskID, Panel: r.Panel}
}

// methodArgs resolves a descriptor's arg template on the object it runs on:
// each arg equal to `${node.X}` / `${opt.X}` is replaced (typed) from the
// object's fields / the panel's options, a placeholder inside a larger string
// is substituted as text. The one resolver of WorkspaceCallMethod and
// WorkspaceRunMethod.
func (a *App) methodArgs(sender ktsp.Sender, universeName, nodeID, argsJSON, optionsJSON string) (map[string]any, error) {
	args := map[string]any{}
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return nil, fmt.Errorf("bad args json: %v", err)
		}
	}
	opts := map[string]any{}
	if optionsJSON != "" {
		_ = json.Unmarshal([]byte(optionsJSON), &opts)
	}

	// The object's fields for ${node.*}: its content body and its capability
	// groups (objectPlaceholderFields), so a descriptor written with the
	// `node.` prefix reads the object it runs on.
	var nodeTS map[string]any
	if nodeID != "" {
		module, _ := cwmapi.ModuleOfObjectID(nodeID)
		if oRaw, oerr := sender.Call(a.ctx, "ktw-agents:cwm/object-get", cwmapi.ObjectGetInput{Module: module, ID: nodeID}); oerr == nil {
			var og cwmapi.ObjectGetOutput
			if json.Unmarshal(oRaw, &og) == nil {
				nodeTS = objectPlaceholderFields(og.Object)
				// `${node.revision}`: the stored version read - what a
				// publication names as the version of the object it comes
				// from.
				nodeTS["revision"] = og.Revision
			}
		}
		// `${node.module_dir}`: the folder of the object's module — where its
		// files are — absolute, so a sidecar acting on it (a Confluence sync of
		// `${node.module_dir}/confluence`) reaches it whatever its own working
		// directory. CWM answers it project-relative; it is anchored on the
		// project that holds it.
		if sRaw, serr := sender.Call(a.ctx, "ktw-agents:cwm/object-file-stat", cwmapi.ObjectFileStatInput{ID: nodeID, Path: "."}); serr == nil {
			var st cwmapi.ObjectFileStatOutput
			if json.Unmarshal(sRaw, &st) == nil && st.ProjectPath != "" {
				if nodeTS == nil {
					nodeTS = map[string]any{}
				}
				nodeTS["module_dir"] = resolveProjectRelative(filepath.FromSlash(st.ProjectPath), a.projectRootsOf(universeName))
			}
		}
	}

	// Resolve ${node.X} / ${opt.X} placeholders.
	for k, v := range args {
		s, ok := v.(string)
		if !ok {
			continue
		}
		// Exact match → typed replace (keeps bool/number source type).
		if m := tmplRe.FindStringSubmatch(s); m != nil {
			args[k] = resolvePlaceholder(m[1], m[2], nodeTS, opts)
			continue
		}
		// Embedded placeholder(s) in a larger string → string substitution
		// (e.g. `${node.module_dir}/confluence`).
		if tmplEmbedRe.MatchString(s) {
			args[k] = tmplEmbedRe.ReplaceAllStringFunc(s, func(match string) string {
				sm := tmplEmbedRe.FindStringSubmatch(match)
				return stringifyPlaceholder(resolvePlaceholder(sm[1], sm[2], nodeTS, opts))
			})
		}
	}
	return args, nil
}

// projectRootsOf is the folders a universe serves: its attached projects, or
// the universe folder itself for a universe that lives in a checkout.
func (a *App) projectRootsOf(universeName string) []string {
	for _, u := range a.listUniverses().Universes {
		if u.Name != universeName || u.HomePath == "" {
			continue
		}
		if roots := cenv.AttachedProjects(application.EnvOf(u.HomePath).HomeDir()); len(roots) > 0 {
			return roots
		}
		return []string{u.HomePath}
	}
	return nil
}

// resolveProjectRelative anchors a project-relative path on the first root
// where it exists. An absolute path, or one found under no root, is returned
// as it was: guessing a root for a folder that exists nowhere would only move
// the error.
func resolveProjectRelative(p string, roots []string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	for _, r := range roots {
		cand := filepath.Join(r, p)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return p
}

// objectPlaceholderFields flattens an object into the map `${node.X}` reads:
// every capability group's fields, then the type's own fields on top — they
// win a name clash.
func objectPlaceholderFields(o cwmapi.Object) map[string]any {
	out := map[string]any{"id": o.ID, "type": o.Type, "name": o.Name, "description": o.Description, "parent": o.Parent}
	for _, group := range o.Capabilities {
		for k, v := range group {
			out[k] = v
		}
	}
	for k, v := range o.Fields {
		out[k] = v
	}
	return out
}

// resolvePlaceholder returns the source value for a `${scope.field}` reference:
// node type_specific for `node`, panel options for `opt`. nil when absent.
func resolvePlaceholder(scope, field string, nodeTS, opts map[string]any) any {
	switch scope {
	case "node":
		if nodeTS != nil {
			return nodeTS[field]
		}
	case "opt":
		return opts[field]
	}
	return nil
}

// stringifyPlaceholder renders a resolved placeholder value for embedding in a
// larger string. nil → "" (so a missing field drops cleanly out of a path).
func stringifyPlaceholder(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}

// splitMethodURI parses `<provider>:<slot>/<method>` (the tool/action URI
// flavour) into its parts. ok=false on any missing segment.
func splitMethodURI(uri string) (provider, slot, method string, ok bool) {
	ci := strings.IndexByte(uri, ':')
	if ci <= 0 {
		return "", "", "", false
	}
	provider = uri[:ci]
	rest := uri[ci+1:]
	si := strings.IndexByte(rest, '/')
	if si <= 0 {
		return "", "", "", false
	}
	slot = rest[:si]
	method = rest[si+1:]
	return provider, slot, method, provider != "" && slot != "" && method != ""
}

// slotMethodTimeoutS returns the declared per-call TimeoutS (seconds) of
// <slot>/<method> in a provider manifest, or 0 when undeclared — the SAME value
// the toolproxy applies (MethodDef.TimeoutS → SlotMethod.TimeoutS). The slot
// token is a slot INSTANCE name; it links to a SlotType (which carries the
// methods) via Slots[].Type. Falls back to a method-name match across all types.
func slotMethodTimeoutS(m *mbapi.Manifest, slot, method string) int {
	if m == nil {
		return 0
	}
	slotType := slot
	for _, s := range m.Slots {
		if s.Name == slot {
			slotType = s.Type
			break
		}
	}
	for _, st := range m.SlotTypes {
		if st.Type != slotType {
			continue
		}
		for _, mm := range st.Methods {
			if mm.Name == method {
				return mm.TimeoutS
			}
		}
	}
	for _, st := range m.SlotTypes {
		for _, mm := range st.Methods {
			if mm.Name == method {
				return mm.TimeoutS
			}
		}
	}
	return 0
}
