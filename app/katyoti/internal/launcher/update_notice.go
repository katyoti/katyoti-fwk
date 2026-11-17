// SPDX-License-Identifier: Apache-2.0

package launcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// checkInterval is how often a launch actually looks for updates.
//
// Every launch would be wrong twice over: it puts a network round-trip in front
// of the app on a path we already know is latency-sensitive, and it nags. Every
// launch WITHIN the interval prints nothing at all — silence means "checked
// recently", not "up to date", which is why the notice is only ever printed by
// a real check.
const checkInterval = 6 * time.Hour

// checkTimeout bounds the whole check. A launcher must never fail to launch, or
// even feel slow, because a channel was unreachable.
const checkTimeout = 3 * time.Second

// stampFile records when the last check ran. Only the TIMESTAMP is cached, not
// the result: caching "an update is available" would keep announcing it after
// the user updated.
const stampFile = ".update-check"

// noteAvailableUpdates prints a one-line notice when the control plane or an
// installed sidecar is behind, then returns.
//
// NOTIFY, NEVER MUTATE. Updating infra silently just before launching the GUI
// is tempting — nothing is running, so the swap is safe — but it would replace
// the binaries under a user who asked to open an app, and pay a ~300MB download
// before the window appears. Telling them, with the exact command, keeps the
// decision theirs.
//
// Every failure path is silent by design: a launcher that refuses to launch
// because an update check failed is worse than one that never checked.
func noteAvailableUpdates(cmd *cobra.Command, enabled bool) {
	if !enabled || !checkDue() {
		return
	}
	svc, err := application.UseService[mktapi.SetupService](cmd.Context())
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), checkTimeout)
	defer cancel()

	// Record the attempt BEFORE the slow part. A channel that is unreachable
	// every time would otherwise retry — and pay the timeout — on every launch.
	stampChecked()

	var lines []string

	// Infra reads the channel POINTER only (a few bytes), not the per-platform
	// checksum file, so the common "nothing to do" case costs one tiny read.
	if v, err := svc.InfraVersions(ctx); err == nil {
		if v.Installed != "" && v.Installed != v.Current {
			lines = append(lines, fmt.Sprintf(
				"  control plane %s → %s   `katyoti update --infra`", v.Installed, v.Current))
		}
	}
	if ups, err := svc.Outdated(ctx); err == nil && len(ups) > 0 {
		names := make([]string, 0, len(ups))
		for _, u := range ups {
			names = append(names, u.Name)
		}
		lines = append(lines, fmt.Sprintf(
			"  %d sidecar(s) behind: %s   `katyoti update --sidecars`", len(ups), strings.Join(names, ", ")))
	}
	if len(lines) == 0 {
		return
	}
	w := cmd.ErrOrStderr()
	if inst := cenv.CurrentInstallation(); inst != nil && inst.Locked {
		// A locked installation is told, never offered the update it refuses.
		fmt.Fprintf(w, "updates available, not applied: the installation %s is locked - its versions move only when a release is installed on it\n", inst.Name)
		for _, l := range lines {
			if head, _, ok := strings.Cut(l, "   `"); ok {
				l = head
			}
			fmt.Fprintln(w, l)
		}
		return
	}
	fmt.Fprintln(w, "updates available:")
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
	fmt.Fprintln(w, "  (`katyoti update` takes everything)")
}

func stampPath() string {
	return filepath.Join(application.Env().GlobalHomeDir(), stampFile)
}

// checkDue reports whether the interval has elapsed. A missing or unreadable
// stamp means "due" — a fresh machine should learn about updates on its first
// launch, not six hours later.
func checkDue() bool {
	info, err := os.Stat(stampPath())
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) >= checkInterval
}

func stampChecked() {
	p := stampPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	now := time.Now()
	if err := os.WriteFile(p, []byte(now.Format(time.RFC3339)), 0o644); err != nil {
		return
	}
	// Written content is informational; the MTIME is what checkDue reads, so
	// keep them in step even on a filesystem that would round the write time.
	_ = os.Chtimes(p, now, now)
}
