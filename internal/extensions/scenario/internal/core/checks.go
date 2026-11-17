// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/projectcmd"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// CHECKS — what must hold after a stage, read from the universe itself (CWM
// objects and plans through the hub), the project's files, or a command in the
// project (`bun test`). Programmatic, never an LLM's judgement.

// check evaluates one check: whether it holds, and what was seen.
func (e *Engine) check(ctx context.Context, b *Bus, c api.Check, st State) (bool, string) {
	switch {
	case c.Objects != nil:
		objs, err := e.objects(ctx, b, c.Objects.Type, c.Objects.Module)
		if err != nil {
			return false, err.Error()
		}
		n := 0
		var names []string
		for _, o := range objs {
			if c.Objects.Name != "" && !strings.Contains(strings.ToLower(str(o["name"])), strings.ToLower(c.Objects.Name)) {
				continue
			}
			if c.Objects.State != "" && e.stateOf(ctx, b, str(o["id"])) != c.Objects.State {
				continue
			}
			n++
			names = append(names, str(o["id"]))
		}
		return inRange(n, c.Objects.Min, c.Objects.Max), fmt.Sprintf("%d match %v", n, names)
	case c.Plans != nil:
		raw, err := b.Call(ctx, "ktw-agents:plans/List", map[string]any{"module": c.Plans.Module, "state": c.Plans.State})
		if err != nil {
			return false, err.Error()
		}
		var out struct {
			Plans []map[string]any `json:"plans"`
		}
		_ = json.Unmarshal(raw, &out)
		var ids []string
		for _, p := range out.Plans {
			ids = append(ids, str(p["id"]))
		}
		return inRange(len(out.Plans), c.Plans.Min, c.Plans.Max), fmt.Sprintf("%d plans %v", len(out.Plans), ids)
	case c.File != "":
		_, err := os.Stat(filepath.Join(e.RT.Project(), filepath.FromSlash(c.File)))
		if err != nil {
			return false, "missing"
		}
		return true, "present"
	case c.Cmd != nil:
		code, out := e.runCmd(ctx, *c.Cmd)
		want := 0
		if c.Cmd.Exit != nil {
			want = *c.Cmd.Exit
		}
		return code == want, fmt.Sprintf("exit %d (want %d): %s", code, want, tail(out, 600))
	case c.Call != nil:
		raw, err := b.Call(ctx, c.Call.URI, e.expand(c.Call.Input, st))
		if c.Call.Refused {
			// The refusal is the answer read.
			if err == nil {
				return false, "answered, want refused: " + tail(string(raw), 300)
			}
			return c.Call.Holds(err.Error()), tail(err.Error(), 300)
		}
		if err != nil {
			return false, err.Error()
		}
		return c.Call.Holds(string(raw)), tail(string(raw), 300)
	}
	return false, "empty check"
}

// objects lists CWM objects through the hub.
func (e *Engine) objects(ctx context.Context, b *Bus, typ, module string) ([]map[string]any, error) {
	raw, err := b.Call(ctx, "ktw-agents:cwm/ObjectList", map[string]any{"type": typ, "module": module})
	if err != nil {
		return nil, err
	}
	var out struct {
		Objects []map[string]any `json:"objects"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Objects, nil
}

// stateOf is an object's lifecycle state, read from the whole object (a
// listing carries summaries only); "" when it has none yet — its workflow's
// initial state — or cannot be read.
func (e *Engine) stateOf(ctx context.Context, b *Bus, id string) string {
	module, _, ok := strings.Cut(id, ".")
	if !ok {
		module = id
	}
	raw, err := b.Call(ctx, "ktw-agents:cwm/ObjectGet", map[string]any{"module": module, "id": id})
	if err != nil {
		return ""
	}
	var out struct {
		Object map[string]any `json:"object"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return ""
	}
	return lifecycleState(out.Object)
}

// lifecycleState is a whole object's lifecycle state ("" when it has none).
func lifecycleState(o map[string]any) string {
	caps, _ := o["capabilities"].(map[string]any)
	lc, _ := caps["lifecycle"].(map[string]any)
	return str(lc["state"])
}

// runCmd runs a command in the project through bash - the project's command
// runner, on every platform; with no bash it runs nothing and
// says how to get one.
func (e *Engine) runCmd(ctx context.Context, r api.RunStep) (int, string) {
	dir := e.RT.Project()
	if r.Dir != "" {
		dir = filepath.Join(dir, filepath.FromSlash(r.Dir))
	}
	out, err := projectcmd.RunIn(ctx, dir, r.Cmd, scenarioCmdTimeout)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), out
		}
		return -1, err.Error() + ": " + out
	}
	return 0, out
}

// scenarioCmdTimeout bounds one run step or command check; a stage's own
// timeout bounds the whole.
const scenarioCmdTimeout = time.Hour

func inRange(n int, min, max *int) bool {
	lo := 1
	if min != nil {
		lo = *min
	}
	if n < lo {
		return false
	}
	return max == nil || n <= *max
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
