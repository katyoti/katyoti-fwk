// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// NewInitCommand returns `ktw-univers init [flags]`.
//
// `init` initializes an EXISTING folder as a universe — writes the
// `<homePath>/.katyoti/config/Univers.yaml` manifest + skeleton files, and
// adds the universe to `~/.config/katyoti/universes.yaml`.
//
// Differs from `create`:
//   - `create <name>` creates a NEW folder under `~/.config/katyoti/universes/<name>/`
//   - `init` takes an EXISTING folder (typically the value of `KTW_WORKSPACE`)
//     and turns it into a universe in place.
//
// Default home: KTW_WORKSPACE → CWD. The folder must exist and must NOT already
// contain a Univers.yaml.
func NewInitCommand() *cobra.Command {
	var (
		name        string
		displayName string
		description string
		icon        string
		color       string
		homePath    string
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize an existing folder as a universe",
		Long: `Writes <homePath>/.katyoti/config/Univers.yaml + skeleton files (config/Settings.yaml, .env) and registers the universe in ~/.config/katyoti/universes.yaml.

The folder must exist; it must NOT already contain a Univers.yaml.

Default homePath:
  1. --home flag
  2. $KTW_WORKSPACE env var
  3. current working directory

Example:
  cd ~/projects/my-universe
  ktw-univers init --name my-universe --display "My Universe" --icon M --color teal`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve target homePath.
			home := homePath
			if home == "" {
				home = os.Getenv("KTW_WORKSPACE")
			}
			if home == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("resolve cwd: %w", err)
				}
				home = cwd
			}
			abs, err := filepath.Abs(home)
			if err != nil {
				return fmt.Errorf("absolutize %q: %w", home, err)
			}
			home = filepath.Clean(abs)

			// Folder must exist.
			info, err := os.Stat(home)
			if err != nil {
				return fmt.Errorf("homePath %q: %w", home, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("homePath %q is not a directory", home)
			}

			// Univers.yaml must NOT already exist (post-cutover layout).
			manifestPath := filepath.Join(application.EnvOf(home).ConfigDir(), "Univers.yaml")
			if _, err := os.Stat(manifestPath); err == nil {
				return fmt.Errorf("Univers.yaml already exists at %s — this folder is already a universe", manifestPath)
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("stat %s: %w", manifestPath, err)
			}
			// Refuse if a pre-cutover Univers.yaml is present — user must
			// migrate first via the GUI (which runs cenv.MigrateLegacyLayout)
			// or by hand.
			if _, err := os.Stat(filepath.Join(home, "config", "Univers.yaml")); err == nil {
				return fmt.Errorf("universe %s is in pre-cutover layout (config/Univers.yaml) — open it in ktw-gui to auto-migrate, or delete config/Univers.yaml and re-init", home)
			}

			// Default name: basename of home (sanitized).
			if name == "" {
				name = sanitizeUniverseName(filepath.Base(home))
			}
			if name == "" {
				return fmt.Errorf("could not derive a valid universe name from %q — pass --name", home)
			}
			if displayName == "" {
				displayName = strings.ToUpper(name[:1]) + name[1:]
			}
			if icon == "" {
				icon = strings.ToUpper(name[:1])
			}
			if color == "" {
				color = "violet"
			}

			// Build + write the manifest.
			manifest := &catalog.Manifest{
				Version:     1,
				Name:        name,
				DisplayName: displayName,
				Description: description,
				Icon:        icon,
				Color:       color,
			}
			if err := catalog.SaveManifest(home, manifest); err != nil {
				return err
			}

			// Skeleton: .env and Settings.yaml inside the workspace's home
			// + config dirs. Each write is a no-op if the file already exists
			// (don't overwrite user content). Sidecars live in Univers.yaml,
			// in its sidecars: block.
			env := application.EnvOf(home)
			katyoti := env.HomeDir()
			configDir := env.ConfigDir()
			if err := writeIfAbsent(filepath.Join(katyoti, ".env"), nil); err != nil {
				return err
			}
			if err := writeIfAbsent(filepath.Join(configDir, "Settings.yaml"), []byte("# Universe-scoped Settings.yaml\n")); err != nil {
				return err
			}

			// Add registry entry. If the name is already in the registry but
			// points elsewhere, refuse — the user must rename.
			regPath, err := catalog.DefaultRegistryPath()
			if err != nil {
				return fmt.Errorf("resolve registry: %w", err)
			}
			reg, err := catalog.LoadRegistry(regPath)
			if err != nil {
				return fmt.Errorf("load registry %s: %w", regPath, err)
			}
			if existing, err := reg.Find(name); err == nil {
				if existing.HomePath != home {
					return fmt.Errorf("universe %q already in registry pointing at %s — pass a different --name", name, existing.HomePath)
				}
				// Already registered at this home — idempotent success.
			} else {
				if err := reg.Add(catalog.RegistryEntry{Name: name, HomePath: home}); err != nil {
					return fmt.Errorf("register universe: %w", err)
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Initialized universe %q at %s\n", name, home)
			fmt.Fprintf(cmd.OutOrStdout(), "  display:  %s\n", displayName)
			fmt.Fprintf(cmd.OutOrStdout(), "  icon:     %s\n", icon)
			fmt.Fprintf(cmd.OutOrStdout(), "  color:    %s\n", color)
			fmt.Fprintf(cmd.OutOrStdout(), "  registry: %s\n", regPath)
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Add sidecars to the sidecars: block of Univers.yaml; run any ktw-* command with KTW_WORKSPACE pointing here.")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Universe slug (default: basename of home folder, sanitized)")
	cmd.Flags().StringVar(&displayName, "display", "", "Human-readable name (default: capitalized <name>)")
	cmd.Flags().StringVar(&description, "description", "", "Optional description")
	cmd.Flags().StringVar(&icon, "icon", "", "1-2 char IconRibbon icon (default: first char of name uppercased)")
	cmd.Flags().StringVar(&color, "color", "", "Mantine color preset (default: violet)")
	cmd.Flags().StringVar(&homePath, "home", "", "Folder to initialize (default: $KTW_WORKSPACE → CWD)")
	return cmd
}

// sanitizeUniverseName converts a folder basename into a valid universe slug
// (`^[a-z0-9-]{1,64}$`). Strips leading dots, lowercases, replaces invalid
// chars with hyphens, collapses repeated hyphens, trims edges. Returns "" if
// the result is empty.
func sanitizeUniverseName(in string) string {
	in = strings.TrimLeft(in, ".")
	in = strings.ToLower(in)
	var b strings.Builder
	for _, r := range in {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-")
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// writeIfAbsent writes data to path if the file doesn't exist. Creates parent
// dirs as needed. Returns nil if the file already exists (idempotent).
func writeIfAbsent(path string, data []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Compile-time interface check (catches drift if api.SidecarSpec ever
// changes shape — the manifest reuses it).
var _ api.SidecarSpec = api.SidecarSpec{}
