// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/app/ktw-utils/internal/journal"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/spf13/cobra"
)

var (
	followIntervalMs int
	followOnce       bool
)

func init() {
	sessionFollowCmd.Flags().IntVar(&followIntervalMs, "interval", 1500, "Refresh interval in milliseconds")
	sessionFollowCmd.Flags().BoolVar(&followOnce, "once", false, "Render a single frame and exit (no live loop) — for scripting/tests")
	sessionCmd.AddCommand(sessionFollowCmd)
}

var sessionFollowCmd = &cobra.Command{
	Use:   "follow [session-id]",
	Short: "Live-tail a running session's delegation tree + per-child progress",
	Long: `Watch a RUNNING session live: its delegation tree (parent → children), each
child's status / turns / tool activity, refreshing on an interval until the root
run completes (or Ctrl-C). READ-ONLY — it only reads the session journals on
disk; it never sends messages, answers prompts, or mutates the run.

If no session-id is given, follows the most recent top-level session.

  ktw-utils session follow                 # newest top-level run
  ktw-utils session follow <id>            # a specific run
  ktw-utils session follow <id> --once     # one frame (scripting)`,
	RunE: runSessionFollow,
}

func runSessionFollow(cmd *cobra.Command, args []string) error {
	sessionsDir := application.Env().SessionsDir()
	rootID := ""
	if len(args) > 0 {
		rootID = args[0]
	}

	// Ctrl-C → clean stop with a final frame.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	interval := time.Duration(followIntervalMs) * time.Millisecond

	render := func() (*journal.DelegationNode, bool, error) {
		tree, err := journal.BuildDelegationTree(sessionsDir, rootID)
		if err != nil {
			return nil, false, err
		}
		agg, _ := journal.AggregateTree(tree) // best-effort; nil-safe below
		byRun := map[string]journal.AgentMetrics{}
		if agg != nil {
			for _, m := range agg.PerAgent {
				byRun[m.RunID] = m
			}
		}
		done := sessionComplete(tree.Dir)
		clearScreen()
		fmt.Printf("● following %s   (refresh %dms — Ctrl-C to stop)\n", tree.SessionID, followIntervalMs)
		if done {
			fmt.Println("  status: COMPLETE")
		} else {
			fmt.Println("  status: running…")
		}
		fmt.Println(strings.Repeat("─", 60))
		fmt.Print(formatFollowTree(tree, byRun, 0))
		return tree, done, nil
	}

	for {
		_, done, err := render()
		if err != nil {
			return err
		}
		if done || followOnce {
			return nil
		}
		select {
		case <-sigCh:
			// Final frame already drawn; exit cleanly.
			_, _, _ = render()
			fmt.Println("\n(stopped)")
			return nil
		case <-time.After(interval):
		}
	}
}

// formatFollowTree renders the delegation tree with each node's live metrics
// (status / turns / tool-call count + the top tools), looked up by run id.
func formatFollowTree(node *journal.DelegationNode, byRun map[string]journal.AgentMetrics, depth int) string {
	if node == nil {
		return ""
	}
	prefix := strings.Repeat("  ", depth)
	branch := ""
	if depth > 0 {
		branch = "└─ "
	}
	mode := ""
	if node.Mode != "" {
		mode = " [" + node.Mode + "]"
	}
	out := fmt.Sprintf("%s%s%s%s\n", prefix, branch, node.Task, mode)

	if m, ok := byRun[node.RunID]; ok {
		tools := 0
		for _, c := range m.ToolsByName {
			tools += c
		}
		line := fmt.Sprintf("%s   %s · turns %d · tools %d", prefix, statusGlyph(m.Status), m.Turns, tools)
		if m.ErrorCount > 0 {
			line += fmt.Sprintf(" · errors %d", m.ErrorCount)
		}
		if top := topTools(m.ToolsByName, 3); top != "" {
			line += " · " + top
		}
		out += line + "\n"
	} else {
		out += fmt.Sprintf("%s   (starting…)\n", prefix)
	}
	for _, child := range node.Children {
		out += formatFollowTree(child, byRun, depth+1)
	}
	return out
}

func statusGlyph(status string) string {
	switch status {
	case "complete", "completed":
		return "✓ complete"
	case "failed", "error":
		return "✗ " + status
	case "", "running":
		return "▸ running"
	default:
		return status
	}
}

// topTools renders the n most-used tools as "file-read×35 glob×14 …".
func topTools(m map[string]int, n int) string {
	if len(m) == 0 {
		return ""
	}
	type kv struct {
		name string
		n    int
	}
	kvs := make([]kv, 0, len(m))
	for k, v := range m {
		kvs = append(kvs, kv{k, v})
	}
	// simple selection of the top n by count
	for i := 0; i < len(kvs); i++ {
		for j := i + 1; j < len(kvs); j++ {
			if kvs[j].n > kvs[i].n {
				kvs[i], kvs[j] = kvs[j], kvs[i]
			}
		}
	}
	parts := []string{}
	for i := 0; i < len(kvs) && i < n; i++ {
		parts = append(parts, fmt.Sprintf("%s×%d", kvs[i].name, kvs[i].n))
	}
	return strings.Join(parts, " ")
}

// sessionComplete reports whether the root session has reached a terminal state,
// by scanning its journal for a framework-emitted terminal event (task.output /
// session_end). summary.json (written at run end) is the fast path.
func sessionComplete(dir string) bool {
	if dir == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "summary.json")); err == nil {
		return true
	}
	f, err := os.Open(filepath.Join(dir, "llm.jsonl"))
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, `"type":"task_output"`) || strings.Contains(line, `"type":"session_end"`) {
			return true
		}
	}
	return false
}

// clearScreen moves the cursor home + clears, for a live-updating frame.
func clearScreen() {
	fmt.Print("\033[H\033[2J")
}
