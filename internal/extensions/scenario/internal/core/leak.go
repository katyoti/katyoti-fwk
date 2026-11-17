// SPDX-License-Identifier: Apache-2.0

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// THE LEAK CHECK — a case lives in its sandbox; the installation of the person
// running it is not touched. Around every verb the tool records the machine
// files of that installation a case could reach — its global root (the
// registries, Profile.yaml, the .env, the control plane, each universe's
// config), the hub lockfiles of its universes and their data folders (one
// made or removed in the person's data root), and its cache — by size and
// modification time (never their content), and fails the verb with the paths
// that changed. The installation's own running processes write their logs,
// sessions and stores elsewhere, so a person's open GUI does not trip it.
//
// What the installation's own processes do meanwhile is not the case's
// : a cache file rewritten with the same content (a hub
// rescanning its sidecars) is judged by its content, not its time; a hub lock
// is the case's only when the hub holding it runs from the case's own folders,
// and is otherwise a note - a person's hub starting or stopping.
//
// The person's own processes write too while a case runs - their katyoti gui
// starting its hubs: a sidecar manifest record of the cache
// (<cache>/sidecars/<name>@<key>/…) is told by its metadata.yaml's exe_path -
// a record created for a binary outside every case folder is the
// installation's, a note; one under a case folder, or a record with no
// readable exe_path, stays a leak. A record changed or removed for a binary
// outside the case folders is a note only when a hub of the installation not
// running from the case's folders holds a lock before or after the verb
// ; with none it is a leak - nothing of the person's was
// running to rewrite it. The global root's config/Profile.yaml (written by
// the installation's admin hub) changed follows the same hub condition. That
// rule is for Profile.yaml alone - the registries, the .env and a universe's
// config stay leaks whoever runs.

// leakMark is what is recorded of a file: its size and time, the hash of a
// small cache file (rewritten with the same content, it has not changed), the
// data folder of a hub lock, the binary a sidecar cache record was written
// for (its metadata's exe_path), whether it is the global Profile.yaml.
type leakMark struct {
	Size    int64
	Mod     time.Time
	Sum     string
	LockDir string
	Writer  string
	Profile bool
}

// leakOwner tells what is the case's: a hub lock's holder, a binary path.
type leakOwner struct {
	lock func(lockDir string) bool
	exe  func(path string) bool
}

// recordWriter is the exe_path of the sidecar cache record holding p
// (<…>/sidecars/<name>@<key>/<file>); "" when p is not in such a record or its
// metadata.yaml cannot be read.
func recordWriter(p string) string {
	dir := filepath.Dir(p)
	if !strings.Contains(filepath.Base(dir), "@") || filepath.Base(filepath.Dir(dir)) != "sidecars" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "metadata.yaml"))
	if err != nil {
		return ""
	}
	var meta struct {
		ExePath string `yaml:"exe_path"`
	}
	if config.DecodeBytes(data, marshal.YAML, &meta) != nil {
		return ""
	}
	return strings.TrimSpace(meta.ExePath)
}

// cacheHashMax is the largest cache file judged by its content.
const cacheHashMax = 1 << 20

// leakSnapshot is the watched files of an installation, by path.
type leakSnapshot map[string]leakMark

// takeLeakSnapshot records the watched files of the installation host. prev,
// when given, is the snapshot taken before: a cache file with the same size
// and time keeps its recorded hash instead of being read again - the same
// size and time is what says "unchanged" for every other watched file.
func takeLeakSnapshot(host application.MachineDirs, prev leakSnapshot) leakSnapshot {
	s := leakSnapshot{}
	var toHash []string
	add := func(p string, info fs.FileInfo) {
		s[p] = leakMark{Size: info.Size(), Mod: info.ModTime()}
	}
	addLock := func(dataDir string) {
		p := hubclient.LockFilePath(dataDir)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			s[p] = leakMark{Size: info.Size(), Mod: info.ModTime(), LockDir: dataDir}
		}
	}
	// A sidecar record's metadata is read once per record, not per file.
	writers := map[string]string{}
	writer := func(p string) string {
		dir := filepath.Dir(p)
		w, ok := writers[dir]
		if !ok {
			w = recordWriter(p)
			writers[dir] = w
		}
		return w
	}
	addCache := func(p string, info fs.FileInfo) {
		m := leakMark{Size: info.Size(), Mod: info.ModTime(), Writer: writer(p)}
		if info.Size() <= cacheHashMax {
			if b, ok := prev[p]; ok && b.Sum != "" && b.Size == m.Size && b.Mod.Equal(m.Mod) {
				m.Sum = b.Sum
			} else {
				toHash = append(toHash, p)
			}
		}
		s[p] = m
	}
	// A file's size and time come from its folder's listing (on Windows,
	// without opening the file - each open is scanned by an antivirus).
	walk := func(dir string, keep func(rel string) bool, record func(string, fs.FileInfo)) {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if rel, _ := filepath.Rel(dir, p); !d.IsDir() && keep(filepath.ToSlash(rel)) {
				if info, err := d.Info(); err == nil && !info.IsDir() {
					record(p, info)
				}
			}
			return nil
		})
	}
	if host.Root != "" {
		walk(host.Root, func(rel string) bool {
			parts := strings.Split(rel, "/")
			switch {
			case len(parts) == 1: // universes.yaml, sidecars.yaml, .env, …
				return true
			case parts[0] == "config" || parts[0] == "control-plane":
				return true
			case parts[0] == "universes": // each universe's config, not its working files
				return slices.Contains(parts, "config")
			}
			return false
		}, add)
		profile := filepath.Join(host.Root, "config", "Profile.yaml")
		if m, ok := s[profile]; ok {
			m.Profile = true
			s[profile] = m
		}
	}
	if host.Data != "" {
		entries, _ := os.ReadDir(filepath.Join(host.Data, "universes"))
		for _, e := range entries {
			addLock(filepath.Join(host.Data, "universes", e.Name()))
			// Each universe's data folder, by its presence only (its
			// content is its running hub's): one made or removed is a
			// change of the person's data root.
			if e.IsDir() {
				s[filepath.Join(host.Data, "universes", e.Name())] = leakMark{Size: -1}
			}
		}
		addLock(host.Data)
	}
	if host.Cache != "" {
		walk(host.Cache, func(string) bool { return true }, addCache)
		for p, sum := range hashFiles(toHash) {
			m := s[p]
			m.Sum = sum
			s[p] = m
		}
	}
	return s
}

// hashFiles is the content hash of each readable file of paths, read in
// parallel: the cache holds a thousand small files.
func hashFiles(paths []string) map[string]string {
	sums := make([]string, len(paths))
	work := make(chan int)
	var wg sync.WaitGroup
	for range min(runtime.NumCPU(), 8, len(paths)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				if data, err := os.ReadFile(paths[i]); err == nil {
					sum := sha256.Sum256(data)
					sums[i] = hex.EncodeToString(sum[:])
				}
			}
		}()
	}
	for i := range paths {
		work <- i
	}
	close(work)
	wg.Wait()
	out := make(map[string]string, len(paths))
	for i, p := range paths {
		if sums[i] != "" {
			out[p] = sums[i]
		}
	}
	return out
}

// leaks are the paths whose record differs between two snapshots - changed,
// created or removed - and the notes: what changed that the case did not do.
// own says whether a hub lock is held by the case's own hub and whether a
// binary is the case's.
func (before leakSnapshot) leaks(after leakSnapshot, own leakOwner) (leaks, notes []string) {
	// Each lock's holder is told once: telling it looks up its process.
	owned := map[string]bool{}
	ownsLock := own.lock
	own.lock = func(dataDir string) bool {
		v, ok := owned[dataDir]
		if !ok {
			v = ownsLock(dataDir)
			owned[dataDir] = v
		}
		return v
	}
	// A hub of the installation, not the case's, holds a lock before or
	// after the verb - asked only when a change needs it:
	// telling a lock's holder looks up its process.
	personHubKnown, personHubIs := false, false
	personHub := func() bool {
		if !personHubKnown {
			personHubKnown = true
			for _, snap := range []leakSnapshot{before, after} {
				for _, m := range snap {
					if m.LockDir != "" && !own.lock(m.LockDir) {
						personHubIs = true
						return true
					}
				}
			}
		}
		return personHubIs
	}
	judge := func(what, p string, m leakMark) {
		switch {
		case m.LockDir != "" && !own.lock(m.LockDir):
			notes = append(notes, what+"  "+p+" (a hub of the installation, not the case's)")
		case m.Writer != "" && !own.exe(m.Writer) && (what == "created" || personHub()):
			notes = append(notes, what+"  "+p+" (a sidecar record of the installation's hubs, not the case's)")
		case m.Profile && what != "removed" && personHub():
			notes = append(notes, what+"  "+p+" (the installation's hubs are up, not the case's)")
		default:
			leaks = append(leaks, what+"  "+p)
		}
	}
	for p, a := range after {
		b, ok := before[p]
		switch {
		case !ok && a.Size == -1:
			// A data folder made: a person's hub starting a universe for
			// the first time holds its lock there - not the case's (its lock
			// is said as a note).
			if l, held := after[hubclient.LockFilePath(p)]; held && l.LockDir != "" && !own.lock(l.LockDir) {
				continue
			}
			leaks = append(leaks, "created  "+p)
		case !ok:
			judge("created", p, a)
		case a.Sum != "" && b.Sum != "":
			if a.Sum != b.Sum {
				judge("changed", p, a)
			}
		case a != b:
			judge("changed", p, a)
		}
	}
	for p, b := range before {
		if _, ok := after[p]; !ok {
			if b.LockDir != "" {
				notes = append(notes, "removed  "+p+" (a hub of the installation stopped)")
				continue
			}
			judge("removed", p, b)
		}
	}
	sort.Strings(leaks)
	sort.Strings(notes)
	return leaks, notes
}

// LeakGuard watches the installation a command was run from while a verb runs.
type LeakGuard struct {
	host   application.MachineDirs
	own    []string
	before leakSnapshot
	// Notes are what changed in the installation that the case did not do,
	// said after Check.
	Notes []string
}

// WatchLeaks starts watching host; own are the case's folders (its binaries,
// its sandbox's control plane): a hub running from them is the case's.
func WatchLeaks(host application.MachineDirs, own ...string) *LeakGuard {
	return &LeakGuard{host: host, own: own, before: takeLeakSnapshot(host, nil)}
}

// ownsLock is whether the hub holding dataDir's lock runs from the case's
// folders. A lock that cannot be read, or whose process cannot be told, is
// the case's: a guess never clears a leak.
func (g *LeakGuard) ownsLock(dataDir string) bool {
	lock, err := hubclient.ReadLockFile(dataDir)
	if err != nil || lock == nil || lock.PID <= 0 {
		return true
	}
	path := processPath(lock.PID)
	if path == "" {
		return true
	}
	return g.ownsExe(path)
}

// ownsExe is whether the binary at path lies in the case's folders; an empty
// path is the case's (a guess never clears a leak).
func (g *LeakGuard) ownsExe(path string) bool {
	if path == "" {
		return true
	}
	for _, dir := range g.own {
		if dir != "" && within(dir, path) {
			return true
		}
	}
	return false
}

// Check is an error naming every watched file of the installation the case
// changed since the guard started; nil when none did. What changed that the
// case did not do is left in Notes.
func (g *LeakGuard) Check() error {
	leaks, notes := g.before.leaks(takeLeakSnapshot(g.host, g.before), leakOwner{lock: g.ownsLock, exe: g.ownsExe})
	g.Notes = notes
	if len(leaks) == 0 {
		return nil
	}
	return fmt.Errorf("LEAK: the case wrote outside its sandbox, into the installation it was run from:\n  %s", strings.Join(leaks, "\n  "))
}
