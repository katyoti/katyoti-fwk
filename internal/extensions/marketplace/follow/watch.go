// SPDX-License-Identifier: Apache-2.0

package follow

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/configset"
)

// Places is what a universe's members read and the watch follows: its keys
// files (the universe's .env, the global .env) and its config folders (each
// file in them).
type Places struct {
	EnvFiles   []string
	ConfigDirs []string
}

// Snapshot is the state of the places at one moment: each key's value and each
// config file's content, as digests - a value is never kept.
type Snapshot struct {
	keys  map[string]string // key → digest of its value, over every keys file
	files map[string]string // config file → digest of its content
}

// Take reads the places now. A keys file or a folder that does not exist
// holds nothing.
func Take(p Places) Snapshot {
	s := Snapshot{keys: map[string]string{}, files: map[string]string{}}
	for _, f := range p.EnvFiles {
		vals, err := configset.ReadEnv(f)
		if err != nil {
			continue
		}
		for k, v := range vals {
			// The same key in two files is one key read: a change of
			// either changes what a member may read.
			s.keys[k] += digest([]byte(v))
		}
	}
	for _, d := range p.ConfigDirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			path := filepath.Join(d, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			s.files[path] = digest(data)
		}
	}
	return s
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:8])
}

// Changes is what changed between two snapshots: the keys whose value was
// set, changed or removed, and the config files changed or removed. A config
// file that appears is no change: a member writes its own first-run file
// when it starts, and a file that was not there was read by no one.
func Changes(before, after Snapshot) (keys, files map[string]bool) {
	keys, files = map[string]bool{}, map[string]bool{}
	for k, v := range after.keys {
		if before.keys[k] != v {
			keys[k] = true
		}
	}
	for k := range before.keys {
		if _, ok := after.keys[k]; !ok {
			keys[k] = true
		}
	}
	for f, d := range before.files {
		if a, ok := after.files[f]; !ok || a != d {
			files[f] = true
		}
	}
	return keys, files
}

// Watch follows the places until ctx ends: each change (debounced) restarts
// the running members that read it (RestartReaders), each restart said on
// log. It answers once the watch is set (ready, closed) and returns when ctx
// ends or the watch cannot be set.
func Watch(ctx context.Context, p Places, hub Hub, reads ReadsFunc, skip func(string) bool, log io.Writer, ready chan<- struct{}) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("follow: %w", err)
	}
	defer w.Close()
	watched := map[string]bool{}
	for _, d := range append(dirsOf(p.EnvFiles), p.ConfigDirs...) {
		d = filepath.Clean(d)
		if watched[d] {
			continue
		}
		if err := w.Add(d); err != nil {
			continue // a folder that does not exist holds nothing to follow
		}
		watched[d] = true
	}
	ours := func(path string) bool {
		path = samePath(path)
		for _, f := range p.EnvFiles {
			if samePath(f) == path {
				return true
			}
		}
		for _, d := range p.ConfigDirs {
			if samePath(filepath.Dir(path)) == samePath(d) {
				return true
			}
		}
		return false
	}

	var (
		mu      sync.Mutex // the debounce timer
		applyMu sync.Mutex // one apply at a time, the event loop never waits on it
		last    = Take(p)
		timer   *time.Timer
	)
	apply := func() {
		applyMu.Lock()
		defer applyMu.Unlock()
		now := Take(p)
		keys, files := Changes(last, now)
		last = now
		restarted, err := RestartReaders(ctx, hub, keys, files, reads, skip)
		if err != nil {
			fmt.Fprintf(log, "follow: the members running could not be read: %v\n", err)
			return
		}
		for _, r := range restarted {
			what := strings.Join(append(append([]string{}, r.Keys...), r.Files...), ", ")
			if r.Error != "" {
				fmt.Fprintf(log, "follow: %s reads %s, which changed - its restart failed: %s\n", r.Member, what, r.Error)
				continue
			}
			fmt.Fprintf(log, "follow: %s restarted - it reads %s, which changed\n", r.Member, what)
		}
	}
	const debounce = 300 * time.Millisecond
	if ready != nil {
		close(ready)
	}
	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			if timer != nil {
				timer.Stop()
			}
			mu.Unlock()
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if !ours(ev.Name) {
				continue
			}
			mu.Lock()
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounce, apply)
			mu.Unlock()
		case _, ok := <-w.Errors:
			if !ok {
				return nil
			}
		}
	}
}

func dirsOf(files []string) []string {
	var out []string
	for _, f := range files {
		out = append(out, filepath.Dir(f))
	}
	sort.Strings(out)
	return out
}
