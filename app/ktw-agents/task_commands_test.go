// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"testing"

	ktwcmd "github.com/katyoti/katyoti-fwk/app/ktw-agents/cmd"
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
	"github.com/spf13/cobra"
)

// A command an embedded task or skill names is one the binary has: a worker
// follows its verbs to the letter, and a verb that does not exist (« ktw-agents
// features add ») stops it on « unknown command ». Every `ktw-agents …` of a
// task or a skill - in a code span, or a code line starting with it - resolves
// in the command tree this binary builds (its own commands and the verbs
// generated from its services), a leaf command taking the words left only when
// its arguments allow them.
func assertTasksNameRealCommands(t *testing.T, generated *cobra.Command) {
	t.Helper()
	if generated == nil {
		return // the command line did not build: said already
	}
	// Execute adds `dirs` and `describe` once the application has booted,
	// out of the tree the census builds: named here as they ship.
	late := &cobra.Command{Use: "ktw-agents"}
	late.AddCommand(appcore.NewDirsCommand(), appcore.NewDescribeCommand(appcore.AppConfig{AppName: "ktw-agents"}))
	roots := []*cobra.Command{ktwcmd.RootCommand(), generated, late}
	checked := 0
	for _, d := range census.EmbeddedDocs(t) {
		for _, words := range census.CommandWords(d.Text) {
			checked++
			if why := unknownCommand(roots, words); why != "" {
				t.Errorf("%s names `ktw-agents %s`: %s", d.Path, strings.Join(words, " "), why)
			}
		}
	}
	if checked == 0 {
		t.Error("no `ktw-agents …` command found in the embedded tasks and skills: the extraction is broken")
	}
}

// unknownCommand says why words name no command of the roots, "" when one of
// them has it: each word is a subcommand of the one before until a command that
// takes no subcommand - the words left are its arguments, which its Args
// validator must accept (a misspelt trailing verb on a command that takes none
// is refused).
func unknownCommand(roots []*cobra.Command, words []string) string {
	why := "no command " + words[0]
	for _, root := range roots {
		cur := child(root, words[0])
		if cur == nil {
			continue
		}
		missing := ""
		rest := words[1:]
		for len(rest) > 0 && cur.HasSubCommands() {
			next := child(cur, rest[0])
			if next == nil {
				missing = "`" + cur.CommandPath() + "` has no verb " + rest[0]
				break
			}
			cur, rest = next, rest[1:]
		}
		if missing == "" && len(rest) > 0 {
			missing = refusedArgs(cur, rest)
		}
		if missing == "" {
			return ""
		}
		why = missing
	}
	return why
}

// refusedArgs says why cmd refuses rest as its arguments, "" when it takes
// them. The extraction stops at the first placeholder, so a command given
// fewer words than it needs is not refused: only one that refuses rest
// whatever placeholders could follow (too many words, a misspelt verb on a
// command that takes none).
func refusedArgs(cmd *cobra.Command, rest []string) string {
	if cmd.Args == nil {
		// No validator: cobra takes any word, so a misspelt verb would be
		// silently dropped. A command whose usage line names no argument
		// (`list`, not `parse <file>`) takes none.
		if strings.Contains(strings.TrimSpace(cmd.Use), " ") {
			return ""
		}
		return fmt.Sprintf("`%s` takes no argument (its usage names none), yet is given %s", cmd.CommandPath(), strings.Join(rest, " "))
	}
	args := append([]string(nil), rest...)
	var err error
	for k := 0; k <= 3; k++ {
		if err = cmd.Args(cmd, args); err == nil {
			return ""
		}
		args = append(args, "<placeholder>")
	}
	return fmt.Sprintf("`%s` takes no %s: %v", cmd.CommandPath(), strings.Join(rest, " "), cmd.Args(cmd, rest))
}

// child is cmd's subcommand named or aliased name, or nil.
func child(cmd *cobra.Command, name string) *cobra.Command {
	for _, c := range cmd.Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return c
		}
	}
	return nil
}
