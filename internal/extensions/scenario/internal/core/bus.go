// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// THE BUS — the case's universe reached through its hub, exactly as the GUI
// reaches it: a slot call `<provider>:<slot>/<Method>` (the hub spawns a member
// on its first call), or an action URI run by ktw-agents' ActionService. No
// CWM is hosted here: the universe's own ktw-agents holds the workspace.

// Bus is a live connection to a case's universe.
type Bus struct {
	hc     *hubclient.HubClient
	sender *messagebus.RetrySender
}

func (e *Engine) connect(ctx context.Context) (*Bus, error) {
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: e.RT.Data(), Mode: hubclient.ModeReuse})
	if err != nil {
		return nil, fmt.Errorf("the universe %s is not running (scenario start): %w", e.universeName(), err)
	}
	tenant, _ := cenv.ResolveTenantUniverse(e.RT.Universe())
	inner := messagebus.NewSender(hc.MessageBus(), tenant, e.universeName())
	return &Bus{hc: hc, sender: messagebus.NewRetrySender(inner, "ktw-hub")}, nil
}

// Close drops the connection (the universe keeps running).
func (b *Bus) Close() { _ = b.hc.Close() }

// Call calls a sidecar method: `<provider>:<slot>/<Method>`.
func (b *Bus) Call(ctx context.Context, uri string, input any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if input == nil {
		input = map[string]any{}
	}
	return b.sender.Call(ctx, uri, input)
}

// actionOutput is what ktw-agents' ActionService answers.
type actionOutput struct {
	URI    string          `json:"uri"`
	TaskID string          `json:"task_id,omitempty"`
	Value  json.RawMessage `json:"value,omitempty"`
}

// Action runs an action URI as the GUI does: a task starts (its run id
// answers), a method or a script answers its value.
func (b *Bus) Action(ctx context.Context, uri string, input any) (actionOutput, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return actionOutput{}, err
	}
	if input == nil {
		raw = []byte("{}")
	}
	out, err := b.Call(ctx, "ktw-agents:action/Run", map[string]any{"uri": uri, "input": json.RawMessage(raw)})
	if err != nil {
		return actionOutput{}, err
	}
	var o actionOutput
	err = json.Unmarshal(out, &o)
	return o, err
}

// CallAny calls a method or runs an action, by the URI's shape: a slot call
// names its method in PascalCase (`ktw-agents:plans/List`), an action URI is
// kebab (`ktw-agents:plans/list`, `ktw-agents:tasks/intake`).
func (b *Bus) CallAny(ctx context.Context, uri string, input any, action bool) (json.RawMessage, error) {
	if !action {
		return b.Call(ctx, uri, input)
	}
	o, err := b.Action(ctx, uri, input)
	if err != nil {
		return nil, err
	}
	if o.TaskID != "" {
		return json.Marshal(map[string]any{"task_id": o.TaskID})
	}
	return o.Value, nil
}

var tmpl = regexp.MustCompile(`\$\{([a-z]+)\.([^}]+)\}`)

// expand fills `${saved.<name>.<path>}`, `${item.<path>}` (an each step's
// element) and `${case.<field>}` in the strings of v.
func (e *Engine) expand(v any, st State) any {
	switch x := v.(type) {
	case string:
		whole := tmpl.FindStringSubmatch(x)
		if whole != nil && whole[0] == x {
			if got, ok := e.lookup(whole[1], whole[2], st); ok {
				return got
			}
			return x
		}
		return tmpl.ReplaceAllStringFunc(x, func(m string) string {
			p := tmpl.FindStringSubmatch(m)
			got, ok := e.lookup(p[1], p[2], st)
			if !ok {
				return m
			}
			if s, isStr := got.(string); isStr {
				return s
			}
			b, _ := json.Marshal(got)
			return string(b)
		})
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			out[k] = e.expand(val, st)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = e.expand(val, st)
		}
		return out
	}
	return v
}

func (e *Engine) lookup(scope, path string, st State) (any, bool) {
	switch scope {
	case "case":
		switch path {
		case "name":
			return e.Case.Name, true
		case "universe":
			return e.universeName(), true
		case "project":
			return e.RT.Project(), true
		}
		// ${case.repositories.<name>}: the folder of one of its other
		// repositories.
		if name, ok := strings.CutPrefix(path, "repositories."); ok {
			for _, r := range e.Case.Repositories {
				if r.Name == name {
					return e.RT.Repository(name), true
				}
			}
		}
		return nil, false
	case "saved":
		cur := walk(any(st.Saved), path)
		return cur, cur != nil
	case eachItem:
		cur := walk(st.Saved[eachItem], path)
		return cur, cur != nil
	}
	return nil, false
}
