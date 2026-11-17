// SPDX-License-Identifier: Apache-2.0

package bash

import "strings"

// CommandInterpretation is the semantic result of analyzing a command's exit code.
type CommandInterpretation struct {
	IsError bool
	Message string // human-readable; empty when exit code is unambiguously success
}

// commandSemantic interprets an exit code for a specific command.
type commandSemantic func(exitCode int, stdout, stderr string) CommandInterpretation

// commandSemantics maps base command names to their exit code interpretation.
// Commands not in this map use the default semantic (0 = success, else error).
//
// Reference: Claude Code commandSemantics.ts
var commandSemantics = map[string]commandSemantic{
	// grep / ripgrep: 0=matches found, 1=no matches, 2+=error
	"grep": grepSemantic,
	"rg":   grepSemantic,

	// find: 0=success, 1=partial (some dirs inaccessible), 2+=error
	"find": func(code int, _, _ string) CommandInterpretation {
		if code == 1 {
			return CommandInterpretation{IsError: false, Message: "Some directories were inaccessible"}
		}
		return defaultSemantic(code, "", "")
	},

	// diff: 0=no differences, 1=differences found, 2+=error
	"diff": func(code int, _, _ string) CommandInterpretation {
		if code == 1 {
			return CommandInterpretation{IsError: false, Message: "Files differ"}
		}
		return defaultSemantic(code, "", "")
	},

	// test / [: 0=true, 1=false, 2+=error
	"test": testSemantic,
	"[":    testSemantic,
}

func grepSemantic(code int, _, _ string) CommandInterpretation {
	if code == 1 {
		return CommandInterpretation{IsError: false, Message: "No matches found"}
	}
	if code >= 2 {
		return CommandInterpretation{IsError: true, Message: "grep error"}
	}
	return CommandInterpretation{IsError: false}
}

func testSemantic(code int, _, _ string) CommandInterpretation {
	if code == 1 {
		return CommandInterpretation{IsError: false, Message: "Condition is false"}
	}
	return defaultSemantic(code, "", "")
}

func defaultSemantic(code int, _, _ string) CommandInterpretation {
	if code == 0 {
		return CommandInterpretation{IsError: false}
	}
	return CommandInterpretation{IsError: true}
}

// InterpretResult interprets a command's exit code using command-specific
// semantics. The command string is parsed to extract the base command.
func InterpretResult(command string, exitCode int, stdout, stderr string) CommandInterpretation {
	base := extractBaseCommand(command)
	if sem, ok := commandSemantics[base]; ok {
		return sem(exitCode, stdout, stderr)
	}
	return defaultSemantic(exitCode, stdout, stderr)
}

// extractBaseCommand heuristically extracts the primary command name.
// For pipelines, takes the LAST command (determines exit code).
// For chains (&&, ||, ;), takes the last segment.
func extractBaseCommand(command string) string {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return ""
	}

	// Split on pipe — last command determines exit code.
	if idx := strings.LastIndex(cmd, "|"); idx >= 0 {
		// Make sure it's not || (logical OR).
		if idx+1 < len(cmd) && cmd[idx+1] == '|' {
			// It's ||, split on that instead.
		} else {
			cmd = strings.TrimSpace(cmd[idx+1:])
		}
	}

	// Split on chain operators — take the last segment.
	for _, sep := range []string{"&&", "||", ";"} {
		if idx := strings.LastIndex(cmd, sep); idx >= 0 {
			cmd = strings.TrimSpace(cmd[idx+len(sep):])
		}
	}

	// Skip env var assignments (VAR=val cmd).
	parts := strings.Fields(cmd)
	for i, p := range parts {
		if !strings.Contains(p, "=") || strings.HasPrefix(p, "-") {
			if i < len(parts) {
				return parts[i]
			}
			break
		}
	}

	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}
