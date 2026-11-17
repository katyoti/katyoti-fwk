// SPDX-License-Identifier: Apache-2.0

// Package local wires the profile service and the `profile` command as
// application.Extensions.
//
//   - KTSPServiceExtension: the ProfileService, served in the admin universe by
//     ktw-univers — the only writer of Profile.yaml.
//   - CommandExtension: `katyoti profile`, a pure KTSP client of that service.
package local

import (
	"context"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/commands"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/internal/passkey"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/internal/permission"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/internal/proof"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// ProfileChangedEvent is the payload of a profile-changed notification, sent on
// every write (the profile or its guide section). It says that the profile
// changed, not what it is: subscribers re-read through Get / GuideGet.
type ProfileChangedEvent struct {
	ID string `json:"id"`
}

// KTSPServiceExtension publishes the ProfileService over KTSP. Auto-CLI is
// suppressed (cli:false): `katyoti profile` owns the CLI.
func KTSPServiceExtension() application.Extension {
	return appcore.PublishSimpleService[api.ProfileService](
		func(_ context.Context) (api.ProfileService, error) {
			return core.New(core.Options{
				Dir:     filepath.Join(application.Env().GlobalHomeDir(), "config"),
				Suggest: core.DetectOS,
				OnChange: func(ctx context.Context, p api.Profile) {
					admin.PublishEvent(ctx, subjects.ProfileChangedSubject, ProfileChangedEvent{ID: p.ID})
				},
			}), nil
		},
		appcore.WithServiceDef(api.ProfileServiceDef),
		appcore.WithKTSPContainer(),
		appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
	)
}

// PasskeyServiceExtension publishes the person's PasskeyService over KTSP,
// served by the process serving the universe (ktw-agents), which serves the
// approval page: its ceremonies live in that process; the credentials are
// sealed in the global config beside the authenticator code's. Auto-CLI is
// suppressed: the approval page drives it.
func PasskeyServiceExtension() application.Extension {
	return appcore.PublishSimpleService[api.PasskeyService](
		func(_ context.Context) (api.PasskeyService, error) {
			return passkey.Service{S: passkey.New(filepath.Join(application.Env().GlobalHomeDir(), "config"), proof.Seal, proof.Unseal)}, nil
		},
		appcore.WithServiceDef(api.PasskeyServiceDef),
		appcore.WithKTSPContainer(),
		appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
	)
}

// PermissionServiceExtension publishes the PermissionService over KTSP
// (ktw-agents:permission/<Verb>, with its auto-CLI), served by the process
// serving the universe: the permission codes of the methods, actions and tasks
// it offers, derived from their URIs, and those every contributed source
// declares (api.ContributePermissionSource) - human-task kinds, agent tools.
func PermissionServiceExtension() application.Extension {
	return appcore.PublishSimpleService[api.PermissionService](
		func(_ context.Context) (api.PermissionService, error) {
			dir := filepath.Join(application.Env().GlobalHomeDir(), "config")
			return permission.Service{
				Person:  func() string { return core.ReadHandle(dir) },
				Sources: permissionSources,
			}, nil
		},
		appcore.WithServiceDef(api.PermissionServiceDef),
		appcore.WithKTSPContainer(),
	)
}

// permissionSources is the binary's callables, then every contributed source.
func permissionSources(ctx context.Context) []api.PermissionSource {
	callables := permission.Callables{List: func(ctx context.Context) ([]string, error) {
		llm, err := application.UseService[sidecarapi.LLMService](ctx)
		if err != nil {
			return nil, err
		}
		got, err := llm.ListTools(ctx, sidecarapi.LLMListToolsInput{})
		if err != nil {
			return nil, err
		}
		uris := make([]string, 0, len(got.Tools))
		for _, t := range got.Tools {
			uris = append(uris, t.Name)
		}
		return uris, nil
	}}
	return append([]api.PermissionSource{callables}, application.UseExtensions[api.PermissionSource](ctx)...)
}

// CommandExtension adds `profile` to the root command.
func CommandExtension() application.Extension {
	return appcore.CommandExt(commands.NewProfileCommand())
}
