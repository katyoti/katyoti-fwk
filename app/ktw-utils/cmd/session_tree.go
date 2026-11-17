// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/app/ktw-utils/internal/journal"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/spf13/cobra"
)

var treeJSON bool

func init() {
	sessionTreeCmd.Flags().BoolVar(&treeJSON, "json", false, "Output raw JSON")
	sessionCmd.AddCommand(sessionTreeCmd)
}

var sessionTreeCmd = &cobra.Command{
	Use:   "tree [session-id]",
	Short: "Show the delegation tree for a session",
	Long: `Reconstruct the parent → child delegation tree for a session by walking
session_start events across all session journals in the sessions directory.

If no session-id is given, uses the most recent top-level session (one with no parent).
Each node shows the run id, task name, mode, and start time.`,
	RunE: runSessionTree,
}

func runSessionTree(cmd *cobra.Command, args []string) error {
	sessionsDir := application.Env().SessionsDir()

	rootID := ""
	if len(args) > 0 {
		rootID = args[0]
	}

	tree, err := journal.BuildDelegationTree(sessionsDir, rootID)
	if err != nil {
		return fmt.Errorf("build delegation tree: %w", err)
	}

	if treeJSON {
		return printJSON(tree)
	}

	fmt.Print(formatTree(tree, 0))
	return nil
}

// formatTree renders a delegation tree as indented text.
func formatTree(node *journal.DelegationNode, depth int) string {
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
	out += fmt.Sprintf("%s   run: %s  session: %s  started: %s\n", prefix, node.RunID, node.SessionID, node.StartedAt)
	for _, child := range node.Children {
		out += formatTree(child, depth+1)
	}
	return out
}
