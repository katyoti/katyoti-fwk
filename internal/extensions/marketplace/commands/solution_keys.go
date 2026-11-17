// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// askTeamPassword asks, once, the team password that opens every key set
// (vault) of a solution.
func askTeamPassword(cmd *cobra.Command, sets []mktapi.VaultState) (string, error) {
	names := make([]string, 0, len(sets))
	for _, v := range sets {
		names = append(names, v.Name+"@"+v.Version)
	}
	o := cmd.OutOrStdout()
	fmt.Fprintf(o, "Team password (opens %s): ", strings.Join(names, ", "))
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(o)
	return string(b), err
}

// solutionKeysCommand builds `solution keys`: the key sets a solution
// provides imported with the team password asked once, then the keys still
// missing listed (the same KeySets / ImportKeySets the GUI calls).
func solutionKeysCommand() *cobra.Command {
	var (
		universe, passwordEnv string
		overwrite, list       bool
	)
	cmd := &cobra.Command{
		Use:   "keys <solution>[@version]",
		Short: "Import every key set (vault) of an installed solution with the team password asked once, then list the keys still missing",
		Long: `Import the key sets a solution provides — its vaults, sealed keys files — into
each of its universes with ONE password, the team's: every set is opened first,
and a set the password does not open refuses the whole import (nothing is
written). Then the keys the universe's config check still asks for, that no key
set provides, are listed: give them with ` + "`katyoti config-set apply`" + `.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			home := resolveUniverseTarget(cmd, universe)
			ref := parseRef(args[0])
			sets, err := s.KeySets(cmd.Context(), mktapi.KeySetsInput{Solution: ref, UniverseHome: home})
			if err != nil {
				return err
			}
			o := cmd.OutOrStdout()
			remaining := sets.Remaining
			if sets.PasswordNeeded && !list {
				password := ""
				switch {
				case passwordEnv != "":
					if password = os.Getenv(passwordEnv); password == "" {
						return fmt.Errorf("--password-env %s: the variable is not set", passwordEnv)
					}
				case isTerminal():
					if password, err = askTeamPassword(cmd, sets.KeySets); err != nil {
						return err
					}
				default:
					return fmt.Errorf("no terminal to ask the team password on: give --password-env <VAR>")
				}
				res, err := s.ImportKeySets(cmd.Context(), mktapi.ImportKeySetsInput{Solution: ref, UniverseHome: home, Password: password, Overwrite: overwrite})
				if err != nil {
					return err
				}
				for _, v := range res.KeySets {
					fmt.Fprintf(o, "key set %s@%s: %d key(s)\n", v.Name, v.Version, len(v.Keys))
				}
				reportKeys(cmd, res.Keys)
				reportRestarts(cmd, res.Restarts)
				remaining = res.Remaining
			} else {
				for _, v := range sets.KeySets {
					fmt.Fprintf(o, "key set %s@%s (sealed: the team password opens it)\n", v.Name, v.Version)
				}
			}
			if len(remaining) == 0 {
				fmt.Fprintln(o, "\nno key is missing")
				return nil
			}
			fmt.Fprintln(o, "\nkeys still missing:")
			w := tabwriter.NewWriter(o, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "  NAME\tSCOPE\tUNIVERSE\tGIVE IT WITH\tDESCRIPTION")
			for _, k := range remaining {
				with := "the sidecar's config (" + strings.TrimPrefix(k.Source, "sidecar:") + ")"
				if k.ConfigSet != "" {
					with = "config-set apply " + k.ConfigSet
				}
				fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", k.Name, k.Scope, k.UniverseHome, with, k.Description)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "The solution's main universe: a registered name or a folder")
	cmd.Flags().StringVar(&passwordEnv, "password-env", "", "Read the team password from this environment variable (default: asked once)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Replace keys already set to another value")
	cmd.Flags().BoolVar(&list, "list", false, "Only list the key sets and the keys still missing; import nothing")
	_ = cmd.MarkFlagRequired("universe")
	return cmd
}
