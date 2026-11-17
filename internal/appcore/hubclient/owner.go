// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// A HUB ONE COMMAND STARTED SERVES EVERY COMMAND UNTIL NONE USES IT. A hub
// spawned owned (not Detach) is told its spawner's pid in OwnerEnv, which
// marks it as a command's hub rather than the universe's long-lived one. It
// does not end with its spawner: other commands found it running and are
// connected to it, and ending it under them failed their calls (Slot not
// found) and made the next command spawn again — one hub boot every few
// seconds while several sessions worked. It ends once no client
// is connected (EndWhenUnused), whichever process spawned it and however that
// process ended; the spawner's Close waits for that end when it was the last
// client, so a command leaves no hub behind. A detached hub (`--hub keep`,
// `katyoti universe start`) carries no owner and is never ended this way.

// OwnerEnv names the variable a spawner sets to its pid on an owned hub.
const OwnerEnv = "KTW_HUB_OWNER"

// ClientName is the connection name every HubClient of an outside client
// dials its hub with: what the hub counts as its outside clients.
const ClientName = "hubclient"

// MemberClientName prefixes the connection name of a HubClient dialled from a
// process the hub serves (a member): the hub never counts it.
const MemberClientName = "hubclient-member"

// A MEMBER'S OWN HUBCLIENT IS NOT AN OUTSIDE CLIENT. A process
// the hub serves dials its hub for its own needs as well — the universe
// catalog at boot, the shared HubClient a serving hook resolves (task
// restore), a bus sender. Dialled under ClientName, such a connection counted
// as an outside client for the member's whole life, and an owned hub serving
// that member never ended after its last command. A member is known by its
// own command line — the hub starts it as `<binary> [host flags] sc ktsp nats`
// (connmgr) —, read before anything dials, at boot included: never by an
// environment variable, which the commands a member starts would inherit
// while they are real clients and dial as such.
var processArgs = func() []string { return os.Args }

// servedCommand is the command a member is started with by its hub.
var servedCommand = []string{"sc", "ktsp", "nats"}

// dialName is the connection name this process's HubClients dial under:
// MemberClientName+":"+<binary> in a member, ClientName in any other process.
func dialName() string {
	args := processArgs()
	if len(args) == 0 {
		return ClientName
	}
	for i := 1; i+len(servedCommand) <= len(args); i++ {
		if slices.Equal(args[i:i+len(servedCommand)], servedCommand) {
			binary := strings.TrimSuffix(filepath.Base(args[0]), filepath.Ext(args[0]))
			return MemberClientName + ":" + binary
		}
	}
	return ClientName
}

// Owned reports whether this hub was spawned owned by a command (OwnerEnv).
func Owned() bool {
	pid, err := strconv.Atoi(os.Getenv(OwnerEnv))
	return err == nil && pid > 0
}

var (
	// unusedPoll is how often an owned hub counts its clients.
	unusedPoll = 50 * time.Millisecond
	// firstClientWait bounds how long an owned hub waits for its first
	// client: its spawner connects right after the lockfile appears, and one
	// that died before connecting must not leave the hub running.
	firstClientWait = 15 * time.Second
)

// EndWhenUnused calls end once clients answers none open, after at least one
// client ever connected (or after firstClientWait when none ever did); clients
// answers the connections open now — the callers on their way to it included
// (Waiting) — and those ever made. It returns then, or
// when ctx ends. Before ending it hides the hub (hide: its lockfile removed,
// so no new client finds it) and counts once more: a client that connected
// meanwhile shows the hub again (show) and the watch goes on, so a command
// arriving at the last moment is never served by a hub on its way out. A hide
// that answers false did not hide (a command is starting or finding a hub —
// HideOwnLockFile): the watch counts again at the next poll.
func EndWhenUnused(ctx context.Context, clients func() (open, ever int), hide func() bool, show, end func()) {
	started := time.Now()
	tick := func() bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(unusedPoll):
			return true
		}
	}
	for tick() {
		open, ever := clients()
		if open > 0 || (ever == 0 && time.Since(started) < firstClientWait) {
			continue
		}
		if !hide() {
			continue
		}
		if !tick() {
			return
		}
		if open, _ := clients(); open > 0 {
			show()
			continue
		}
		end()
		return
	}
}

// closeWatch bounds how long a spawner's Close looks for its hub ending: a
// hub left with no client hides its lockfile within a poll or two; one still
// shown past this is used by other clients and left to them.
const closeWatch = time.Second

// hubEndWait bounds the wait for a hub that is ending (its shutdown budget).
const hubEndWait = 15 * time.Second

// awaitOwnedEnd is the spawner's side of a hub's end, after its own
// connection closed: it waits for the hub to exit when it was the last client
// (the hub hides its lockfile, then exits), and returns at once when the hub
// stays up for others. exited closes when the hub process has ended.
func awaitOwnedEnd(dataDir string, pid int, exited <-chan struct{}) {
	shown := func() bool {
		lf, err := ReadLockFile(dataDir)
		return err == nil && lf != nil && lf.PID == pid
	}
	watchEnd := time.Now().Add(closeWatch)
	var endBy time.Time
	for {
		select {
		case <-exited:
			return
		case <-time.After(unusedPoll):
		}
		switch {
		case endBy.IsZero() && !shown():
			endBy = time.Now().Add(hubEndWait) // hidden: it is ending
		case endBy.IsZero() && time.Now().After(watchEnd):
			return // still shown: other clients use it
		case !endBy.IsZero() && (shown() || time.Now().After(endBy)):
			return // shown again for a late client, or past its budget
		}
	}
}
