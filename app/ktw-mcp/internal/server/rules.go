// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// AllowListFile is the channel's allow-list, beside the server: what this
// installation's server refuses on top of the person's acts.
const AllowListFile = "ktw-mcp.allow.yaml"

// Rule refuses a command line. It matches when the line's verb starts with its
// verb words (flags and their values may come before and between them), every
// flag it names carries the value given, and, when By is set, the --by actor is
// of that kind.
type Rule struct {
	Verb []string          `yaml:"verb"`
	Flag map[string]string `yaml:"flag,omitempty"`
	By   string            `yaml:"by,omitempty"`
	Why  string            `yaml:"why"`
}

// ProgramPolicy is what the allow-list says of one program.
type ProgramPolicy struct {
	// Expose false takes the program's tool away even when its binary is here.
	Expose *bool  `yaml:"expose,omitempty"`
	Refuse []Rule `yaml:"refuse,omitempty"`
}

// AllowList is the file's content.
type AllowList struct {
	Channel  string                   `yaml:"channel,omitempty"`
	Programs map[string]ProgramPolicy `yaml:"programs,omitempty"`
}

// personActs are the person's own acts: refused through the program tools
// whatever the channel's allow-list says - they are done through the act tools
// (acts.go), each asking the person first.
var personActs = map[string][]Rule{
	"ktw-agents": {
		{Verb: []string{"todos", "report"}, By: "person", Why: "reporting a person's to-do is the person's act"},
		{Verb: []string{"action", "run"}, Flag: map[string]string{"--uri": "ktw-methods:decisions/resolve"}, Why: "deciding a decision is the person's act"},
		{Verb: []string{"inbox", "answer"}, Why: "answering an Inbox entry is the person's act"},
		{Verb: []string{"inbox", "dismiss"}, Why: "dismissing an Inbox entry is the person's act"},
		{Verb: []string{"requests", "waive"}, Why: "waiving a module update is the person's act"},
		{Verb: []string{"requests", "hold"}, Why: "putting an item on hold is the person's act"},
		{Verb: []string{"requests", "unhold"}, Why: "resuming an item on hold is the person's act"},
		{Verb: []string{"requests", "test"}, Why: "testing a deliverable by hand is the person's act"},
		{Verb: []string{"requests", "verify"}, Why: "verifying a deliverable by hand is the person's act"},
	},
}

// loadAllowList reads the channel's allow-list beside the server; none is an
// empty one (the person's acts are still refused).
func loadAllowList(dir string) (AllowList, error) {
	var al AllowList
	path := filepath.Join(dir, AllowListFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return al, nil
	}
	if err != nil {
		return al, fmt.Errorf("ktw-mcp: read %s: %w", path, err)
	}
	if err := config.DecodeBytes(data, marshal.YAML, &al); err != nil {
		return al, fmt.Errorf("ktw-mcp: decode %s: %w", path, err)
	}
	for prog, p := range al.Programs {
		for i, r := range p.Refuse {
			if len(r.Verb) == 0 && len(r.Flag) == 0 {
				return al, fmt.Errorf("ktw-mcp: %s: programs.%s.refuse[%d] names neither a verb nor a flag", path, prog, i)
			}
		}
	}
	return al, nil
}

// refusal answers why args are refused for program, or "" when they run.
func (s *Server) refusal(program string, args []string) string {
	line := parseLine(args)
	for _, r := range personActs[program] {
		if r.matches(line) {
			return fmt.Sprintf("refused: %s - not exposed through the MCP server's program tools; use its act tool, which asks the person first, or the person does it in the Inbox or the request view", r.Why)
		}
	}
	return s.allowListRefusal(program, line)
}

// allowListRefusal is why the channel's allow-list refuses line, or "".
func (s *Server) allowListRefusal(program string, line line) string {
	for _, r := range s.allow.Programs[program].Refuse {
		if r.matches(line) {
			ch := s.allow.Channel
			if ch == "" {
				ch = "this installation"
			}
			why := r.Why
			if why == "" {
				why = "refused by the allow-list"
			}
			return fmt.Sprintf("refused on %s: %s (%s %s, %s)", ch, why, program, strings.Join(r.Verb, " "), AllowListFile)
		}
	}
	return ""
}

// line is a command line read for matching: its words and its flags.
type line struct {
	words []word
	flags map[string]string
}

// word is a token that is not a flag. A word right after a flag given without
// "=" may be that flag's value or the next word: the server does not know the
// programs' flags, so both readings are tried.
type word struct {
	text       string
	maybeValue bool
}

// parseLine reads args for matching. A flag "--x=v" carries v; a flag "--x"
// followed by a word carries that word (and the word stays a word too).
func parseLine(args []string) line {
	l := line{flags: map[string]string{}}
	afterFlag := false
	for i, a := range args {
		if a == "--" {
			for _, w := range args[i+1:] {
				l.words = append(l.words, word{text: w})
			}
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			l.words = append(l.words, word{text: a, maybeValue: afterFlag})
			afterFlag = false
			continue
		}
		name, val, hasVal := strings.Cut(a, "=")
		afterFlag = !hasVal
		if !hasVal && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			val = args[i+1]
		}
		l.flags[name] = val
	}
	return l
}

func (r Rule) matches(l line) bool {
	if len(r.Verb) > 0 && !leadsWith(l.words, r.Verb) {
		return false
	}
	for name, want := range r.Flag {
		got, ok := l.flags[name]
		if !ok || got != want {
			return false
		}
	}
	if r.By != "" {
		var actor struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal([]byte(l.flags["--by"]), &actor); err != nil || actor.Kind != r.By {
			return false
		}
	}
	return true
}

// leadsWith reports whether the command's verb is verb: its words, in order,
// at the start of the line, with nothing between or before them but words
// that may be flag values ("--hub keep requests hold", "requests --out json
// waive"). Where a word reads both ways, the reading that matches wins: the
// matching errs towards refusing.
func leadsWith(words []word, verb []string) bool {
	if len(verb) == 0 {
		return true
	}
	for i, w := range words {
		if w.text == verb[0] && leadsWith(words[i+1:], verb[1:]) {
			return true
		}
		if !w.maybeValue {
			return false
		}
	}
	return false
}
