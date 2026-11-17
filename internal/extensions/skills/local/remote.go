// SPDX-License-Identifier: Apache-2.0

package local

import (
	skillcore "github.com/katyoti/katyoti-fwk/internal/extensions/skills/internal/core"
)

// NewCompositeSkillManager creates a SkillManager that queries multiple sources.
// Re-exported from internal/core for use by bootstrap wiring.
//
// Note (2026-05-24): the previous re-exports `NewRemoteSkillManager` +
// `SenderFactory` were dropped along with the SkillManager RPC surface.
// Remote skill enumeration + L1 instruction fetches now ride on the
// catalog directly; references and standalone scripts go through
// ResourceService and the scripts slot respectively.
var NewCompositeSkillManager = skillcore.NewCompositeSkillManager
