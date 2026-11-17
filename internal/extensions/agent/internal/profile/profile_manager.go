// SPDX-License-Identifier: Apache-2.0

package profile

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/models/api"
)

// LLMProfiles resolves profile names to configured agent profiles
// and creates Model instances from profile + overrides.
type LLMProfiles interface {
	// ResolveProfile returns the profile for the given name (or default if empty).
	ResolveProfile(profileName string) (AgentProfile, error)

	// CreateModelFromProfile creates a Model from a profile's model reference + overrides.
	CreateModelFromProfile(profile AgentProfile, extraOverrides map[string]any) (api.Model, error)

	// DefaultProfileName returns the default profile name.
	DefaultProfileName() string

	// ModelCallLogging reports whether HTTP-level model-call logging is
	// enabled for the given profile (per-profile override, else the
	// config-level `log_model_calls` default).
	ModelCallLogging(profile AgentProfile) bool
}

// --- Implementation ---

type profileManager struct {
	profilesCfg  LLMProfilesConfig
	modelManager api.LLMProviders
}

// LLMProfilesExtension registers LLMProfiles as a DI service.
var LLMProfilesExtension = application.DeclareService[LLMProfiles, LLMProfilesConfig](
	func(ctx context.Context, cfg *LLMProfilesConfig) (LLMProfiles, error) {
		modelMgr, err := application.UseService[api.LLMProviders](ctx)
		if err != nil {
			return nil, fmt.Errorf("LLMProfiles: resolve LLMProviders: %w", err)
		}
		return &profileManager{
			profilesCfg:  *cfg,
			modelManager: modelMgr,
		}, nil
	},
)

func (pm *profileManager) ResolveProfile(profileName string) (AgentProfile, error) {
	if profileName == "" {
		profileName = pm.profilesCfg.Default
	}
	if profileName == "" {
		profileName = "smart" // ultimate fallback
	}

	profile, ok := pm.profilesCfg.Profiles[profileName]
	if !ok {
		return AgentProfile{}, fmt.Errorf("profile %q not found. Available: %v", profileName, profileNames(pm.profilesCfg))
	}
	profile.Name = profileName
	return profile, nil
}

func (pm *profileManager) CreateModelFromProfile(profile AgentProfile, extraOverrides map[string]any) (api.Model, error) {
	modelName := profile.Model.Name
	if modelName == "" {
		modelName = pm.modelManager.DefaultModelName()
	}

	// Merge profile overrides + extra overrides
	overrides := make(map[string]any)
	for k, v := range profile.Model.Overrides {
		overrides[k] = v
	}
	for k, v := range extraOverrides {
		overrides[k] = v
	}

	return pm.modelManager.CreateModel(modelName, overrides)
}

func (pm *profileManager) ModelCallLogging(profile AgentProfile) bool {
	if profile.LogModelCalls != nil {
		return *profile.LogModelCalls
	}
	return pm.profilesCfg.LogModelCalls
}

func (pm *profileManager) DefaultProfileName() string {
	if pm.profilesCfg.Default != "" {
		return pm.profilesCfg.Default
	}
	return "smart"
}

func profileNames(cfg LLMProfilesConfig) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	return names
}
