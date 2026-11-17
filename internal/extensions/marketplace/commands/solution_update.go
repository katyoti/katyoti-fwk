// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/wire"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// UPDATE AN INSTALLED SOLUTION: the end-user update path, the
// one the workshop takes too. In one action: the running work drained (a
// universe with work running is refused, never cut), the universe stopped, its
// project's data backed up, the new version's members installed and every
// universe of the solution repinned, the universe started, the data migrated
// with the new ktw-agents (dry run first), and a smoke test. A step that fails
// after the backup rolls back: the universe stopped, the data restored, the
// previous version repinned, the universe started again.

// solutionUpdater is what an update does to a universe, one call per step: a
// seam, so the order and the rollback are tested without a hub.
type solutionUpdater interface {
	// Repin moves (or, dry, says it would move) the solution to version.
	Repin(ctx context.Context, version string, dry bool) (mktapi.RepinSolutionOutput, error)
	// RunningWork says what runs in the universe ("" nothing).
	RunningWork(ctx context.Context) (string, error)
	Stop(ctx context.Context) error
	// Backup copies the project's data aside and answers where.
	Backup() (string, error)
	Restore(dir string) error
	Start(ctx context.Context) error
	// Migrate brings the data to the serving ktw-agents's data version.
	Migrate(ctx context.Context, dry bool) (string, error)
	// Smoke checks the universe serves at to.
	Smoke(ctx context.Context, to mktapi.RepinSolutionOutput) error
}

// updateSolution runs the update to version ("" the newest), writing what it
// does to out. It answers the repin's report.
func updateSolution(ctx context.Context, u solutionUpdater, version string, dry bool, out io.Writer) (mktapi.RepinSolutionOutput, error) {
	plan, err := u.Repin(ctx, version, true)
	if err != nil {
		return plan, err
	}
	if plan.To == plan.From {
		fmt.Fprintf(out, "%s is already at %s\n", plan.Solution, plan.To)
		return plan, nil
	}
	fmt.Fprintf(out, "%s %s → %s\n", plan.Solution, plan.From, plan.To)
	for _, m := range plan.Members {
		fmt.Fprintf(out, "  %s %s → %s\n", m.Name, orAbsent(m.From), orAbsent(m.To))
	}
	running, err := u.RunningWork(ctx)
	if err != nil {
		return plan, fmt.Errorf("update refused: cannot read the running work: %w", err)
	}
	if running != "" {
		return plan, fmt.Errorf("update refused: work is running (%s) - update once it has ended", running)
	}
	if dry {
		fmt.Fprintln(out, "dry run: nothing changed (the migration's dry run comes with the update, from the new version)")
		return plan, nil
	}
	if err := u.Stop(ctx); err != nil {
		return plan, fmt.Errorf("update refused: the universe did not stop: %w", err)
	}
	backup, err := u.Backup()
	if err != nil {
		return plan, fmt.Errorf("update refused: the data was not backed up (nothing changed): %w", err)
	}
	if backup != "" {
		fmt.Fprintf(out, "data backed up to %s\n", backup)
	}
	fail := func(step string, cause error) (mktapi.RepinSolutionOutput, error) {
		fmt.Fprintf(out, "%s failed: %v - rolling back to %s\n", step, cause, plan.From)
		var problems []string
		if err := u.Stop(ctx); err != nil {
			problems = append(problems, "stop: "+err.Error())
		}
		if backup != "" {
			if err := u.Restore(backup); err != nil {
				problems = append(problems, "restore the data: "+err.Error())
			}
		}
		if _, err := u.Repin(ctx, plan.From, false); err != nil {
			problems = append(problems, "repin "+plan.From+": "+err.Error())
		}
		if err := u.Start(ctx); err != nil {
			problems = append(problems, "start: "+err.Error())
		}
		if len(problems) > 0 {
			return plan, fmt.Errorf("update to %s failed at %s (%v), and the rollback did not complete: %s - the data is kept at %s",
				plan.To, step, cause, strings.Join(problems, "; "), backup)
		}
		return plan, fmt.Errorf("update to %s failed at %s (%v): rolled back to %s, the data restored", plan.To, step, cause, plan.From)
	}
	if _, err := u.Repin(ctx, plan.To, false); err != nil {
		return fail("the repin", err)
	}
	if err := u.Start(ctx); err != nil {
		return fail("the start", err)
	}
	dryRun, err := u.Migrate(ctx, true)
	if err != nil {
		return fail("the migration's dry run", err)
	}
	fmt.Fprintf(out, "migration (dry run): %s\n", dryRun)
	done, err := u.Migrate(ctx, false)
	if err != nil {
		return fail("the migration", err)
	}
	fmt.Fprintf(out, "migration: %s\n", done)
	if err := u.Smoke(ctx, plan); err != nil {
		return fail("the smoke test", err)
	}
	fmt.Fprintf(out, "%s is at %s\n", plan.Solution, plan.To)
	return plan, nil
}

func orAbsent(v string) string {
	if v == "" {
		return "(none)"
	}
	return v
}

// hubUpdater updates the universe at home through the marketplace service and
// the universe's own hub.
type hubUpdater struct {
	svc  mktapi.SetupService
	home string
	out  io.Writer
}

func (h *hubUpdater) Repin(ctx context.Context, version string, dry bool) (mktapi.RepinSolutionOutput, error) {
	return h.svc.RepinSolution(ctx, mktapi.RepinSolutionInput{UniverseHome: h.home, Version: version, DryRun: dry})
}

// call asks the universe's running hub; ok false when none runs.
func (h *hubUpdater) call(ctx context.Context, uri string, in any) (json.RawMessage, bool, error) {
	env := application.EnvOf(h.home)
	if lf, err := hubclient.ReadLockFile(env.DataDir()); err != nil || lf == nil || !hubclient.ProcessAlive(lf.PID) {
		return nil, false, nil
	}
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: env.DataDir(), Mode: hubclient.ModeReuse})
	if err != nil {
		return nil, true, err
	}
	defer hc.Close()
	tenantID, universeID := cenv.ResolveTenantUniverse(env.UniverseDir())
	sender := messagebus.NewRetrySender(messagebus.NewSender(hc.MessageBus(), tenantID, universeID), "ktw-hub")
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	raw, err := sender.Call(cctx, uri, in)
	return raw, true, err
}

// countIn counts the entries of the first list in a JSON answer.
func countIn(raw json.RawMessage) int {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return 0
	}
	for _, v := range m {
		var list []json.RawMessage
		if json.Unmarshal(v, &list) == nil && len(list) > 0 {
			return len(list)
		}
	}
	return 0
}

func (h *hubUpdater) RunningWork(ctx context.Context) (string, error) {
	runs, up, err := h.call(ctx, "ktw-agents:/tasks/runs", map[string]any{"status": "active"})
	if !up || err != nil {
		return "", err
	}
	todos, _, err := h.call(ctx, "ktw-agents:/todos/list", map[string]any{"state": "claimed"})
	if err != nil {
		return "", err
	}
	var parts []string
	if n := countIn(runs); n > 0 {
		parts = append(parts, fmt.Sprintf("%d task run(s)", n))
	}
	if n := countIn(todos); n > 0 {
		parts = append(parts, fmt.Sprintf("%d to-do(s) held by a worker", n))
	}
	return strings.Join(parts, ", "), nil
}

func (h *hubUpdater) Stop(ctx context.Context) error { return stopUniverse(ctx, h.home, h.out) }

func (h *hubUpdater) Start(ctx context.Context) error {
	_, err := startUniverse(ctx, h.home, runtimeOptions{}, h.out)
	return err
}

// project is the folder the universe serves ("" none).
func (h *hubUpdater) project() string {
	if ps := cenv.AttachedProjects(application.EnvOf(h.home).HomeDir()); len(ps) > 0 {
		return ps[0]
	}
	return ""
}

func (h *hubUpdater) Backup() (string, error) {
	project := h.project()
	if project == "" {
		return "", nil
	}
	dir := filepath.Join(application.EnvOf(h.home).DataDir(), "backups", "update-"+time.Now().UTC().Format("20060102-150405"))
	return dir, backupProject(project, dir)
}

func (h *hubUpdater) Restore(dir string) error { return restoreProject(h.project(), dir) }

func (h *hubUpdater) Migrate(ctx context.Context, dry bool) (string, error) {
	raw, up, err := h.call(ctx, "ktw-agents:/cwm/migrate", map[string]any{"dry_run": dry})
	if err != nil {
		return "", err
	}
	if !up {
		return "", fmt.Errorf("the universe's hub is not running")
	}
	var mo struct {
		From  int `json:"from"`
		To    int `json:"to"`
		Steps []struct {
			Changed []string `json:"changed"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(raw, &mo); err != nil {
		return "", err
	}
	if mo.From == mo.To {
		return fmt.Sprintf("the data is at version %d, nothing to migrate", mo.To), nil
	}
	n := 0
	for _, s := range mo.Steps {
		n += len(s.Changed)
	}
	return fmt.Sprintf("data version %d → %d, %d step(s), %d object change(s)", mo.From, mo.To, len(mo.Steps), n), nil
}

// Smoke: the serving ktw-agents answers through the hub, and every universe
// of the solution pins the new version's members.
func (h *hubUpdater) Smoke(ctx context.Context, to mktapi.RepinSolutionOutput) error {
	if _, up, err := h.call(ctx, "ktw-agents:/tasks/groups", map[string]any{}); err != nil || !up {
		return fmt.Errorf("ktw-agents does not answer through the hub (%v)", err)
	}
	for _, home := range to.Universes {
		for _, m := range to.Members {
			if m.To != "" && !wire.PinsVersion(home, m.Name, m.To) {
				return fmt.Errorf("%s does not pin %s@%s", filepath.Base(home), m.Name, m.To)
			}
		}
	}
	return nil
}

// backupProject copies the project object and the modules folder to dir.
func backupProject(project, dir string) error {
	f, err := projectfile.Read(project)
	if err != nil {
		return err
	}
	if err := copyPath(filepath.Join(project, projectfile.FileName), filepath.Join(dir, projectfile.FileName)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return copyPath(filepath.Join(project, f.ModulesDir), filepath.Join(dir, f.ModulesDir))
}

// restoreProject puts back what backupProject copied: the modules folder and
// the project object as they were, what came after them removed.
func restoreProject(project, dir string) error {
	if project == "" || dir == "" {
		return nil
	}
	f, err := projectfile.Read(dir)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(project, f.ModulesDir)); err != nil {
		return err
	}
	if err := copyPath(filepath.Join(dir, f.ModulesDir), filepath.Join(project, f.ModulesDir)); err != nil {
		return err
	}
	if err := copyPath(filepath.Join(dir, projectfile.FileName), filepath.Join(project, projectfile.FileName)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// copyPath copies a file or a folder tree.
func copyPath(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		b, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, st.Mode().Perm())
	}
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return copyPath(p, filepath.Join(dst, rel))
	})
}

func solutionUpdateCommand() *cobra.Command {
	var universe, to string
	var dry bool
	cmd := &cobra.Command{
		Use:   "update --universe <name|folder> [--to <version>]",
		Short: "Update an installed solution to a newer version: work drained, data backed up and migrated, members repinned, smoke-tested, rolled back on failure",
		Long: `Move the universes a solution installed to another of its versions (default:
its newest) in one action. A universe with work running (task runs, to-dos a
worker holds) is refused. Then: the universe stopped, its project's data
backed up (<data>/backups/update-<time>), the version's members installed and
every universe of the solution repinned, the universe started, its data
migrated by the new ktw-agents (dry run first), and a smoke test. A failure
after the backup rolls back: the data restored, the previous version repinned,
the universe started again. --dry-run says what would move.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(universe) == "" {
				return fmt.Errorf("name the solution's main universe (--universe <name|folder>)")
			}
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			u := &hubUpdater{svc: s, home: resolveUniverseTarget(cmd, universe), out: cmd.OutOrStdout()}
			_, err = updateSolution(cmd.Context(), u, to, dry, cmd.OutOrStdout())
			return err
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "The solution's main universe: a registered name or its folder")
	cmd.Flags().StringVar(&to, "to", "", "The version to move to (default: the newest)")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "Say what would move and change nothing")
	return cmd
}
