// SPDX-License-Identifier: Apache-2.0

package diagnose

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/appcore/selfupdate"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/registry"
	inv "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// Repair repairs what Diagnose names in the installation.
// Before it changes a file it copies it into a new folder under backupRoot
// (doctor-<time>), with a manifest naming where each copy came from: a
// registry is rewritten without the rows to vanished paths or left by tests,
// a control-plane program replaced or gone is put back from its store copy
// once that copy matches the checksum it was installed with. What it cannot
// repair is left, saying what the person does. A dry run changes nothing and
// says what it would do.
func Repair(f Files, backupRoot string, dryRun bool) (mktapi.InstallationRepair, error) {
	out := mktapi.InstallationRepair{DryRun: dryRun}
	if backupRoot == "" && !dryRun {
		return out, fmt.Errorf("doctor: no backup folder - nothing is changed without a backup")
	}
	d := Diagnose(f)
	if len(d.Problems) == 0 {
		return out, nil
	}
	b := &backup{root: backupRoot, dryRun: dryRun}
	rows := map[string][]int{} // registry -> the problems that remove one of its rows
	for i, p := range d.Problems {
		out.Actions = append(out.Actions, mktapi.RepairAction{Problem: p})
		switch {
		case p.Entry == "":
			out.Actions[i].Did = "left: the file cannot be read - correct it or remove it by hand, then run katyoti status"
		case p.File == f.Instances || p.File == f.Universes || p.File == f.Sidecars || p.File == f.Marketplaces:
			rows[p.File] = append(rows[p.File], i)
		case f.ControlPlane != "" && cenv.Within(p.File, f.ControlPlane):
			out.Actions[i].Done, out.Actions[i].Did = f.restoreProgram(b, p)
		case p.Kind == mktapi.ProblemProgramReplaced || p.Kind == mktapi.ProblemProgramMissing:
			out.Actions[i].Did = "left: the store's copy is not the one installed and nothing verified can replace it here - install it again: katyoti install " + p.Entry
		default:
			out.Actions[i].Did = "left: no repair is known for it"
		}
	}
	for _, file := range []string{f.Instances, f.Universes, f.Sidecars, f.Marketplaces} {
		idx := rows[file]
		if len(idx) == 0 {
			continue
		}
		var ps []mktapi.InstallationProblem
		for _, i := range idx {
			ps = append(ps, out.Actions[i].Problem)
		}
		removed, err := f.dropRows(b, file, ps)
		for _, i := range idx {
			p := out.Actions[i].Problem
			switch {
			case err != nil:
				out.Actions[i].Did = fmt.Sprintf("left: %s could not be rewritten: %v", filepath.Base(file), err)
			case removed[rowKey(p.Entry, p.Path)] == "":
				out.Actions[i].Did = "left: the row is not there any more"
			case removed[rowKey(p.Entry, p.Path)] != "removed":
				out.Actions[i].Did = "left: " + removed[rowKey(p.Entry, p.Path)]
			case dryRun:
				out.Actions[i].Done = true
				out.Actions[i].Did = "would remove the row from " + filepath.Base(file) + b.verb()
			default:
				out.Actions[i].Done = true
				out.Actions[i].Did = "removed the row from " + filepath.Base(file) + b.verb()
			}
		}
	}
	if err := b.writeManifest(); err != nil {
		return out, err
	}
	out.Backup = b.dir
	return out, nil
}

func rowKey(name, path string) string { return name + "\x00" + filepath.Clean(path) }

// dropRows rewrites a registry without the problems' rows (matched by name
// and path), after its backup. Returns, per row key, "removed" or why a row
// was kept.
func (f Files) dropRows(b *backup, file string, ps []mktapi.InstallationProblem) (map[string]string, error) {
	want := map[string]bool{}
	for _, p := range ps {
		want[rowKey(p.Entry, p.Path)] = true
	}
	result := map[string]string{}
	switch file {
	case f.Instances:
		var r struct {
			Instances []controlplane.KnownInstallation `yaml:"instances"`
		}
		if _, err := decode(file, &r); err != nil {
			return nil, err
		}
		var kept []controlplane.KnownInstallation
		for _, k := range r.Instances {
			if want[rowKey(k.Name, k.Folder)] {
				result[rowKey(k.Name, k.Folder)] = "removed"
				continue
			}
			kept = append(kept, k)
		}
		r.Instances = kept
		return result, b.rewrite(file, func() error { return writeYAML(file, r) })
	case f.Universes:
		reg, err := inv.LoadRegistry(file)
		if err != nil {
			return nil, err
		}
		var drop []string
		for _, u := range reg.List() {
			k := rowKey(u.Name, u.HomePath)
			if !want[k] {
				continue
			}
			if u.Protected {
				result[k] = "the universe is protected (the admin universe): it is recreated where it belongs when its hub starts - remove it by hand only if it is not"
				continue
			}
			result[k] = "removed"
			drop = append(drop, u.Name)
		}
		if len(drop) == 0 {
			return result, nil
		}
		return result, b.rewrite(file, func() error {
			for _, n := range drop {
				if _, err := reg.Remove(n); err != nil {
					return err
				}
			}
			return nil
		})
	case f.Sidecars:
		var r inv.SidecarsYaml
		if _, err := decode(file, &r); err != nil {
			return nil, err
		}
		var kept []inv.SidecarsYamlEntry
		for _, e := range r.Sidecars {
			path := e.BinaryPath
			if path == "" {
				path = e.FolderPath
			}
			if want[rowKey(e.Name, path)] {
				result[rowKey(e.Name, path)] = "removed"
				continue
			}
			kept = append(kept, e)
		}
		r.Sidecars = kept
		return result, b.rewrite(file, func() error { return inv.SaveSidecarsYaml(file, &r) })
	case f.Marketplaces:
		// A row is said removed only once DropUserRows removed it - matched,
		// like the other registries, by its name and its URL.
		match := func(r registry.UserRow) bool { return want[rowKey(r.Name, r.URL)] }
		if b.dryRun {
			var r struct {
				Marketplaces []struct {
					Name string `yaml:"name"`
					URL  string `yaml:"url"`
				} `yaml:"marketplaces"`
			}
			if _, err := decode(file, &r); err != nil {
				return nil, err
			}
			for _, m := range r.Marketplaces {
				if match(registry.UserRow{Name: m.Name, URL: m.URL}) {
					result[rowKey(m.Name, m.URL)] = "removed"
				}
			}
			return result, nil
		}
		err := b.rewrite(file, func() error {
			dropped, err := registry.DropUserRows(file, match)
			if err != nil {
				return err
			}
			for _, r := range dropped {
				result[rowKey(r.Name, r.URL)] = "removed"
			}
			return err
		})
		return result, err
	}
	return nil, fmt.Errorf("%s is no registry the doctor repairs", file)
}

func writeYAML(file string, v any) error {
	data, err := marshal.Marshal(v, marshal.YAML)
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o644)
}

// restoreProgram puts a control-plane program back from its store copy, once
// that copy matches the checksum it was installed with; the replaced program
// is backed up first.
func (f Files) restoreProgram(b *backup, p mktapi.InstallationProblem) (bool, string) {
	row, ok := f.infraRow(p.Entry)
	if !ok {
		return false, "left: no store copy is recorded for it - install the control plane again: katyoti install-infra"
	}
	got, err := FileSHA256(row.BinaryPath)
	if err != nil || !strings.EqualFold(got, row.Sha256) {
		return false, "left: its store copy " + row.BinaryPath + " does not match its checksum either - install the control plane again: katyoti install-infra"
	}
	if b.dryRun {
		return true, "would put back the store's copy " + row.BinaryPath + b.verb()
	}
	how := " (it was gone: nothing to back up)"
	if _, err := os.Stat(p.File); err == nil {
		if err := b.save(p.File); err != nil {
			return false, fmt.Sprintf("left: the replaced program could not be backed up: %v", err)
		}
		how = b.verb()
	}
	if err := selfupdate.Replace(p.File, row.BinaryPath); err != nil {
		return false, fmt.Sprintf("left: the store's copy could not be put back: %v", err)
	}
	return true, "put back the store's copy " + row.BinaryPath + how
}

// infraRow is the control-plane program's row of sidecars.yaml, with its
// store copy and checksum.
func (f Files) infraRow(name string) (inv.SidecarsYamlEntry, bool) {
	var r inv.SidecarsYaml
	if _, err := decode(f.Sidecars, &r); err != nil {
		return inv.SidecarsYamlEntry{}, false
	}
	for _, e := range r.Sidecars {
		if e.Name == name && e.Source == SourceInfraChannel && e.Sha256 != "" && e.BinaryPath != "" {
			return e, true
		}
	}
	return inv.SidecarsYamlEntry{}, false
}

// backup copies each file the doctor changes, once, before it changes it,
// into one folder made at the first copy, and names the copies in its
// manifest.
type backup struct {
	root   string
	dryRun bool
	dir    string
	copies []backupCopy
	saved  map[string]bool
}

type backupCopy struct {
	From string `yaml:"from"`
	Copy string `yaml:"copy"`
}

func (b *backup) verb() string {
	if b.dryRun {
		return " (dry run: nothing changed)"
	}
	return " (backed up first)"
}

// rewrite backs the file up, then runs the change; nothing in a dry run.
func (b *backup) rewrite(file string, change func() error) error {
	if b.dryRun {
		return nil
	}
	if err := b.save(file); err != nil {
		return fmt.Errorf("back up %s: %w", file, err)
	}
	return change()
}

func (b *backup) save(file string) error {
	if b.saved[file] {
		return nil
	}
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return nil
	}
	if b.dir == "" {
		dir := filepath.Join(b.root, "doctor-"+time.Now().UTC().Format("20060102-150405.000"))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		b.dir, b.saved = dir, map[string]bool{}
	}
	dst := filepath.Join(b.dir, fmt.Sprintf("%02d-%s", len(b.copies)+1, filepath.Base(file)))
	if err := copyFile(file, dst); err != nil {
		return err
	}
	b.saved[file] = true
	b.copies = append(b.copies, backupCopy{From: file, Copy: filepath.Base(dst)})
	return nil
}

func (b *backup) writeManifest() error {
	if b.dir == "" {
		return nil
	}
	return writeYAML(filepath.Join(b.dir, "manifest.yaml"), struct {
		Copies []backupCopy `yaml:"copies"`
	}{b.copies})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
