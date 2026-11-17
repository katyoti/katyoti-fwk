// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/frozenhost"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	scriptslocal "github.com/katyoti/katyoti-fwk/internal/extensions/scripts/local"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
)

// hostManifest is a folder's ktw-host.yaml as the host reads it: its
// declaration (manifestcontrib.FolderDecl, the one reading the hub's folder
// member shares) and its `config:` block, the sidecar's own configuration
// needs, so the synthesized `sidecar config check` reports them — letting
// `katyoti configure` drive a files-only sidecar exactly like a compiled one.
type hostManifest struct {
	manifestcontrib.FolderDecl `yaml:",inline"`
	Config                     sidecar.FolderConfig `yaml:"config,omitempty"`
}

// configInfra translates the manifest `config:` block into an infra
// PublishedService carrying the declared needs (sidecar.FolderConfig, the one
// reading of the block: a folder member is checked from it in process). Returns
// nil when nothing is declared (no empty service contributed).
func configInfra(provider string, c sidecar.FolderConfig) application.Extension {
	opts := c.PublishOptions()
	if len(opts) == 0 {
		return nil
	}
	return publish.DeclareInfra(provider+"-config", opts...)
}

// remapOf turns the manifest's `dirs:` into the pack reader's remap, cwm/ and
// scripts/ left to their readers.
func remapOf(m hostManifest, kinds contribapi.Kinds) map[string]string {
	return m.Remap(kinds, func(key string, err error) {
		slog.Warn("ktw-host: dirs entry is neither a kind's folder nor a valid kind name — skipped", "key", key, "err", err)
	})
}

// buildExtensions wires the generic host. Given a content root, it reads the
// folder's `ktw-host.yaml`, adopts its identity as the RUNTIME provider name
// (proving runtime-named registration, risk-1), and serves each declared/
// conventional directory as its contribution type.
//
// The root is resolved before Cobra parses, because it decides which extensions
// exist at all — see contentRoot for where it comes from.
func buildExtensions(efs embed.FS) []application.Extension {
	folder := contentRoot()
	// sealed records that the content came from inside this executable rather
	// than from a folder someone can edit. A sealed pack cannot change while
	// the process runs — changing it means rebuilding the exe, which the
	// aggregator sees in the exe's own stat — so its reads are cached outright
	// instead of being re-checked.
	sealed := false
	if folder == "" {
		// A FROZEN host (freeze-to-exe) carries its content folder appended to
		// its own executable — recover + extract it so the single-file exe runs
		// as that sidecar with no `--folder`.
		if exe, err := os.Executable(); err == nil {
			if f, ferr := frozenhost.FrozenFolder(exe, os.TempDir()); ferr == nil && f != "" {
				folder = f
				sealed = true
			}
		}
	}
	provider := "ktw-host"
	display := ""
	short := "ktw-host — run a files-only sidecar from a folder"
	// The host binary's own version, used only when no folder is adopted.
	reported := version

	extra := []application.Extension{withFolderFlag()}
	if folder != "" {
		var m hostManifest
		if mm, err := readManifest(folder); err == nil && mm.Name != "" {
			m = mm
			provider = mm.Name
			display = mm.DisplayName
			if mm.Description != "" {
				short = mm.Description
			}
			// Report the CONTENT's version. When the folder declares none, say so
			// rather than borrowing the host's — a number with no relationship to
			// the content is worse than an honest absence.
			if mm.Version != "" {
				reported = mm.Version
			} else {
				reported = "unversioned"
			}
		}
		extra = append(extra, wireFolderTypes(folder, provider, m, sealed)...)
		if infra := configInfra(provider, m.Config); infra != nil {
			extra = append(extra, infra)
		}
	}

	// The host ADOPTS the folder's identity: AppName = the manifest name, so the
	// self-provider, config/overlay dir, KTSP slots, and `describe` all align
	// with the manifest (and `resources list` surfaces the folder's
	// contributions, which are registered under that same provider). Falls back
	// to "ktw-host" with no folder.
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName:     provider,
		DisplayName: display,
		Version:     reported,
		Short:       short,
		EmbeddedFS:  efs,
	},
		sidecarkit.WithoutPack(), // no embedded pack — we serve the folder
		sidecarkit.WithoutHTTPServer(),
		sidecarkit.WithExtraExtensions(extra...),
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}

// wireFolderTypes reads the folder as a pack: every first-level folder a
// kind claims as that kind (manifestcontrib.ContributePack, the `dirs:`
// entries as remaps), and the folders that are not one kind's — the CWM pack
// and the scripts. Everything reads through a single FSLoader rooted at the
// folder.
func wireFolderTypes(folder, provider string, m hostManifest, sealed bool) []application.Extension {
	fsl := loader.NewFSLoader(folder)
	if sealed {
		// Content extracted from this executable: fixed for the process, so
		// reads are cached without re-checking. A LIVE folder must not be
		// marked — it is the source designed to be edited.
		fsl.MarkImmutable()
	}
	kinds, err := builtin.Kinds()
	if err != nil {
		slog.Error("ktw-host: the system kinds do not read; serving no kind", "err", err)
		return nil
	}
	out := []application.Extension{
		manifestcontrib.ContributePack(provider, fsl, ".", kinds, manifestcontrib.WithRemap(remapOf(m, kinds))),
	}
	// Content packaging moved to the object store leaves no file — and a
	// content dir whose only content was bulk leaves no DIRECTORY either. The
	// recorded manifest is what still says that content is there.
	recorded, _ := contribapi.ReadLayerManifest(folder)
	if dir := m.HostDir(folder, "scripts", recorded); dir != "" {
		// Scripts need the ScriptService runtime AND the loader that feeds it.
		out = append(out, scriptslocal.Extension(), scriptslocal.ContributeScriptLoader(fsl, dir, provider))
	}
	if dir := m.HostDir(folder, "cwm", recorded); dir != "" {
		// The CWM pack is a part of the folder's pack, read from an fs.FS
		// sub-tree: every CWM kind's folder, the extensions' (gates/,
		// object-panels/, agent-contexts/, …) and its data-types included.
		if sub, err := fs.Sub(os.DirFS(folder), dir); err == nil {
			out = append(out, manifestcontrib.ContributePackPart(provider, provider, dir, sub))
		}
	}
	return out
}

// withFolderFlag registers `--folder` on the root so Cobra accepts it (the
// value is read separately via folderFromArgs at AppInit).
func withFolderFlag() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddBootstrap("ktw-host", ".folderflag", func(app *application.Application) error {
			cr, err := application.AppUseNamedService[appcore.CobraCommand](app, "root")
			if err != nil {
				return err
			}
			if cr.Cmd.PersistentFlags().Lookup("folder") == nil {
				cr.Cmd.PersistentFlags().String("folder", "", "path to the files-only sidecar folder")
			}
			return nil
		})
	}
}

// contentRootEnv is how a universe DECLARES the folder this host serves. The
// membership carries it as `content_root:` and the connection manager passes it
// through; the host reads it here, before Cobra has parsed anything.
const contentRootEnv = "KTW_CONTENT_ROOT"

// contentRoot resolves the folder to serve.
//
// The declared root wins. It exists because the folder used to live only inside
// the spawn arguments, which meant everything that needed it — the version
// resolver, the configure path, this function — recovered it by re-parsing
// argv, and the aggregator could not cheaply tell that a member served a folder
// at all. That last part matters: a served folder is the one source whose
// changes the host's own executable cannot show.
//
// `--folder` remains for a person running the host directly, where no universe
// declared anything. It is still read from argv because the folder decides
// which extensions exist, and that is settled before Cobra parses.
func contentRoot() string {
	if root := strings.TrimSpace(os.Getenv(contentRootEnv)); root != "" {
		return root
	}
	return folderFromArgs()
}

func folderFromArgs() string {
	for i, a := range os.Args {
		if a == "--folder" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(a, "--folder=") {
			return strings.TrimPrefix(a, "--folder=")
		}
	}
	return ""
}

func readManifest(folder string) (hostManifest, error) {
	var m hostManifest
	b, err := os.ReadFile(filepath.Join(folder, manifestcontrib.FolderManifestFile))
	if err != nil {
		return m, err
	}
	if err := config.DecodeBytes(b, marshal.YAML, &m); err != nil {
		return m, err
	}
	return m, nil
}
