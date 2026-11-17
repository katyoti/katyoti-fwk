// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// installationLock answers the name of this process's installation when it
// is locked (katyoti.yml `locked: true`). A variable so a test can lock one.
var installationLock = func() (string, bool) {
	if inst := cenv.CurrentInstallation(); inst != nil && inst.Locked {
		return inst.Name, true
	}
	return "", false
}

// refuseWhenLocked refuses a move of a version on a locked installation:
// nothing updates it by itself, a version changes only when a release is
// installed on it.
func refuseWhenLocked(what string) error {
	if name, locked := installationLock(); locked {
		return fmt.Errorf("%w: %s not done - the installation %s is locked: its versions move only when a release is installed on it (katyoti config set locked off unlocks it)",
			mktapi.ErrLocked, what, name)
	}
	return nil
}
