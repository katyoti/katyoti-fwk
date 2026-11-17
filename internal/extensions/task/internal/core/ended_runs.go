// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// AN ENDED RUN STAYS LISTED — how a run ended (its status, its reason, its
// times) is kept in `runs/ended/`, so Runs still lists it after a restart: the
// last keepEndedRuns runs, for keepEndedFor.

const (
	keepEndedRuns = 200
	keepEndedFor  = 7 * 24 * time.Hour
)

func (r *runStore) endedDir() string { return filepath.Join(r.dir, "ended") }

// endedCache is the ended runs as last read or saved: Runs (polled) reads it,
// not the folder.
type endedCache struct {
	once sync.Once
	mu   sync.Mutex
	runs map[string]taskapi.RunInfo
}

func (r *runStore) ended() *endedCache {
	r.endedRunsCache.once.Do(func() {
		r.endedRunsCache.runs = map[string]taskapi.RunInfo{}
		for _, info := range r.readEnded() {
			r.endedRunsCache.runs[info.RunID] = info
		}
	})
	return &r.endedRunsCache
}

// saveEnded keeps how a run ended, and lets the oldest go.
func (r *runStore) saveEnded(info taskapi.RunInfo) {
	if err := os.MkdirAll(r.endedDir(), 0o700); err != nil {
		slog.Warn("tasks: an ended run not kept", "run_id", info.RunID, "err", err)
		return
	}
	if err := writeJSONAtomic(filepath.Join(r.endedDir(), info.RunID+".json"), info); err != nil {
		slog.Warn("tasks: an ended run not kept", "run_id", info.RunID, "err", err)
		return
	}
	c := r.ended()
	c.mu.Lock()
	c.runs[info.RunID] = info
	c.mu.Unlock()
	r.pruneEnded()
}

func (r *runStore) pruneEnded() {
	entries, err := os.ReadDir(r.endedDir())
	if err != nil {
		return
	}
	type kept struct {
		path string
		at   time.Time
	}
	var all []kept
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if fi, err := e.Info(); err == nil {
			all = append(all, kept{filepath.Join(r.endedDir(), e.Name()), fi.ModTime()})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at.After(all[j].at) })
	c := r.ended()
	for i, k := range all {
		if i >= r.endedLimit() || time.Since(k.at) > keepEndedFor {
			id := strings.TrimSuffix(filepath.Base(k.path), ".json")
			c.mu.Lock()
			parent := c.runs[id].ParentRunID
			c.mu.Unlock()
			// A child of a run still going is kept: its parent reads its
			// result (a foreach of a thousand items).
			if _, err := os.Stat(r.manifestPath(parent)); parent != "" && err == nil {
				continue
			}
			_ = os.Remove(k.path) // pruning: one left over is pruned at the next save
			_ = os.Remove(r.resultPath(id))
			c.mu.Lock()
			delete(c.runs, id)
			c.mu.Unlock()
		}
	}
}

// savedResult is how a run ended and what it gave — its Result after its
// entry left the registry (a restart, five minutes).
type savedResult struct {
	taskapi.ResultResponse
	ParentRunID string    `json:"parentRunId,omitempty"`
	EndedAt     time.Time `json:"endedAt"`
	// Request is what the run was started with: a person may start it again
	// (Repeat).
	Request *taskapi.TaskRunRequest `json:"request,omitempty"`
}

func (r *runStore) resultPath(runID string) string {
	return filepath.Join(r.dir, "results", runID+".json")
}

// saveResult keeps a run's result; an output JSON cannot hold is kept as its
// error.
func (r *runStore) saveResult(res savedResult) {
	if err := os.MkdirAll(filepath.Dir(r.resultPath(res.RunID)), 0o700); err != nil {
		slog.Warn("tasks: a run's result not kept", "run_id", res.RunID, "err", err)
		return
	}
	if _, err := json.Marshal(res.Output); err != nil {
		res.Output = nil
		res.Error = strings.TrimPrefix(res.Error+"; its output could not be kept: "+err.Error(), "; ")
	}
	if err := writeJSONAtomic(r.resultPath(res.RunID), res); err != nil {
		slog.Warn("tasks: a run's result not kept", "run_id", res.RunID, "err", err)
	}
}

// loadResult reads a run's kept result.
func (r *runStore) loadResult(runID string) (savedResult, error) {
	var res savedResult
	b, err := atomicio.ReadAtomic(r.resultPath(runID))
	if err != nil {
		return res, err
	}
	err = json.Unmarshal(b, &res)
	return res, err
}

// endedLimit is how many ended runs the store keeps (keepEndedRuns, unless a
// test sets keepEnded).
func (r *runStore) endedLimit() int {
	if r.keepEnded > 0 {
		return r.keepEnded
	}
	return keepEndedRuns
}

// endedRuns is the runs kept ended.
func (r *runStore) endedRuns() []taskapi.RunInfo {
	c := r.ended()
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]taskapi.RunInfo, 0, len(c.runs))
	for _, info := range c.runs {
		out = append(out, info)
	}
	return out
}

// readEnded reads the ended runs kept in the folder.
func (r *runStore) readEnded() []taskapi.RunInfo {
	entries, err := os.ReadDir(r.endedDir())
	if err != nil {
		return nil
	}
	var out []taskapi.RunInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := atomicio.ReadAtomic(filepath.Join(r.endedDir(), e.Name()))
		if err != nil {
			continue
		}
		var info taskapi.RunInfo
		if json.Unmarshal(b, &info) == nil && info.RunID != "" {
			out = append(out, info)
		}
	}
	return out
}

// isEnd: a transition after which the run has ended.
func isEnd(t engine.RunTransition) bool {
	switch t {
	case engine.RunCompleted, engine.RunFailed, engine.RunCancelled, engine.RunKilled, engine.RunInterrupted:
		return true
	}
	return false
}
