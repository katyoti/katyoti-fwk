// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ONE HUB PER UNIVERSE, AND ITS LOCKFILE IS ITS OWN. The
// lockfile is how every client finds the universe's hub; a hub whose lockfile
// is gone runs on, unseen, holding its members — and a member holds its
// provider's serving lock for the universe, so every hub started after it
// finds its own member unable to serve. Three rules keep that from happening:
//
//   - a hub removes the lockfile only while it names the hub itself
//     (RemoveOwnLockFile): one hub ending, hiding or timing out never
//     unregisters another;
//   - a hub writes the lockfile only while no other live hub's is there
//     (ClaimLockFile): a second hub for a universe whose hub runs refuses to
//     serve, and its spawner joins the running one;
//   - a client never takes a live hub that is slow to answer for a dead one:
//     only a lockfile naming no live hub process is replaced by a new spawn
//     (LiveHub).

// HubRunningError refuses a second hub: another live hub's lockfile names it.
type HubRunningError struct {
	PID     int
	NATSURL string
}

func (e *HubRunningError) Error() string {
	return fmt.Sprintf("a hub already serves this universe: %s at %s - one hub per universe", DescribeProcess(e.PID), e.NATSURL)
}

// ErrHubNotAnswering: the universe's hub runs but does not accept a
// connection; no second hub is started beside it.
var ErrHubNotAnswering = errors.New("hubclient: the universe's hub runs but does not answer")

// A LOCKFILE IS REMOVED ONLY ON PROOF ITS HUB IS GONE. A
// client that misjudged a live hub dead removed its lockfile under the spawn
// guard and started a second hub; the first one, unregistered, was ended by
// the second (EndWhenSupplanted) and every client of it dropped. So a hub is
// alive while either the liveness probe says so or the system's process list
// still shows its pid running a ktw-hub (hubAlive), and a lockfile is removed
// only while, read again right before, it still names a hub that reads dead
// (removeDeadLockFile) - never by path on an earlier judgement.

// liveProbe is ProcessAlive; a test stands in for it to make the probe lie.
var liveProbe = ProcessAlive

// hubAlive reports whether the hub pid still runs: the liveness probe, or
// the process list showing pid as a ktw-hub.
func hubAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return liveProbe(pid) || listedHub(pid)
}

// listedHub reports whether the system's process list shows pid running a
// ktw-hub; a test stands in for it.
var listedHub = func(pid int) bool {
	program, _, ok := processInfo(pid)
	return ok && isHubProgram(program)
}

// removeDeadLockFile removes the lockfile only while it names a hub that is
// not alive (hubAlive), read again right before; a lockfile that cannot be
// read, or names a live hub, is left. It answers whether it removed one.
func removeDeadLockFile(dataDir string) bool {
	lf, err := ReadLockFile(dataDir)
	if err != nil || lf == nil || hubAlive(lf.PID) {
		return false
	}
	removed, err := removeLockFileNaming(dataDir, lf)
	return removed && err == nil
}

// LiveHub answers the lockfile when it names a live hub process — a process
// alive whose program is a ktw-hub, or whose program cannot be read (a hub is
// assumed rather than a second one started) — else nil: no lockfile, a dead
// pid, or a pid the system has given to another program.
func LiveHub(dataDir string) *LockFile {
	lf := Running(dataDir)
	if lf == nil || !hubProcess(lf.PID) {
		return nil
	}
	return lf
}

// hubProcess is isHubProcess; a test stands in for it to make a live process
// of its own a hub.
var hubProcess = isHubProcess

// isHubProcess reports whether a live pid runs a ktw-hub; true when its
// program cannot be read.
func isHubProcess(pid int) bool {
	program, _, ok := processInfo(pid)
	if !ok || program == "" {
		return true
	}
	return isHubProgram(program)
}

// isHubProgram reports a program name of the hub (ktw-hub, ktw-hub.exe).
func isHubProgram(program string) bool {
	name := strings.TrimSuffix(strings.ToLower(program), ".exe")
	return name == "ktw-hub"
}

// ClaimLockFile writes the lockfile for this process unless another live
// hub's lockfile is there: then it answers *HubRunningError and leaves it.
func ClaimLockFile(dataDir string, lf LockFile) error {
	if lf.PID == 0 {
		lf.PID = os.Getpid()
	}
	if cur := LiveHub(dataDir); cur != nil && cur.PID != lf.PID {
		return &HubRunningError{PID: cur.PID, NATSURL: cur.NATSURL}
	}
	return WriteLockFile(dataDir, lf)
}

// RemoveOwnLockFile removes the lockfile only while it names pid: another
// hub's lockfile, and one that cannot be read now, are left in place - a hub
// never unregisters another; a file left naming a hub that
// ended is removed by the next client (removeDeadLockFile).
func RemoveOwnLockFile(dataDir string, pid int) error {
	lf, err := ReadLockFile(dataDir)
	if err != nil || lf == nil || lf.PID != pid {
		return nil
	}
	_, err = removeLockFileNaming(dataDir, lf)
	return err
}

// removeLockFileNaming removes the lockfile only while, read again right
// before, it is still lf (same pid, same start): one a hub wrote meanwhile is
// that hub's, and kept. It answers whether it removed it.
func removeLockFileNaming(dataDir string, lf *LockFile) (bool, error) {
	again, err := ReadLockFile(dataDir)
	if err != nil || again == nil || again.PID != lf.PID || !again.StartedAt.Equal(lf.StartedAt) {
		return false, nil
	}
	if err := RemoveLockFile(dataDir); err != nil {
		return false, err
	}
	return true, nil
}

// DescribeProcess names a process for a person: "pid N (program), started by
// pid M (program)" — as much of it as the system shows.
func DescribeProcess(pid int) string {
	out := "pid " + strconv.Itoa(pid)
	program, ppid, ok := processInfo(pid)
	if !ok {
		return out
	}
	if program != "" {
		out += " (" + program + ")"
	}
	if ppid > 0 && ProcessAlive(ppid) {
		out += ", started by pid " + strconv.Itoa(ppid)
		if parent, _, ok := processInfo(ppid); ok && parent != "" {
			out += " (" + parent + ")"
		}
	}
	return out
}

// DescribeHolder names the process a lock note ("pid N", what a serving
// process writes in its lock) designates; the note itself when it names no
// pid, "" for an empty note.
func DescribeHolder(note string) string {
	pidText, ok := strings.CutPrefix(strings.TrimSpace(note), "pid ")
	if !ok {
		return note
	}
	pid, err := strconv.Atoi(strings.TrimSpace(pidText))
	if err != nil || pid <= 0 {
		return note
	}
	return DescribeProcess(pid)
}
