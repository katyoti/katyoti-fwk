// SPDX-License-Identifier: Apache-2.0

package journal

import "errors"

// PickSession: the interactive session picker is not part of the published
// build (it needs a terminal UI library the published build leaves out);
// the session is named on the command line instead.
func PickSession(sessions []SessionInfo) (string, error) {
	return "", errors.New("the interactive session picker is not part of this build: name the session (its id or the end of its id) as the argument")
}
