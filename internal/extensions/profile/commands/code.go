// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person"
)

// codeCmd is `profile code`: the person's authenticator code, which proves a
// relayed act (level 2). Enrolling, confirming and revoking are the person's
// own: refused from an agent's session, where the secret must never show.
func codeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "code",
		Short: "Your authenticator code: enrol it once, then it proves your acts relayed from a Claude Code conversation",
		Long: `A code from your authenticator app (Microsoft Authenticator, Google
Authenticator, 1Password...) proves an act a Claude Code session relays for you:
it is checked by KaTyoti, used once, for that act only. The secret is kept on
this machine, sealed for your Windows account, apart from your profile.`,
		Args: cobra.NoArgs,
	}
	mine := func(c *cobra.Command) error {
		if _, agent := person.AgentSessionCause(os.Getenv); agent {
			return fmt.Errorf("profile code %s is yours alone - not from an agent's session: run it in your own terminal", c.Name())
		}
		return nil
	}
	enrol := &cobra.Command{
		Use:   "enrol",
		Short: "Enrol a new code (replaces the old one): add the key to your app, then confirm with its first code",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := mine(c); err != nil {
				return err
			}
			account := core.ReadHandle(filepath.Join(application.Env().GlobalHomeDir(), "config"))
			if account == "" {
				return fmt.Errorf("set your profile first (katyoti profile init)")
			}
			uri, key, err := person.CodeStore().Enrol("KaTyoti", account)
			if err != nil {
				return err
			}
			w := c.OutOrStdout()
			fmt.Fprintf(w, "In your authenticator app, add an account by key (time-based, 6 digits, 30 s):\n\n  account: KaTyoti (%s)\n  key:     %s\n\nor from this link (a QR code generator turns it into a code to scan):\n\n  %s\n\nThen confirm with the code the app shows: katyoti profile code confirm <code>\n", account, key, uri)
			return nil
		},
	}
	confirm := &cobra.Command{
		Use:   "confirm <code>",
		Short: "Confirm the enrolment with the code your app shows",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := mine(c); err != nil {
				return err
			}
			if err := person.CodeStore().Confirm(args[0]); err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), "code: confirmed - it now proves your relayed acts")
			return nil
		},
	}
	revoke := &cobra.Command{
		Use:   "revoke",
		Short: "Remove the enrolment: no code proves anything until you enrol again",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := mine(c); err != nil {
				return err
			}
			if err := person.CodeStore().Revoke(); err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), "code: revoked")
			return nil
		},
	}
	status := &cobra.Command{
		Use:   "status",
		Short: "Whether a code is enrolled and confirmed",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			b, _ := json.MarshalIndent(person.CodeStore().Status(), "", "  ")
			fmt.Fprintln(c.OutOrStdout(), string(b))
			return nil
		},
	}
	cmd.AddCommand(enrol, confirm, revoke, status)
	return cmd
}
