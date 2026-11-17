// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/app/ktw-utils/internal/journal"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/spf13/cobra"
)

var exportFlag bool

func init() {
	journalCmd.Flags().BoolVar(&exportFlag, "export", false, "Export session to llm.md instead of browsing")
	rootCmd.AddCommand(journalCmd)
}

var journalCmd = &cobra.Command{
	Use:   "journal [session-id]",
	Short: "Browse or export LLM session journals",
	Long: `Browse LLM session journals in a human-readable format.

Without arguments: shows a session picker, then launches the TUI browser.
With --export: writes the conversation to llm.md in the current directory.
With a session ID argument: opens that session directly.`,
	RunE: runJournal,
}

func runJournal(cmd *cobra.Command, args []string) error {
	sessionsDir := application.Env().SessionsDir()

	sessions, err := journal.ListSessions(sessionsDir)
	if err != nil {
		return fmt.Errorf("scan sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	var selectedDir string

	if len(args) > 0 {
		// Direct session ID
		for _, s := range sessions {
			if s.ID == args[0] || strings.HasSuffix(s.ID, "_"+args[0]) {
				selectedDir = s.Dir
				break
			}
		}
		if selectedDir == "" {
			return fmt.Errorf("session not found: %s", args[0])
		}
	} else {
		// Interactive picker
		picked, err := journal.PickSession(sessions)
		if err != nil {
			return err
		}
		selectedDir = picked
	}

	// Parse the conversation
	conv, err := journal.ParseConversation(selectedDir)
	if err != nil {
		return fmt.Errorf("parse session: %w", err)
	}
	conv.SessionID = filepath.Base(selectedDir)

	if exportFlag {
		return exportToFile(conv)
	}

	// TUI browse mode
	return journal.Browse(conv)
}

func exportToFile(conv *journal.Conversation) error {
	md := journal.RenderMarkdown(conv)
	filename := "llm.md"
	if err := os.WriteFile(filename, []byte(md), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filename, err)
	}
	fmt.Printf("Exported %d turns to %s\n", len(conv.Turns), filename)
	return nil
}
