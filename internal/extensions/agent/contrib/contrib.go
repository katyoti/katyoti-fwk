// SPDX-License-Identifier: Apache-2.0

// Package contrib publishes the agent extension's declarations: the
// `permission-profile` kind, its data-type, the base profiles the agent ships
// and the helper a member publishes its own profiles with.
package contrib

import (
	"embed"
	"path"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed embed/contribution-types embed/data-types embed/permission-profiles
var kindFS embed.FS

// DeclareKinds publishes the `permission-profile` kind and its data-type as
// provider ("" = the host's name).
func DeclareKinds(provider string) application.Extension {
	ldr := loader.NewEmbedFSLoader(kindFS, ".")
	kinds := manifestcontrib.ContributeContributionTypeResources(provider, ldr, "embed/contribution-types")
	types := manifestcontrib.ContributeDataTypeResources(provider, ldr, "embed/data-types")
	return func(init *application.ApplicationInit) {
		kinds(init)
		types(init)
	}
}

// OwnProfiles publishes the agent's base permission profiles - workspace-edit
// (the default), plan-mode and full-access - as provider ("" = the host's name).
func OwnProfiles(provider string) application.Extension {
	return ContributeProfiles(provider, loader.NewEmbedFSLoader(kindFS, "."), "embed/permission-profiles")
}

// ContributeProfiles publishes each `<root>/<name>.yaml` in ldr as a
// `permission-profile` entry `<provider>:/permission-profile/<name>`: how a
// member offers a profile. The kind is declared by the agent extension; a
// member publishes it as a foreign kind, so the picker fields are written
// here, from the file.
func ContributeProfiles(provider string, ldr loader.Loader, root string) application.Extension {
	return manifestcontrib.ContributeKindResources(provider, permrules.ContributionTypePermissionProfile,
		ldr, root, manifestcontrib.WithMetadata(profileMeta))
}

func profileMeta(itemPath string, raw []byte) *contribapi.ContributionMeta {
	var p permrules.ProfileSpec
	if config.DecodeBytes(raw, marshal.YAML, &p) != nil {
		return nil
	}
	name := strings.TrimSuffix(path.Base(itemPath), path.Ext(itemPath))
	return &contribapi.ContributionMeta{Name: name, Description: p.Description}
}
