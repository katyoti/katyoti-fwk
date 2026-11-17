// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// NewSolutionCommand builds `solution list | show | install | keys`: a solution is a
// fixed version of a working installation — the infra version, its sidecars,
// config-sets, seed and vaults, and its documents — installed whole into a
// universe.
func NewSolutionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "solution",
		Short: "Solutions: fixed versions of a working installation (infra, sidecars, config-sets, seed, vaults, documents)",
	}
	cmd.AddCommand(solutionListCommand(), solutionShowCommand(), solutionInstallCommand(), solutionUpdateCommand(), solutionKeysCommand())
	return cmd
}

func itemsOfKind(cmd *cobra.Command, s mktapi.SetupService, kind mktapi.ItemKind) ([]mktapi.ItemSummary, error) {
	all, err := s.SearchItems(cmd.Context(), mktapi.SearchItemsInput{})
	if err != nil {
		return nil, err
	}
	var out []mktapi.ItemSummary
	for _, it := range all {
		if it.Kind == kind {
			out = append(out, it)
		}
	}
	return out, nil
}

func solutionListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "The solutions your marketplaces offer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			items, err := itemsOfKind(cmd, s, mktapi.KindSolution)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "SOLUTION\tLATEST\tMARKETPLACE\tDESCRIPTION")
			for _, it := range items {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", it.Name, it.Latest, it.Marketplace, it.Description)
			}
			return w.Flush()
		},
	}
}

func solutionShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>[@version]",
		Short: "What a solution pins and delivers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			ref := parseRef(args[0])
			it, err := s.GetItem(cmd.Context(), mktapi.GetItemInput{Name: ref.Name})
			if err != nil {
				return err
			}
			if it.Kind != mktapi.KindSolution {
				return fmt.Errorf("%s is a %s item, not a solution", ref.Name, it.Kind)
			}
			for _, v := range it.Versions {
				if ref.Version != "" && v.Version != ref.Version {
					continue
				}
				o := cmd.OutOrStdout()
				fmt.Fprintf(o, "%s@%s  infra %s\n", it.Name, v.Version, orNone(v.Infra))
				printUniverses(o, v)
				for _, vt := range v.Vaults {
					fmt.Fprintf(o, "  vault       %s@%s\n", vt.Name, vt.Version)
				}
				for _, d := range v.Documents {
					fmt.Fprintf(o, "  document    %s/%s\n", d.Lang, d.Name)
				}
				printNeeds(o, v.Requires)
				if ref.Version == "" {
					break // the newest
				}
			}
			return nil
		},
	}
}

// printUniverses lists what a solution version pins, universe by universe
// when it composes several (the main one first, marked): its sidecars,
// config-sets and seed.
func printUniverses(o io.Writer, v mktapi.ItemVersion) {
	universes := v.SolutionUniverses()
	indent := "  "
	for _, u := range universes {
		if len(universes) > 1 {
			main := ""
			if u.Main {
				main = "  (main: opens after install)"
			}
			fmt.Fprintf(o, "  universe    %s%s\n", u.Name, main)
			indent = "    "
		}
		for _, m := range v.UniverseMembers(u) {
			fmt.Fprintf(o, "%ssidecar     %s@%s\n", indent, m.Name, m.Version)
		}
		for _, c := range u.ConfigSets {
			fmt.Fprintf(o, "%sconfig-set  %s@%s\n", indent, c.Name, c.Version)
		}
		if u.Seed != nil {
			fmt.Fprintf(o, "%sseed        %s@%s\n", indent, u.Seed.Name, u.Seed.Version)
		}
	}
}

// printNeeds lists what a solution needs before it is installed: each
// program (required or optional, its version, the platforms a recipe covers),
// each key and each login, with the sidecars that need it.
func printNeeds(o io.Writer, r mktapi.Requires) {
	if len(r.Tools)+len(r.APIKeys)+len(r.Auth) == 0 {
		return
	}
	fmt.Fprintln(o, "  needs before install:")
	for _, t := range r.Tools {
		line := fmt.Sprintf("    program  %s (%s", t.Name, requiredWord(t.Required))
		if t.Version != "" {
			line += ", " + t.Version
		}
		line += ")"
		if len(t.Platforms) > 0 {
			line += "  recipes: " + strings.Join(t.Platforms, ", ")
		} else if t.InstallSkill != "" {
			line += "  manual: " + t.InstallSkill
		}
		fmt.Fprintf(o, "%s  for %s\n", line, strings.Join(t.NeededBy, ", "))
	}
	for _, k := range r.APIKeys {
		fmt.Fprintf(o, "    key      %s (%s)  for %s\n", k.Name, requiredWord(k.Required), strings.Join(k.NeededBy, ", "))
	}
	for _, a := range r.Auth {
		line := "    login    " + a.Name
		if a.LoginHint != "" {
			line += "  (" + a.LoginHint + ")"
		}
		fmt.Fprintf(o, "%s  for %s\n", line, strings.Join(a.NeededBy, ", "))
	}
}

func requiredWord(required bool) string {
	if required {
		return "required"
	}
	return "optional"
}

func orNone(s string) string {
	if s == "" {
		return "(none recorded)"
	}
	return s
}

func solutionInstallCommand() *cobra.Command {
	var (
		universe, project, envFile, keysFile, passwordEnv, vaultPasswordEnv string
		sets                                                                []string
		nonInteractive                                                      bool
	)
	cmd := &cobra.Command{
		Use:   "install <solution>[@version]",
		Short: "Install a solution into a universe: its members, config-sets, seed, vaults' keys and documents",
		Long: `Install a solution — a fixed version of a working installation — into a
universe: each pinned sidecar at its exact version, the universe created from the
captured wiring, the vaults' keys imported (the team password asked once), each
config-set applied, the seed laid out as the universe's project, the documents
placed into it. Nothing is installed while a vault does not open or a key the
config-sets need (not optional) has no value. No upgrade is taken.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			values, err := collectPreset(sets, envFile)
			if err != nil {
				return err
			}
			keys, password, err := readKeysFile(cmd, keysFile, passwordEnv)
			if err != nil {
				return err
			}
			in := mktapi.InstallSolutionInput{
				Ref:            parseRef(args[0]),
				UniverseHome:   resolveUniverseTarget(cmd, universe),
				ProjectDir:     project,
				Values:         values,
				Keys:           keys,
				KeysPassword:   password,
				VaultPasswords: map[string]string{},
				DryRun:         true,
			}
			// A dry run first: which vaults, which keys — nothing installed.
			dry, err := s.InstallSolution(cmd.Context(), in)
			if err != nil {
				return err
			}
			// The vaults are the solution's key sets: the team password is
			// asked ONCE and opens them all.
			if len(dry.Vaults) > 0 {
				switch {
				case vaultPasswordEnv != "":
					in.KeySetsPassword = os.Getenv(vaultPasswordEnv)
				case !nonInteractive && isTerminal():
					p, err := askTeamPassword(cmd, dry.Vaults)
					if err != nil {
						return err
					}
					in.KeySetsPassword = p
				}
			}
			if len(dry.Vaults) > 0 {
				if dry, err = s.InstallSolution(cmd.Context(), in); err != nil {
					return err
				}
			}
			// The same collection loop as `config-set apply`, per config-set.
			if !nonInteractive && isTerminal() {
				for _, cs := range dry.ConfigSets {
					if len(cs.Missing) == 0 {
						continue
					}
					collected, perr := promptFor(cmd, cs)
					if perr != nil {
						return perr
					}
					for k, v := range collected {
						values[k] = v
					}
				}
			}
			in.DryRun = false
			res, err := s.InstallSolution(cmd.Context(), in)
			if err != nil {
				return err
			}
			o := cmd.OutOrStdout()
			fmt.Fprintf(o, "installed %s into universe at %s\n", args[0], res.Home)
			if len(res.Universes) > 1 {
				for _, u := range res.Universes {
					role := ""
					if u.Main {
						role = " (main: opens after install)"
					}
					fmt.Fprintf(o, "  universe %s%s at %s: %s\n", u.Name, role, u.Home, strings.Join(u.Members, ", "))
				}
			}
			for _, v := range res.Vaults {
				fmt.Fprintf(o, "  vault %s@%s: %d key(s)\n", v.Name, v.Version, len(v.Keys))
			}
			if res.Project != "" {
				if res.ProjectKept {
					fmt.Fprintf(o, "  project %s already held files: kept as it was, attached\n", res.Project)
				} else {
					fmt.Fprintf(o, "  project %s: the seed laid out and attached\n", res.Project)
				}
				if res.ProjectNotRecorded != "" {
					fmt.Fprintf(o, "  note: not recorded as created on this PC - %s\n", res.ProjectNotRecorded)
				}
			}
			for _, d := range res.Documents {
				fmt.Fprintf(o, "  document %s\n", d)
			}
			if res.Prereqs != nil && len(missingRequired(*res.Prereqs)) > 0 {
				fmt.Fprintln(o, "\nprograms the sidecars need on this machine:")
				renderPlan(o, *res.Prereqs)
				fmt.Fprintf(o, "install them: katyoti prereqs install --universe %s\n", res.Home)
			}
			var unset []string
			for _, cs := range res.ConfigSets {
				reportApply(cmd, cs)
				unset = append(unset, unsetRequired(cs)...)
			}
			if len(unset) > 0 {
				return fmt.Errorf("%d key(s) still unset: %s", len(unset), strings.Join(unset, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "universe to install into: a registered name, or a folder to create it in (required when the solution brings config-sets, vaults or a seed); a solution of several universes installs its main one there, each other one beside it as <universe>-<its name>")
	cmd.Flags().StringVar(&project, "project", "", "project folder for the solution's seed (default: <universe name>-project beside the universe; one that holds files is kept)")
	cmd.Flags().StringArrayVar(&sets, "set", nil, "Supply a config-set key non-interactively: --set KEY=VALUE (repeatable)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "Read config-set KEY=VALUE lines from this file")
	cmd.Flags().StringVar(&keysFile, "keys", "", "Import this sealed keys file (keys export) into the universe")
	cmd.Flags().StringVar(&passwordEnv, "password-env", "", "Read the keys file's password from this environment variable (default: asked)")
	cmd.Flags().StringVar(&vaultPasswordEnv, "vault-password-env", "", "Read the team password that opens every vault from this environment variable (default: asked once)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Never prompt; fail if a vault or a key is missing")
	return cmd
}

// NewVaultCommand builds `vault list | import`: vaults are sealed keys files
// published in a marketplace, opened only with their password.
func NewVaultCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vault",
		Short: "Vaults: sealed keys files published in your marketplaces",
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "The vaults your marketplaces offer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			items, err := itemsOfKind(cmd, s, mktapi.KindVault)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "VAULT\tLATEST\tMARKETPLACE")
			for _, it := range items {
				fmt.Fprintf(w, "%s\t%s\t%s\n", it.Name, it.Latest, it.Marketplace)
			}
			return w.Flush()
		},
	}
	var universe, passwordEnv string
	var overwrite, dryRun bool
	imp := &cobra.Command{
		Use:   "import <vault>[@version]",
		Short: "Import a vault's keys into their scopes' .env (its password asked)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			home, err := resolveUniverseHome(cmd, universe)
			if err != nil {
				return err
			}
			password, err := readPassword(cmd, passwordEnv, false)
			if err != nil {
				return err
			}
			out, err := s.ImportVault(cmd.Context(), mktapi.ImportVaultInput{Name: args[0], UniverseHome: home, Password: password, Overwrite: overwrite, DryRun: dryRun})
			if err != nil {
				return err
			}
			reportKeys(cmd, out.Keys)
			reportRestarts(cmd, out.Restarts)
			return nil
		},
	}
	imp.Flags().StringVar(&universe, "universe", "", "Universe whose .env receives the universe-scoped keys: a registered name, or a folder")
	imp.Flags().StringVar(&passwordEnv, "password-env", "", "Read the password from this environment variable (default: asked)")
	imp.Flags().BoolVar(&overwrite, "overwrite", false, "Replace keys already set to another value")
	imp.Flags().BoolVar(&dryRun, "dry-run", false, "Say what would be written; write nothing")
	_ = imp.MarkFlagRequired("universe")
	cmd.AddCommand(list, imp)
	return cmd
}
