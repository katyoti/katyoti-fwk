// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
	synccmds "github.com/katyoti/katyoti-fwk/internal/extensions/sync/commands"
	synclocal "github.com/katyoti/katyoti-fwk/internal/extensions/sync/local"
)

// buildExtensions declares all ApplicationExtensions for the ktw-sync sidecar.
// FlatCLI intentionally omitted: 3 domain services (job, workspace, provider)
// benefit from grouped CLI to avoid verb collision at the top level.
// Migrated to the sidecarkit builder 2026-05-23 (SN-12).
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName: "ktw-sync",
		Version: version,
		Short:   "ktw-sync — multi-provider cloud storage synchronizer",
		Long: `ktw-sync synchronizes local directories with cloud storage providers
(Google Drive, S3, OneDrive, SFTP, WebDAV, etc.) using rclone as the
embedded engine. Supports push, pull, mirror, and bidirectional sync
with cron scheduling and file watching.`,
		EmbeddedFS: efs,
	},
		sidecarkit.WithDomain(synclocal.Extension()),
		sidecarkit.WithKTSPDescription("Cloud storage synchronization"),
		// No WithFlatCLI — domain services need grouped CLI.
		sidecarkit.WithExtraExtensions(appcore.CommandExt(synccmds.RcloneCommand())),
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}
