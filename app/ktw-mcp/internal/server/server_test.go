// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The test binary plays three parts. Run as the server (KTW_MCP_TEST_SERVE
// names the installation folder) it serves MCP on its own stdin and stdout, as
// ktw-mcp does. Copied beside it under a program's name it is that program: it
// prints its arguments and the CLAUDECODE it sees, logs the call, and exits
// with the code "exit=<n>" asks for. Otherwise it runs the tests.
func TestMain(m *testing.M) {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), exeSuffix())
	for _, p := range Programs {
		if name == p {
			os.Exit(fakeProgram(p, os.Args[1:]))
		}
	}
	if dir := os.Getenv("KTW_MCP_TEST_SERVE"); dir != "" {
		srv, err := New(Options{Dir: dir, Version: "test"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeActions answer the actions another program serves, by the command line:
// the output and true, or false when the line is not theirs.
var fakeActions []func(line string) (string, bool)

func fakeProgram(name string, args []string) int {
	exe, _ := os.Executable()
	if f, err := os.OpenFile(filepath.Join(filepath.Dir(exe), "calls.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(f, "%s %s\n", name, strings.Join(args, " "))
		f.Close()
	}
	if name == "katyoti" && strings.Join(args, " ") == "profile show --json" {
		fmt.Println(`{"id":"jdoe","source":"local"}`)
		return 0
	}
	// an action another program serves, as ktw-agents' action run answers it
	line := strings.Join(args, " ")
	for _, answer := range fakeActions {
		if out, ok := answer(line); ok {
			fmt.Println(out)
			return 0
		}
	}
	fmt.Printf("%s says: %s\nCLAUDECODE=%s\n%s=%s\n", name, strings.Join(args, " "), os.Getenv("CLAUDECODE"), RelayedActEnv, os.Getenv(RelayedActEnv))
	for _, a := range args {
		var code int
		if _, err := fmt.Sscanf(a, "exit=%d", &code); err == nil {
			fmt.Fprintf(os.Stderr, "%s failed on purpose\n", name)
			return code
		}
	}
	return 0
}

// installation copies the test binary beside an allow-list under the names of
// the programs given, and returns the folder.
func installation(t *testing.T, allow string, programs ...string) string {
	t.Helper()
	dir := t.TempDir()
	self, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range programs {
		if err := os.WriteFile(filepath.Join(dir, p+exeSuffix()), self, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if allow != "" {
		if err := os.WriteFile(filepath.Join(dir, AllowListFile), []byte(allow), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// client drives a server process over its stdin and stdout.
type client struct {
	t   *testing.T
	in  io.WriteCloser
	out *bufio.Scanner
	cmd *exec.Cmd
	id  int
}

func startServer(t *testing.T, dir string) *client {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CLAUDECODE=") {
			env = append(env, kv) // the server must mark its programs itself
		}
	}
	cmd.Env = append(env, "KTW_MCP_TEST_SERVE="+dir)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &client{t: t, in: in, out: bufio.NewScanner(out), cmd: cmd}
	c.out.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	t.Cleanup(func() {
		in.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("server exited: %v", err)
			}
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			t.Error("server did not end when its stdin closed")
		}
	})
	return c
}

func (c *client) notify(method string) {
	c.t.Helper()
	fmt.Fprintf(c.in, `{"jsonrpc":"2.0","method":%q}`+"\n", method)
}

// call sends one request and reads its reply.
func (c *client) call(method string, params any) map[string]any {
	c.t.Helper()
	c.id++
	msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	if _, err := c.in.Write(append(msg, '\n')); err != nil {
		c.t.Fatal(err)
	}
	if !c.out.Scan() {
		c.t.Fatalf("%s: no reply (%v)", method, c.out.Err())
	}
	var reply map[string]any
	if err := json.Unmarshal(c.out.Bytes(), &reply); err != nil {
		c.t.Fatalf("%s: reply is not JSON: %s", method, c.out.Text())
	}
	if id, _ := reply["id"].(float64); int(id) != c.id {
		c.t.Fatalf("%s: reply to id %v, want %d", method, reply["id"], c.id)
	}
	return reply
}

// tool calls a tool and returns its text and isError.
func (c *client) tool(name string, args ...string) (string, bool) {
	c.t.Helper()
	reply := c.call("tools/call", map[string]any{"name": name, "arguments": map[string]any{"args": args}})
	res, ok := reply["result"].(map[string]any)
	if !ok {
		c.t.Fatalf("tools/call %s %v: no result: %v", name, args, reply)
	}
	content := res["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	isErr, _ := res["isError"].(bool)
	return text, isErr
}

const testAllow = `channel: test
programs:
  katyoti:
    refuse:
      - verb: [universe, stop]
        why: stopping a universe is not done from a session
`

// TestServer_OverStdio drives a server process as Claude Code does: the
// handshake, one tool per program, a read verb answered with the program's
// output, refusals by the channel's allow-list and of the person's acts, and a
// failing command.
func TestServer_OverStdio(t *testing.T) {
	dir := installation(t, testAllow, "ktw-agents", "katyoti", "ktw-compose")
	c := startServer(t, dir)

	init := c.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "0"}})
	res := init["result"].(map[string]any)
	if res["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion = %v", res["protocolVersion"])
	}
	if info := res["serverInfo"].(map[string]any); info["name"] != "ktw-mcp" || info["version"] != "test" {
		t.Errorf("serverInfo = %v", info)
	}
	if _, ok := res["capabilities"].(map[string]any)["tools"]; !ok {
		t.Errorf("no tools capability: %v", res["capabilities"])
	}
	c.notify("notifications/initialized")

	list := c.call("tools/list", map[string]any{})
	var names []string
	for _, tl := range list["result"].(map[string]any)["tools"].([]any) {
		tm := tl.(map[string]any)
		if _, isAct := actNamed(tm["name"].(string)); isAct {
			continue // the person's acts: TestThePersonsActsAskThePersonAndRelay
		}
		names = append(names, tm["name"].(string))
		schema := tm["inputSchema"].(map[string]any)
		if _, ok := schema["properties"].(map[string]any)["args"]; !ok {
			t.Errorf("tool %s takes no args: %v", tm["name"], schema)
		}
	}
	if strings.Join(names, ",") != "ktw-agents,katyoti,ktw-compose" {
		t.Fatalf("tools = %v, want one per program", names)
	}

	// A read verb: the program's own output, its exit code, run as an agent's session.
	text, isErr := c.tool("ktw-agents", "requests", "list", "--module", "katyoti")
	if isErr || !strings.HasPrefix(text, "exit: 0\n") || !strings.Contains(text, "ktw-agents says: requests list --module katyoti") {
		t.Errorf("read verb: isError=%v text=%q", isErr, text)
	}
	if !strings.Contains(text, "CLAUDECODE=1") {
		t.Errorf("the program did not run as an agent's session: %q", text)
	}
	if text, isErr := c.tool("ktw-compose", "scenario", "list"); isErr || !strings.Contains(text, "ktw-compose says: scenario list") {
		t.Errorf("ktw-compose: isError=%v text=%q", isErr, text)
	}

	// The channel's allow-list refuses, with its reason, and nothing runs.
	text, isErr = c.tool("katyoti", "--hub", "keep", "universe", "stop", "dev")
	if !isErr || !strings.Contains(text, "refused on test") || !strings.Contains(text, "stopping a universe") {
		t.Errorf("allow-list refusal: isError=%v text=%q", isErr, text)
	}
	if text, isErr := c.tool("katyoti", "universe", "list"); isErr {
		t.Errorf("an allowed katyoti verb was refused: %q", text)
	}

	// The person's acts are not served, whatever the allow-list says.
	person := `{"id":"jdoe","kind":"person"}`
	for _, args := range [][]string{
		{"todos", "report", "--id", "x", "--by", person, "--status", "success"},
		{"--hub", "keep", "inbox", "answer", "--id", "x"},
		{"action", "run", "--uri=ktw-methods:decisions/resolve", "--input", "{}"},
		{"requests", "verify", "--id", "x"},
	} {
		text, isErr := c.tool("ktw-agents", args...)
		if !isErr || !strings.Contains(text, "the person's act") {
			t.Errorf("person's act %v: isError=%v text=%q", args, isErr, text)
		}
	}
	// An agent reports its own to-do.
	if text, isErr := c.tool("ktw-agents", "todos", "report", "--id", "x", "--by", `{"id":"a","kind":"agent"}`); isErr {
		t.Errorf("an agent's report was refused: %q", text)
	}

	// A failing command: its exit code and stderr, as an error result.
	text, isErr = c.tool("ktw-agents", "exit=3")
	if !isErr || !strings.HasPrefix(text, "exit: 3\n") || !strings.Contains(text, "failed on purpose") {
		t.Errorf("failing command: isError=%v text=%q", isErr, text)
	}

	// An unknown tool and an unknown method are protocol errors.
	if r := c.call("tools/call", map[string]any{"name": "ktw-hub", "arguments": map[string]any{"args": []string{}}}); r["error"] == nil {
		t.Errorf("unknown tool answered: %v", r)
	}
	if r := c.call("resources/list", map[string]any{}); r["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Errorf("unknown method: %v", r)
	}

	// Nothing refused ever ran.
	log, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
	for _, never := range []string{"universe stop", "inbox answer", "decisions/resolve", "requests verify", person} {
		if strings.Contains(string(log), never) {
			t.Errorf("a refused call ran (%s):\n%s", never, log)
		}
	}
}

// TestNew_ServesOnlyTheProgramsBesideIt: a program whose binary is not in the
// installation, or that its allow-list switches off, has no tool.
func TestNew_ServesOnlyTheProgramsBesideIt(t *testing.T) {
	dir := installation(t, "programs:\n  katyoti:\n    expose: false\n", "ktw-agents", "katyoti")
	s, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.programs, ","); got != "ktw-agents" {
		t.Errorf("served = %s, want ktw-agents", got)
	}
}

// TestNew_RefusesAnEmptyRule: a rule naming neither a verb nor a flag would
// refuse everything; the allow-list is refused instead.
func TestNew_RefusesAnEmptyRule(t *testing.T) {
	dir := installation(t, "programs:\n  ktw-agents:\n    refuse:\n      - why: oops\n")
	if _, err := New(Options{Dir: dir}); err == nil {
		t.Fatal("an empty rule was accepted")
	}
}

// TestRefusal_VerbNotHiddenByAFlag: a value-less flag before a verb never hides
// it from the person's acts.
func TestRefusal_VerbNotHiddenByAFlag(t *testing.T) {
	s := &Server{}
	for _, args := range [][]string{
		{"--verbose", "requests", "hold", "--id", "x"},
		{"requests", "--out", "json", "waive", "--id", "x"},
		{"--", "inbox", "dismiss"},
	} {
		if s.refusal("ktw-agents", args) == "" {
			t.Errorf("%v was not refused", args)
		}
	}
	for _, args := range [][]string{
		{"requests", "list", "--module", "katyoti"},
		{"requests", "list", "--state", "hold"},
		{"todos", "list", "--module", "requests", "--step", "verify"},
	} {
		if why := s.refusal("ktw-agents", args); why != "" {
			t.Errorf("%v was refused: %s", args, why)
		}
	}
}

// TestDevChannelAllowList: the dev channel's file, as make puts it beside
// bin/ktw-mcp, loads and refuses stopping a universe; reading verbs run.
func TestDevChannelAllowList(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "channels", "dev.allow.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := installation(t, string(data), "katyoti")
	s, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if s.allow.Channel != "dev" {
		t.Errorf("channel = %q", s.allow.Channel)
	}
	if why := s.refusal("katyoti", []string{"universe", "stop", "dev"}); !strings.Contains(why, "refused on dev") {
		t.Errorf("universe stop: %q", why)
	}
	if why := s.refusal("katyoti", []string{"universe", "list"}); why != "" {
		t.Errorf("universe list refused: %s", why)
	}
}
