// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane/shortcut"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// Command builds `instance create | list | gui | run | remove | delete`.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance",
		Short: "Several katyoti installations side by side: each its own universes, data and control plane at its own version",
		Long: `An instance is a katyoti installation of its own — its registry, universes, data,
caches and control plane at its own version — kept in one folder and named.
The folder's katyoti.yml says where its roots are and how it starts; every
program of the installation reads it, so two instances never share a hub, a
registry or a binary, and two versions run at the same time. The installer
makes one too: every installation that ran is listed.

  katyoti instance create next --infra v1.4.0     # its own control plane, at v1.4.0
  katyoti instance gui next                       # the GUI of that instance
  katyoti instance run next -- katyoti universe list
  katyoti instance list
  katyoti instance delete old                     # what it takes away, and the token to confirm`,
	}
	cmd.AddCommand(createCommand(), adoptCommand(), listCommand(), guiCommand(), runCommand(), removeCommand(), deleteCommand())
	return cmd
}

func createCommand() *cobra.Command {
	var folder, channel, infra string
	var noShortcut bool
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an instance: its folder, its control plane from the channel (at --infra), its marketplace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !validName(name) {
				return fmt.Errorf("%q: an instance name is one word (letters, digits, dashes)", name)
			}
			reg, err := Load()
			if err != nil {
				return err
			}
			if _, err := reg.Find(name); err == nil {
				return fmt.Errorf("instance %q already exists", name)
			}
			if folder == "" {
				folder = DefaultFolder(name)
			}
			if folder, err = filepath.Abs(folder); err != nil {
				return err
			}
			if entries, _ := os.ReadDir(folder); len(entries) > 0 {
				return fmt.Errorf("%s already holds files: an instance starts in an empty folder", folder)
			}
			inst := Instance{Name: name, Folder: folder}
			if err := os.MkdirAll(folder, 0o755); err != nil {
				return err
			}
			if err := inst.EnsureFile(); err != nil {
				return err
			}
			for _, d := range []string{inst.Root(), inst.Data(), inst.Cache()} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					return err
				}
			}
			// The channel the control plane comes from: given, else the one this
			// installation uses (its record, with its trust).
			if err := writeChannel(inst, channel); err != nil {
				return err
			}
			// Registered before the control plane is installed: a failed install
			// leaves an instance to repair (`katyoti instance run <name> -- katyoti
			// setup`), not a folder nobody knows about.
			reg.Instances = append(reg.Instances, inst)
			if err := reg.Save(); err != nil {
				return err
			}
			self, err := os.Executable()
			if err != nil {
				return err
			}
			// setup makes the instance's shortcuts « KaTyoti <name> », as for
			// any installation.
			setupArgs := []string{"setup", "--no-gui"}
			if noShortcut {
				setupArgs = append(setupArgs, "--no-shortcut")
			}
			if err := inst.run(cmd, self, setupArgs...); err != nil {
				return fmt.Errorf("instance %s: setup: %w", name, err)
			}
			if infra != "" {
				if err := inst.run(cmd, self, "update", "--infra", "--to", infra); err != nil {
					return fmt.Errorf("instance %s: control plane to %s: %w", name, infra, err)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "instance %s created in %s — open it with `katyoti instance gui %s`\n", name, folder, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&folder, "folder", "", "Folder of the instance (default: the user's programs folder, Katyoti/<name>; must be empty)")
	cmd.Flags().StringVar(&channel, "channel", "", "Infra channel the control plane comes from (default: this installation's)")
	cmd.Flags().StringVar(&infra, "infra", "", "Control-plane version (default: the channel's current)")
	cmd.Flags().BoolVar(&noShortcut, "no-shortcut", false, "Do not make its Start-menu and Desktop shortcuts (Windows)")
	return cmd
}

// writeChannel records the instance's infra channel in its root.
func writeChannel(inst Instance, channel string) error {
	dst := filepath.Join(inst.Root(), "infra-channel.yaml")
	if channel != "" {
		return os.WriteFile(dst, []byte("root: "+filepath.ToSlash(channel)+"\n"), 0o644)
	}
	src := filepath.Join(application.Machine().Root, "infra-channel.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("this installation records no infra channel (%s): give --channel", src)
	}
	return os.WriteFile(dst, data, 0o644)
}

func listCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the instances: control-plane version, folder, size and last use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, err := Load()
			if err != nil {
				return err
			}
			if len(reg.Instances) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no instance — create one with `katyoti instance create <name>`")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tINSTALLATION\tCONTROL PLANE\tFOLDER\tSIZE\tLAST USE")
			for _, i := range reg.Instances {
				u := i.Usage()
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", i.Name, i.Title(), i.Version(), i.Folder, humanBytes(u.Bytes), lastUse(u.LastUsed))
			}
			return w.Flush()
		},
	}
}

// Version is the instance's control-plane version, from its installed record.
func (i Instance) Version() string {
	sy, err := catalog.LoadSidecarsYaml(filepath.Join(i.Root(), "sidecars.yaml"))
	if err != nil || sy == nil {
		return "?"
	}
	for _, e := range sy.Sidecars {
		if e.Name == "katyoti" && e.Version != "" {
			return e.Version
		}
	}
	return "(none)"
}

func guiCommand() *cobra.Command {
	var universe string
	cmd := &cobra.Command{
		Use:   "gui <name>",
		Short: "Open the GUI of an instance (its own control plane, at its version)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := find(args[0])
			if err != nil {
				return err
			}
			if err := inst.EnsureFile(); err != nil {
				return err
			}
			katyoti := inst.ControlPlane("katyoti")
			if _, err := os.Stat(katyoti); err != nil {
				return fmt.Errorf("instance %s has no control plane (%s): repair it with `katyoti instance run %s -- katyoti setup --no-gui`", inst.Name, katyoti, inst.Name)
			}
			a := []string{"gui"}
			if universe != "" {
				a = append(a, "--universe", universe)
			}
			return inst.run(cmd, katyoti, a...)
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "Universe to open")
	return cmd
}

func runCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run <name> -- <command> [args…]",
		Short: "Run a command inside an instance (it reads the instance's katyoti.yml: root, data, cache)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := find(args[0])
			if err != nil {
				return err
			}
			if err := inst.EnsureFile(); err != nil {
				return err
			}
			prog := args[1]
			// `katyoti` inside an instance is the instance's own, at its version.
			if prog == "katyoti" {
				if cp := inst.ControlPlane("katyoti"); fileExists(cp) {
					prog = cp
				}
			}
			cmd.SilenceUsage = true
			return inst.run(cmd, prog, args[2:]...)
		},
	}
}

func removeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Forget an instance and remove its shortcuts (its folder is kept: delete it yourself once nothing of it runs)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := Load()
			if err != nil {
				return err
			}
			inst, err := reg.Find(args[0])
			if err != nil {
				return err
			}
			reg.Instances = slicesDelete(reg.Instances, inst.Name)
			if err := reg.Save(); err != nil {
				return err
			}
			shortcut.Remove(inst.installation().Name)
			fmt.Fprintf(cmd.OutOrStdout(), "instance %s forgotten; its folder %s is kept\n", inst.Name, inst.Folder)
			return nil
		},
	}
}

// deleteCommand deletes an earlier installation once the person acknowledged
// what goes with it: without --ack it only says what.
func deleteCommand() *cobra.Command {
	var ack string
	var last bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an installation - its folder, its config, data and cache, its shortcuts - once you acknowledge what is lost (--ack <token>)",
		Long: `Without --ack, says what deleting the installation takes away - each folder
with its size and last use, its shortcuts - and the token that acknowledges it;
nothing is touched. With --ack <that token> it is deleted. The installation this
command runs from is never deleted, nor one whose projects folder lies inside
it; the last installation is kept unless --last.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := Load()
			if err != nil {
				return err
			}
			current := ""
			if inst := cenv.CurrentInstallation(); inst != nil {
				current = inst.Dir()
			}
			plan, err := PlanDelete(reg, args[0], current, !last)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if ack == "" {
				fmt.Fprintf(out, "Deleting %s takes away:\n", plan.Instance.Name)
				for _, f := range plan.Folders {
					fmt.Fprintf(out, "  %s  %s, last used %s\n", f.Path, humanBytes(f.Bytes), lastUse(f.LastUsed))
				}
				fmt.Fprintf(out, "  its shortcuts « KaTyoti %s »\n", plan.Shortcuts)
				fmt.Fprintln(out, "Its universes, their sessions and its settings go with it; projects in your projects folder stay.")
				fmt.Fprintf(out, "To delete it: katyoti instance delete %s --ack %s\n", plan.Instance.Name, plan.Token)
				return nil
			}
			cmd.SilenceUsage = true
			if err := plan.Apply(reg, ack); err != nil {
				return err
			}
			fmt.Fprintf(out, "%s deleted\n", plan.Instance.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&ack, "ack", "", "The token the plan gave: you acknowledge what it lists is lost")
	cmd.Flags().BoolVar(&last, "last", false, "Delete it even when it is the last installation")
	return cmd
}

func find(name string) (Instance, error) {
	reg, err := Load()
	if err != nil {
		return Instance{}, err
	}
	return reg.Find(name)
}

// run runs prog inside the instance, attached to the command's streams.
func (i Instance) run(cmd *cobra.Command, prog string, args ...string) error {
	c := exec.Command(prog, args...)
	c.Env = i.Env(os.Environ())
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr()
	err := c.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return fmt.Errorf("%s exited with %d", filepath.Base(prog), exit.ExitCode())
	}
	return err
}

func slicesDelete(in []Instance, name string) []Instance {
	out := in[:0]
	for _, i := range in {
		if i.Name != name {
			out = append(out, i)
		}
	}
	return out
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// adoptCommand makes an existing katyoti root an installation in place: the
// dev channel's root (~/.config/katyoti), which make install and make dev
// fill, adopted once - its control plane then belongs to an
// installation, isolated from the stable one, and nothing moves.
func adoptCommand() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "adopt <root>",
		Short: "Make an existing katyoti root an installation in place: a katyoti.yml with its roots where they are (the dev channel's root)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := cenv.AdoptRoot(args[0], name, application.Env().CacheDir())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s is the installation %s (config %s, data %s, cache %s)\n",
				inst.Path(), inst.Name, inst.ConfigRoot(), inst.DataRoot(), inst.CacheRoot())
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "katyoti-dev", "The installation's name")
	return cmd
}
