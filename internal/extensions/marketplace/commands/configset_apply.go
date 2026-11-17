// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configset"
)

// NewConfigSetApplyCommand builds `config-set apply [<name>[@version]]`.
//
// With no name it applies the universe's own key set (the keys its members
// declare), so --universe is then required.
//
// The engine does the applying and every write; this command only COLLECTS. It
// imports a keys file when given one, asks the engine for a dry run (with
// whatever --set/--env-file supplied), prompts for what is still missing when
// it has a terminal, then applies ONCE. The engine refuses — writing nothing —
// while a non-optional key of the config-set has no value, so a
// non-interactive run with keys missing fails loudly instead of leaving a
// half-configured universe.
func NewConfigSetApplyCommand() *cobra.Command {
	var (
		universe       string
		force          bool
		sets           []string
		envFile        string
		keysFile       string
		passwordEnv    string
		nonInteractive bool
	)
	cmd := &cobra.Command{
		Use:   "apply [<name>[@version]]",
		Short: "Apply a config-set to a universe: drop its templates + collect every key it needs in one pass",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			if len(args) == 0 && strings.TrimSpace(universe) == "" {
				return fmt.Errorf("name the config-set to apply, or give --universe to save that universe's own keys")
			}
			var ref mktapi.ItemRef
			if len(args) == 1 {
				ref = parseRef(args[0])
			}

			// --universe is a NAME first, a folder second — the same rule as
			// `universe wire`. Applying to a universe that does not exist is a
			// typo, not a creation request, so this is the strict resolver.
			home, err := resolveUniverseHome(cmd, universe)
			if err != nil {
				return err
			}

			values, err := collectPreset(sets, envFile)
			if err != nil {
				return err
			}
			if keysFile != "" {
				if home == "" {
					return fmt.Errorf("--keys needs --universe: the universe whose .env receives them")
				}
				file, password, err := readKeysFile(cmd, keysFile, passwordEnv)
				if err != nil {
					return err
				}
				imp, err := s.ImportKeys(cmd.Context(), mktapi.ImportKeysInput{UniverseHome: home, File: file, Password: password})
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "imported %s: %d key(s) written\n", keysFile, len(imp.Written))
			}

			in := mktapi.ApplyConfigSetInput{
				Name: ref.Name, Version: ref.Version,
				UniverseHome: home, Force: force, Values: values, DryRun: true,
			}
			dry, err := s.ApplyConfigSet(cmd.Context(), in)
			if err != nil {
				return err
			}
			// One prompt pass over whatever is still missing, then the one apply.
			if len(dry.Missing) > 0 && !nonInteractive && isTerminal() {
				collected, perr := promptFor(cmd, dry)
				if perr != nil {
					return perr
				}
				for k, v := range collected {
					values[k] = v
				}
			}
			in.DryRun = false
			out, err := s.ApplyConfigSet(cmd.Context(), in)
			if err != nil {
				return err
			}

			reportApply(cmd, out)
			if unset := unsetRequired(out); len(unset) > 0 {
				// Exiting non-zero matters: a half-configured universe that reports
				// success would be discovered only when something fails to start.
				return fmt.Errorf("%d key(s) still unset: %s", len(unset), strings.Join(unset, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "Target universe: a registered name, or a folder (default: the current workspace)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite config templates that already exist")
	cmd.Flags().StringArrayVar(&sets, "set", nil, "Supply a key non-interactively: --set KEY=VALUE (repeatable)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "Read KEY=VALUE lines from this file")
	cmd.Flags().StringVar(&keysFile, "keys", "", "Import this sealed keys file (keys export) first")
	cmd.Flags().StringVar(&passwordEnv, "password-env", "", "Read the keys file's password from this environment variable (default: asked)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Never prompt; fail if keys are still missing")
	return cmd
}

// collectPreset merges --env-file then --set (so an explicit --set wins).
func collectPreset(sets []string, envFile string) (map[string]string, error) {
	values := map[string]string{}
	if envFile != "" {
		fromFile, err := configset.ReadEnv(envFile)
		if err != nil {
			return nil, err
		}
		for k, v := range fromFile {
			values[k] = v
		}
	}
	for _, kv := range sets {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("--set %q is not KEY=VALUE", kv)
		}
		values[strings.TrimSpace(k)] = v
	}
	return values, nil
}

func isTerminal() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// promptFor asks for each missing key, showing its description and (for a key
// that already has a value) offering to keep it. Secrets are read without echo.
func promptFor(cmd *cobra.Command, out mktapi.ApplyConfigSetOutput) (map[string]string, error) {
	byName := map[string]mktapi.ConfigSetKeyState{}
	for _, k := range out.Keys {
		byName[k.Name] = k
	}
	missing := append([]string(nil), out.Missing...)
	sort.Strings(missing)

	o := cmd.OutOrStdout()
	fmt.Fprintf(o, "\n%s needs %d value(s):\n\n", out.Name, len(missing))

	reader := bufio.NewReader(os.Stdin)
	collected := map[string]string{}
	for _, name := range missing {
		k := byName[name]
		if k.Description != "" {
			fmt.Fprintf(o, "  %s — %s\n", name, k.Description)
		} else {
			fmt.Fprintf(o, "  %s\n", name)
		}
		if k.Source != "" && k.Source != "config-set" {
			fmt.Fprintf(o, "    (required by %s)\n", k.Source)
		}
		fmt.Fprintf(o, "    %s [%s]: ", name, k.Scope)

		var val string
		if k.Secret {
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(o)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", name, err)
			}
			val = string(b)
		} else {
			line, err := reader.ReadString('\n')
			if err != nil && line == "" {
				return nil, fmt.Errorf("read %s: %w", name, err)
			}
			val = line
		}
		if val = strings.TrimSpace(val); val != "" {
			collected[name] = val
		}
	}
	return collected, nil
}

// unsetRequired is Missing without the keys marked optional: those may stay
// unset, everything else is a universe not yet configured.
func unsetRequired(out mktapi.ApplyConfigSetOutput) []string {
	optional := map[string]bool{}
	for _, k := range out.Keys {
		optional[k.Name] = k.Optional
	}
	var unset []string
	for _, name := range out.Missing {
		if !optional[name] {
			unset = append(unset, name)
		}
	}
	return unset
}

func reportApply(cmd *cobra.Command, out mktapi.ApplyConfigSetOutput) {
	o := cmd.OutOrStdout()
	if out.Name == "" {
		fmt.Fprintf(o, "\nthe universe's own keys → %s\n", out.UniverseHome)
	} else {
		fmt.Fprintf(o, "\napplied %s@%s → %s\n", out.Name, out.Version, out.UniverseHome)
	}

	if len(out.TemplatesCopied) > 0 {
		fmt.Fprintf(o, "  templates copied:  %s\n", strings.Join(out.TemplatesCopied, ", "))
	}
	if len(out.TemplatesSkipped) > 0 {
		// Say WHY they were skipped — otherwise this reads as a failure.
		fmt.Fprintf(o, "  already present:   %s  (use --force to overwrite)\n", strings.Join(out.TemplatesSkipped, ", "))
	}
	// Membership, when the config-set declared any. Reported BEFORE keys
	// because a member wired by this call is why some of those keys appeared.
	if len(out.MembersAdded) > 0 {
		fmt.Fprintf(o, "  members added:     %s\n", strings.Join(out.MembersAdded, ", "))
	}
	if len(out.MembersKept) > 0 {
		// Say that nothing moved. Silence here reads as "it did something to
		// them", and the whole point is that an existing member is untouched.
		fmt.Fprintf(o, "  members kept:      %s  (already wired — version unchanged)\n", strings.Join(out.MembersKept, ", "))
	}
	if len(out.KeysWritten) > 0 {
		fmt.Fprintf(o, "  keys written:      %s\n", strings.Join(out.KeysWritten, ", "))
		for _, p := range out.EnvWritten {
			fmt.Fprintf(o, "    → %s\n", p)
		}
		reportRestarts(cmd, out.Restarts)
	}

	if len(out.Keys) > 0 {
		fmt.Fprintln(o, "\nKeys:")
		w := tabwriter.NewWriter(o, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "  NAME\tSCOPE\tSTATUS\tSOURCE\tDESCRIPTION")
		for _, k := range out.Keys {
			status := "MISSING"
			if k.Present {
				status = "set"
			} else if k.Optional {
				status = "optional"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", k.Name, k.Scope, status, k.Source, k.Description)
		}
		_ = w.Flush()
	}
	if len(unsetRequired(out)) == 0 {
		fmt.Fprintf(o, "\nall keys set — run `katyoti doctor` to verify the universe.\n")
	}
}
