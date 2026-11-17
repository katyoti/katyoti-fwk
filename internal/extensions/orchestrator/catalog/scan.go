// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// HostBinaries are ktw-* binaries that match the scan pattern but are NOT
// sidecars — they are pure host applications. Filtered out so they never
// appear in universe-creation / edit-universe sidecar pickers. ktw-display is
// the bun widget server displaybridge spawns on demand: it answers no `sc`
// verb, so as a member its manifest probe hangs until the hub kills it.
//
// ktw-agents and ktw-univers are deliberately NOT here: both can run as
// sidecars (ktw-agents serves the agent-loop + Terminal slots over ktsp;
// ktw-univers serves universe-management slots in the admin universe).
var HostBinaries = map[string]bool{
	"ktw-gui":     true,
	"ktw-utils":   true,
	"ktw-display": true,
}

// ScanRoot is one directory the scan walks, plus a label that identifies
// where the result came from (used as the `Source` field on entries).
type ScanRoot struct {
	Dir    string
	Source string
}

// The two provenances a live scan can produce. Every other Source value on a
// sidecars.yaml row ("marketplace", "infra-channel", …) was written by an
// installer and is MANAGED: a scan may refresh its LastSeen but never replace it.
const (
	SourceExecutableDir = "executable-dir"
	SourcePATH          = "PATH"
)

// IsScanSource reports whether source is one a live scan writes — i.e. the row
// is a discovered dev binary, not a managed install.
func IsScanSource(source string) bool {
	return source == SourceExecutableDir || source == SourcePATH
}

// IsManagedSource reports whether a sidecars.yaml row was written by an
// installer (marketplace, infra-channel, …) rather than by a scan. An EMPTY
// source is a legacy scan row from before provenance was recorded, so it is
// NOT managed: a later scan may refresh it.
func IsManagedSource(source string) bool {
	return source != "" && !IsScanSource(source)
}

// DefaultScanRoots returns the scan roots in precedence order:
//  1. dirname(os.Executable()) — sidecars colocated with the running binary
//  2. each $PATH entry in order
//
// A program of an installation discovers the installation's programs only
// (ScanRootsIn): a checkout's bin/ or another installation's folder on PATH is
// no program it runs.
//
// Duplicates are filtered by the caller (ScanLocal) via abs-path dedupe so
// the same physical directory isn't walked twice.
func DefaultScanRoots() []ScanRoot {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		exeDir = filepath.Dir(exe)
	}
	path := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
	if inst := cenv.CurrentInstallation(); inst != nil {
		dev, _ := controlplane.DevDir()
		return ScanRootsIn(inst.Dir(), dev, exeDir, path)
	}
	var roots []ScanRoot
	if exeDir != "" {
		roots = append(roots, ScanRoot{Dir: exeDir, Source: SourceExecutableDir})
	}
	for _, dir := range path {
		if dir == "" {
			continue
		}
		roots = append(roots, ScanRoot{Dir: dir, Source: SourcePATH})
	}
	return roots
}

// ScanRootsIn are the scan roots of a program of the installation in instDir:
// its own folder when it lies in the installation, the installation's dev
// programs (devDir, what make install fills), then the PATH entries inside the
// installation. Nothing outside instDir is scanned.
func ScanRootsIn(instDir, devDir, exeDir string, path []string) []ScanRoot {
	var roots []ScanRoot
	if cenv.Within(exeDir, instDir) {
		roots = append(roots, ScanRoot{Dir: exeDir, Source: SourceExecutableDir})
	}
	if devDir != "" && cenv.Within(devDir, instDir) {
		roots = append(roots, ScanRoot{Dir: devDir, Source: SourceExecutableDir})
	}
	for _, dir := range path {
		if dir != "" && cenv.Within(dir, instDir) {
			roots = append(roots, ScanRoot{Dir: dir, Source: SourcePATH})
		}
	}
	return roots
}

// ScanLocal walks the given roots in order, looking for `ktw-*` binaries
// (`ktw-*.exe` on Windows). First-found per name wins so a sidecar
// colocated with `ktw-univers` takes precedence over one on $PATH.
// `HostBinaries` are excluded.
//
// Returns SidecarsYamlEntry values (the canonical on-disk shape) so the
// result can be merged into sidecars.yaml without translation.
func ScanLocal(roots []ScanRoot) []SidecarsYamlEntry {
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}

	seen := map[string]bool{}
	seenDirs := map[string]bool{}
	var out []SidecarsYamlEntry

	for _, root := range roots {
		abs, err := filepath.Abs(root.Dir)
		if err != nil {
			abs = root.Dir
		}
		if seenDirs[abs] {
			continue
		}
		seenDirs[abs] = true

		entries, err := os.ReadDir(root.Dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n := e.Name()
			if !strings.HasPrefix(n, "ktw-") {
				continue
			}
			if suffix != "" && !strings.HasSuffix(n, suffix) {
				continue
			}
			binaryName := strings.TrimSuffix(n, suffix)
			if HostBinaries[binaryName] {
				continue
			}
			if seen[binaryName] {
				continue
			}
			seen[binaryName] = true
			out = append(out, SidecarsYamlEntry{
				Name:       binaryName,
				BinaryPath: filepath.Join(root.Dir, n),
				Source:     root.Source,
			})
		}
	}
	return out
}
