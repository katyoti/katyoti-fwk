// SPDX-License-Identifier: Apache-2.0

// Package core hosts the in-process implementation of ktw-gocode's
// services (GocodeService + GoToolService): the Go language pack's
// module, use-case, verify, lint, audit, promote and tasktest verbs
// and the Go toolchain wrappers.
package core

// Config is the decoded shape of the workspace-level
// `<HomeDir>/config/GocodeService.yaml` file. Loaded by the
// substrate's config service at PublishService factory time.
type Config struct {
	// WorkspaceRoot is the absolute path to the workspace folder
	// (the parent of `.katyoti/`). Auto-injected by the bootstrap
	// layer when running inside ktw-agents / ktw-hub / ktw-gocode.
	WorkspaceRoot string `yaml:"workspace_root,omitempty" json:"workspace_root,omitempty"`

	// DefaultTierTarget is the default tier target for new modules.
	// `module create` uses this when --tier-target is not given.
	DefaultTierTarget int `yaml:"default_tier_target,omitempty" json:"default_tier_target,omitempty" envkey:"KTW_GOCODE_DEFAULT_TIER_TARGET" default:"1"`

	// GoToolchainPath optionally overrides PATH lookup for `go`.
	// Useful in CI where multiple Go versions coexist.
	GoToolchainPath string `yaml:"go_toolchain_path,omitempty" json:"go_toolchain_path,omitempty" envkey:"KTW_GOCODE_GO_PATH"`

	// AuditPerspectives is the list of audit perspectives the
	// `module audit` verb runs by default. Empty means "all 15".
	AuditPerspectives []string `yaml:"audit_perspectives,omitempty" json:"audit_perspectives,omitempty"`
}

// GocodeServiceConfig is Config under the name of its file: GocodeService.yaml.
// GoToolService reads the same file as GocodeService through it
// (application.UseConfig resolves a file from the config type's name), rather
// than a GoToolService.yaml named after it that nothing writes.
type GocodeServiceConfig Config
