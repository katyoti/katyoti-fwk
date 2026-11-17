// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/core/server"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/busauth"
)

// streamHubStderr forwards the spawned ktw-hub's stderr to the parent's
// logger, line by line, prefixed with `[ktw-hub]`. ktw-hub itself prefixes
// each downstream sidecar's stderr with `[<sidecar>]` (see
// `connmgr/internal/core/manager.go::streamSidecarStderr`), so a failure
// in `sc ktsp nats` boot shows up here as
// `[ktw-hub] [ktw-agents] <actual error>` in ktw-gui's log file.
func streamHubStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		log.Printf("[ktw-hub] %s", scanner.Text())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("[ktw-hub] stderr scan error: %v", err)
	}
}

// Mode controls how ConnectToHub finds (or starts) a hub.
type Mode int

const (
	// ModeReuse fails if no hub is running for the universe.
	ModeReuse Mode = iota

	// ModeSpawn always spawns a fresh hub, ignoring any existing one.
	// The spawned hub is owned: it ends once no client uses it.
	ModeSpawn

	// ModeFindOrSpawn reuses an existing hub if one is running, else
	// spawns one. Most callers (ktw-gui universe activation, ktw-agents
	// CLI) want this mode. Owner-lifetime applies only to the
	// just-spawned case — connecting to an existing hub doesn't grant
	// us its lifetime.
	ModeFindOrSpawn
)

// ConnectOptions configures ConnectToHub.
type ConnectOptions struct {
	// DataDir is the universe-scoped data root (typically
	// `application.Env().DataDir()`). The lockfile lives at
	// <DataDir>/hub/hub.lock.
	DataDir string

	// Mode selects the find / spawn behaviour. First-impl-pass: only
	// ModeReuse is wired. Other modes return ErrModeNotImplemented.
	Mode Mode

	// HubBinaryPath, when non-empty, overrides the default discovery
	// of the ktw-hub binary. Used by tests and by sidecar deployments
	// that ship a non-standard hub binary path. (Honored by Spawn
	// modes when wired in the follow-up.)
	HubBinaryPath string

	// SpawnEnv, when non-empty, replaces `os.Environ()` for the spawned
	// hub subprocess. Honored only by `connectSpawn` (ModeSpawn /
	// ModeFindOrSpawn). The host process's env is NOT inherited when
	// SpawnEnv is supplied — callers that want the host env plus
	// additions must build the combined slice themselves (typical
	// pattern: `append(os.Environ(), "KTW_WORKSPACE="+workspaceRoot)`).
	//
	// Use case: ktw-gui activates multiple universes, each with its
	// own KTW_WORKSPACE; the host ktw-gui process has none of them set.
	// SpawnEnv lets the orchestrator inject the per-workspace KTW_WORKSPACE
	// without polluting the host env globally.
	SpawnEnv []string

	// ExpectVersion is the caller's own control-plane version. A hub found
	// running (reused) at another version is reported — logged, and kept on
	// the client (VersionSkew) — rather than silently reused: it belongs to
	// another installation or version of katyoti sharing this data folder.
	// Empty: no check (a sidecar runs its own version, never the hub's).
	ExpectVersion string

	// Detach, with a spawn mode, leaves the hub this call spawns RUNNING
	// after the client closes: it is not attached to the parent's job, not
	// owned by the client (Close drops the connection only), and its stderr
	// goes nowhere (no pipe back to a parent that will be gone). A kept hub
	// ends with `katyoti universe stop`, or when its own lifetime says so.
	// A hub found running is never owned, whatever Detach says.
	Detach bool

	// ClientKind is the kind of client this program connects as - the key
	// it signs with (busauth.KindCLI or busauth.KindGUI); empty is a CLI.
	ClientKind string
}

// HubClient is a live connection to a ktw-hub. Hold the result of
// ConnectToHub for the lifetime of the operation; call Close() when
// done. The MessageBus is the only application-level surface exposed
// today — control-plane / sidecar-spawn calls layer on top of the
// MessageBus.
//
// Hub lifetime: Close() only drops the NATS connection, whether this client
// spawned the hub or found it running. A hub spawned owned (ModeSpawn, or
// ModeFindOrSpawn that resulted in a spawn, without Detach) ends by itself
// once no client is connected (owner.go), so the other
// commands using it are never cut off; a detached one runs until stopped.
type HubClient struct {
	handle   *natsxport.ClientHandle
	mu       sync.Mutex // guards lockfile, which the re-dial path rewrites
	lockfile *LockFile
	dataDir  string
	ownedHub *exec.Cmd           // non-nil while this client spawned the hub and owes its Wait
	spawned  bool                // this client spawned the hub and owes no Wait (detached, or reaped at Close)
	skew     string              // non-empty when a reused hub runs another version
	key      *busauth.Credential // the client kind's bus key; nil when the hub minted none
}

// loadKey reads the bus key of the client kind into c: none when the hub
// minted none (a hub of a version before keys, which checks none).
func (c *HubClient) loadKey(kind string) error {
	if kind == "" {
		kind = busauth.KindCLI
	}
	key, err := busauth.LoadClient(c.dataDir, kind)
	if err != nil {
		return fmt.Errorf("hubclient: %w", err)
	}
	c.key = key
	return nil
}

// VersionSkew says, when non-empty, that the hub this client reused runs
// another version than ConnectOptions.ExpectVersion.
func (c *HubClient) VersionSkew() string { return c.skew }

// HubVersionSkew compares a client's control-plane version with the version a
// running hub recorded in its lockfile; it returns the sentence to report, or
// "" when they match or either is unknown ("", "dev").
func HubVersionSkew(client, hub string) string {
	if client == "" || hub == "" || client == "dev" || hub == "dev" || client == hub {
		return ""
	}
	return fmt.Sprintf("the hub running for this universe is version %s, this client %s: "+
		"it belongs to another installation or version of katyoti sharing this data folder; "+
		"stop it, or give each version its own installation (katyoti instance create)", hub, client)
}

// connectOptions is the ONE way a HubClient dials its hub: infinite
// reconnects, and the lock file re-read before every attempt.
//
// The hub's embedded NATS listens on a random port, and the lock file is how
// a client learns it. Read once at connect, a hub that restarted on another
// port was never reached again: nats-go kept dialling the old URL for ever,
// so `OnReconnect` — the one hook that resyncs a consumer's universe view —
// could not fire, and the view answered from memory for the life of the
// process (blind audit H5). Following the lock file on each attempt keeps the
// random port (several universes share a host, each with its own hub, and a
// pinned port per universe would need allocating) and keeps nats-go's own
// reconnect loop, subscriptions re-established and the reconnect hooks
// included; only the address it dials is current.
func (c *HubClient) connectOptions() natsxport.ConnectOptions {
	return natsxport.ConnectOptions{
		Name:           dialName(),
		MaxReconnects:  -1,
		AllowReconnect: true,
		ResolveURL:     c.resolveFromLockFile,
		Credential:     c.key,
	}
}

// resolveFromLockFile reads the hub's lock file and returns the URL it names,
// remembering it so LockFile() and NATSURL() report the hub actually reached.
// No lock file (the hub is down, or not yet up again) resolves to nothing, and
// the dial falls back to the last known address, which is refused until the
// hub is back — the reconnect loop's ordinary wait.
func (c *HubClient) resolveFromLockFile() (string, error) {
	lf, err := ReadLockFile(c.dataDir)
	if err != nil || lf == nil || lf.NATSURL == "" {
		return "", err
	}
	c.mu.Lock()
	c.lockfile = lf
	c.mu.Unlock()
	return lf.NATSURL, nil
}

// MessageBus returns the connected MessageBus.
func (c *HubClient) MessageBus() mbapi.MessageBus { return c.handle.MessageBus() }

// OnReconnect registers fn to run each time the link to the hub comes back
// after a drop. See natsxport.ClientHandle.OnReconnect.
func (c *HubClient) OnReconnect(fn func()) {
	if c == nil || c.handle == nil {
		return
	}
	c.handle.OnReconnect(fn)
}

// LockFile returns the lockfile that was used to discover the hub.
// Useful for diagnostics (PID, hub version, started_at).
func (c *HubClient) LockFile() *LockFile {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lockfile == nil {
		return nil
	}
	clone := *c.lockfile
	return &clone
}

// NATSURL returns the URL of the hub last reached — after a re-dial, the new
// hub's.
func (c *HubClient) NATSURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lockfile == nil {
		return ""
	}
	return c.lockfile.NATSURL
}

// Close terminates the NATS connection. It never ends the hub, not even one
// this client spawned: other clients may be connected to it, and an owned hub
// ends by itself once none is (owner.go, EndWhenUnused). A client that spawned
// the hub waits for that end when it was the last client, so the process
// leaves no hub behind, and returns at once when others still use it.
func (c *HubClient) Close() error {
	if c.handle != nil {
		c.handle.Close()
	}
	if cmd := c.ownedHub; cmd != nil {
		c.ownedHub, c.spawned = nil, true
		exited := make(chan struct{})
		go func() { _ = cmd.Wait(); close(exited) }()
		awaitOwnedEnd(c.dataDir, cmd.Process.Pid, exited)
	}
	return nil
}

// Errors surfaced by ConnectToHub.

// ErrNoHub is returned by ConnectToHub in ModeReuse when no lockfile
// exists at <DataDir>/hub/hub.lock — i.e. no hub is running.
var ErrNoHub = errors.New("hubclient: no hub running for this universe (no lockfile)")

// ErrStaleLockfile is returned when the lockfile points at a process
// that's no longer alive. Today's first impl pass returns this error
// rather than auto-cleaning — the operator decides (manual cleanup or
// spawn).
var ErrStaleLockfile = errors.New("hubclient: lockfile points at a dead process")

// ErrHubUnreachable is returned when a lockfile exists and its recorded
// PID appears alive, yet the NATS URL refuses the connection. This is the
// PID-reuse case: the original hub died and the OS recycled its PID to an
// unrelated process, so ProcessAlive can't detect the staleness — the only
// reliable signal is that nothing is listening on the recorded URL. Treated
// as respawnable by ModeFindOrSpawn (the lock is, in effect, stale).
var ErrHubUnreachable = errors.New("hubclient: lockfile present but hub is unreachable (likely a stale lock with a reused PID)")

// ErrHubBinaryNotFound is returned by ModeSpawn / ModeFindOrSpawn when
// the ktw-hub binary can't be located (HubBinaryPath unset and not on
// PATH).
// ErrForeignHub refuses to start a hub of an installation this program is not
// part of: that hub would drop the universe it is given and
// never come up.
var ErrForeignHub = errors.New("hubclient: the hub belongs to an installation this program is not part of")

var ErrHubBinaryNotFound = errors.New("hubclient: ktw-hub binary not found (set ConnectOptions.HubBinaryPath or place ktw-hub on PATH)")

// ConnectToHub finds (and optionally spawns) a hub matching the
// configured DataDir, then opens a NATS connection to it and wraps
// the connection in a MessageBus.
//
// Modes:
//   - ModeReuse: error if no hub running.
//   - ModeSpawn: always exec a fresh hub; client owns its lifetime.
//   - ModeFindOrSpawn (most common for ktw-gui / ktw-agents CLI):
//     reuse if running, else spawn. A spawned hub serves every client
//     until none is connected (owner.go).
func ConnectToHub(ctx context.Context, opts ConnectOptions) (*HubClient, error) {
	if opts.DataDir == "" {
		return nil, fmt.Errorf("hubclient: ConnectOptions.DataDir is required")
	}

	switch opts.Mode {
	case ModeReuse:
		return connectReuse(ctx, opts)
	case ModeSpawn:
		return connectSpawn(ctx, opts)
	case ModeFindOrSpawn:
		return connectFindOrSpawn(ctx, opts)
	default:
		return nil, fmt.Errorf("hubclient: unknown Mode %d", opts.Mode)
	}
}

func connectReuse(ctx context.Context, opts ConnectOptions) (*HubClient, error) {
	lf, err := ReadLockFile(opts.DataDir)
	if err != nil {
		return nil, err
	}
	if lf == nil {
		return nil, ErrNoHub
	}
	if !hubAlive(lf.PID) {
		// The hub ended without its cleanup: the lockfile is nobody's. It is
		// removed here, so the next reader finds no hub rather than a dead one
		// - only while it still names that dead hub.
		removeDeadLockFile(opts.DataDir)
		return nil, fmt.Errorf("%w: pid %d (its lockfile removed: no hub runs for this universe)", ErrStaleLockfile, lf.PID)
	}

	c := &HubClient{lockfile: lf, dataDir: opts.DataDir, skew: HubVersionSkew(opts.ExpectVersion, lf.HubVersion)}
	if c.skew != "" {
		log.Printf("hubclient: WARNING %s", c.skew)
	}
	if err := c.loadKey(opts.ClientKind); err != nil {
		return nil, err
	}
	handle, err := natsxport.Connect(ctx, lf.NATSURL, c.connectOptions())
	if err != nil {
		// The PID looked alive but nothing accepts connections on the
		// recorded URL. On Windows especially this is PID reuse: the hub
		// died, its PID got recycled to an unrelated process, so the
		// liveness probe is fooled. Surface a distinct sentinel so
		// ModeFindOrSpawn can recover by respawning.
		return nil, fmt.Errorf("%w: %s: %v", ErrHubUnreachable, lf.NATSURL, err)
	}
	c.handle = handle
	return c, nil
}

// connectFindOrSpawn tries ModeReuse first; on ErrNoHub or
// ErrStaleLockfile (hub crashed without removing its lockfile), falls
// back to ModeSpawn. Stale-lockfile cleanup happens implicitly: the
// spawn path overwrites the lockfile when the new hub starts.
func connectFindOrSpawn(ctx context.Context, opts ConnectOptions) (*HubClient, error) {
	c, err := connectReuse(ctx, opts)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, ErrNoHub) && !errors.Is(err, ErrStaleLockfile) && !errors.Is(err, ErrHubUnreachable) {
		return nil, err
	}

	// SINGLE-FLIGHT in this process first. Callers that arrive together in one
	// process (ktw-gui activating the admin universe from several frontend calls
	// at a restart) share ONE find-or-spawn: the first leads, the others wait
	// for its outcome and attach to the hub it started. A follower never spawns
	// on its own — after a failed or cancelled lead it reports that failure,
	// where it used to take the cross-process guard next and start a hub of its
	// own, one per waiting caller.
	f, leader := joinSpawnFlight(opts.DataDir)
	if !leader {
		select {
		case <-f.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if f.err != nil {
			return nil, fmt.Errorf("hubclient: the hub this process was starting for %s did not come up: %w", opts.DataDir, f.err)
		}
		return connectReuse(ctx, opts)
	}
	c, err = findOrSpawnLeader(ctx, opts, err)
	f.finish(err)
	return c, err
}

// findOrSpawnLeader is the find-or-spawn of the one caller per process that
// leads it (connectFindOrSpawn); probeErr is why the first reuse failed.
func findOrSpawnLeader(ctx context.Context, opts ConnectOptions, probeErr error) (*HubClient, error) {
	err := probeErr
	// On its way to a hub: announced, so the hub it waits for stays up until
	// it has connected (waiting.go).
	defer announceWaiting(opts.DataDir)()
	// A SLOW HUB IS THE UNIVERSE'S HUB. A live hub that did not
	// answer the first dial in time (busy, its machine loaded) is waited for
	// and joined; a client never starts a hub beside it, nor fails at once.
	// A hub that ended while waited for is followed by the guarded
	// find-or-spawn below, as a dead one.
	if errors.Is(err, ErrHubUnreachable) && LiveHub(opts.DataDir) != nil {
		c, werr, gone := awaitSlowHub(ctx, opts, err)
		if !gone {
			return c, werr
		}
		err = werr
	}
	// SERIALISE the spawn across processes. Everything from here to the spawned
	// hub's lockfile appearing is a critical section: connectSpawn REMOVES the
	// lockfile before exec'ing, so two callers arriving together would both
	// spawn AND the second would delete the first hub's lockfile — leaving live
	// hubs nobody can address and clients wedged against them. Several binaries
	// activate the same universe (ktw-gui, the katyoti CLI, ktw-agents), so a
	// mutex in this process is not enough.
	guard, got, gerr := acquireSpawnGuard(opts.DataDir)
	if gerr != nil {
		return nil, guardErr(opts.DataDir, gerr)
	}
	// Someone else is spawning. Wait for them, then RE-CHECK: the hub we need
	// almost certainly now exists, and reusing it is the entire point of
	// queueing rather than racing. A holder that finished without leaving a
	// usable hub (it failed, or its hub ended at once) is followed by whoever
	// takes the guard next — this caller, or another one it waits for in turn.
	deadline := time.Now().Add(spawnGuardWait)
	for !got {
		if !waitForGuard(opts.DataDir, time.Until(deadline)) {
			return nil, fmt.Errorf("hubclient: another process is starting a hub for %s and did not finish in %v",
				opts.DataDir, spawnGuardWait)
		}
		if c, rerr := connectReuse(ctx, opts); rerr == nil {
			return c, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		guard, got, gerr = acquireSpawnGuard(opts.DataDir)
		if gerr != nil {
			return nil, guardErr(opts.DataDir, gerr)
		}
	}
	defer guard.release()

	// Re-check under the guard. Between our first probe and taking it, a
	// concurrent caller may have started a perfectly good hub.
	c, rerr := connectReuse(ctx, opts)
	if rerr == nil {
		return c, nil
	}
	err = rerr

	// Only a universe with no hub, or whose lockfile names a dead one, gets a
	// new hub: any other failure of the re-check (a lockfile that cannot be
	// read, the client's key, a cancelled call) is reported, never answered by
	// a spawn that would supplant a hub still running.
	if !errors.Is(err, ErrNoHub) && !errors.Is(err, ErrStaleLockfile) && !errors.Is(err, ErrHubUnreachable) {
		return nil, err
	}

	// A hub that runs but does not answer (busy, swapping) is the universe's
	// hub still: a second one beside it would find its members' serving locks
	// held by the first one's and never serve. Only
	// a lockfile naming no live hub process — the pid dead or reused by
	// another program — is replaced.
	if errors.Is(err, ErrHubUnreachable) {
		if lf := LiveHub(opts.DataDir); lf != nil {
			return nil, notAnswering(lf, err)
		}
	}

	// Stale or unreachable lockfile from a crashed hub — remove it before
	// the new hub starts so the new hub's deferred cleanup doesn't fight a
	// non-existent file. Best-effort. (ErrNoHub has no lockfile to remove.)
	if errors.Is(err, ErrStaleLockfile) || errors.Is(err, ErrHubUnreachable) {
		removeDeadLockFile(opts.DataDir)
	}
	// Said where the command's log goes: why this caller starts a hub is what
	// a person reads when two hubs met.
	log.Printf("hubclient: no live hub for %s (%v): starting one", opts.DataDir, err)
	return spawnHub(ctx, opts)
}

// spawnHub starts the hub; a test stands a fake in for the real exec.
var spawnHub = connectSpawn

// slowHubWait bounds how long a client waits for a live hub that does not
// answer its dial (each dial is the transport's handshake timeout); a hub
// still silent past it is reported, never replaced.
var slowHubWait = 30 * time.Second

// awaitSlowHub dials the live hub again until it answers, it is no longer the
// universe's live hub (gone: the caller goes on as for a dead hub), or
// slowHubWait runs out.
func awaitSlowHub(ctx context.Context, opts ConnectOptions, err error) (c *HubClient, _ error, gone bool) {
	deadline := time.Now().Add(slowHubWait)
	for {
		lf := LiveHub(opts.DataDir)
		if lf == nil {
			return nil, err, true
		}
		if !time.Now().Before(deadline) {
			return nil, notAnswering(lf, err), false
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err(), false
		case <-time.After(250 * time.Millisecond):
		}
		c, rerr := connectReuse(ctx, opts)
		if rerr == nil {
			return c, nil, false
		}
		err = rerr
	}
}

// notAnswering is the error of a live hub that does not accept a connection.
func notAnswering(lf *LockFile, err error) error {
	return fmt.Errorf("%w: %s does not accept a connection on %s (%v); no second hub is started beside it - "+
		"retry, or stop it (`katyoti universe stop`)", ErrHubNotAnswering, DescribeProcess(lf.PID), lf.NATSURL, err)
}

// spawnFlight is one in-process find-or-spawn for a DataDir: done closes when
// its leader is through, err says how it went.
type spawnFlight struct {
	key  string
	done chan struct{}
	err  error
}

var (
	spawnFlightsMu sync.Mutex
	spawnFlights   = map[string]*spawnFlight{}
)

// spawnFlightKey names a DataDir the same however a caller spelt it — the
// cross-process guard is a file, so two spellings of one folder share it; this
// map must agree.
func spawnFlightKey(dataDir string) string {
	key := dataDir
	if abs, err := filepath.Abs(dataDir); err == nil {
		key = abs
	}
	key = filepath.Clean(key)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

// joinSpawnFlight returns the DataDir's flight in progress (leader false), or
// opens one this caller leads (leader true) — who must then finish it.
func joinSpawnFlight(dataDir string) (*spawnFlight, bool) {
	key := spawnFlightKey(dataDir)
	spawnFlightsMu.Lock()
	defer spawnFlightsMu.Unlock()
	if f, ok := spawnFlights[key]; ok {
		return f, false
	}
	f := &spawnFlight{key: key, done: make(chan struct{})}
	spawnFlights[key] = f
	return f, true
}

// finish records the outcome, wakes the followers and closes the flight: a
// caller arriving after it probes afresh.
func (f *spawnFlight) finish(err error) {
	spawnFlightsMu.Lock()
	delete(spawnFlights, f.key)
	spawnFlightsMu.Unlock()
	f.err = err
	close(f.done)
}

// spawnGuardWait is how long to queue behind another process's spawn before
// giving up on it and spawning ourselves. It must cover a cold hub start, which
// re-probes every stale sidecar manifest before writing its lockfile — the
// same budget connectSpawn allows for the lockfile to appear.
const spawnGuardWait = 90 * time.Second

// connectSpawn execs `ktw-hub serve`, polls for the lockfile to
// appear (the hub's "ready" signal), then connects via the NATS URL
// the lockfile carries. The client owns the spawned subprocess —
// Close() SIGTERMs it.
//
// Spawn budget: 15s for the lockfile to appear. Most hubs come up in
// under 2s on a warm cache; the wider budget covers cold starts on
// slow disks. Cancellable via ctx.
func connectSpawn(ctx context.Context, opts ConnectOptions) (*HubClient, error) {
	binary, err := resolveHubBinary(opts.HubBinaryPath)
	if err != nil {
		return nil, err
	}
	// A hub of an installation this program is not part of takes no
	// universe from it: say so now, and how to launch, rather than wait for
	// a lockfile that never comes.
	if inst := cenv.ForeignInstallation(binary); inst != nil {
		self, _ := os.Executable()
		return nil, fmt.Errorf("%w: %s is the installation %s (%s), and this program (%s) is not of it, so the hub would drop the universe it is given. "+
			"Launch KaTyoti from that installation: `katyoti gui` (or its shortcut), or `katyoti universe start <universe>`",
			ErrForeignHub, binary, inst.Name, inst.Path(), self)
	}

	// Make sure no stale lockfile is around — we want to detect the
	// new hub's freshly-written one, not the previous run's leftover. A live
	// hub's is never removed: the hub spawned here refuses to serve beside
	// it, and this client joins it (below).
	removeDeadLockFile(opts.DataDir)

	cmd := exec.Command(binary, "serve")
	if opts.Detach {
		publish.Detach(cmd) // its own group / session and a hidden console: it outlives this process
	} else {
		publish.NoConsoleWindow(cmd) // a background hub opens no terminal window
	}
	// Inherit env so KTW_WORKSPACE / KTW_UNIVERSE flow through; that's
	// how the hub picks the same DataDir as us. Callers (e.g. ktw-gui)
	// that manage multiple universes pass SpawnEnv to scope each spawn
	// to a specific universe without polluting the host process's env.
	if len(opts.SpawnEnv) > 0 {
		cmd.Env = opts.SpawnEnv
	} else {
		cmd.Env = os.Environ()
	}
	// An owned hub is a command's: it ends once no client uses it (owner.go);
	// a detached one is the universe's and is never ended that way.
	if !opts.Detach {
		cmd.Env = append(cmd.Env, OwnerEnv+"="+strconv.Itoa(os.Getpid()))
	}
	cmd.Stdout = nil
	// Stream ktw-hub's stderr to the parent's logger so spawn / readiness
	// diagnostics (including downstream sidecar stderr that ktw-hub forwards
	// via the connmgr stream) reach ktw-gui's log file. Without this, a
	// failure in `sc ktsp nats` boot for any sidecar is invisible — the
	// readiness probe times out with no actionable message. A DETACHED hub
	// outlives this process: no pipe to a parent that will be gone.
	if !opts.Detach {
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return nil, fmt.Errorf("hubclient: stderr pipe for %s: %w", binary, err)
		}
		go streamHubStderr(stderr)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("hubclient: exec %s serve: %w", binary, err)
	}

	// No job object: a job would kill an owned hub at the instant its spawner
	// ends, with no cleanup and under the other clients connected to it. The
	// owned hub ends by itself once none is (OwnerEnv, EndWhenUnused); a
	// detached hub outlives us by design.

	// Poll for the lockfile. Budget covers the worst-case cold boot:
	// ktw-hub serve writes the lockfile only AFTER its cache hydration,
	// which re-probes every Univers.yaml sidecar whose tier-1 manifest
	// cache entry is missing or stale (an exec-and-kill
	// `<binary> sc resources manifest`, appcore.RunResourcesList). No
	// sidecar is spawned at boot — the connection manager spawns one
	// lazily on its first Acquire (internal/extensions/connmgr). For
	// universes with 10+ stale sidecars on a cold disk this can take
	// tens of seconds.
	const (
		lockfileTimeout = 90 * time.Second
		pollInterval    = 100 * time.Millisecond
	)
	deadline := time.Now().Add(lockfileTimeout)
	var lf *LockFile
	for {
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
			return nil, fmt.Errorf("hubclient: spawn timeout — lockfile didn't appear within %v", lockfileTimeout)
		}
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
			return nil, fmt.Errorf("hubclient: spawn cancelled: %w", ctx.Err())
		case <-time.After(pollInterval):
		}
		got, readErr := ReadLockFile(opts.DataDir)
		if readErr != nil {
			continue // partial-write window; retry
		}
		if got != nil && got.NATSURL != "" {
			lf = got
			break
		}
	}

	// The lockfile names another hub: one already served the universe, and
	// the hub spawned here refused to serve beside it (ClaimLockFile) and is
	// ending. This client joins the running hub, which is not its own.
	if lf.PID != cmd.Process.Pid {
		go func() { _ = cmd.Wait() }()
		return connectReuse(ctx, opts)
	}

	c := &HubClient{lockfile: lf, dataDir: opts.DataDir, ownedHub: cmd}
	if err := c.loadKey(opts.ClientKind); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, err
	}
	// A hub just up may refuse the first dial (its key registry read held a
	// moment by a scanner reads empty: Authorization Violation); it reads the
	// registry again on the next one. Killing it at once left its lockfile
	// naming a dead hub and the next command spawned again - three hubs
	// started and killed in a row before one served.
	handle, err := natsxport.Connect(ctx, lf.NATSURL, c.connectOptions())
	for retryBy := time.Now().Add(spawnedDialBudget); err != nil && ctx.Err() == nil && time.Now().Before(retryBy); {
		time.Sleep(250 * time.Millisecond)
		_ = c.loadKey(opts.ClientKind) // this side's key read again too
		handle, err = natsxport.Connect(ctx, lf.NATSURL, c.connectOptions())
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, fmt.Errorf("hubclient: connect to spawned hub at %s: %w", lf.NATSURL, err)
	}
	c.handle = handle
	if opts.Detach {
		// The hub is up and reachable: from here on it is nobody's child.
		// Release the process handle so no Wait is owed and Close leaves it.
		_ = cmd.Process.Release()
		c.ownedHub = nil
		c.spawned = true
	}
	return c, nil
}

// spawnedDialBudget bounds how long a spawner dials the hub it just started
// again before it gives that hub up.
var spawnedDialBudget = 5 * time.Second

// Spawned reports whether this client's connect STARTED the hub (owned or
// detached), as opposed to finding one running.
func (c *HubClient) Spawned() bool { return c != nil && (c.ownedHub != nil || c.spawned) }

// resolveHubBinary returns the absolute path to the ktw-hub binary.
// Order:
//  1. opts.HubBinaryPath if set, else the KTW_HUB_BINARY variable (a
//     session or a test running a freshly built hub over the installed one)
//  2. the control-plane location — a `katyoti install-infra` / `self-update`
//     puts the canonical binary there; same first-choice rule as
//     launcher.ResolveGUI, so both control-plane binaries resolve alike
//  3. project-local ./bin/ktw-hub(.exe) — convenient for dev
//  4. exec.LookPath("ktw-hub") on $PATH
//
// A plain dev checkout has no populated control-plane dir, so (3) still wins
// there. If you HAVE installed a control plane but want a freshly built hub,
// pass ConnectOptions.HubBinaryPath (or remove the installed copy).
func resolveHubBinary(override string) (string, error) {
	if override == "" {
		override = os.Getenv(HubBinaryEnv)
	}
	if override != "" {
		if abs, err := filepath.Abs(override); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs, nil
			}
		}
		return "", fmt.Errorf("%w: %s", ErrHubBinaryNotFound, override)
	}
	// The installed control plane wins over dev/PATH copies.
	if p, err := controlplane.TargetPath("ktw-hub"); err == nil {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Project-local fallback (works during dev when ./bin/ktw-hub is built).
	candidate := "ktw-hub"
	if runtime.GOOS == "windows" {
		candidate = "ktw-hub.exe"
	}
	for _, base := range []string{"./bin", "."} {
		full, err := filepath.Abs(filepath.Join(base, candidate))
		if err == nil {
			if _, err := os.Stat(full); err == nil {
				return full, nil
			}
		}
	}
	if path, err := exec.LookPath("ktw-hub"); err == nil {
		return path, nil
	}
	return "", ErrHubBinaryNotFound
}

// HubBinaryEnv names the variable that chooses the ktw-hub binary a spawn
// runs, over the installed control plane's.
const HubBinaryEnv = "KTW_HUB_BINARY"

// ProcessAlive reports whether the PID corresponds to a live OS process —
// the one check, server.IsProcessAlive (kept under this name for its callers).
func ProcessAlive(pid int) bool { return server.IsProcessAlive(pid) }
