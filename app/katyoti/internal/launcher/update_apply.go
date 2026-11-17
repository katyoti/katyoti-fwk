// SPDX-License-Identifier: Apache-2.0

package launcher

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	mktcommands "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/commands"
)

// applyInfraBeforeLaunch moves the CONTROL PLANE only — to an explicit channel
// version (update or roll back), or placed again from the channel (repair) —
// then returns so the caller launches the version just placed. Sidecars are not
// touched: they update from the Marketplace's Sidecars tab. The same pre-launch
// moment as --update, for the same reason: nothing being replaced is running.
func applyInfraBeforeLaunch(cmd *cobra.Command, version string, repair bool) {
	w := cmd.ErrOrStderr()
	infraSvc, err := application.UseService[mktapi.InfraService](cmd.Context())
	if err != nil {
		fmt.Fprintf(w, "control plane unavailable (%v) — launching anyway\n", err)
		return
	}
	if version != "" {
		done, err := infraSvc.SetInfraVersion(cmd.Context(), version)
		if err != nil {
			fmt.Fprintf(w, "control plane not moved to %s (%v)\n", version, err)
		}
		for _, u := range done {
			fmt.Fprintf(w, "control plane %s %s → %s\n", u.Name, u.Installed, u.Latest)
		}
		if err == nil && len(done) == 0 {
			fmt.Fprintf(w, "control plane already at %s\n", version)
		}
	}
	if repair {
		out, err := infraSvc.InstallInfra(cmd.Context())
		if err != nil {
			fmt.Fprintf(w, "control plane not reinstalled (%v)\n", err)
			return
		}
		for _, ins := range out.Installed {
			fmt.Fprintf(w, "control plane %s %s placed again\n", ins.Name, ins.Version)
		}
	}
}

// applyUpdatesBeforeLaunch brings the machine current, then returns so the
// caller can launch.
//
// OPT-IN ONLY, via `katyoti gui --update`. Launch is the safest moment there is
// to swap the control plane — the GUI is not running and neither is its hub, so
// the rename-aside swap has nothing to race — which is exactly why it is
// tempting to do it every time. It is still not done every time: a user who
// asked to open an app did not ask to wait on a ~300MB download, and a binary
// replaced under them without being asked is a surprise, not a service.
//
// The flag is on `gui` rather than on a bare `katyoti` because bare `katyoti` is
// the shortcut / double-click path, where no flag can be passed and a long
// silent wait is worst. That path notifies instead (see update_notice.go).
//
// Order matters: the swap lands the NEW ktw-gui at the control-plane path
// before we spawn it, so `--update` opens the version it just installed rather
// than the one it replaced.
//
// A failure here does NOT stop the launch. The user asked for two things — be
// current, and open the app — and failing the first is no reason to withhold
// the second, especially when the reason is usually an unreachable channel.
func applyUpdatesBeforeLaunch(cmd *cobra.Command) {
	w := cmd.ErrOrStderr()
	svc, err := application.UseService[mktapi.SetupService](cmd.Context())
	if err != nil {
		fmt.Fprintf(w, "update: unavailable (%v) — launching anyway\n", err)
		return
	}

	changed := false
	// Infra first: it is the thing being launched. A sidecar failure must not
	// leave the control plane half-updated behind it.
	//
	// InfraService is resolved separately because it is NOT published over
	// KTSP: replacing these binaries only makes sense in a process where
	// nothing being replaced is running, which is exactly here — pre-launch.
	infraSvc, ierr := application.UseService[mktapi.InfraService](cmd.Context())
	if ierr != nil {
		fmt.Fprintf(w, "update: control plane unavailable (%v)\n", ierr)
	} else if updateControlPlane(cmd.Context(), w, svc, infraSvc) {
		changed = true
	}

	if done, err := svc.Update(cmd.Context(), mktapi.UpdateInput{}); err != nil {
		fmt.Fprintf(w, "update: sidecars not updated (%v)\n", err)
	} else {
		for _, u := range done {
			fmt.Fprintf(w, "updated %s %s → %s\n", u.Name, u.Installed, u.Latest)
			changed = true
		}
	}

	if !changed {
		fmt.Fprintln(w, "already up to date")
	}
}

// selfUpdater is the control-plane move updateControlPlane makes
// (mktapi.InfraService answers it).
type selfUpdater interface {
	SelfUpdate(ctx context.Context) ([]mktapi.Update, error)
}

// updateControlPlane brings the control plane to the infra channel's set, as
// `katyoti update --all` does: a control plane make dev placed - a dev build of
// a checkout - is left as it is, saying so (only `katyoti update --infra`
// replaces it). Answers whether it said something moved or
// was left.
func updateControlPlane(ctx context.Context, w io.Writer, setup mktcommands.InfraVersionsReader, infra selfUpdater) bool {
	if commit, dev := mktcommands.DevControlPlane(ctx, setup); dev {
		mktcommands.WriteDevControlPlaneLeft(w, "update: ", commit)
		return true
	}
	done, err := infra.SelfUpdate(ctx)
	if err != nil {
		fmt.Fprintf(w, "update: control plane not updated (%v)\n", err)
		return false
	}
	mktcommands.WriteInfraUpdates(w, "updated ", done, false)
	return len(done) > 0
}
