// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// TestDebugPlugin_EmitsPerToolCall (UC-122) — the trace plugin
// (`debugPlugin`) writes one NDJSON entry per significant lifecycle event:
// turn_start, pre_api_call, pre_tool_exec, post_tool_exec, etc. Each line
// is a self-describing JSON object with a `type:` discriminator.
//
// This L1 closure exercises the OnTurnStart and OnPreToolExec hooks against
// a captured `io.Writer`. Other hooks (OnPostAPICall, OnModelStream) follow
// the same pattern and are covered transitively by the same emit machinery.
func TestDebugPlugin_EmitsPerToolCall(t *testing.T) {
	var buf bytes.Buffer
	plugin := NewDebugPlugin(&buf)

	hooks := plugin.Hooks()

	// 1. OnTurnStart emits a turn_start NDJSON entry.
	if err := hooks.OnTurnStart(nil, 3); err != nil {
		t.Fatalf("OnTurnStart: %v", err)
	}

	// 2. OnPreToolExec emits a pre_tool_exec entry tagged with the tool name.
	if _, err := hooks.OnPreToolExec(nil, "file-read", map[string]any{
		"file_path": "/tmp/uc-122.txt",
	}); err != nil {
		t.Fatalf("OnPreToolExec: %v", err)
	}

	// 3. Parse each emitted line; assert it's well-formed JSON with a `type`
	// discriminator.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d:\n%s", len(lines), buf.String())
	}

	wantTypes := []string{"turn_start", "pre_tool_exec"}
	for i, line := range lines {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			t.Errorf("line %d: not valid JSON: %v\nline: %s", i, err, line)
			continue
		}
		typeStr, _ := parsed["type"].(string)
		if typeStr != wantTypes[i] {
			t.Errorf("line %d: type = %q, want %q", i, typeStr, wantTypes[i])
		}
	}

	// 4. The pre_tool_exec entry carries the tool name (essential for the
	// trace to be navigable).
	var second map[string]any
	_ = json.Unmarshal([]byte(lines[1]), &second)
	if second["tool"] != "file-read" {
		t.Errorf("pre_tool_exec entry should carry tool=file_read, got %v", second["tool"])
	}

	// 5. The plugin's metadata satisfies the contract.
	if plugin.Name() != "debug" {
		t.Errorf("plugin name = %q, want debug", plugin.Name())
	}
	if !plugin.Stable() {
		t.Error("debug plugin should be Stable (no LLM-visible tools)")
	}
	if defs := plugin.ToolDefs(); len(defs) != 0 {
		t.Errorf("debug plugin should expose no tools, got %d", len(defs))
	}

	// 6. HandleToolCall returns an error (plugin has no tools).
	if _, err := plugin.HandleToolCall(nil, "any", nil); err == nil {
		t.Error("HandleToolCall should error (no tools)")
	}

	_ = api.PluginHooks{} // keep api import alive even if unused above
}
