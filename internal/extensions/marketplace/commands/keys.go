// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// NewKeysCommand builds `keys export | import`: keys travel between machines
// as ONE file sealed with a password, holding only the keys a manifest
// declares, each with the scope (universe or global) it is written back into.
func NewKeysCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Export and import keys as one file sealed with a password",
	}
	cmd.AddCommand(newKeysExportCommand(), newKeysImportCommand())
	return cmd
}

func newKeysExportCommand() *cobra.Command {
	var (
		universe, configSet, manifest, passwordEnv string
		skipMissing, yes                           bool
	)
	cmd := &cobra.Command{
		Use:   "export <file>",
		Short: "Seal the keys a config-set (or a keys.yaml) declares into <file>",
		Long: `Seal into <file> the keys ONE manifest declares — a config-set's keys.yaml
(--config-set) or a keys.yaml file (--manifest) — never a whole .env, so a key
you added by hand does not leave by inadvertence. Each value is read from its
scope's .env (the universe's or the global one), never the process environment.
The keys are listed and confirmed before anything is written.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			home, err := resolveUniverseHome(cmd, universe)
			if err != nil {
				return err
			}
			in := mktapi.ExportKeysInput{UniverseHome: home, ConfigSet: configSet, Manifest: manifest, SkipMissing: skipMissing, Preview: true}
			pre, err := s.ExportKeys(cmd.Context(), in)
			if err != nil {
				return err
			}
			o := cmd.OutOrStdout()
			fmt.Fprintf(o, "%d key(s) to seal into %s:\n", len(pre.Keys), args[0])
			reportKeys(cmd, pre.Keys)
			if len(pre.Skipped) > 0 {
				fmt.Fprintf(o, "  left out (no value): %s\n", strings.Join(pre.Skipped, ", "))
			}
			if !yes {
				ok, err := confirm(cmd, "Export these keys?")
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("export cancelled")
				}
			}
			if in.Password, err = readPassword(cmd, passwordEnv, true); err != nil {
				return err
			}
			in.Preview = false
			out, err := s.ExportKeys(cmd.Context(), in)
			if err != nil {
				return err
			}
			if err := os.WriteFile(args[0], out.File, 0o600); err != nil {
				return err
			}
			fmt.Fprintf(o, "sealed %d key(s) into %s\n", len(out.Keys), args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "Universe whose .env holds the universe-scoped values: a registered name, or a folder")
	cmd.Flags().StringVar(&configSet, "config-set", "", "Export the keys this config-set declares (name[@version])")
	cmd.Flags().StringVar(&manifest, "manifest", "", "Export the keys a keys.yaml file declares")
	cmd.Flags().BoolVar(&skipMissing, "skip-missing", false, "Leave out declared keys that have no value (default: refuse)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Do not ask for confirmation")
	cmd.Flags().StringVar(&passwordEnv, "password-env", "", "Read the password from this environment variable (default: asked twice)")
	_ = cmd.MarkFlagRequired("universe")
	return cmd
}

func newKeysImportCommand() *cobra.Command {
	var (
		universe, passwordEnv string
		overwrite, dryRun     bool
	)
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Write the keys of a sealed file into their scopes' .env",
		Long: `Open <file> with its password and write each key into its scope's .env —
the universe's or the global one. A key already set to another value is kept
unless --overwrite.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			home, err := resolveUniverseHome(cmd, universe)
			if err != nil {
				return err
			}
			file, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			password, err := readPassword(cmd, passwordEnv, false)
			if err != nil {
				return err
			}
			out, err := s.ImportKeys(cmd.Context(), mktapi.ImportKeysInput{UniverseHome: home, File: file, Password: password, Overwrite: overwrite, DryRun: dryRun})
			if err != nil {
				return err
			}
			reportKeys(cmd, out.Keys)
			for _, k := range out.Keys {
				if k.State == "differs" {
					fmt.Fprintln(cmd.OutOrStdout(), "  keys marked differs are set to another value and were kept (--overwrite replaces them); the import does not declare a global one, so no member reads it from there")
					break
				}
			}
			reportRestarts(cmd, out.Restarts)
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "Universe whose .env receives the universe-scoped keys: a registered name, or a folder")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Replace keys already set to another value")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Say what would be written; write nothing")
	cmd.Flags().StringVar(&passwordEnv, "password-env", "", "Read the password from this environment variable (default: asked)")
	_ = cmd.MarkFlagRequired("universe")
	return cmd
}

// reportRestarts says which running members were restarted because they read
// a key written — the hub restarts them by itself — and which
// could not be.
func reportRestarts(cmd *cobra.Command, restarts []mktapi.MemberRestart) {
	if len(restarts) == 0 {
		return
	}
	o := cmd.OutOrStdout()
	fmt.Fprintln(o, "\nrestarted to use the keys saved:")
	for _, r := range restarts {
		where := filepath.Base(r.Universe)
		switch {
		case r.Member == "":
			fmt.Fprintf(o, "  universe %s: %s\n", where, r.Error)
		case r.Error != "":
			fmt.Fprintf(o, "  %s (universe %s, reads %s): NOT restarted: %s\n", r.Member, where, strings.Join(r.Keys, ", "), r.Error)
		default:
			fmt.Fprintf(o, "  %s (universe %s, reads %s)\n", r.Member, where, strings.Join(r.Keys, ", "))
		}
	}
}

// readKeysFile reads a keys file given by --keys and its password; nothing
// when no file is given.
func readKeysFile(cmd *cobra.Command, path, passwordEnv string) ([]byte, string, error) {
	if path == "" {
		return nil, "", nil
	}
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	password, err := readPassword(cmd, passwordEnv, false)
	return file, password, err
}

// readPassword takes the password from the named environment variable, or
// asks for it on the terminal (twice when it seals a new file).
func readPassword(cmd *cobra.Command, envVar string, twice bool) (string, error) {
	if envVar != "" {
		p := os.Getenv(envVar)
		if p == "" {
			return "", fmt.Errorf("--password-env %s: the variable is not set", envVar)
		}
		return p, nil
	}
	if !isTerminal() {
		return "", fmt.Errorf("no terminal to ask the password on: give --password-env <VAR>")
	}
	o := cmd.OutOrStdout()
	fmt.Fprint(o, "Password: ")
	p, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(o)
	if err != nil {
		return "", err
	}
	if twice {
		fmt.Fprint(o, "Password again: ")
		again, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(o)
		if err != nil {
			return "", err
		}
		if string(again) != string(p) {
			return "", fmt.Errorf("the two passwords differ")
		}
	}
	return string(p), nil
}

func confirm(cmd *cobra.Command, question string) (bool, error) {
	if !isTerminal() {
		return false, fmt.Errorf("no terminal to confirm on: give --yes")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N] ", question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes", nil
}

func reportKeys(cmd *cobra.Command, keys []mktapi.KeyEntry) {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "  NAME\tSCOPE\tSTATE")
	for _, k := range keys {
		fmt.Fprintf(w, "  %s\t%s\t%s\n", k.Name, k.Scope, k.State)
	}
	_ = w.Flush()
}
