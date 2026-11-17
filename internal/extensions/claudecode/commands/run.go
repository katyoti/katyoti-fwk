// SPDX-License-Identifier: Apache-2.0

// Package commands holds the run command: one Claude Code worker in the
// foreground — its conversation printed, its permissions asked on the
// terminal — ended when its turn is done.
package commands

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/api"
)

// NewRunCommand is `run`.
func NewRunCommand() *cobra.Command {
	var in api.StartInput
	var keep bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one Claude Code worker in the foreground on the person's subscription",
		Long: `Starts a Claude Code worker on a prompt, prints its conversation and asks its
permissions here: [y] allow once, [s] allow for the session, [n] deny. The worker
ends when its turn is done (or keeps taking prompts with --keep). An API key is
refused: the worker runs on the person's own Claude subscription.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := application.UseService[api.ClaudeCode](cmd.Context())
			if err != nil {
				return err
			}
			return Run(cmd.Context(), svc, in, keep, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.Prompt, "prompt", "", "the first user message")
	f.StringVar(&in.Cwd, "cwd", "", "the working directory (default: the current one)")
	f.StringVar(&in.Model, "model", "", "the model (default: Claude Code's)")
	f.StringVar(&in.PermissionMode, "permission-mode", "", "default, acceptEdits, plan, dontAsk or auto (default: ClaudeCode.yaml's, auto)")
	f.StringVar(&in.Name, "name", "", "the session's display name")
	f.StringVar(&in.Resume, "resume", "", "a Claude Code session id to continue")
	f.IntVar(&in.MaxTurns, "max-turns", 0, "the most agentic turns per prompt")
	f.BoolVar(&in.RemoteControl, "remote-control", false, "open the worker to claude.ai/code and the Claude app (Remote Control): watch and drive it there too")
	f.BoolVar(&keep, "keep", false, "after a turn, read the next prompt from the terminal (an empty line ends)")
	_ = cmd.MarkFlagRequired("prompt")
	return cmd
}

// Run drives one worker in the foreground.
func Run(ctx context.Context, svc api.ClaudeCode, in api.StartInput, keep bool, stdin io.Reader, out io.Writer) error {
	fol, ok := svc.(api.Follower)
	if !ok {
		return fmt.Errorf("claude: run needs the in-process service")
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	w, err := svc.Start(ctx, in)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "worker %s - Claude Code %s, %s\n", w.ID, w.ClaudeVersion, account(w.Account))
	if w.RemoteURL != "" {
		fmt.Fprintf(out, "Remote Control: watch and drive it at %s (or in the Claude app)\n", w.RemoteURL)
	} else if strings.HasPrefix(w.RemoteState, "refused") {
		fmt.Fprintf(out, "Remote Control %s\n", w.RemoteState)
	}
	past, events, cancel, err := fol.Follow(w.ID)
	if err != nil {
		return err
	}
	defer cancel()
	lines := bufio.NewScanner(stdin)
	end := func() error {
		_, _ = svc.Stop(context.WithoutCancel(ctx), api.WorkerInput{ID: w.ID})
		i, _ := svc.Get(context.WithoutCancel(ctx), api.WorkerInput{ID: w.ID})
		fmt.Fprintf(out, "worker %s %s - %d turn(s), $%.4f\n", i.ID, i.State, i.Turns, i.CostUSD)
		if i.State == api.StateCrashed {
			return fmt.Errorf("claude: %s", i.LastError)
		}
		return nil
	}
	handle := func(e api.Event) (done bool, err error) {
		switch e.Kind {
		case api.EventText:
			fmt.Fprintf(out, "Claude: %s\n", e.Text)
		case api.EventToolUse:
			fmt.Fprintf(out, "  > %s %s\n", e.Tool, short(e.Input))
		case api.EventToolResult:
			mark := "ok"
			if e.IsError {
				mark = "error"
			}
			fmt.Fprintf(out, "  < %s %s\n", mark, shortText(e.Text))
		case api.EventPermission:
			p := e.Permission
			fmt.Fprintf(out, "Permission: %s %s\n  [y] allow once  [s] allow for the session  [n] deny > ", p.ToolName, short(p.Input))
			ans := api.AnswerInput{ID: w.ID, RequestID: p.RequestID, Behavior: api.BehaviorDeny, Message: "Denied on the terminal."}
			if lines.Scan() {
				switch strings.ToLower(strings.TrimSpace(lines.Text())) {
				case "y", "yes":
					ans = api.AnswerInput{ID: w.ID, RequestID: p.RequestID, Behavior: api.BehaviorAllow, Scope: api.ScopeOnce}
				case "s", "session":
					ans = api.AnswerInput{ID: w.ID, RequestID: p.RequestID, Behavior: api.BehaviorAllow, Scope: api.ScopeSession}
				}
			}
			if _, err := svc.Answer(ctx, ans); err != nil {
				fmt.Fprintf(out, "  (not answered: %v)\n", err)
			}
		case api.EventResult:
			fmt.Fprintf(out, "Result ($%.4f): %s\n", e.CostUSD, e.Text)
			if !keep {
				return true, nil
			}
			fmt.Fprint(out, "next prompt (empty ends) > ")
			if !lines.Scan() || strings.TrimSpace(lines.Text()) == "" {
				return true, nil
			}
			if _, err := svc.Send(ctx, api.SendInput{ID: w.ID, Prompt: lines.Text()}); err != nil {
				return true, err
			}
		case api.EventState:
			if e.State.Ended() {
				return true, nil
			}
		case api.EventRemote:
			if e.Text == "connected" || e.Text == "disconnected" || e.Text == "failed" {
				fmt.Fprintf(out, "Remote Control %s\n", e.Text)
			}
		case api.EventRateLimit:
			if e.Text != "allowed" {
				fmt.Fprintf(out, "rate limit: %s\n", e.Text)
			}
		}
		return false, nil
	}
	for _, e := range past {
		if done, err := handle(e); done || err != nil {
			if err != nil {
				return err
			}
			return end()
		}
	}
	for {
		select {
		case e, ok := <-events:
			if !ok {
				return end()
			}
			if done, err := handle(e); done || err != nil {
				if err != nil {
					return err
				}
				return end()
			}
		case <-ctx.Done():
			fmt.Fprintln(out, "interrupted")
			_, _ = svc.Interrupt(context.WithoutCancel(ctx), api.WorkerInput{ID: w.ID})
			return end()
		}
	}
}

func account(a *api.Account) string {
	if a == nil {
		return "account not reported"
	}
	return fmt.Sprintf("%s subscription (%s)", a.SubscriptionType, a.Email)
}

func short(raw json.RawMessage) string { return shortText(string(raw)) }

func shortText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		return s[:157] + "..."
	}
	return s
}
