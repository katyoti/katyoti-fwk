// SPDX-License-Identifier: Apache-2.0

// Package local wires the scenario test cases into a binary: the `scenario`
// command (ktw-compose carries it). It needs the orchestrator service, which
// ktw-compose hosts in-process.
package local

import (
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/commands"
)

// Extension contributes the `scenario` command.
func Extension() application.Extension {
	return appcore.CommandExt(commands.NewScenarioCommand())
}
