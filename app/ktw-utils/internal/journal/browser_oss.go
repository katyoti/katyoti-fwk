// SPDX-License-Identifier: Apache-2.0

package journal

import "errors"

// Browse: the interactive journal browser is not part of the published build
// (it needs a terminal UI library the published build leaves out).
func Browse(conv *Conversation) error {
	return errors.New("the interactive journal browser is not part of this build: use --export to write the conversation to llm.md")
}
