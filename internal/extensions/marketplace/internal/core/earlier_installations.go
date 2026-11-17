// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// THE EARLIER INSTALLATIONS: the hub lists the
// installations of this machine and deletes one once the person acknowledged
// what goes, by the control plane's rule (controlplane.PlanDelete) - the one
// `katyoti instance delete` applies. The installation answering is never
// deleted from here.

// EarlierInstallations lists the installations of this machine.
func (s *service) EarlierInstallations(_ context.Context) (mktapi.EarlierInstallationsOutput, error) {
	out := mktapi.EarlierInstallationsOutput{Installations: []mktapi.EarlierInstallation{}}
	known, err := knownInstallations()
	if err != nil {
		return out, err
	}
	current := currentFolder()
	for _, k := range known {
		inst := controlplane.InstallationOf(k)
		row := mktapi.EarlierInstallation{Name: k.Name, Title: inst.Name, Folder: k.Folder, Version: controlPlaneVersion(inst.ConfigRoot()),
			Current: current != "" && sameFolder(k.Folder, current)}
		var usage controlplane.Usage
		plan, err := controlplane.PlanDelete(known, k.Name, current, true)
		if err != nil {
			row.Refused = err.Error()
			var r *controlplane.Refusal
			if errors.As(err, &r) {
				row.Reason = r.Reason
			}
			usage = controlplane.UsageOf(k)
		} else {
			for _, f := range plan.Folders {
				usage.Bytes += f.Bytes
				if f.LastUsed.After(usage.LastUsed) {
					usage.LastUsed = f.LastUsed
				}
			}
		}
		row.Bytes, row.LastUsed = usage.Bytes, rfc3339(usage.LastUsed)
		out.Installations = append(out.Installations, row)
	}
	return out, nil
}

// DeleteInstallation answers the plan of deleting an installation, or deletes
// it given that plan's token.
func (s *service) DeleteInstallation(_ context.Context, in mktapi.DeleteInstallationInput) (mktapi.DeleteInstallationOutput, error) {
	known, err := knownInstallations()
	if err != nil {
		return mktapi.DeleteInstallationOutput{}, err
	}
	plan, err := controlplane.PlanDelete(known, in.Name, currentFolder(), !in.DeleteLast)
	if err != nil {
		return mktapi.DeleteInstallationOutput{}, err
	}
	out := mktapi.DeleteInstallationOutput{Name: plan.Installation.Name, Shortcuts: plan.Shortcuts, Token: plan.Token, Folders: []mktapi.InstallationFolder{}}
	for _, f := range plan.Folders {
		out.Folders = append(out.Folders, mktapi.InstallationFolder{Path: f.Path, Bytes: f.Bytes, LastUsed: rfc3339(f.LastUsed)})
	}
	if in.Ack == "" {
		return out, nil
	}
	if err := plan.Apply(in.Ack); err != nil {
		return out, err
	}
	out.Deleted = true
	return out, nil
}

// currentFolder is the folder of the installation answering; empty for a
// program of none (a development build), which then deletes no installation
// by mistake for its own: every one is someone else's.
func currentFolder() string {
	if inst := currentInstallation(); inst != nil {
		return inst.Dir()
	}
	return ""
}

// controlPlaneVersion is the control plane's version an installation recorded
// in its config root (sidecars.yaml), empty when none.
func controlPlaneVersion(configRoot string) string {
	sy, err := orchcatalog.LoadSidecarsYaml(filepath.Join(configRoot, "sidecars.yaml"))
	if err != nil || sy == nil {
		return ""
	}
	for _, e := range sy.Sidecars {
		if e.Name == "katyoti" && e.Version != "" {
			return e.Version
		}
	}
	return ""
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
