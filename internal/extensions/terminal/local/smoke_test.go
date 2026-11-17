// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// TestSmoke_TerminalSlotResponds proves the Terminal slot is wired into
// the KTSP container and reachable over stdio.
//
// Post bus-split-ktw-gui-hub-cutover, the Terminal service publishes its
// streaming events on session-scoped NATS subjects — over `sc ktsp stdio`
// the bus + scope headers are unavailable, so `Terminal/start-task`
// returns a structured "missing scope headers" RPC error. The test
// asserts on THAT error: getting a typed RPC error back from the slot
// proves:
//   - The Terminal slot is registered in the binary's KTSP container
//   - The KTSP request/response round-trip works over stdio
//   - The slot's pre-flight scope check fires (deferred bus init wasn't
//     accidentally dropped)
//
// The full happy-path (event/done streaming) lives in the bus-mode
// integration test that spawns ktw-hub + the GUI cowork scenario; see
// `app/ktw-gui/internal/supervisor/terminalrouter_test.go`.
//
// Skipped automatically when ./bin/ktw-agents[.exe] is absent so this test
// never blocks unrelated CI runs that don't ship the binary.
func TestSmoke_TerminalSlotResponds(t *testing.T) {
	long.Test(t)
	binary := findKTWAgentsBinary(t)
	if binary == "" {
		t.Skip("ktw-agents binary not found at ./bin/ or /tmp/; skipping smoke")
	}

	// Set up a minimal valid universe in a temp dir.
	homeDir := makeTempUniverse(t)
	// ktw-agents fetches catalog data via the bus (2026-06-05 layering
	// rule); start a hub for this tempdir before spawning.
	hubharness.Start(t, homeDir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "sc", "ktsp", "stdio")
	cmd.Env = append(os.Environ(), "KTW_WORKSPACE="+workspaceOf(homeDir), "KTW_DATA="+homeDir)
	// Run in repo root so ./bin/ktw-agents binary search etc. resolves.
	if root, _ := repoRootFromTest(); root != "" {
		cmd.Dir = root
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	// Drain stderr so the subprocess doesn't block on a full pipe; tee
	// to test logs for diagnostic value when something goes wrong.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	go drainStderr(t, stderr)

	if err := hubharness.Tie(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start ktw-agents: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// JSON-RPC request: Terminal/start-task with a placeholder taskName.
	// We use "hello-world" because it's a bundled task in embed/tasks/;
	// even if it errors during execution (no LLM creds), StartTask itself
	// must return a runID synchronously.
	// Slot name on the wire is the kebab BusName per
	// bus-named-slot-addressing — "Terminal" → "terminal".
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      "smoke-1",
		"method":  "terminal/start-task",
		"params": map[string]any{
			"taskName":    "hello-world",
			"clientLabel": "smoke-test",
		},
	}
	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')
	if _, err := stdin.Write(reqBytes); err != nil {
		t.Fatalf("write request: %v", err)
	}

	// Read response. The first JSON-RPC message we receive should be the
	// StartTask response (not a notification — notifications have no id).
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	deadline := time.Now().Add(20 * time.Second)
	linesRead := 0
	for time.Now().Before(deadline) {
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("scan stdout: %v (lines read: %d)", err, linesRead)
			}
			t.Fatalf("scanner returned EOF after %d lines — subprocess likely crashed; check stderr log above", linesRead)
		}
		linesRead++
		line := scanner.Bytes()
		var msg struct {
			ID     any             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue // skip non-JSON lines (logs leaked to stdout)
		}
		// We want the response to id "smoke-1".
		if msg.ID == nil || msg.ID == "" {
			// notification — keep reading
			continue
		}
		// Over stdio neither the bus nor scope headers are available,
		// so Terminal/start-task is expected to return a structured RPC
		// error. Either "MessageBus not found" (bus check fires first)
		// or "scope headers" (if a bus is somehow available) is fine —
		// both prove the slot is wired AND enforcing the bus-only
		// precondition. Any OTHER outcome is a regression:
		//   - success                       → bus-only check bypassed
		//   - "slot not found" / "no method" → slot isn't registered
		if msg.Error == nil || len(msg.Error) == 0 {
			t.Fatalf("expected bus-only enforcement error over stdio, got success: %s", string(msg.Result))
		}
		errStr := string(msg.Error)
		if !strings.Contains(errStr, "MessageBus") && !strings.Contains(errStr, "scope") {
			t.Fatalf("terminal/start-task returned unexpected error (want 'MessageBus' or 'scope'): %s", errStr)
		}
		return // pass
	}
	t.Fatalf("timeout waiting for terminal/start-task response")
}

// TestSmoke_SkillsListResponds proves the `resources/list` KTSP method
// (filtered to `type=skill`) round-trips end-to-end: the per-universe
// core spawned by `ktw-agents sc ktsp stdio` accepts the request, returns
// a manifest result, and the wire matches what app/ktw-gui's
// listSkillsKTSP expects.
//
// Post step-13 of bus-split-aggregator-to-catalog, `skills/list` was
// removed from the wire surface (catalog-served now). The regression
// net moved to the `resources/list` slot which the catalog populates.
//
// Originally tested for sidecar-skills-tasks-visibility — without this,
// the `App.ListSkills` Wails binding could break silently next time
// someone refactors the resources slot registration.
func TestSmoke_SkillsListResponds(t *testing.T) {
	long.Test(t)
	binary := findKTWAgentsBinary(t)
	if binary == "" {
		t.Skip("ktw-agents binary not found at ./bin/ or /tmp/; skipping smoke")
	}

	homeDir := makeTempUniverse(t)
	// ktw-agents now fetches catalog data via the bus (2026-06-05 layering
	// rule; see hub-owns-cache-layering-rule). Start a hub for this
	// tempdir before spawning ktw-agents, else the new fetchManifestsFromHub
	// path would surface as empty-catalog test failures.
	hubharness.Start(t, homeDir)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "sc", "ktsp", "stdio")
	cmd.Env = append(os.Environ(), "KTW_WORKSPACE="+workspaceOf(homeDir), "KTW_DATA="+homeDir)
	if root, _ := repoRootFromTest(); root != "" {
		cmd.Dir = root
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	go drainStderr(t, stderr)

	if err := hubharness.Tie(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start ktw-agents: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      "smoke-skills-1",
		// The KTSP slot was renamed `resources` → `contributions` in a
		// prior milestone (catalog-served, post step-13 of
		// bus-split-aggregator-to-catalog). The slot's List method
		// takes a ContributionListInput with a Filter.Types[] array.
		"method": "resources/list",
		"params": map[string]any{"filter": map[string]any{"types": []string{"skill"}}},
	}
	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')
	if _, err := stdin.Write(reqBytes); err != nil {
		t.Fatalf("write request: %v", err)
	}

	// `resources/list` returns `{contributions: [...]}` (list
	// shape) — switched from the old `resources/list` (manifest shape).
	expectListResponse(t, stdout, "smoke-skills-1", "resources")
}

// TestSmoke_TasksListResponds is the parallel test for tasks/list (the
// renamed TaskService.List slot). Together with the skills test it proves
// the universe-aware GUI listings paths (App.ListSkills, App.ListTasksDetail)
// reach a real core and decode correctly.
func TestSmoke_TasksListResponds(t *testing.T) {
	long.Test(t)
	binary := findKTWAgentsBinary(t)
	if binary == "" {
		t.Skip("ktw-agents binary not found at ./bin/ or /tmp/; skipping smoke")
	}

	homeDir := makeTempUniverse(t)
	// ktw-agents now fetches catalog data via the bus (2026-06-05 layering
	// rule; see hub-owns-cache-layering-rule). Start a hub for this
	// tempdir before spawning ktw-agents, else the new fetchManifestsFromHub
	// path would surface as empty-catalog test failures.
	hubharness.Start(t, homeDir)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "sc", "ktsp", "stdio")
	cmd.Env = append(os.Environ(), "KTW_WORKSPACE="+workspaceOf(homeDir), "KTW_DATA="+homeDir)
	if root, _ := repoRootFromTest(); root != "" {
		cmd.Dir = root
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	go drainStderr(t, stderr)

	if err := hubharness.Tie(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start ktw-agents: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      "smoke-tasks-1",
		"method":  "tasks/list",
		"params":  map[string]any{},
	}
	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')
	if _, err := stdin.Write(reqBytes); err != nil {
		t.Fatalf("write request: %v", err)
	}

	expectListResponse(t, stdout, "smoke-tasks-1", "tasks")
}

// expectManifestResponse reads JSON-RPC frames off stdout until it finds
// the response matching wantID, then asserts the result decodes to a
// `mbapi.ListOutput`-shaped payload with a `manifest` object. The
// manifest's resources array may be empty — what we're proving is that
// the wire surface works (no method-not-found, no decode panic).
func expectManifestResponse(t *testing.T, stdout io.Reader, wantID string) {
	t.Helper()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("scan stdout: %v", err)
			}
			t.Fatal("scanner returned EOF — subprocess likely crashed")
		}
		var msg struct {
			ID     any             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.ID == nil || msg.ID == "" || msg.ID != wantID {
			continue
		}
		if msg.Error != nil && len(msg.Error) > 0 {
			t.Fatalf("%s returned JSON-RPC error: %s", wantID, string(msg.Error))
		}
		var resultMap map[string]json.RawMessage
		if err := json.Unmarshal(msg.Result, &resultMap); err != nil {
			t.Fatalf("decode result as map: %v (raw=%s)", err, string(msg.Result))
		}
		if _, ok := resultMap["manifest"]; !ok {
			t.Fatalf("response missing 'manifest' field; got keys: %v (raw=%s)", mapKeys(resultMap), string(msg.Result))
		}
		return
	}
	t.Fatal("deadline reached without matching response")
}

// expectListResponse reads JSON-RPC frames off stdout until it finds the
// response matching wantID, then asserts the result decodes to a list-shaped
// payload with the named array field. The field's contents may be empty —
// what we're proving is that the wire works, not that any specific items
// are present (those depend on which sidecars the universe activates).
func expectListResponse(t *testing.T, stdout io.Reader, wantID, listField string) {
	t.Helper()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	deadline := time.Now().Add(20 * time.Second)
	linesRead := 0
	for time.Now().Before(deadline) {
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("scan stdout: %v (lines read: %d)", err, linesRead)
			}
			t.Fatalf("scanner returned EOF after %d lines — subprocess likely crashed; check stderr log above", linesRead)
		}
		linesRead++
		line := scanner.Bytes()
		var msg struct {
			ID     any             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.ID == nil || msg.ID == "" || msg.ID != wantID {
			continue
		}
		if msg.Error != nil && len(msg.Error) > 0 {
			t.Fatalf("%s returned JSON-RPC error: %s", wantID, string(msg.Error))
		}
		// Decode the result as a generic map and verify the expected list field exists.
		var resultMap map[string]json.RawMessage
		if err := json.Unmarshal(msg.Result, &resultMap); err != nil {
			t.Fatalf("decode result as map: %v (raw=%s)", err, string(msg.Result))
		}
		raw, ok := resultMap[listField]
		if !ok {
			t.Fatalf("response missing %q field; got keys: %v (raw=%s)", listField, mapKeys(resultMap), string(msg.Result))
		}
		// Field may be `[]` or `[{...}]` — both are valid (empty list = no
		// skills/tasks contributed by this universe). Just check it's a JSON array.
		if len(raw) == 0 || (raw[0] != '[' && raw[0] != 'n') {
			t.Fatalf("%s field is not a JSON array: %s", listField, string(raw))
		}
		return // pass
	}
	t.Fatalf("timeout waiting for %s response", wantID)
}

func mapKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestSmoke_KtwDemoExposesDemos proves ktw-demo (the carved-out sidecar)
// publishes its 7 demo tasks via the canonical contributions path. We
// launch ktw-demo's stdio KTSP surface and call `resources/list`
// filtered to `Type: "task"` — the Phase E §5 model where sidecars
// surface tasks as Contributions (no per-sidecar `task-library` slot).
//
// Skipped if bin/ktw-demo[.exe] isn't built.
func TestSmoke_KtwDemoExposesDemos(t *testing.T) {
	binary := findKtwDemoBinary(t)
	if binary == "" {
		t.Skip("ktw-demo binary not found at ./bin/ or /tmp/; skipping smoke")
	}

	// This test exercises ktw-demo's OWN stdio KTSP surface — it does NOT
	// spawn ktw-agents, so it doesn't need a hub. (The replace_all that
	// added `hubharness.Start` to the ktw-agents-spawning smokes in this
	// file initially caught this test too — wrong, so it was reverted.)
	homeDir := makeTempUniverse(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "sc", "ktsp", "stdio")
	cmd.Env = append(os.Environ(), "KTW_WORKSPACE="+workspaceOf(homeDir), "KTW_DATA="+homeDir)
	if root, _ := repoRootFromTest(); root != "" {
		cmd.Dir = root
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	go drainStderr(t, stderr)

	if err := hubharness.Tie(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start ktw-demo: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      "smoke-demo-1",
		"method":  "resources/list",
		"params":  map[string]any{},
	}
	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')
	if _, err := stdin.Write(reqBytes); err != nil {
		t.Fatalf("write request: %v", err)
	}

	expected := []string{
		"hello-world", "demo-delegate-magentic", "demo-delegate-parallel-fallback",
		"demo-delegate-pipeline", "demo-delegate-team-coordination",
		"demo-delegate-team-messaging", "display-document-demo",
	}
	// Per Phase E §5: tasks appear in `resources/list` as entries with
	// type="task" — the ID is the task name; assert against that field.
	expectContributionsContaining(t, stdout, "smoke-demo-1", expected)
}

// expectContributionsContaining reads JSON-RPC frames until it finds the
// `resources/list` response matching wantID, then asserts the
// `contributions` array contains every entry in `mustHave` matched on
// the Contribution.id field. Used by Phase E §5 sidecar-task smokes
// (the canonical task-discovery path now that the per-sidecar
// `task-library` slot is gone).
func expectContributionsContaining(t *testing.T, stdout io.Reader, wantID string, mustHave []string) {
	t.Helper()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("scan stdout: %v", err)
			}
			t.Fatal("scanner returned EOF — subprocess likely crashed")
		}
		line := scanner.Bytes()
		var msg struct {
			ID     any             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.ID == nil || msg.ID != wantID {
			continue
		}
		if msg.Error != nil && len(msg.Error) > 0 {
			t.Fatalf("%s returned JSON-RPC error: %s", wantID, string(msg.Error))
		}
		var resultMap map[string]json.RawMessage
		if err := json.Unmarshal(msg.Result, &resultMap); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		raw, ok := resultMap["resources"]
		if !ok {
			t.Fatalf("response missing \"contributions\" field")
		}
		var entries []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &entries); err != nil {
			t.Fatalf("decode entries: %v", err)
		}
		got := map[string]bool{}
		for _, e := range entries {
			got[e.ID] = true
		}
		var missing []string
		for _, want := range mustHave {
			if !got[want] {
				missing = append(missing, want)
			}
		}
		if len(missing) > 0 {
			t.Fatalf("contributions missing entries: %v (got %d entries: %v)", missing, len(entries), got)
		}
		return // pass
	}
	t.Fatalf("timeout waiting for %s response", wantID)
}

// expectListContaining reads JSON-RPC frames until it finds the response
// matching wantID, then asserts the named list field contains every entry
// in `mustHave` (matched on the `name` sub-field of each list element).
func expectListContaining(t *testing.T, stdout io.Reader, wantID, listField string, mustHave []string) {
	t.Helper()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("scan stdout: %v", err)
			}
			t.Fatal("scanner returned EOF — subprocess likely crashed")
		}
		line := scanner.Bytes()
		var msg struct {
			ID     any             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.ID == nil || msg.ID != wantID {
			continue
		}
		if msg.Error != nil && len(msg.Error) > 0 {
			t.Fatalf("%s returned JSON-RPC error: %s", wantID, string(msg.Error))
		}
		var resultMap map[string]json.RawMessage
		if err := json.Unmarshal(msg.Result, &resultMap); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		raw, ok := resultMap[listField]
		if !ok {
			t.Fatalf("response missing %q field", listField)
		}
		var entries []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &entries); err != nil {
			t.Fatalf("decode entries: %v", err)
		}
		got := map[string]bool{}
		for _, e := range entries {
			got[e.Name] = true
		}
		var missing []string
		for _, want := range mustHave {
			if !got[want] {
				missing = append(missing, want)
			}
		}
		if len(missing) > 0 {
			t.Fatalf("%s missing entries: %v (got %d entries: %v)", listField, missing, len(entries), got)
		}
		return // pass
	}
	t.Fatalf("timeout waiting for %s response", wantID)
}

// findKtwDemoBinary mirrors findKTWAgentsBinary for the new sidecar.
func findKtwDemoBinary(t *testing.T) string {
	t.Helper()
	return hubharness.Binary(t, "ktw-demo")
}

// TestSmoke_TasksProviderURI proves the provider:name URI shape lands
// end-to-end on the `tasks/list` and `tasks/get` KTSP slots, and that
// `tasks/get` accepts both the full URI and a bare-name (when unique).
//
// Skipped if bin/ktw-agents is missing.
func TestSmoke_TasksProviderURI(t *testing.T) {
	long.Test(t)
	binary := findKTWAgentsBinary(t)
	if binary == "" {
		t.Skip("ktw-agents binary not found; skipping smoke")
	}

	homeDir := makeTempUniverse(t)
	// ktw-agents now fetches catalog data via the bus (2026-06-05 layering
	// rule; see hub-owns-cache-layering-rule). Start a hub for this
	// tempdir before spawning ktw-agents, else the new fetchManifestsFromHub
	// path would surface as empty-catalog test failures.
	hubharness.Start(t, homeDir)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "sc", "ktsp", "stdio")
	cmd.Env = append(os.Environ(), "KTW_WORKSPACE="+workspaceOf(homeDir), "KTW_DATA="+homeDir)
	if root, _ := repoRootFromTest(); root != "" {
		cmd.Dir = root
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	go drainStderr(t, stderr)

	if err := hubharness.Tie(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start ktw-agents: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// ── Request 1: tasks/list — every entry must have provider+uri ──
	req1 := map[string]any{
		"jsonrpc": "2.0", "id": "smoke-uri-list",
		"method": "tasks/list", "params": map[string]any{},
	}
	b1, _ := json.Marshal(req1)
	if _, err := stdin.Write(append(b1, '\n')); err != nil {
		t.Fatalf("write list req: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	deadline := time.Now().Add(20 * time.Second)
	listOK := false
	getBareOK := false
	getURIOK := false

	// Discovered from the list response — the test universe may not have
	// sidecars activated, so we pick whatever task is available rather
	// than hard-coding one whose provider depends on registry state.
	var wantBare, wantURI string

	for time.Now().Before(deadline) && (!listOK || !getBareOK || !getURIOK) {
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("scan stdout: %v", err)
			}
			t.Fatalf("EOF before all 3 responses (list=%v bare=%v uri=%v)", listOK, getBareOK, getURIOK)
		}
		line := scanner.Bytes()
		var msg struct {
			ID     any             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		switch msg.ID {
		case "smoke-uri-list":
			if msg.Error != nil && len(msg.Error) > 0 {
				t.Fatalf("tasks/list error: %s", string(msg.Error))
			}
			var resp struct {
				Tasks []struct {
					Name     string `json:"name"`
					Provider string `json:"provider"`
					URI      string `json:"uri"`
				} `json:"tasks"`
			}
			if err := json.Unmarshal(msg.Result, &resp); err != nil {
				t.Fatalf("decode list: %v", err)
			}
			if len(resp.Tasks) == 0 {
				t.Fatal("tasks/list returned 0 entries")
			}
			for _, e := range resp.Tasks {
				if e.Provider == "" {
					t.Errorf("task %q has empty provider", e.Name)
				}
				if e.URI == "" {
					t.Errorf("task %q has empty uri", e.Name)
				}
			}
			// Pick the first task and use its name/URI for the get tests.
			wantBare = resp.Tasks[0].Name
			wantURI = resp.Tasks[0].URI
			listOK = true
			// Fire requests 2 + 3 now that we've seen the list.
			req2 := map[string]any{
				"jsonrpc": "2.0", "id": "smoke-uri-bare",
				"method": "tasks/get", "params": map[string]any{"name": wantBare},
			}
			req3 := map[string]any{
				"jsonrpc": "2.0", "id": "smoke-uri-full",
				"method": "tasks/get", "params": map[string]any{"name": wantURI},
			}
			b2, _ := json.Marshal(req2)
			b3, _ := json.Marshal(req3)
			_, _ = stdin.Write(append(b2, '\n'))
			_, _ = stdin.Write(append(b3, '\n'))

		case "smoke-uri-bare":
			if msg.Error != nil && len(msg.Error) > 0 {
				t.Errorf("tasks/get(bare=%q) error: %s", wantBare, string(msg.Error))
			}
			var info struct {
				Name string `json:"name"`
				URI  string `json:"uri"`
			}
			if err := json.Unmarshal(msg.Result, &info); err == nil {
				if info.Name != wantBare {
					t.Errorf("tasks/get(bare): name=%q want %q", info.Name, wantBare)
				}
			}
			getBareOK = true

		case "smoke-uri-full":
			if msg.Error != nil && len(msg.Error) > 0 {
				t.Errorf("tasks/get(uri=%q) error: %s", wantURI, string(msg.Error))
			}
			var info struct {
				Name string `json:"name"`
				URI  string `json:"uri"`
			}
			if err := json.Unmarshal(msg.Result, &info); err == nil {
				if info.URI != wantURI {
					t.Errorf("tasks/get(uri): uri=%q want %q", info.URI, wantURI)
				}
			}
			getURIOK = true
		}
	}
}

// TestSmoke_RunnableSkillsAppearAsTasks proves the addRunnableSkills
// mechanism wires skills with `provides: [task]` frontmatter into
// `tasks/list` as type=agent wrappers.
//
// Expects at least one of the canonical opt-in skills (llm-catalog,
// requirements-analysis) to appear with provider=ktw-agents and type=agent.
//
// Skipped if bin/ktw-agents is missing.
func TestSmoke_RunnableSkillsAppearAsTasks(t *testing.T) {
	long.Test(t)
	binary := findKTWAgentsBinary(t)
	if binary == "" {
		t.Skip("ktw-agents binary not found; skipping smoke")
	}

	homeDir := makeTempUniverse(t)
	// ktw-agents now fetches catalog data via the bus (2026-06-05 layering
	// rule; see hub-owns-cache-layering-rule). Start a hub for this
	// tempdir before spawning ktw-agents, else the new fetchManifestsFromHub
	// path would surface as empty-catalog test failures.
	hubharness.Start(t, homeDir)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "sc", "ktsp", "stdio")
	cmd.Env = append(os.Environ(), "KTW_WORKSPACE="+workspaceOf(homeDir), "KTW_DATA="+homeDir)
	if root, _ := repoRootFromTest(); root != "" {
		cmd.Dir = root
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	go drainStderr(t, stderr)

	if err := hubharness.Tie(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start ktw-agents: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	req := map[string]any{
		"jsonrpc": "2.0", "id": "smoke-skill-task",
		"method": "tasks/list", "params": map[string]any{},
	}
	b, _ := json.Marshal(req)
	if _, err := stdin.Write(append(b, '\n')); err != nil {
		t.Fatalf("write req: %v", err)
	}

	// Skills that ship with `provides: [task]` in their frontmatter.
	// At least one must appear in the task list as a type=agent wrapper.
	candidates := map[string]bool{
		"llm-catalog":           true,
		"requirements-analysis": true,
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if !scanner.Scan() {
			t.Fatal("EOF before tasks/list response")
		}
		var msg struct {
			ID     any             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.ID != "smoke-skill-task" {
			continue
		}
		if msg.Error != nil && len(msg.Error) > 0 {
			t.Fatalf("tasks/list error: %s", string(msg.Error))
		}
		var resp struct {
			Tasks []struct {
				Name     string `json:"name"`
				Provider string `json:"provider"`
				Type     string `json:"type"`
			} `json:"tasks"`
		}
		if err := json.Unmarshal(msg.Result, &resp); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		var found []string
		for _, e := range resp.Tasks {
			if !candidates[e.Name] {
				continue
			}
			if e.Type != "agent" {
				t.Errorf("skill-derived task %q has type=%q, want agent", e.Name, e.Type)
			}
			if e.Provider != "ktw-agents" {
				t.Errorf("skill-derived task %q has provider=%q, want ktw-agents", e.Name, e.Provider)
			}
			found = append(found, e.Name)
		}
		if len(found) == 0 {
			names := make([]string, 0, len(resp.Tasks))
			for _, e := range resp.Tasks {
				names = append(names, e.Name)
			}
			t.Fatalf("no opt-in skill (%v) found in tasks/list. Got %d tasks.", candidates, len(resp.Tasks))
		}
		t.Logf("found %d skill-derived tasks: %v", len(found), found)
		return // pass
	}
	t.Fatal("timeout waiting for tasks/list response")
}

// findKTWAgentsBinary checks a few known locations for the binary.
// Skips the test if none is present.
func findKTWAgentsBinary(t *testing.T) string {
	t.Helper()
	return hubharness.Binary(t, "ktw-agents")
}

// repoRootFromTest walks up from the test working directory to find go.mod.
func repoRootFromTest() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", errors.New("go.mod not found")
		}
		wd = parent
	}
}

// makeTempUniverse writes a minimal valid universe in a temp dir.
// makeTempUniverse builds an isolated universe and returns its HOME dir
// (`<workspace>/.katyoti`), which is also what KTW_DATA points at.
//
// The `.katyoti` nesting is not decoration: a spawned binary resolves its home
// from the WORKSPACE, so a universe written anywhere else is one the binary
// will not find — it falls back to the current directory, which for these
// tests is the repo itself.
func makeTempUniverse(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".katyoti")
	cfgDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	manifest := []byte(`version: 1
name: smoke-test
display_name: Smoke Test Universe
icon: S
color: gray
`)
	if err := os.WriteFile(filepath.Join(cfgDir, "Univers.yaml"), manifest, 0o644); err != nil {
		t.Fatalf("write Univers.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "Settings.yaml"), []byte("# test universe\n"), 0o644); err != nil {
		t.Fatalf("write Settings.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte{}, 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extensions.yaml"), []byte("version: 1\nsidecars: []\n"), 0o644); err != nil {
		t.Fatalf("write extensions.yaml: %v", err)
	}
	return dir
}

// drainStderr reads stderr and forwards lines to test logs (filtered to
// signals: panic, fatal, or the first 5 lines for cold-start diagnostics).
// Required to prevent the subprocess from blocking on a full stderr pipe.
func drainStderr(t *testing.T, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	var mu sync.Mutex
	lines := 0
	for scanner.Scan() {
		line := scanner.Text()
		lines++
		// Always log first 5 lines + anything that looks like a crash so
		// we can diagnose cold-start failures without flooding the log.
		if lines <= 5 || strings.Contains(line, "panic") || strings.Contains(line, "fatal") || strings.Contains(line, "ERROR") {
			mu.Lock()
			t.Logf("[ktw-agents stderr] %s", line)
			mu.Unlock()
		}
	}
}

// workspaceOf returns the workspace root for a home dir — the parent of
// `.katyoti`.
//
// # Why this test could not run beside another
//
// These spawns used to set KTW_HOME, which was REMOVED when the workspace
// model landed (CLAUDE.md: "KTW_WORKSPACE … replaces old KTW_HOME"). A removed
// variable is not an error, it is simply ignored — so every spawned
// ktw-agents ignored the temp universe, fell back to the working directory
// (the repo), and took the CWM workspace lock on the developer's OWN
// `.katyoti`.
//
// That made the suite un-runnable beside anything else that touches a
// workspace: whichever process got there first won, and the others died with
// `cwm: workspace lock held by another process` — which reads like a product
// bug and is a test that was never isolated. It also meant these tests were
// operating on live local state rather than on a fixture.
func workspaceOf(homeDir string) string { return filepath.Dir(homeDir) }
