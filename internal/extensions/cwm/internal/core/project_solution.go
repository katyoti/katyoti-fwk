// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// refuseMissingSolution refuses a project whose type needs a solution the
// universe was not installed from: the project type's `solution`
// against the universe's (<solution>@<version>), naming both. A universe with
// no solution is told the verb that gives it one (katyoti universe
// set-solution); a universe of another solution cannot take a
// second, so it is told to open the project in a universe of the right one.
// A project with no project object, an undeclared type or a type needing none
// opens. The refusal wraps api.ErrProjectRefused and reads `project refused:
// <reason>`; New keeps it on the service, which starts and answers it to every
// read and write of the project (Service.refused).
func refuseMissingSolution(projectRoot string, types []api.ObjectType, universeSolution, universe string) error {
	if projectRoot == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(projectRoot, api.ProjectFileName))
	if err != nil {
		return nil
	}
	var head struct {
		Type string `yaml:"type"`
	}
	if marshal.Unmarshal(raw, marshal.YAML, &head) != nil || head.Type == "" {
		return nil
	}
	for _, t := range types {
		if t.Name != head.Type || t.Solution == "" {
			continue
		}
		have, _, _ := strings.Cut(universeSolution, "@")
		if have == t.Solution {
			return nil
		}
		label := t.Label
		if label == "" {
			label = t.Name
		}
		if have == "" {
			if universe == "" {
				universe = "<universe>"
			}
			return fmt.Errorf("%w: this project is a %s, which needs the solution %s; this universe has none - give it the solution with `katyoti universe set-solution %s %s`, or open the project in a universe of %s",
				api.ErrProjectRefused, label, t.Solution, universe, t.Solution, t.Solution)
		}
		return fmt.Errorf("%w: this project is a %s, which needs the solution %s; this universe has %s - open it in a universe of %s",
			api.ErrProjectRefused, label, t.Solution, have, t.Solution)
	}
	return nil
}
