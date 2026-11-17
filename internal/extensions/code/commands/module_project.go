// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/projection"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucobj"
	"github.com/spf13/cobra"
)

func newModuleProjectCommand() *cobra.Command {
	var (
		project string
		apply   bool
		check   bool
		docsDir string
		title   string
	)
	cmd := &cobra.Command{
		Use:   "project <module>",
		Short: "Regenerate the module's projections from its objects",
		Long: `Render the module's projections from its objects (use cases,
acceptance criteria, the latest test-run verdicts, decisions, audits):

  1. charter.md, WHOLE — through the charter publish (code.CharterPublish),
     its "## Test plan" section included
  2. manifest.json, WHOLE — through the manifest write (code.ManifestWrite),
     its "coverage" block included
  3. the browsable index use-cases/README.md (+ the docs use-cases page
     with --docs-dir)

Each file has ONE writer: this verb calls the publish, it never splices.

DRY-RUN BY DEFAULT — prints what would change. --apply writes;
--check exits 1 if any projection is stale (the CI guard).`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			module := args[0]
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			svc, err := application.UseService[api.CodeService](ctx)
			if err != nil {
				return fmt.Errorf("resolve CodeService: %w", err)
			}
			// The publish renders; a dry run hands back the bytes so the
			// staleness check compares exactly what --apply would write.
			charter, err := svc.CharterPublish(ctx, api.CharterPublishInput{ModuleID: module, DryRun: true})
			if err != nil {
				return err
			}
			manifest, err := svc.ManifestWrite(ctx, api.ManifestWriteInput{ModuleID: module, DryRun: true})
			if err != nil {
				return err
			}
			moduleDir := filepath.Dir(charter.Path)
			if project != "" {
				if err := underProject(project, moduleDir); err != nil {
					return err
				}
			}

			// The use cases are the module's use-case and acceptance-criterion
			// objects; verdicts come from its latest test-run, never from the
			// use cases themselves — a criterion declares what must be true; a
			// run says whether it was.
			verdicts, err := ucobj.LoadVerdicts(moduleDir, module)
			if err != nil {
				return fmt.Errorf("load verdicts: %w", err)
			}
			files, err := ucobj.LoadDir(moduleDir, module, verdicts)
			if err != nil {
				return fmt.Errorf("load use-case objects: %w", err)
			}
			if len(files) == 0 {
				return fmt.Errorf("no use-case objects under %s", moduleDir)
			}
			if findings := ucfile.Errors(ucfile.Lint(files, ucfile.Options{ModuleDir: moduleDir})); len(findings) > 0 {
				for _, f := range findings {
					fmt.Fprintf(out, "LINT %s\n", f)
				}
				return fmt.Errorf("the use cases have %d error findings — fix before projecting", len(findings))
			}

			type target struct {
				path string
				next []byte
				// publish writes the file through its one writer; nil means
				// this verb is that writer.
				publish func() error
			}
			targets := []target{
				{charter.Path, []byte(charter.Markdown), func() error {
					_, err := svc.CharterPublish(ctx, api.CharterPublishInput{ModuleID: module})
					return err
				}},
				{manifest.Path, []byte(manifest.JSON), func() error {
					_, err := svc.ManifestWrite(ctx, api.ManifestWriteInput{ModuleID: module})
					return err
				}},
			}

			// The browsable UC index, always rendered INSIDE the
			// use-cases dir. (Deliberately NOT the module's own
			// use-cases.md by default: those are doc-convention usage
			// scenarios — a different concept that must not be overwritten.)
			if title == "" {
				title = strings.ToUpper(module[:1]) + module[1:]
			}
			page := projection.RenderDocsUseCases(module, title, files)
			targets = append(targets, target{path: filepath.Join(moduleDir, "use-cases", "README.md"), next: []byte(page)})

			// Opt-in extra copy for modules whose docs use-cases page IS the
			// UC list.
			if docsDir != "" {
				targets = append(targets, target{path: filepath.Join(docsDir, "use-cases.md"), next: []byte(page)})
			}

			stale := 0
			for _, tg := range targets {
				current, _ := os.ReadFile(tg.path)
				if string(current) == string(tg.next) {
					fmt.Fprintf(out, "up-to-date  %s\n", tg.path)
					continue
				}
				stale++
				switch {
				case check:
					fmt.Fprintf(out, "STALE       %s\n", tg.path)
				case apply:
					if tg.publish != nil {
						if err := tg.publish(); err != nil {
							return err
						}
					} else {
						if err := os.MkdirAll(filepath.Dir(tg.path), 0o755); err != nil {
							return err
						}
						if err := os.WriteFile(tg.path, tg.next, 0o644); err != nil {
							return err
						}
					}
					fmt.Fprintf(out, "regenerated %s\n", tg.path)
				default:
					fmt.Fprintf(out, "would write %s\n", tg.path)
				}
			}

			if check && stale > 0 {
				cmd.SilenceErrors = true
				return fmt.Errorf("projections stale: %d file(s) — run `module project %s --apply`", stale, module)
			}
			if !apply && !check && stale > 0 {
				fmt.Fprintln(out, "\nDRY-RUN — pass --apply to write.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project root the module must belong to (default: the project the CWM service serves)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Write the regenerated projections (default is dry-run)")
	cmd.Flags().BoolVar(&check, "check", false, "Exit 1 if any projection is stale (CI guard; no writes)")
	cmd.Flags().StringVar(&docsDir, "docs-dir", "", "Also regenerate <docs-dir>/use-cases.md (e.g. modules/tasks)")
	cmd.Flags().StringVar(&title, "title", "", "Display title for the docs page (default: capitalized module name)")
	return cmd
}

// underProject refuses a module folder outside the project the caller named:
// the publish writes where the CWM service's project is, so a --project that
// names another one would regenerate files the caller is not looking at.
func underProject(project, moduleDir string) error {
	p, err := filepath.Abs(project)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(p, moduleDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("the CWM service serves the module at %s, outside --project %s", moduleDir, p)
	}
	return nil
}
