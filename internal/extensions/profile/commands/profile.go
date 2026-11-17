// SPDX-License-Identifier: Apache-2.0

// Package commands provides `katyoti profile`: show, set and init the profile
// of the person using the platform. It is a pure KTSP client of the
// ProfileService served in the admin universe — it never touches Profile.yaml.
package commands

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/remote"
)

// connect reaches the ProfileService; release frees the connection and stops a
// hub started for this command. Replaced in tests.
var connect = func() (svc api.ProfileService, release func(), err error) {
	// The launcher's pre-hub bootstrap: the admin universe must exist before
	// its hub can be started.
	if err := admin.EnsureDefault(); err != nil {
		return nil, nil, fmt.Errorf("prepare the admin universe: %w", err)
	}
	s := appcore.NewAdminHubSender()
	return remote.NewRemote(s), func() { _ = s.Close() }, nil
}

func withService(fn func(ctx context.Context, svc api.ProfileService) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		svc, release, err := connect()
		if err != nil {
			return err
		}
		defer release()
		return fn(cmd.Context(), svc)
	}
}

// NewProfileCommand builds `profile` (= `profile show`) with `set` and `init`.
func NewProfileCommand() *cobra.Command {
	show := showCmd()
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Show or set who you are (handle, name, email)",
		Long: `The profile of the person using KaTyoti: a stable handle (recorded as the
person on everything you do — never your email), first and last name, email.
Until you set one, you are the placeholder John Doe.

It is held by the admin universe's hub; this command starts that hub if none is
running. A local profile is what you say you are — it is not a login.`,
		Args: cobra.NoArgs,
		RunE: show.RunE,
	}
	cmd.Flags().AddFlagSet(show.Flags())
	cmd.AddCommand(show, setCmd(), initCmd(), relayCmd(), codeCmd())
	return cmd
}

func showCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the profile",
		Args:  cobra.NoArgs,
	}
	cmd.RunE = withService(func(ctx context.Context, svc api.ProfileService) error {
		p, err := svc.Get(ctx)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if asJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(p)
		}
		printProfile(out, p)
		return nil
	})
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the profile as JSON")
	return cmd
}

// relayCmd is `profile relay [on|off]`: whether an agent's session may relay
// the person's own acts once they confirmed each one. Turning it on or off is
// the person's act: the command carries its cause (person.CommandCause), so
// the service refuses it from an agent's session.
func relayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relay [on|off]",
		Short: "Show, or turn on or off, relaying your own acts from a Claude Code conversation",
		Long: `Relaying lets a Claude Code session do your own acts - decide, approve or send
back, answer, test or verify by hand, hold, waive - through the KaTyoti MCP
server's act tools, each one confirmed by you at Claude Code's prompt, and
recorded as relayed. It is off until you turn it on, and only you can.`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"on", "off"},
	}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return withService(func(ctx context.Context, svc api.ProfileService) error {
			var (
				r   api.RelayState
				err error
			)
			switch {
			case len(args) == 0:
				r, err = svc.RelayGet(ctx)
			case args[0] == "on" || args[0] == "off":
				r, err = svc.RelaySet(person.CommandCause(ctx), api.RelaySetInput{Enabled: args[0] == "on"})
			default:
				return fmt.Errorf("relay takes on or off, got %q", args[0])
			}
			if err != nil {
				return err
			}
			state := "off"
			if r.Enabled {
				state = "on"
			}
			fmt.Fprintf(c.OutOrStdout(), "relay: %s\n", state)
			return nil
		})(c, args)
	}
	return cmd
}

func printProfile(w io.Writer, p api.Profile) {
	source := p.Source
	if p.Source == api.SourcePlaceholder {
		source += " — not set yet; run `katyoti profile init`"
	}
	fmt.Fprintf(w, "id:      %s\nname:    %s\nemail:   %s\nsource:  %s\n", p.ID, p.DisplayName(), p.Email, source)
	if p.Language != "" {
		fmt.Fprintf(w, "language: %s\n", p.Language)
	}
}

func setCmd() *cobra.Command {
	var in api.UpdateInput
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set profile fields (the others keep their value)",
		Example: `  katyoti profile set --id jdoe --first-name Jane --last-name Doe --email jane.doe@example.com
  katyoti profile set --email new@example.com`,
		Args: cobra.NoArgs,
	}
	cmd.RunE = withService(func(ctx context.Context, svc api.ProfileService) error {
		cur, err := svc.Get(ctx)
		if err != nil {
			return err
		}
		next := api.UpdateInput{}
		if cur.Source != api.SourcePlaceholder { // never carry John Doe's values into a real profile
			next = api.UpdateInput{ID: cur.ID, FirstName: cur.FirstName, LastName: cur.LastName, Email: cur.Email, Language: cur.Language}
		}
		f := cmd.Flags()
		if f.Changed("id") {
			next.ID = in.ID
		}
		if f.Changed("first-name") {
			next.FirstName = in.FirstName
		}
		if f.Changed("last-name") {
			next.LastName = in.LastName
		}
		if f.Changed("email") {
			next.Email = in.Email
		}
		if f.Changed("language") {
			next.Language = in.Language
		}
		p, err := svc.Update(ctx, next)
		if err != nil {
			return err
		}
		printProfile(cmd.OutOrStdout(), p)
		return nil
	})
	cmd.Flags().StringVar(&in.ID, "id", "", "handle: lowercase letters, digits, hyphens (never the email)")
	cmd.Flags().StringVar(&in.FirstName, "first-name", "", "first name")
	cmd.Flags().StringVar(&in.LastName, "last-name", "", "last name")
	cmd.Flags().StringVar(&in.Email, "email", "", "email address")
	cmd.Flags().StringVar(&in.Language, "language", "", "language you read: en, fr, or empty to follow the operating system")
	return cmd
}

func initCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set the profile, pre-filled from what this machine knows about you",
		Long: `Proposes a profile from your OS account (account name, display name) and the
email addresses this machine knows (Office sign-in, login name, git). Each value
can be kept (Enter) or replaced. Nothing is saved until you confirm.`,
		Args: cobra.NoArgs,
	}
	cmd.RunE = withService(func(ctx context.Context, svc api.ProfileService) error {
		cur, err := svc.Get(ctx)
		if err != nil {
			return err
		}
		sug, err := svc.Suggest(ctx)
		if err != nil {
			return err
		}
		draft, emails := initialDraft(cur, sug)
		out := cmd.OutOrStdout()
		if !yes {
			var ok bool
			if draft, ok, err = prompt(bufio.NewReader(cmd.InOrStdin()), out, draft, emails); err != nil || !ok {
				if err == nil {
					fmt.Fprintln(out, "Nothing saved.")
				}
				return err
			}
		}
		p, err := svc.Update(ctx, draft)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "Saved:")
		printProfile(out, p)
		return nil
	})
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "accept the proposed values without prompting")
	return cmd
}

// initialDraft: the current profile when one is set, else the OS suggestion
// (first email candidate). The email candidates list the current email first.
func initialDraft(cur api.Profile, sug api.Suggestion) (api.UpdateInput, []api.EmailCandidate) {
	emails := sug.Emails
	if cur.Source != api.SourcePlaceholder {
		emails = append([]api.EmailCandidate{{Email: cur.Email, Source: "current"}}, withoutEmail(emails, cur.Email)...)
		return api.UpdateInput{ID: cur.ID, FirstName: cur.FirstName, LastName: cur.LastName, Email: cur.Email, Language: cur.Language}, emails
	}
	d := api.UpdateInput{ID: sug.ID, FirstName: sug.FirstName, LastName: sug.LastName}
	if len(emails) > 0 {
		d.Email = emails[0].Email
	}
	return d, emails
}

func withoutEmail(in []api.EmailCandidate, email string) []api.EmailCandidate {
	var out []api.EmailCandidate
	for _, c := range in {
		if !strings.EqualFold(c.Email, email) {
			out = append(out, c)
		}
	}
	return out
}

// prompt asks for each field (Enter keeps the proposal) and a confirmation.
func prompt(r *bufio.Reader, w io.Writer, d api.UpdateInput, emails []api.EmailCandidate) (api.UpdateInput, bool, error) {
	fmt.Fprintln(w, "Your profile (Enter keeps the value in brackets; nothing is saved until you confirm):")
	var err error
	ask := func(label, cur string) string {
		if err != nil {
			return cur
		}
		fmt.Fprintf(w, "  %s [%s]: ", label, cur)
		var line string
		line, err = readLine(r)
		if line == "" {
			return cur
		}
		return line
	}
	for {
		d.ID = ask("Handle (your id — never your email)", d.ID)
		if err != nil || api.ValidateHandle(d.ID) == nil {
			break
		}
		fmt.Fprintf(w, "  %v\n", api.ValidateHandle(d.ID))
	}
	d.FirstName = ask("First name", d.FirstName)
	d.LastName = ask("Last name", d.LastName)
	if len(emails) > 0 && err == nil {
		fmt.Fprintln(w, "  Email addresses found on this machine:")
		for i, c := range emails {
			fmt.Fprintf(w, "    %d) %s (%s)\n", i+1, c.Email, c.Source)
		}
	}
	for {
		v := ask("Email (a number above, or an address)", d.Email)
		if n, convErr := strconv.Atoi(v); convErr == nil && n >= 1 && n <= len(emails) {
			v = emails[n-1].Email
		}
		d.Email = v
		if err != nil || api.ValidateEmail(d.Email) == nil {
			break
		}
		fmt.Fprintf(w, "  %v\n", api.ValidateEmail(d.Email))
	}
	if err != nil {
		return d, false, err
	}
	fmt.Fprintf(w, "Save id=%s, name=%q, email=%s? [Y/n]: ", d.ID, strings.TrimSpace(d.FirstName+" "+d.LastName), d.Email)
	line, err := readLine(r)
	if err != nil {
		return d, false, err
	}
	return d, line == "" || strings.EqualFold(line, "y") || strings.EqualFold(line, "yes"), nil
}

// readLine returns the trimmed line; io.EOF on an empty, closed input.
func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err == io.EOF && line != "" {
		err = nil
	}
	return strings.TrimSpace(line), err
}
