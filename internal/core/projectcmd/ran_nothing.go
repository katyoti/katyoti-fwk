// SPDX-License-Identifier: Apache-2.0

package projectcmd

import "strings"

// RanNoTest says why a test command that exited 0 proves nothing: a go test
// that ran no test (no package line, or each one "[no tests to run]" or "[no
// test files]") or a bun test that passed none — a -run naming a test not
// written yet, or renamed. "" for any other command, or one that ran tests.
// A request's deliverable check and an acceptance criterion's command probe
// both judge their command so: a check that runs nothing is never a pass.
// Leading "cd <dir> &&" steps are skipped; a command whose output another
// program reads - piped (go test … | grep -c …), sent to a file, or captured
// (out=$(go test …); grep …) - checks it itself: judged by its exit code alone.
// Only a pipe or a redirection outside quotes counts: the | of a quoted -run
// alternation (-run '^(TestA|TestB)$') is part of the regex, not a pipe.
func RanNoTest(command, out string) string {
	if pipedOrRedirected(command) {
		return ""
	}
	f := strings.Fields(command)
	for len(f) >= 3 && f[0] == "cd" && f[2] == "&&" {
		f = f[3:]
	}
	if len(f) < 2 || f[1] != "test" {
		return ""
	}
	switch f[0] {
	case "go":
		ran := false
		for _, l := range strings.Split(out, "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "ok ") || strings.HasPrefix(l, "ok\t") || strings.HasPrefix(l, "--- PASS") {
				if !strings.Contains(l, "[no tests to run]") {
					ran = true
				}
			}
		}
		if !ran {
			return "the go test ran no test: none matched (a test not written yet, or renamed) - a check that runs nothing proves nothing"
		}
	case "bun":
		if strings.Contains(out, " 0 pass") {
			return "the bun test passed no test: none matched - a check that runs nothing proves nothing"
		}
	}
	return ""
}

// pipedOrRedirected says whether a shell command holds a pipe or an output
// redirection outside single and double quotes; 2>&1 alone is no redirection
// (the output still reaches the reader). A backslash escapes the next byte
// outside single quotes. A | inside $(...) is a real pipe and counts.
func pipedOrRedirected(command string) bool {
	var quote byte
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case c == '\\':
			i++
		case quote == '"':
			if c == '"' {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '|':
			return true
		case c == '>':
			if !strings.HasPrefix(command[i:], ">&1") || i == 0 || command[i-1] != '2' {
				return true
			}
		}
	}
	return false
}
