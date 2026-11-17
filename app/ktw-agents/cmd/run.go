// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/redact"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	agentlocal "github.com/katyoti/katyoti-fwk/internal/extensions/agent/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/humangate"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tui"
	termadapter "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/adapter"
	termserver "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/server"
	"github.com/spf13/cobra"
)

// runConfig bundles CLI-resolved parameters for a task run.
// Type-specific config (profile, plugins, max_turns) is handled by the strategy —
// NOT by runConfig. TaskService passes raw config to the MachineFactory.
type runConfig struct {
	TaskName      string
	TaskSvc       taskapi.TaskService
	IO            *IOConfig
	SessionsDir   string
	TracePath     string
	WorkspaceRoot string
	// Instructions is the optional inline instructions string from the second
	// positional CLI argument. When set, it becomes the agent's initial user
	// message via the user_input key (same path delegated children use).
	// Takes precedence over --if/--is content for the user_input slot.
	Instructions string

	// DryRun activates dry-run mode: every spawned LLM/program child (agent,
	// task, human-gate, program) is replaced with a deterministic simulator.
	// Used to validate workflow topology + event flow at zero LLM cost.
	DryRun bool

	// DryTracePath, when set with DryRun, writes captured workflow/child
	// events as JSONL for regression comparisons or programmatic consumption.
	DryTracePath string

	// Target runs the task on a CWM object: when set, its id is injected into
	// the task's plugin context (via ConfigOverride["context"]) so the cwm
	// plugin renders the `<cwm_object_context>` block — the CLI equivalent of
	// launching a task from an object in the GUI.
	Target string

	// Console renders a READ-ONLY multi-pane console: one pane per task run
	// (parent + every delegated child), each pane the terminal-protocol
	// transcoding fed by a per-run translator. For observing headless / delegated
	// runs (the agent-as-human harness) that otherwise produce no visible output.
	Console bool
}

func init() {
	rootCmd.AddCommand(runCmd)

	// I/O flags
	runCmd.Flags().String("if", "", "Read input from file")
	runCmd.Flags().Bool("ic", false, "Read input from clipboard")
	runCmd.Flags().Bool("is", false, "Read input from stdin")
	runCmd.Flags().String("of", "", "Write output to file")
	runCmd.Flags().Bool("os", false, "Write output to stdout (default)")
	runCmd.Flags().Bool("raw", false, "Output raw text (no styling)")
	runCmd.Flags().Bool("pipe", false, "Shortcut for --is --os --raw")
	runCmd.Flags().String("input", "", "Task input parameters (validated against the task's input-schema): inline JSON, file path (.json/.yaml), or stdin")

	// Engine override flags
	runCmd.Flags().Int("max-turns", 0, "Override maximum agent turns")
	runCmd.Flags().String("workspace", "", "Override workspace directory")
	runCmd.Flags().String("target", "", "Target CWM object id — runs the task ON that object, like the GUI (the cwm plugin renders its context)")

	// Observability flags
	runCmd.Flags().String("trace", "", "Trace output: file path, or '-' for stderr (omit to disable)")
	runCmd.Flags().String("permission-profile", "", "Permission profile name (workspace-edit, plan-mode, full-access). Defaults to the value in PermissionProfiles.yaml.")

	// Dry-run (zero-LLM topology validation)
	runCmd.Flags().Bool("dry-run", false, "Replace LLM/program child spawns with simulator: validate topology + event flow at zero cost")
	runCmd.Flags().Bool("console", false, "Render a read-only multi-pane console — one pane per task run (parent + every delegated child). For observing headless/delegated runs live.")
	runCmd.Flags().String("dry-trace", "", "With --dry-run, write captured workflow/child events as JSONL to this file")
}

var runCmd = &cobra.Command{
	Use:   "run <task> [instructions]",
	Short: "Execute a task",
	Long: `Run a task by name. The task is loaded from embedded or user-defined tasks,
the LLM conversation loop is started, and tools are dispatched as needed.

The optional second positional argument provides inline instructions that become
the agent's initial user message (instead of the default "Please start the task.").
This is the same mechanism delegated children use to receive their input.

Engine:
  --max-turns <n>     Override maximum agent turns (default: 50)
  --workspace <path>  Override workspace directory

Input/Output:
  --if <file>     Read input from a file (content + filename available to the LLM)
  --ic            Read input from clipboard
  --is            Read input from stdin
  --of <file>     Write output to a file
  --os            Write output to stdout (default)
  --raw           Output raw text without styling
  --pipe          Shortcut for --is --os --raw
  --input <arg>   Task input parameters (schema-validated): inline JSON, file path (.json/.yaml), or stdin

Observability:
  --trace <file>  Write trace events to file (use '-' for stderr)

Dry-run (validate without burning LLM tokens):
  --dry-run             Replace agent/task/program/human-gate children with a deterministic
                        simulator. Workflow topology + event flow are exercised end-to-end
                        with zero LLM cost. Useful for validating a TASK.md before paying
                        for tokens, or as a smoke test for orchestration changes.
  --dry-trace <file>    With --dry-run, capture workflow/child events as JSONL.

Example:
  ktw-agents run specify-data-flow
  ktw-agents run goal-planner "Build a CRM for small businesses"
  ktw-agents run transform --if input.md --of output.pdf
  ktw-agents run summarize --pipe --max-turns=10
  ktw-agents run analyze --if report.md --input '{"format":"brief"}'
  ktw-agents run demo-delegate-magentic --dry-run --dry-trace events.jsonl

Note: when both [instructions] and --if are given, the inline instructions take
precedence as the agent's initial message; --if content is still available via
the input file path.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runTask,
}

func runTask(cmd *cobra.Command, args []string) error {
	taskName := args[0]
	var instructions string
	if len(args) >= 2 {
		instructions = args[1]
	}

	// Resolve I/O
	inputFile, _ := cmd.Flags().GetString("if")
	inputClipboard, _ := cmd.Flags().GetBool("ic")
	inputStdin, _ := cmd.Flags().GetBool("is")
	outputFilePath, _ := cmd.Flags().GetString("of")
	outputStdout, _ := cmd.Flags().GetBool("os")
	raw, _ := cmd.Flags().GetBool("raw")
	pipe, _ := cmd.Flags().GetBool("pipe")
	inputArg, _ := cmd.Flags().GetString("input")

	ioCfg, err := ResolveIO(inputFile, inputArg, inputClipboard, inputStdin, outputStdout, outputFilePath != "", outputFilePath, raw, pipe)
	if err != nil {
		return fmt.Errorf("resolve I/O: %w", err)
	}

	env := application.Env()
	ctx := cmd.Context()

	// Permission profile selection — overrides PermissionProfiles.yaml
	// default_profile when set. Stored in context so the kernel
	// permission gate (applyPermissionGate) can read it per-call.
	if profileName, _ := cmd.Flags().GetString("permission-profile"); profileName != "" {
		ctx = agentlocal.WithPermissionProfile(ctx, profileName)
	}

	// Resolve workspace
	workspaceFlag, _ := cmd.Flags().GetString("workspace")
	wsRoot := workspaceFlag
	if wsRoot == "" {
		wsRoot = env.WorkspaceDir()
	}
	if wsRoot == "" || wsRoot == "." {
		wsRoot, _ = os.Getwd()
	}
	workspaceRoot := filepath.Join(wsRoot, "ktw_workspace")

	sessionsDir := env.SessionsDir()
	tracePath, _ := cmd.Flags().GetString("trace")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	dryTracePath, _ := cmd.Flags().GetString("dry-trace")
	if dryTracePath != "" && !dryRun {
		return fmt.Errorf("--dry-trace requires --dry-run")
	}

	// Decide mode: TUI (interactive) vs pipe (non-interactive).
	// --dry-run forces pipe mode: the dry-run tracer + final report are only
	// wired in runPipe, and dry-run emits no chat content for the TUI to render.
	useTUI := tui.IsTerminal() && !ioCfg.IsPipe && !ioCfg.Raw && !dryRun

	taskSvc, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return fmt.Errorf("task service: %w", err)
	}

	target, _ := cmd.Flags().GetString("target")
	console, _ := cmd.Flags().GetBool("console")

	rc := &runConfig{
		TaskName:      taskName,
		TaskSvc:       taskSvc,
		IO:            ioCfg,
		SessionsDir:   sessionsDir,
		TracePath:     tracePath,
		WorkspaceRoot: workspaceRoot,
		Instructions:  instructions,
		DryRun:        dryRun,
		DryTracePath:  dryTracePath,
		Target:        target,
		Console:       console,
	}

	// Enable trace writer if --trace flag is set
	if tracePath != "" {
		tw, closer, err := openTraceWriter(tracePath)
		if err != nil {
			return fmt.Errorf("trace: %w", err)
		}
		if closer != nil {
			defer closer.Close()
		}
		// Trace files record raw LLM + tool I/O and are the artifact most
		// likely to be shared in bug reports — scrub secrets with the same
		// redactor the session journals use (Redaction.yaml; zero value =
		// enabled with built-ins).
		settings, _ := application.UseConfig[redact.RedactionSettings](ctx)
		ctx = agentapi.WithTraceWriter(ctx, redact.Writer(tw, redact.New(settings)))
	}

	// Read-only multi-pane console: one pane per task run. Independent of the
	// TUI/pipe split — it observes the run from the shared bus rather than
	// driving it.
	if rc.Console {
		return runConsole(ctx, rc)
	}

	// All task types go through TaskService
	if useTUI {
		return runTUI(ctx, rc)
	}
	return runPipe(ctx, rc)
}

// runTUI launches the Bubbletea TUI via the shared terminal client.
// ServeTask runs in a goroutine; RunTUI blocks on the Bubbletea loop.
func runTUI(ctx context.Context, rc *runConfig) error {
	serverT, clientT := termadapter.NewChannelPair(256)
	defer serverT.Close()

	opts := buildTaskOpts(rc)

	// Server side: Room + Translator + task execution in background.
	go func() {
		_ = termserver.ServeTask(ctx, serverT, opts)
		serverT.Close() // signal TUI that task is done
	}()

	// Client side: Bubbletea TUI consuming terminal events.
	return tui.RunTUI(ctx, clientT)
}

// runPipe runs the task in non-interactive pipe mode.
// Uses the terminal protocol internally but renders plain text output.
func runPipe(ctx context.Context, rc *runConfig) error {
	serverT, clientT := termadapter.NewChannelPair(256)
	defer serverT.Close()

	opts := buildTaskOpts(rc)
	opts.SkipHumanGate = true // pipe mode uses CLI prompter, not terminal protocol

	fmt.Printf("Running task: %s\n", rc.TaskName)
	fmt.Printf("Workspace: %s\n", rc.WorkspaceRoot)
	if rc.IO != nil && rc.IO.InputMode != InputNone {
		fmt.Printf("Input: %s\n", describeInput(rc.IO))
	}
	fmt.Println()

	// In pipe mode, override HumanGate with CLI prompter on stdin/stdout
	// BEFORE ServeTask overrides it with the terminal gate.
	// The pipe renderer handles prompts via stdin, not terminal protocol.
	var promptReader io.Reader = os.Stdin
	if rc.IO != nil && rc.IO.InputData != "" {
		promptReader = strings.NewReader(rc.IO.InputData + "\n")
	}
	prompter := humangate.NewCLIPrompter(promptReader, os.Stdout)
	if app := application.AppFromContext(ctx); app != nil {
		application.PutService[hgapi.HumanGate](app, "default", hgapi.HumanGate(prompter))
	}

	// Attach the dry-run tracer BEFORE ServeTask launches the workflow so we
	// catch the first child.started events. SubscribeAny on the application's
	// shared bus captures every workflow.* / child.* / ledger.* / messaging
	// topic without enumerating them.
	var tracer *dryRunTracer
	var unsubTracer func()
	if rc.DryRun {
		if bus, err := application.UseService[engine.EventBus](ctx); err == nil && bus != nil {
			tracer = newDryRunTracer()
			unsubTracer = tracer.Attach(bus)
		}
	}

	// Run task (blocks). Close serverT when done so clientT.Receive exits.
	errCh := make(chan error, 1)
	go func() {
		errCh <- termserver.ServeTask(ctx, serverT, opts)
		serverT.Close()
	}()

	// Render terminal events as plain text.
	ch, err := clientT.Receive(ctx)
	if err != nil {
		return err
	}
	for raw := range ch {
		renderPipeEvent(raw)
	}

	taskErr := <-errCh

	// Render dry-run output (and optionally dump JSONL) AFTER the task is done.
	if tracer != nil {
		if unsubTracer != nil {
			unsubTracer()
		}
		tracer.Render(os.Stdout, rc.TaskName)
		if rc.DryTracePath != "" {
			f, ferr := os.Create(rc.DryTracePath)
			if ferr != nil {
				fmt.Fprintf(os.Stderr, "warning: could not open --dry-trace file %q: %v\n", rc.DryTracePath, ferr)
			} else {
				_ = tracer.WriteJSONL(f)
				_ = f.Close()
			}
		}
	}

	return taskErr
}

// buildTaskOpts creates TaskOpts from the run config.
func buildTaskOpts(rc *runConfig) termserver.TaskOpts {
	var input map[string]any
	if rc.IO != nil && len(rc.IO.TaskInput) > 0 {
		input = make(map[string]any, len(rc.IO.TaskInput))
		for k, v := range rc.IO.TaskInput {
			input[k] = v
		}
	}
	if rc.Instructions != "" {
		if input == nil {
			input = map[string]any{}
		}
		input["user_input"] = rc.Instructions
	} else if rc.IO != nil && rc.IO.InputData != "" {
		if input == nil {
			input = map[string]any{}
		}
		input["user_input"] = rc.IO.InputData
	}
	opts := termserver.TaskOpts{
		TaskName:      rc.TaskName,
		WorkspaceRoot: rc.WorkspaceRoot,
		Input:         input,
		DryRun:        rc.DryRun,
	}
	// --target → `_target` in the plugin-context envelope. ServeTask
	// forwards ConfigOverride verbatim to TaskRunRequest.ConfigOverride, where
	// the "context" key is decoded onto AgentRunRequest.Context and exposed to
	// the cwm plugin. Same envelope the GUI threads via
	// StartTaskInput.Context.
	if rc.Target != "" {
		opts.ConfigOverride = map[string]any{
			"context": map[string]any{agentapi.PluginContextTarget: rc.Target},
		}
	}
	return opts
}

// renderPipeEvent prints a minimal text rendering of a terminal event.
func renderPipeEvent(raw json.RawMessage) {
	var probe struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return
	}
	switch probe.Type {
	case "msg.delta":
		var evt struct {
			Delta string `json:"delta"`
		}
		json.Unmarshal(raw, &evt)
		fmt.Print(evt.Delta)
	case "msg.done":
		fmt.Println()
	case "msg.new":
		// Handled by tool_render for now — pipe mode is minimal.
	}
}

// describeInput returns a human-readable description of the input source.
func describeInput(cfg *IOConfig) string {
	switch cfg.InputMode {
	case InputFile:
		return fmt.Sprintf("file: %s (%d chars)", cfg.InputFile, len(cfg.InputData))
	case InputClipboard:
		return fmt.Sprintf("clipboard (%d chars)", len(cfg.InputData))
	case InputStdin:
		return fmt.Sprintf("stdin (%d chars)", len(cfg.InputData))
	default:
		return "none"
	}
}

// openTraceWriter opens a trace destination.
// "-" means stderr, anything else is a file path. Returns the writer and an
// optional closer (nil for stderr). The file is opened with O_CREATE|O_TRUNC
// and each Write is followed by an fsync for real-time tail -f consumers.
func openTraceWriter(path string) (io.Writer, io.Closer, error) {
	if path == "-" {
		return os.Stderr, nil, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return &syncWriter{f: f}, f, nil
}

// syncWriter wraps an *os.File and calls Sync after every Write,
// ensuring trace lines are visible to tail -f immediately.
type syncWriter struct {
	f *os.File
}

func (w *syncWriter) Write(p []byte) (int, error) {
	n, err := w.f.Write(p)
	if err != nil {
		return n, err
	}
	_ = w.f.Sync()
	return n, nil
}
