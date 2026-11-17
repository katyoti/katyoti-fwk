// SPDX-License-Identifier: Apache-2.0

package console

import "github.com/spf13/cobra"

// outputDefaultAnnotation is the cobra-annotation key used to store a
// group-level default OutputMode. SetGroupDefault writes it on a
// command; AddOutputFlag walks the parent chain at flag-creation time
// to find the nearest annotation.
const outputDefaultAnnotation = "console.outputDefault"

// SetGroupDefault overrides the default OutputMode for cmd and every
// descendant (subcommands inherit unless they call SetGroupDefault
// themselves). Pass console.OutputJSON on the `llm` group to make
// every `llm <verb>` default to JSON without users needing `--out
// json`.
//
// Per the command-surface decision. The user can always override
// explicitly via `--out <mode>`.
func SetGroupDefault(cmd *cobra.Command, mode OutputMode) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[outputDefaultAnnotation] = string(mode)
}

// resolveDefaultOutputMode walks the cobra parent chain looking for a
// SetGroupDefault annotation. Returns OutputRich when no ancestor
// declares a default — preserving the pre-3L behavior.
func resolveDefaultOutputMode(cmd *cobra.Command) OutputMode {
	for c := cmd; c != nil; c = c.Parent() {
		if v, ok := c.Annotations[outputDefaultAnnotation]; ok && v != "" {
			return OutputMode(v)
		}
	}
	return OutputRich
}
