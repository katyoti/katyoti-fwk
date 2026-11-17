// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// memberBeside checks that the repository at abs may join the members the
// universe attaches already. Nothing to check for the first
// member, nor for one attached already (re-attaching is idempotent). A member
// beside others - served by a CWM store of its own, joined to the others' by
// the store set under its key, cwmapi.ProjectKeyOf: its project
// object's id; before its first opening, the name of its seed, which CWM
// mints the object from before the store set joins it; else its
// folder's name, which the default object is named after:
//   - was created on this PC (until IN-1419 is delivered:
//     notCreatedHere);
//   - holds a key no other member holds: two stores of one key are one
//     project served twice;
//   - publishes its contributions under a token no other member publishes
//     under (projectfile.TokenConflict, members' tokens beside the sidecars').
func (s *Service) memberBeside(universe, abs string) error {
	var others []string
	for _, existing := range s.loadProjects(universe).Projects {
		if samePath(existing, abs) {
			return nil
		}
		others = append(others, existing)
	}
	if len(others) == 0 {
		return nil
	}
	// Until IN-1419, only a repository created on this PC.
	if err := s.notCreatedHere(universe, abs); err != nil {
		return err
	}
	key := cwmapi.ProjectKeyOf(abs)
	token := projectToken(abs)
	for _, other := range others {
		if cwmapi.ProjectKeyOf(other) == key {
			return fmt.Errorf("attach: universe %q already attaches a project %q (%s): two members of one key would be one project served twice - rename one project's id, or detach the other", universe, key, other)
		}
		if token != "" && token == projectToken(other) {
			return fmt.Errorf("attach: the member %s of universe %q publishes its contributions under the token %q too: name another in capabilities.contributions.provider", other, universe, token)
		}
	}
	return nil
}

// projectToken is the token a project publishes its contributions under, ""
// when it holds no contributions folder (it publishes nothing) or no project
// object.
func projectToken(dir string) string {
	if projectNameOf(dir) == "" {
		return ""
	}
	f, err := projectfile.Read(dir)
	if err != nil || f.ContributionsDir(dir) == "" {
		return ""
	}
	return f.Contributions.Provider
}
