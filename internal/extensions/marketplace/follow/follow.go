// SPDX-License-Identifier: Apache-2.0

// Package follow makes a running universe follow a change of what its members
// read when they start: its keys (the universe's and the global
// .env) and its config files. A member reads them once, at start; a change
// made while it runs - a key saved, a file edited by hand, a config-set
// applied - reaches it once it is started again, through the hub that owns it
// . Only the running members that read what changed restart; a
// member that does not run starts with it, one that reads none of it is left
// alone, and a restart that fails is reported, never fatal.
//
// The same reader rule serves the saves the marketplace service makes
// and the universe hub's watch of its files.
package follow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/variables"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configure"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// Hub is a running universe's members, as the hub that owns them.
type Hub interface {
	// Running is the members whose process runs.
	Running(ctx context.Context) ([]string, error)
	// Restart stops a running member and starts it again; false when it
	// was not running.
	Restart(ctx context.Context, member string) (bool, error)
}

// Reads is what a member reads when it starts: the keys (each it declares,
// each its config files read through ${{env:KEY}}) and its config files.
type Reads struct {
	Keys  []string
	Files []string
}

// ReadsFunc is what a member of the universe reads.
type ReadsFunc func(ctx context.Context, member string) (Reads, error)

// Restarted is one member restarted for what it reads: the keys and files
// that changed, or why its restart failed.
type Restarted struct {
	Member string
	Keys   []string
	Files  []string
	Error  string
}

// RestartReaders restarts the running members that read one of keys or files
// (the files' paths as the members' config checks give them). skip leaves a
// member alone (the process serving the call). A member whose reads cannot be
// known reads nothing.
func RestartReaders(ctx context.Context, hub Hub, keys, files map[string]bool, reads ReadsFunc, skip func(string) bool) ([]Restarted, error) {
	if len(keys) == 0 && len(files) == 0 {
		return nil, nil
	}
	running, err := hub.Running(ctx)
	if err != nil {
		return nil, err
	}
	changedFile := map[string]bool{}
	for f := range files {
		changedFile[samePath(f)] = true
	}
	sort.Strings(running)
	var out []Restarted
	for _, member := range running {
		if skip != nil && skip(member) {
			continue
		}
		r, err := reads(ctx, member)
		if err != nil {
			continue
		}
		hit := Restarted{Member: member}
		for _, k := range r.Keys {
			if keys[k] {
				hit.Keys = append(hit.Keys, k)
			}
		}
		for _, f := range r.Files {
			if changedFile[samePath(f)] {
				hit.Files = append(hit.Files, f)
			}
		}
		if len(hit.Keys) == 0 && len(hit.Files) == 0 {
			continue
		}
		sort.Strings(hit.Keys)
		sort.Strings(hit.Files)
		restarted, err := hub.Restart(ctx, member)
		switch {
		case err != nil:
			hit.Error = err.Error()
		case !restarted:
			continue // it stopped meanwhile: it starts with what changed
		}
		out = append(out, hit)
	}
	return out, nil
}

// samePath is a path as compared: cleaned, and case-folded on Windows.
func samePath(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

// ReadsOf is what a member reads, from its config check of a universe: each
// key it declares (its env items), each key its config files there read
// through ${{env:KEY}} - a member reads the keys its universe's copy of its
// config names, whatever it declares (ktw-agents declares ANTHROPIC_API_KEY, a
// universe's LLMProviders.yaml may read ANTHROPIC_FOUNDRY_API_KEY instead) -
// and the config files themselves. A file that cannot be read adds no key.
func ReadsOf(rep sidecarapi.ConfigCheckReport) Reads {
	var out Reads
	seen := map[string]bool{}
	for _, it := range rep.Items {
		if it.Category == "env" && it.Name != "" && !seen[it.Name] {
			seen[it.Name] = true
			out.Keys = append(out.Keys, it.Name)
		}
	}
	for _, f := range rep.ConfigFiles {
		if f.Path == "" {
			continue
		}
		out.Files = append(out.Files, f.Path)
		if !f.Exists {
			continue
		}
		data, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}
		for _, k := range variables.Keys(data, "env") {
			if !seen[k] {
				seen[k] = true
				out.Keys = append(out.Keys, k)
			}
		}
	}
	return out
}

// MemberReads is what a member of the universe at home reads, asked of the
// binary its manifest runs it with (`sc config check`, with the host's flags
// before the invocation).
func MemberReads(ctx context.Context, home, member string) (Reads, error) {
	m, err := orchcatalog.LoadManifest(home)
	if err != nil || m == nil {
		return Reads{}, fmt.Errorf("%s: no manifest: %v", home, err)
	}
	var (
		bin string
		pre []string
	)
	for _, sc := range m.Sidecars {
		if sc.Name != member {
			continue
		}
		bin = sc.Binary
		for _, a := range sc.Args {
			if a == "sc" {
				break
			}
			pre = append(pre, a)
		}
	}
	if bin == "" || orchcatalog.HostBinaries[member] {
		return Reads{}, fmt.Errorf("%q has no configuration of its own", member)
	}
	rep, err := configure.CheckWith(ctx, home, bin, pre)
	if err != nil {
		return Reads{}, err
	}
	return ReadsOf(rep), nil
}
