// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/katyoti/katyoti-fwk/internal/appcore"
)

// CoreInfraExt contributes the core infrastructure for ktw-agents.
var CoreInfraExt = appcore.DeclareInfra("ktw-agents",
	appcore.WithConfigDirs("config", "skills", "tasks", "sessions"),
	appcore.WithConfigFiles("Settings.yaml", "LLMProviders.yaml", "Plugins.yaml", "LLMProfiles.yaml"),
	appcore.WithAPIKeys(
		appcore.APIKeyDef{EnvName: "ANTHROPIC_API_KEY", Label: "Anthropic API key"},
		appcore.APIKeyDef{EnvName: "OPENAI_API_KEY", Label: "OpenAI API key"},
	),
)
