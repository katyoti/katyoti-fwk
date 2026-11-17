// SPDX-License-Identifier: Apache-2.0

// Package server is the KaTyoti MCP server used like the command line
// : MCP over stdio - newline-delimited JSON-RPC 2.0 - serving
// one tool per program of the installation it belongs to. A tool takes the
// command line's arguments and runs that installation's binary, found beside
// the server, once per call: no binary is held open between calls, and none is
// ever looked up on PATH.
package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Programs are the programs of an installation the server can serve, in the
// order their tools are listed.
var Programs = []string{"ktw-agents", "katyoti", "ktw-compose"}

// protocolVersions are the MCP revisions the server speaks; the first is the
// one it answers when the client asks for one it does not know.
var protocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// Options configure a Server.
type Options struct {
	// Dir is the installation's folder: the programs run and the allow-list
	// read are the ones it holds.
	Dir     string
	Version string
	// Timeout bounds one command; zero is ten minutes.
	Timeout time.Duration
}

// Server serves the installation's programs as MCP tools.
type Server struct {
	opts     Options
	allow    AllowList
	programs []string // the served programs, in Programs' order
	exe      map[string]string

	mu  sync.Mutex // serialises replies
	out io.Writer

	personState
	clientRequests
}

// New reads the installation in opts.Dir: its programs present beside the
// server and its channel's allow-list.
func New(opts Options) (*Server, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	allow, err := loadAllowList(opts.Dir)
	if err != nil {
		return nil, err
	}
	s := &Server{opts: opts, allow: allow, exe: map[string]string{}}
	for _, p := range Programs {
		if pol, ok := allow.Programs[p]; ok && pol.Expose != nil && !*pol.Expose {
			continue
		}
		path := filepath.Join(opts.Dir, p+exeSuffix())
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			continue
		}
		s.programs = append(s.programs, p)
		s.exe[p] = path
	}
	return s, nil
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// Serve answers the messages read from in until it ends, writing replies to
// out. Calls run concurrently; each reply is one line.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	s.out = out
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var wg sync.WaitGroup
	for sc.Scan() {
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		var msg request
		if err := json.Unmarshal(raw, &msg); err != nil {
			s.reply(json.RawMessage("null"), nil, &rpcError{Code: -32700, Message: "parse error: " + err.Error()})
			continue
		}
		if len(msg.ID) == 0 || string(msg.ID) == "null" {
			continue // a notification: nothing to answer
		}
		if msg.Method == "" {
			s.answered(msg) // the client's answer to a request of the server's
			continue
		}
		if msg.Method == "tools/call" {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.handle(msg)
			}()
			continue
		}
		s.handle(msg)
	}
	wg.Wait()
	return sc.Err()
}

type request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	// Result and Error: the client's answer to a request of the server's.
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) reply(id json.RawMessage, result any, rerr *rpcError) {
	msg := map[string]any{"jsonrpc": "2.0", "id": id}
	if rerr != nil {
		msg["error"] = rerr
	} else {
		msg["result"] = result
	}
	data, err := json.Marshal(msg)
	if err != nil {
		data, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": rpcError{Code: -32603, Message: err.Error()}})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.out.Write(append(data, '\n'))
}

func (s *Server) handle(msg request) {
	switch msg.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
			Capabilities    struct {
				Elicitation json.RawMessage `json:"elicitation"`
			} `json:"capabilities"`
		}
		_ = json.Unmarshal(msg.Params, &p)
		s.asking.Lock()
		s.elicits = len(p.Capabilities.Elicitation) > 0 && string(p.Capabilities.Elicitation) != "null"
		s.asking.Unlock()
		version := protocolVersions[0]
		for _, v := range protocolVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		s.reply(msg.ID, map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "ktw-mcp", "version": s.opts.Version},
			"instructions":    s.instructions(),
		}, nil)
	case "ping":
		s.reply(msg.ID, map[string]any{}, nil)
	case "tools/list":
		s.reply(msg.ID, map[string]any{"tools": s.tools()}, nil)
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			s.reply(msg.ID, nil, &rpcError{Code: -32602, Message: "invalid params: " + err.Error()})
			return
		}
		if a, ok := actNamed(p.Name); ok && s.exe["ktw-agents"] != "" {
			s.reply(msg.ID, s.callAct(a, p.Arguments), nil)
			return
		}
		if _, ok := s.exe[p.Name]; !ok {
			s.reply(msg.ID, nil, &rpcError{Code: -32602, Message: fmt.Sprintf("unknown tool %q: this installation serves %s", p.Name, strings.Join(s.programs, ", "))})
			return
		}
		var args []string
		if list, ok := p.Arguments["args"].([]any); ok {
			for _, v := range list {
				args = append(args, fmt.Sprint(v))
			}
		}
		s.reply(msg.ID, s.call(p.Name, args), nil)
	default:
		s.reply(msg.ID, nil, &rpcError{Code: -32601, Message: "method not found: " + msg.Method})
	}
}

func (s *Server) instructions() string {
	ch := s.allow.Channel
	if ch == "" {
		ch = "this installation"
	}
	return fmt.Sprintf("KaTyoti's programs of %s (%s), one tool each: pass the command line's arguments after the program's name, exactly as on the command line. The person's own acts (decide, answer or dismiss an Inbox entry, report their to-do, test or verify by hand, hold or resume an item, waive a module update) are tools of their own, each asking the person first; the program tools refuse them.", ch, s.opts.Dir)
}

func (s *Server) tools() []map[string]any {
	tools := make([]map[string]any, 0, len(s.programs))
	for _, p := range s.programs {
		tools = append(tools, map[string]any{
			"name":        p,
			"description": fmt.Sprintf("Run a %s command of this KaTyoti installation, exactly as on the command line: args are the arguments after the program's name, e.g. [\"--help\"]. Returns the exit code and the output.", p),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"args": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "the command line after the program's name, one argument per item",
					},
				},
				"required": []string{"args"},
			},
		})
	}
	return append(tools, s.actTools()...)
}

// call runs one command of program, or refuses it.
func (s *Server) call(program string, args []string) map[string]any {
	if why := s.refusal(program, args); why != "" {
		return toolResult(why, true)
	}
	return s.run(program, args)
}

// run runs one command of program, with the environment entries extra adds
// (a relayed act's), and answers its exit code and output.
func (s *Server) run(program string, args []string, extra ...string) map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), s.opts.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.exe[program], args...)
	cmd.Dir = workDir()
	cmd.Env = append(childEnv(os.Environ()), extra...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	case err != nil:
		return toolResult(fmt.Sprintf("%s did not run: %v", program, err), true)
	}
	if ctx.Err() == context.DeadlineExceeded {
		stderr.WriteString(fmt.Sprintf("\nktw-mcp: stopped after %s\n", s.opts.Timeout))
		if code == 0 {
			code = -1
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exit: %d\n", code)
	b.Write(stdout.Bytes())
	if stderr.Len() > 0 {
		if stdout.Len() > 0 && !bytes.HasSuffix(stdout.Bytes(), []byte("\n")) {
			b.WriteByte('\n')
		}
		b.WriteString("--- stderr ---\n")
		b.Write(stderr.Bytes())
	}
	return toolResult(b.String(), code != 0)
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}

// workDir is where a command runs: the project Claude Code works in when it
// says so, else the server's own working folder.
func workDir() string {
	if d := os.Getenv("CLAUDE_PROJECT_DIR"); d != "" {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return ""
}

// childEnv is the environment a command runs with: the server's, marked as an
// agent's session (CLAUDECODE) - a call through this server is an agent's, so
// the services refuse the person's acts even where a rule here would miss one -
// and never carrying a relayed act of its own: only an act tool sets one.
func childEnv(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "CLAUDECODE=") && !strings.HasPrefix(kv, RelayedActEnv+"=") {
			out = append(out, kv)
		}
	}
	return append(out, "CLAUDECODE=1")
}
