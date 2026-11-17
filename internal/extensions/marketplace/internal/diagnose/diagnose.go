// SPDX-License-Identifier: Apache-2.0

// Package diagnose names what is broken in an installation:
// a program gone or replaced against the checksum it was installed with, a
// registry row to a path that vanished, a row a test left in the registries.
// It reads the installation's files only - no hub, no network - and changes
// nothing: `katyoti status` shows its findings, `katyoti doctor` repairs them.
//
// Every file it reads is named by Files, so a test diagnoses an installation
// of its own (t.TempDir()), never the person's.
package diagnose

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	inv "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// SourceInfraChannel is the source sidecars.yaml records for a control-plane
// program (as the engine writes it).
const SourceInfraChannel = "infra-channel"

// Files are the installation's files a diagnosis reads.
type Files struct {
	// Instances is the registry of installations (katyoti-instances.yaml).
	Instances string
	// Universes is the universe registry (universes.yaml).
	Universes string
	// Sidecars is the available-items registry (sidecars.yaml).
	Sidecars string
	// Marketplaces is the user's subscriptions (marketplaces.yaml).
	Marketplaces string
	// ControlPlane is the control-plane folder: a control-plane program
	// recorded with its checksum is checked there too.
	ControlPlane string
	// TempDirs are where tests make their folders: a row naming a path in
	// one is a test's leftover. Empty when the installation itself lives in
	// one (a test installation's rows are rightly there).
	TempDirs []string
}

// Diagnose reads the files and returns what is broken; a file that cannot be
// read is a problem too, never a reason to stop.
func Diagnose(f Files) mktapi.InstallationDiagnosis {
	var d mktapi.InstallationDiagnosis
	for _, p := range []string{f.Instances, f.Universes, f.Sidecars, f.Marketplaces} {
		if p != "" {
			d.Checked = append(d.Checked, p)
		}
	}
	add := func(ps ...mktapi.InstallationProblem) { d.Problems = append(d.Problems, ps...) }
	add(f.instances()...)
	add(f.universes()...)
	add(f.sidecars()...)
	add(f.marketplaces()...)
	return d
}

// RowRepair is what the doctor does with a registry row a problem names.
const RowRepair = "remove the row (the registry is backed up first)"

// row classifies one registry row naming path: a test's leftover first (it
// says the most), else a vanished path; ok when neither.
func (f Files) row(file, entry, path, what string) (mktapi.InstallationProblem, bool) {
	p := mktapi.InstallationProblem{File: file, Entry: entry, Path: path, Repair: RowRepair}
	switch {
	case f.inTemp(path):
		p.Kind = mktapi.ProblemTestEntry
		p.Detail = fmt.Sprintf("%s %q lies in the temporary folder: a test left it in this registry", what, entry)
	case cenv.Vanished(path):
		p.Kind = mktapi.ProblemVanishedPath
		p.Detail = fmt.Sprintf("%s %q names a path that is gone", what, entry)
	default:
		return p, false
	}
	return p, true
}

func (f Files) inTemp(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	for _, t := range f.TempDirs {
		if t != "" && cenv.Within(path, t) {
			return true
		}
	}
	return false
}

func unreadable(file string, err error) mktapi.InstallationProblem {
	return mktapi.InstallationProblem{Kind: mktapi.ProblemVanishedPath, File: file,
		Detail: fmt.Sprintf("cannot be read: %v", err)}
}

// decode reads file into v; a missing file is an empty registry.
func decode(file string, v any) (bool, error) {
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, config.DecodeBytes(data, marshal.YAML, v)
}

func (f Files) instances() []mktapi.InstallationProblem {
	if f.Instances == "" {
		return nil
	}
	var r struct {
		Instances []controlplane.KnownInstallation `yaml:"instances"`
	}
	if _, err := decode(f.Instances, &r); err != nil {
		return []mktapi.InstallationProblem{unreadable(f.Instances, err)}
	}
	var out []mktapi.InstallationProblem
	for _, k := range r.Instances {
		if p, bad := f.row(f.Instances, k.Name, k.Folder, "installation"); bad {
			out = append(out, p)
		}
	}
	return out
}

func (f Files) universes() []mktapi.InstallationProblem {
	if f.Universes == "" {
		return nil
	}
	var r struct {
		Universes []inv.RegistryEntry `yaml:"universes"`
	}
	if _, err := decode(f.Universes, &r); err != nil {
		return []mktapi.InstallationProblem{unreadable(f.Universes, err)}
	}
	var out []mktapi.InstallationProblem
	for _, u := range r.Universes {
		if p, bad := f.row(f.Universes, u.Name, u.HomePath, "universe"); bad {
			out = append(out, p)
		}
	}
	return out
}

func (f Files) sidecars() []mktapi.InstallationProblem {
	if f.Sidecars == "" {
		return nil
	}
	var r inv.SidecarsYaml
	if _, err := decode(f.Sidecars, &r); err != nil {
		return []mktapi.InstallationProblem{unreadable(f.Sidecars, err)}
	}
	var out []mktapi.InstallationProblem
	for _, e := range r.Sidecars {
		path := e.BinaryPath
		if path == "" {
			path = e.FolderPath
		}
		if p, bad := f.row(f.Sidecars, e.Name, path, "installed item"); bad {
			if p.Kind == mktapi.ProblemVanishedPath && e.Sha256 != "" {
				p.Kind = mktapi.ProblemProgramMissing
				p.Detail = fmt.Sprintf("program %q %s, installed with its checksum, is gone", e.Name, e.Version)
				p.Repair = "remove the row (the registry is backed up first); install it again: katyoti install " + e.Name
			}
			out = append(out, p)
			continue
		}
		if e.Sha256 == "" || e.BinaryPath == "" || !filepath.IsAbs(e.BinaryPath) {
			continue
		}
		if p, bad := checkProgram(e.Name, e.Version, e.BinaryPath, e.Sha256); bad {
			p.Repair = "install it again: katyoti install " + e.Name
			out = append(out, p)
			continue
		}
		if e.Source == SourceInfraChannel && f.ControlPlane != "" {
			target := filepath.Join(f.ControlPlane, e.Name+exeSuffix())
			if p, bad := checkProgram(e.Name, e.Version, target, e.Sha256); bad {
				p.Repair = "put back the store's copy " + e.BinaryPath + " (the replaced program is backed up first)"
				out = append(out, p)
			}
		}
	}
	return out
}

// checkProgram compares the program at path with the checksum it was
// installed with.
func checkProgram(name, version, path, want string) (mktapi.InstallationProblem, bool) {
	p := mktapi.InstallationProblem{File: path, Entry: name, Path: path}
	got, err := FileSHA256(path)
	switch {
	case os.IsNotExist(err):
		p.Kind = mktapi.ProblemProgramMissing
		p.Detail = fmt.Sprintf("program %q %s is gone", name, version)
	case err != nil:
		p.Kind = mktapi.ProblemProgramMissing
		p.Detail = fmt.Sprintf("program %q %s cannot be read: %v", name, version, err)
	case !strings.EqualFold(got, want):
		p.Kind = mktapi.ProblemProgramReplaced
		p.Detail = fmt.Sprintf("program %q %s was replaced: its checksum is %s, installed as %s", name, version, short(got), short(want))
	default:
		return p, false
	}
	return p, true
}

func short(sum string) string {
	if len(sum) > 12 {
		return sum[:12]
	}
	return sum
}

// FileSHA256 is a file's sha256, hex.
func FileSHA256(path string) (string, error) {
	fh, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer fh.Close()
	h := sha256.New()
	if _, err := io.Copy(h, fh); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (f Files) marketplaces() []mktapi.InstallationProblem {
	if f.Marketplaces == "" {
		return nil
	}
	var r struct {
		Marketplaces []struct {
			Name string `yaml:"name"`
			URL  string `yaml:"url"`
		} `yaml:"marketplaces"`
	}
	if _, err := decode(f.Marketplaces, &r); err != nil {
		return []mktapi.InstallationProblem{unreadable(f.Marketplaces, err)}
	}
	var out []mktapi.InstallationProblem
	for _, m := range r.Marketplaces {
		if m.URL == "" {
			continue // an override of the seed's entry (enable/disable)
		}
		if loopback(m.URL) {
			out = append(out, mktapi.InstallationProblem{Kind: mktapi.ProblemTestEntry, File: f.Marketplaces,
				Entry: m.Name, Path: m.URL, Repair: RowRepair,
				Detail: fmt.Sprintf("subscription %q points at a test server on this machine", m.Name)})
			continue
		}
		if p, bad := f.row(f.Marketplaces, m.Name, localPath(m.URL), "subscription"); bad {
			p.Path = m.URL
			out = append(out, p)
		}
	}
	return out
}

// loopback is whether a marketplace URL is an http server on this machine
// with a port of its own: what a test's server is.
func loopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Port() == "" {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// localPath is a file marketplace URL's path, "" for a remote one.
func localPath(raw string) string {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return ""
	}
	p := strings.TrimPrefix(raw, "file://")
	if len(p) > 2 && p[0] == '/' && p[2] == ':' { // file:///C:/...
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
