// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/spf13/cobra"
)

// SidecarDescription is the self-describe metadata a binary emits for
// marketplace catalogs (the katyoti marketplace-index engine reads it). It is
// the machine-readable identity of a distributable sidecar — distinct from the
// KTSP EndpointManifest (which describes the wire surface).
type SidecarDescription struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
	// Formats are the formats (kind → data-type major.minor) this binary
	// writes — the same derivation as its sidecar-info's. A marketplace copies
	// them into the item's requires, so a hub too old to read them refuses
	// the install before downloading anything.
	Formats map[string]string `json:"formats,omitempty"`
	// Tools are the programs this binary runs (publish.ToolDef as data, one
	// per name): required or optional, the version check, the prose install
	// skill and the structured install recipes per platform, so an installer
	// engine plans them from the catalog without knowing the sidecar.
	Tools []publish.ToolRequirement `json:"tools,omitempty"`
	// APIKeys are the keys its published services ask for (publish.APIKeyDef
	// as data, one per variable, required when any declaration requires it).
	APIKeys []publish.APIKeyRequirement `json:"apiKeys,omitempty"`
	// Auth are the logins its published services rely on (publish.AuthDef as
	// data, one per name). A catalog copies tools, keys and logins into the
	// item's requires, so a solution shows what it needs before install.
	Auth []publish.AuthRequirement `json:"auth,omitempty"`
}

// Describe builds the description from an AppConfig, with sensible fallbacks
// and a build-info-resolved version.
func Describe(cfg AppConfig) SidecarDescription {
	display := cfg.DisplayName
	if display == "" {
		display = cfg.AppName
	}
	return SidecarDescription{
		Name:        cfg.AppName,
		DisplayName: display,
		Icon:        cfg.Icon,
		Description: cfg.Short,
		Version:     ResolveVersion(cfg.Version),
	}
}

// NewDescribeCommand builds the top-level `describe` command: the sidecar's
// self-describe metadata (name/displayName/icon/description/version). Machine
// consumers use `describe --out json`.
func NewDescribeCommand(cfg AppConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Print this binary's marketplace metadata (name/version/icon/…)",
	}
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		d := Describe(cfg)
		d.Formats = describeFormats(cmd.Context())
		d.Tools = DescribeTools(cmd.Context())
		d.APIKeys, d.Auth = DescribeKeysAndAuth(cmd.Context())
		return console.RenderConsole(*mode, "describe", d, console.RenderFunc(func(p *console.Printer, val any) error {
			cmd.Printf("%s %s\n", d.Name, d.Version)
			if d.DisplayName != "" && d.DisplayName != d.Name {
				cmd.Printf("  %s\n", d.DisplayName)
			}
			if d.Description != "" {
				cmd.Printf("  %s\n", d.Description)
			}
			if d.Icon != "" {
				cmd.Printf("  icon: %s\n", d.Icon)
			}
			for _, t := range d.Tools {
				need := "required"
				if !t.Required {
					need = "optional"
				}
				cmd.Printf("  program %s (%s, %d install recipes)\n", t.Name, need, len(t.Recipes))
			}
			return nil
		}))
	}
	return cmd
}

// describeFormats derives the formats this binary writes from what its
// providers publish (contribapi.DeriveFormats); nil when the application or a
// provider cannot answer — describe never fails for it.
func describeFormats(ctx context.Context) map[string]string {
	if ctx == nil || application.AppFromContext(ctx) == nil {
		return nil
	}
	var all []contribapi.Contribution
	for _, p := range application.UseExtensions[contribapi.ContributionProvider](ctx) {
		entries, err := p.Enumerate(ctx)
		if err != nil {
			return nil
		}
		all = append(all, entries...)
	}
	return contribapi.DeriveFormats(all)
}

// DescribeTools is the programs the binary's published services declare, as
// data, one per name: the first declaration of a name is kept, required when
// any declaration requires it, its recipes those of the first declaration that
// has some. Nil without an application.
func DescribeTools(ctx context.Context) []publish.ToolRequirement {
	if ctx == nil || application.AppFromContext(ctx) == nil {
		return nil
	}
	var out []publish.ToolRequirement
	at := map[string]int{}
	for _, ps := range application.UseExtensions[publish.PublishedService](ctx) {
		for _, t := range ps.Tools {
			r := t.Requirement()
			i, seen := at[t.Name]
			if !seen {
				at[t.Name] = len(out)
				out = append(out, r)
				continue
			}
			out[i].Required = out[i].Required || r.Required
			if len(out[i].Recipes) == 0 {
				out[i].Recipes = r.Recipes
			}
		}
	}
	return out
}

// DescribeKeysAndAuth is the keys and logins the binary's published services
// declare, as data: one key per variable (the first declaration kept,
// required when any declaration requires it) and one login per name. Nil
// without an application.
func DescribeKeysAndAuth(ctx context.Context) ([]publish.APIKeyRequirement, []publish.AuthRequirement) {
	if ctx == nil || application.AppFromContext(ctx) == nil {
		return nil, nil
	}
	var keys []publish.APIKeyRequirement
	var auth []publish.AuthRequirement
	keyAt := map[string]int{}
	authSeen := map[string]bool{}
	for _, ps := range application.UseExtensions[publish.PublishedService](ctx) {
		for _, k := range ps.APIKeys {
			if i, seen := keyAt[k.EnvName]; seen {
				keys[i].Required = keys[i].Required || k.Required
				continue
			}
			keyAt[k.EnvName] = len(keys)
			keys = append(keys, k.Requirement())
		}
		for _, a := range ps.Auth {
			if authSeen[a.Name] {
				continue
			}
			authSeen[a.Name] = true
			auth = append(auth, a.Requirement())
		}
	}
	return keys, auth
}
