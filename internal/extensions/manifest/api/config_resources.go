// SPDX-License-Identifier: Apache-2.0

package api

// Resource type discriminators for the three config-related resource types
// emitted by every sidecar via the ConfigResourceProvider group (see
// internal/appcore/sidecar/config_resource_provider.go). Values are stable;
// consumers filter by these strings via Manifest.GetContributionsByType[T].
const (
	ResourceTypeConfigFile    = "config-file"
	ResourceTypeConfigProgram = "config-program"
	ResourceTypeEnv           = "env"
)

// ConfigFileBody is the typed body for resources with type "config-file".
//
// The entry's ID is the file name (e.g. "DocService.yaml"); the canonical
// URI is `<provider>:/config-file/<id>`. Runtime properties (current path
// on disk, exists, current contents) are intentionally NOT part of the
// descriptor — they are computed at probe time by ConfigService.Check
// (SidecarConfigService.Check, internal/appcore/sidecar/api/config.go). The descriptor describes what the
// binary needs, not the host's current state.
type ConfigFileBody struct {
	HasDefault  bool   `json:"has_default"`
	Description string `json:"description,omitempty"`

	// Schema is the URI of the data-type describing THIS file's shape —
	// `<provider>:/data-type/<name>`. A consumer follows it to get the fields,
	// and renders an editable form instead of a read-only health report.
	//
	// The reference sits on the INSTANCE rather than on the kind, and that is
	// forced by the subject rather than chosen: every permission-profile looks
	// alike and can share one shape declared by its kind, while two config
	// files of the same kind do not — `WeatherService.yaml` and `TTSVoices.yaml`
	// have nothing in common but being config. So `config-file` stays ONE
	// generic kind whose own schema declares this field, and the per-file shape
	// is reached through it. No per-instance kinds, no second schema path.
	//
	// Empty when no shape could be derived — a config that is not a typed Go
	// struct, for instance. A consumer that finds it empty falls back to the
	// health report; rendering an empty form would be worse than rendering
	// none.
	Schema string `json:"schema,omitempty"`
}

// ConfigProgramBody is the typed body for resources with type "config-program".
// Describes one external program the binary depends on. Entry ID is the
// program's display name (e.g. "pandoc").
type ConfigProgramBody struct {
	Commands        []string `json:"commands,omitempty"`
	Required        bool     `json:"required,omitempty"`
	InstallSkill    string   `json:"install_skill,omitempty"`
	VersionFlag     string   `json:"version_flag,omitempty"`
	RequiredVersion string   `json:"required_version,omitempty"`
	ConfigKey       string   `json:"config_key,omitempty"`
}

// EnvBody is the typed body for resources with type "env". Describes one
// environment variable the binary reads. Entry ID is the env name
// (e.g. "ANTHROPIC_API_KEY").
type EnvBody struct {
	Label       string `json:"label,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}
