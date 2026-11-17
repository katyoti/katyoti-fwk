// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
)

// DELETING AN EARLIER INSTALLATION — its folder, its config
// data and cache roots (wherever its katyoti.yml puts them) and its shortcuts
// go, only once the person has acknowledged what will be lost: the plan names
// every folder with its size and gives a token derived from them, and the
// deletion runs only with that token. The installation the command runs from
// is never deleted, nor one whose projects folder lies inside it (the projects
// would go with it), and the last installation is kept unless asked. The rule
// is the control plane's (controlplane.PlanDelete), which the hub serves the
// GUI too.

// Usage is an installation's footprint: the bytes its roots hold and the last
// time a file in them changed.
type Usage = controlplane.Usage

// FolderUsage is one folder a deletion removes, and what it holds.
type FolderUsage = controlplane.FolderUsage

// known is the registry as the control plane reads it.
func (r Registry) known() []controlplane.KnownInstallation {
	out := make([]controlplane.KnownInstallation, 0, len(r.Instances))
	for _, i := range r.Instances {
		out = append(out, controlplane.KnownInstallation{Name: i.Name, Folder: i.Folder})
	}
	return out
}

// Usage is the instance's footprint over all its roots.
func (i Instance) Usage() Usage {
	return controlplane.UsageOf(controlplane.KnownInstallation{Name: i.Name, Folder: i.Folder})
}

// DeletePlan is what deleting an instance takes away, and the token that
// acknowledges it - the control plane's plan (controlplane.PlanDelete), with
// the instance it is about.
type DeletePlan struct {
	controlplane.DeletePlan
	Instance Instance
}

// PlanDelete says what deleting the instance named name would take away, or
// why it is refused (controlplane.PlanDelete's rule).
func PlanDelete(reg Registry, name, current string, keepLast bool) (DeletePlan, error) {
	i, err := reg.Find(name)
	if err != nil {
		return DeletePlan{}, err
	}
	p, err := controlplane.PlanDelete(reg.known(), i.Name, current, keepLast)
	if err != nil {
		return DeletePlan{}, err
	}
	return DeletePlan{DeletePlan: p, Instance: i}, nil
}

// Apply deletes what the plan names, once ack is its token.
func (p DeletePlan) Apply(_ Registry, ack string) error {
	if ack != p.Token {
		return fmt.Errorf("not acknowledged: deleting %s takes away what the plan lists - run again with --ack %s", p.Instance.Name, p.Token)
	}
	return p.DeletePlan.Apply(ack)
}

// humanBytes is a size as a person reads it.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// lastUse is a footprint's last change as a date, or « never » when none.
func lastUse(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format("2006-01-02 15:04")
}
