// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane/shortcut"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// DELETING AN EARLIER INSTALLATION — its folder, its config
// data and cache roots (wherever its katyoti.yml puts them) and its shortcuts
// go, only once the person has acknowledged what will be lost: the plan names
// every folder with its size and gives a token derived from them, and the
// deletion runs only with that token. The installation asking is never
// deleted, nor one whose projects folder lies inside it (the projects would go
// with it), nor one sharing a folder with another, and the last installation is
// kept unless asked. One rule for its two faces: `katyoti instance delete` and
// the GUI's list of earlier installations (through the hub).

// Usage is an installation's footprint: the bytes its roots hold and the last
// time a file in them changed.
type Usage struct {
	Bytes    int64
	LastUsed time.Time
}

// FolderUsage is one folder a deletion removes, and what it holds.
type FolderUsage struct {
	Path string
	Usage
}

// DeletePlan is what deleting an installation takes away, and the token that
// acknowledges it.
type DeletePlan struct {
	Installation KnownInstallation
	Folders      []FolderUsage
	Shortcuts    string // the installation name its shortcuts carry
	Token        string
}

// Refusal is why an installation cannot be deleted: Reason a code a GUI says in
// its language (unknown, current, last, projects-inside, shared), the error the
// sentence.
type Refusal struct {
	Reason string
	Msg    string
}

func (r *Refusal) Error() string { return r.Msg }

func refuse(reason, format string, a ...any) error {
	return &Refusal{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// RemoveShortcuts removes an installation's shortcuts (tests replace it).
var RemoveShortcuts = shortcut.Remove

// InstallationOf is the installation in k's folder: its katyoti.yml read, or
// the default layout when it has none yet.
func InstallationOf(k KnownInstallation) *cenv.Installation {
	if inst, err := cenv.LoadInstallation(filepath.Join(k.Folder, cenv.InstallationFile)); err == nil {
		return inst
	}
	return cenv.NewInstallation(k.Folder, k.Name, "")
}

// roots are the installation's distinct folders, a root inside another left
// out (it is counted, and deleted, with it).
func roots(inst *cenv.Installation) []string {
	all := []string{inst.Dir(), inst.ConfigRoot(), inst.DataRoot(), inst.CacheRoot()}
	sort.Slice(all, func(a, b int) bool { return len(all[a]) < len(all[b]) })
	var out []string
	for _, r := range all {
		r = filepath.Clean(r)
		inside := false
		for _, o := range out {
			if under(r, o) {
				inside = true
				break
			}
		}
		if !inside {
			out = append(out, r)
		}
	}
	return out
}

// exists is whether p is there: a folder that does not exist is shared with
// no one.
func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// under is whether p is dir or lies under it (case-blind, as on Windows).
func under(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

// usageOf walks a folder: its bytes and its newest file's time (missing: zero).
func usageOf(dir string) Usage {
	var u Usage
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			u.Bytes += info.Size()
			if info.ModTime().After(u.LastUsed) {
				u.LastUsed = info.ModTime()
			}
		}
		return nil
	})
	return u
}

// UsageOf is the installation's footprint over all its roots.
func UsageOf(k KnownInstallation) Usage {
	var total Usage
	for _, r := range roots(InstallationOf(k)) {
		u := usageOf(r)
		total.Bytes += u.Bytes
		if u.LastUsed.After(total.LastUsed) {
			total.LastUsed = u.LastUsed
		}
	}
	return total
}

// findKnown is the installation of that name in known.
func findKnown(known []KnownInstallation, name string) (KnownInstallation, bool) {
	for _, k := range known {
		if strings.EqualFold(k.Name, name) {
			return k, true
		}
	}
	return KnownInstallation{}, false
}

// PlanDelete says what deleting the installation named name would take away,
// or why it is refused: the installation asking (current, its folder), one
// whose projects folder lies inside it, one sharing a folder with another, the
// last one unless keepLast is false.
func PlanDelete(known []KnownInstallation, name, current string, keepLast bool) (DeletePlan, error) {
	k, ok := findKnown(known, name)
	if !ok {
		return DeletePlan{}, refuse("unknown", "no installation %q", name)
	}
	if current != "" && strings.EqualFold(filepath.Clean(current), filepath.Clean(k.Folder)) {
		return DeletePlan{}, refuse("current", "%s is the installation this runs from: delete it from another one", k.Name)
	}
	if keepLast && len(known) <= 1 {
		return DeletePlan{}, refuse("last", "%s is the last installation: it is kept unless you say to delete the last one too", k.Name)
	}
	inst := InstallationOf(k)
	if projects, chosen := inst.ProjectsFolder(); chosen && projects != "" {
		for _, r := range roots(inst) {
			if under(filepath.Clean(projects), r) {
				return DeletePlan{}, refuse("projects-inside", "%s's projects folder %s lies inside it: move the projects out first, a deletion never takes them", k.Name, projects)
			}
		}
	}
	// A root another installation also uses (an adopted root's data folder
	// shared with another, one installation nested in another's) is never
	// deleted: it would take the other's universes with it.
	for _, other := range known {
		if strings.EqualFold(other.Name, k.Name) {
			continue
		}
		for _, r := range roots(inst) {
			if !exists(r) {
				continue
			}
			for _, o := range roots(InstallationOf(other)) {
				if exists(o) && (under(o, r) || under(r, o)) {
					return DeletePlan{}, refuse("shared", "%s's folder %s is shared with the installation %s (%s): a deletion would take it too - delete by hand", k.Name, r, other.Name, o)
				}
			}
		}
	}
	p := DeletePlan{Installation: k, Shortcuts: inst.Name}
	h := sha256.New()
	fmt.Fprintln(h, k.Name)
	for _, r := range roots(inst) {
		if _, err := os.Stat(r); err != nil {
			continue
		}
		p.Folders = append(p.Folders, FolderUsage{Path: r, Usage: usageOf(r)})
		fmt.Fprintln(h, strings.ToLower(r))
	}
	p.Token = k.Name + "-" + hex.EncodeToString(h.Sum(nil))[:8]
	return p, nil
}

// Apply deletes what the plan names, once ack is its token: the registry
// entry, the shortcuts, then each folder. A folder that cannot be removed (a
// program of it still running) is said; the rest is removed.
func (p DeletePlan) Apply(ack string) error {
	if ack != p.Token {
		return fmt.Errorf("not acknowledged: deleting %s takes away what the plan lists - acknowledge it with the token %s", p.Installation.Name, p.Token)
	}
	known, err := KnownInstallations()
	if err != nil {
		return err
	}
	kept := known[:0]
	for _, k := range known {
		if !strings.EqualFold(k.Name, p.Installation.Name) {
			kept = append(kept, k)
		}
	}
	if err := SaveInstallations(kept); err != nil {
		return err
	}
	RemoveShortcuts(p.Shortcuts)
	var failed []string
	for _, f := range p.Folders {
		if err := os.RemoveAll(f.Path); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", f.Path, err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%s is forgotten, but not every folder could be removed - close what of it still runs, then delete by hand: %s", p.Installation.Name, strings.Join(failed, "; "))
	}
	return nil
}

// SaveInstallations writes the registry of installations, an entry whose
// folder is gone left out, the log saying so.
func SaveInstallations(list []KnownInstallation) error {
	kept, gone := pruneVanished(list)
	for _, k := range gone {
		slog.Info("katyoti-instances.yaml: an installation whose folder is gone is forgotten", "name", k.Name, "folder", k.Folder)
	}
	return writeInstallations(kept)
}

// writeInstallations writes the registry as given.
func writeInstallations(list []KnownInstallation) error {
	p, err := InstallationsRegistryPath()
	if err != nil {
		return err
	}
	data, err := marshal.Marshal(struct {
		Instances []KnownInstallation `yaml:"instances"`
	}{list}, marshal.YAML)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
