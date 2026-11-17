// SPDX-License-Identifier: Apache-2.0

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/oslock"
	"github.com/katyoti/katyoti-fwk/internal/core/server"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// runStore keeps, in the DataDir's `runs/` folder, what a restart needs to
// find the runs it interrupted and to resume the ones that were waiting:
//   - `<run>.run.json` — the run's manifest: the request that started it, its
//     session, group and start time. Written when the run is launched, removed
//     when it ends.
//   - `<run>.wait.json` — its checkpoint, kept only while the run waits (a
//     running run is not resumed after a restart: it is interrupted). The
//     executor saves after every step; a step that does not wait removes it.
//   - `ended/`, how runs ended (ended_runs.go); `keep-alive.json`, the
//     kept-alive tasks' state (keepalive.go).
type runStore struct {
	// endedRunsCache: the ended runs kept (ended_runs.go).
	endedRunsCache endedCache
	// onBad is told of a manifest that cannot be read (set aside).
	onBad func(name string, err error)
	// keepEnded, when set (tests), is how many ended runs are kept.
	keepEnded int
	dir       string
	mu        sync.Mutex
	waiting   map[string]bool // runs with a checkpoint file
	// everyStep: runs whose machine restarts after a crash keep the checkpoint
	// of every step, not only of their waits — a restart resumes them from
	// their last step (engine.Restartable).
	everyStep map[string]bool
	// closed: runs that ended (or were let go by a kill) — a late write of
	// their goroutine, still running, is refused: it would bring them back.
	closed map[string]bool

	// owner names this process among those that keep runs here (its PID and
	// a nonce: a PID alone is reused by the OS). Its file, `owners/<owner>.lock`,
	// is locked by the operating system for as long as the process lives —
	// asleep or not: a manifest whose owner's file is not locked was left by a
	// process that is gone (oslock; no PID or heartbeat to judge).
	owner string
	lock  *oslock.Lock
}

// runManifest is what rebuilds a run: the same task, the same request.
type runManifest struct {
	RunID     string                 `json:"runId"`
	Request   taskapi.TaskRunRequest `json:"request"`
	Group     string                 `json:"group,omitempty"`
	StartedAt time.Time              `json:"startedAt"`
	// Owner is the process that runs it (runStore.owner): while its lease is
	// live, the run is that process's, not one to restore. PID is that
	// process's id, read only for a manifest written before owners.
	Owner string `json:"owner,omitempty"`
	PID   int    `json:"pid"`
	// TaskHash identifies the task's definition the run started on (its type,
	// config and body): a run is resumed only on the same definition.
	TaskHash string `json:"taskHash,omitempty"`
	// Restarts counts the restarts after a crash without progress, and
	// RestartVersion is the step version the run was at then: a run that
	// crashes again at the same step is failed past maxRestartsWithoutProgress.
	Restarts       int `json:"restarts,omitempty"`
	RestartVersion int `json:"restartVersion,omitempty"`
	// Ended is how the run ended (its final status), written the moment it
	// ends — before its checkpoint goes: a restart never runs an ended run
	// again (its manifest may outlive its end, while its background children
	// finish).
	Ended taskapi.RunStatus `json:"ended,omitempty"`
	// Stopped is why a person stopped the run, written when they stop it: a
	// restart reports it cancelled, never resumes it.
	Stopped string `json:"stopped,omitempty"`
	// Spawn is where its parent started it, for a child started at a call site
	// (child_runs.go).
	Spawn *spawnSite `json:"spawn,omitempty"`
	// Queued: the run has not had its slot yet — written with the manifest,
	// cleared when it gets one. A queued run never ran: taken up from its
	// beginning whatever its kind (a manifest without it began).
	Queued bool `json:"queued,omitempty"`
}

// taskHash identifies a task's definition: what its machine is built from.
func taskHash(t *taskapi.Task) string {
	b, err := json.Marshal(struct {
		Type   string         `json:"type"`
		Config map[string]any `json:"config"`
		Body   string         `json:"body"`
	}{t.Type, t.Config, t.Body})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// savedCheckpoint is a checkpoint and when it was saved: a wait's timeout is
// counted from then, not from the restart.
type savedCheckpoint struct {
	SavedAt    time.Time         `json:"savedAt"`
	Checkpoint engine.Checkpoint `json:"checkpoint"`
}

// storeSeq tells apart the run stores of one process made in the same tick
// (the clock is coarse on Windows): each owner names one store.
var storeSeq atomic.Int64

func newRunStore(dir string) (*runStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	r := &runStore{dir: dir, waiting: map[string]bool{}, everyStep: map[string]bool{}, closed: map[string]bool{}, owner: fmt.Sprintf("%d-%x-%d", os.Getpid(), time.Now().UnixNano(), storeSeq.Add(1))}
	lock, err := oslock.TryLock(r.ownerPath(r.owner))
	if err != nil {
		return nil, fmt.Errorf("runs owner lock: %w", err)
	}
	_ = lock.Note(fmt.Sprintf("pid %d", os.Getpid())) // for a person reading it
	r.lock = lock
	return r, nil
}

// close gives this process's lock up: its runs are then free to restore.
func (r *runStore) close() { _ = r.lock.Unlock() }

func (r *runStore) ownerPath(owner string) string {
	return filepath.Join(r.dir, "owners", owner+".lock")
}

// ownedByALiveProcess: the manifest's run belongs to a process that still
// lives — this one, or another whose lease is live.
func (r *runStore) ownedByALiveProcess(m runManifest) bool {
	switch {
	case m.Owner == r.owner:
		return true
	case m.Owner != "":
		return r.ownerLive(m.Owner)
	default: // a manifest written before owners: its PID only
		return m.PID == os.Getpid() || (m.PID > 0 && server.IsProcessAlive(m.PID))
	}
}

// ownerLive reports whether the process named owner lives: its file is
// locked. A file that cannot be checked is taken for live — never a gone owner
// on an error alone.
func (r *runStore) ownerLive(owner string) bool {
	held, err := oslock.Held(r.ownerPath(owner))
	if err != nil {
		slog.Warn("tasks: an owner's lock cannot be checked", "owner", owner, "err", err)
		return true
	}
	return held
}

// ownerFileGrace: an owner's file younger than this is never removed — its
// process may have created it and not locked it yet.
const ownerFileGrace = time.Minute

// dropDeadOwners removes the files of processes that are gone: what they left
// has been restored or reported. (A `.lease` file is a process of a former
// build's: removed once it has not been heartbeated for long.)
func (r *runStore) dropDeadOwners() {
	dir := filepath.Join(r.dir, "owners")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < ownerFileGrace {
			continue
		}
		switch name := e.Name(); {
		case strings.HasSuffix(name, ".lease"):
			if time.Since(info.ModTime()) > 10*time.Minute {
				_ = os.Remove(filepath.Join(dir, name))
			}
		case strings.HasSuffix(name, ".lock"):
			if owner := strings.TrimSuffix(name, ".lock"); owner != r.owner && !r.ownerLive(owner) {
				_ = os.Remove(filepath.Join(dir, name))
			}
		}
	}
}

func (r *runStore) manifestPath(runID string) string {
	return filepath.Join(r.dir, runID+".run.json")
}

func (r *runStore) checkpointPath(runID string) string {
	return filepath.Join(r.dir, runID+".wait.json")
}

func (r *runStore) saveManifest(m runManifest) error {
	r.mu.Lock()
	closed := r.closed[m.RunID]
	r.mu.Unlock()
	if closed {
		return nil
	}
	return writeSealedJSON(r.manifestPath(m.RunID), m)
}

// markManifest rewrites a run's manifest with change applied — "" when it
// has none (a run that was never saved, or is forgotten).
func (r *runStore) markManifest(runID string, change func(*runManifest)) error {
	m, err := r.readManifest(runID)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	change(&m)
	return writeSealedJSON(r.manifestPath(runID), m)
}

// markEnded records that a run ended (status) — in its manifest first, then
// its checkpoint goes — and refuses its later writes: from now on a restart
// skips it, however long its manifest stays (its background children).
func (r *runStore) markEnded(runID string, status taskapi.RunStatus) error {
	r.mu.Lock()
	r.closed[runID] = true
	delete(r.waiting, runID)
	r.mu.Unlock()
	// Marked again when its final status is known (the executor marks it
	// completed when it outputs; launch then says how it really ended).
	err := r.markManifest(runID, func(m *runManifest) { m.Ended = status })
	if rmErr := os.Remove(r.checkpointPath(runID)); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
		err = errors.Join(err, rmErr)
	}
	return err
}

// markStopped records that a person stopped a run (why): a restart reports
// it cancelled.
// markBegan writes that the run got its slot and starts.
func (r *runStore) markBegan(runID string) error {
	return r.markManifest(runID, func(m *runManifest) { m.Queued = false })
}

func (r *runStore) markStopped(runID, why string) error {
	return r.markManifest(runID, func(m *runManifest) { m.Stopped = why })
}

// setAside moves a manifest that cannot be read into runs/bad — it is not
// skipped again at every sweep — and reports it.
func (r *runStore) setAside(name string, err error) {
	if err == nil {
		err = errors.New("it names no run")
	}
	bad := filepath.Join(r.dir, "bad")
	if mkErr := os.MkdirAll(bad, 0o700); mkErr == nil {
		_ = os.Rename(filepath.Join(r.dir, name), filepath.Join(bad, name)) // if it fails, it is reported again next sweep
	}
	if r.onBad != nil {
		r.onBad(name, err)
	}
}

// manifests lists the runs a previous process left: every manifest on disk.
func (r *runStore) manifests() ([]runManifest, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, err
	}
	var out []runManifest
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".run.json") {
			continue
		}
		b, err := atomicio.ReadAtomic(filepath.Join(r.dir, e.Name()))
		if err != nil {
			continue
		}
		var m runManifest
		if err := readSealedJSON(b, &m); err != nil || m.RunID == "" {
			r.setAside(e.Name(), err)
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// keepEveryStep makes the store keep the checkpoint of every step of a run —
// a run whose machine restarts after a crash.
func (r *runStore) keepEveryStep(runID string) {
	r.mu.Lock()
	r.everyStep[runID] = true
	r.mu.Unlock()
}

// forget removes everything kept for a run: it has ended.
func (r *runStore) forget(runID string) {
	r.mu.Lock()
	delete(r.waiting, runID)
	delete(r.everyStep, runID)
	delete(r.closed, runID)
	r.mu.Unlock()
	_ = os.Remove(r.manifestPath(runID))
	_ = os.Remove(r.checkpointPath(runID))
	r.dropClaims(runID)
}

// ── engine.StateStore ────────────────────────────────────────────────────

// Save keeps the checkpoint of a waiting run; a step that does not wait
// removes a previous one, so a restart never resumes a run mid-work.
func (r *runStore) Save(runID string, cp engine.Checkpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed[runID] {
		return nil // ended or let go: its late steps are not kept
	}
	if cp.Wait == nil && !r.everyStep[runID] {
		if r.waiting[runID] {
			delete(r.waiting, runID)
			if err := os.Remove(r.checkpointPath(runID)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	}
	if err := writeSealedJSON(r.checkpointPath(runID), savedCheckpoint{SavedAt: time.Now(), Checkpoint: cp}); err != nil {
		// No older checkpoint may stand in for this one: a restart would
		// resume the run at a wait it already passed. Without one, the run is
		// reported interrupted.
		delete(r.waiting, runID)
		if rmErr := os.Remove(r.checkpointPath(runID)); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return errors.Join(err, rmErr)
		}
		return err
	}
	r.waiting[runID] = true
	return nil
}

// Load reads a run's checkpoint. Its wait's timeout is what remained when it
// was saved, less the time since — at least a nanosecond, so an expired wait
// times out at once. The run is then known to have a checkpoint on disk: the
// first step it takes that does not wait removes it (a restart never resumes
// a restored run at the wait it already passed).
func (r *runStore) Load(runID string) (engine.Checkpoint, error) {
	b, err := atomicio.ReadAtomic(r.checkpointPath(runID))
	if err != nil {
		return engine.Checkpoint{}, fmt.Errorf("checkpoint for run %s: %w", runID, err)
	}
	r.mu.Lock()
	r.waiting[runID] = true
	r.mu.Unlock()
	var sc savedCheckpoint
	if err := readSealedJSON(b, &sc); err != nil {
		return engine.Checkpoint{}, fmt.Errorf("checkpoint for run %s: %w", runID, err)
	}
	if w := sc.Checkpoint.Wait; w != nil && w.Timeout > 0 {
		left := w.Timeout - time.Since(sc.SavedAt)
		if left <= 0 {
			left = time.Nanosecond
		}
		w.Timeout = left
	}
	return sc.Checkpoint, nil
}

// Delete is the executor's: the run reached its end (an output) — it is
// marked completed before its checkpoint goes.
func (r *runStore) Delete(runID string) error {
	return r.markEnded(runID, taskapi.RunStatusCompleted)
}

func writeJSONAtomic(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	// atomicio retries the replace (and the readers retry their reads) on
	// Windows, where a rename over a file another process is reading fails —
	// two processes restoring at once read the manifests one rewrites.
	return atomicio.WriteAtomic(path, b, 0o600)
}

// A RUN FILE THAT IS NOT WHAT WAS WRITTEN IS NEVER RESUMED. A manifest and a
// checkpoint are sealed: saved as {"sum": sha256 of data, "data": the value},
// and read only when data still hashes to sum. A file damaged into JSON that
// still parses (a bad sector, a torn page the disk did not report) fails its
// sum and is answered errRunFileCorrupted: the store sets it aside and the run
// is reported, never resumed from wrong state. A file written before the seal
// (no sum) is read as it is.
type sealedJSON struct {
	Sum  string          `json:"sum"`
	Data json.RawMessage `json:"data"`
}

var errRunFileCorrupted = errors.New("its content does not match its checksum")

func writeSealedJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	return writeJSONAtomic(path, sealedJSON{Sum: hex.EncodeToString(sum[:]), Data: data})
}

func readSealedJSON(b []byte, v any) error {
	var sealed sealedJSON
	if err := json.Unmarshal(b, &sealed); err != nil {
		return err
	}
	if sealed.Sum == "" && sealed.Data == nil {
		return json.Unmarshal(b, v) // written before the seal
	}
	sum := sha256.Sum256(sealed.Data)
	if hex.EncodeToString(sum[:]) != sealed.Sum {
		return errRunFileCorrupted
	}
	return json.Unmarshal(sealed.Data, v)
}

func (r *runStore) claimPath(runID, owner string) string {
	return filepath.Join(r.dir, runID+".claim-"+owner)
}

// ownerOf is the identity a manifest's run belongs to: its owner, else (a
// manifest written before owners) its process id.
func ownerOf(m runManifest) string {
	if m.Owner != "" {
		return m.Owner
	}
	return fmt.Sprintf("pid-%d", m.PID)
}

// claim takes, for this process, the restore of m's run — a run whose owner is
// gone — so that one process restores it, however many sweep at once. The
// claim is a file named after the owner it replaces, created exclusively: of
// the processes that read m, one creates it. The winner then reads the
// manifest again and goes on only if it still names that owner (a process
// that read m before another restored it finds the run owned anew). A claim
// left by a restorer that is gone is claimed over the same way, named after
// that restorer. release removes the run's claims once the run is resumed
// (its manifest names this process) or reported interrupted (forgotten).
func (r *runStore) claim(m runManifest) (release func(), ok bool) {
	release = func() { r.dropClaims(m.RunID) }
	lose := func() (func(), bool) { return func() {}, false }
	target := ownerOf(m)
	for hop := 0; hop < 8; hop++ {
		path := r.claimPath(m.RunID, target)
		err := r.createClaim(path)
		if err == nil {
			cur, rerr := r.readManifest(m.RunID)
			if rerr != nil || ownerOf(cur) != ownerOf(m) {
				_ = os.Remove(path)
				return lose()
			}
			return release, true
		}
		if !errors.Is(err, os.ErrExist) {
			return lose()
		}
		b, err := atomicio.ReadAtomic(path)
		if err != nil {
			return lose() // being written by its claimer: it is theirs
		}
		claimer := string(b)
		if claimer == r.owner || r.ownerLive(claimer) {
			return lose() // a live process is restoring it
		}
		target = claimer // its restorer is gone: claim over it
	}
	return lose()
}

// createClaim creates the claim file at path, naming this process, if none
// exists — whole at once: the claim is written to a file of its own, then
// linked to path (a link, like an exclusive create, fails when path exists).
// A claim created empty, then written, could be read empty in between and
// taken for a gone claimer's.
func (r *runStore) createClaim(path string) error {
	tmp := path + "." + r.owner + ".tmp"
	if err := os.WriteFile(tmp, []byte(r.owner), 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	return os.Link(tmp, path)
}

// dropClaims removes every claim on a run.
func (r *runStore) dropClaims(runID string) {
	paths, _ := filepath.Glob(filepath.Join(r.dir, runID+".claim-*"))
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

// readManifest reads one run's manifest from disk.
func (r *runStore) readManifest(runID string) (runManifest, error) {
	var m runManifest
	b, err := atomicio.ReadAtomic(r.manifestPath(runID))
	if err != nil {
		return m, err
	}
	if err := readSealedJSON(b, &m); err != nil {
		return m, err
	}
	return m, nil
}
