// SPDX-License-Identifier: Apache-2.0

package contribapi

import "sync"

// drop.go — what a producer saw and could not publish.
//
// A provider that meets a file it cannot parse SKIPS it. That is right at
// runtime and has been deliberate for a long time: one malformed skill must not
// cost a universe the other two hundred, so the descriptor keeps populating and
// the bad file simply is not there.
//
// It is exactly wrong at packaging. Nothing has been distributed, the author is
// standing there, and the quiet result is a pack that installs cleanly and is
// missing the content it was built to carry. c20's check cannot see this: it
// reasons about contributions, and a dropped file produces none.
//
// So the fact TRAVELS. A provider records what it skipped, the manifest carries
// it, packaging refuses on it, and runtime keeps ignoring it exactly as before.
// Reconstructing the same signal by counting files would mean re-implementing
// every provider's globbing rules, which drift the moment a provider's layout
// changes — the information belongs where it was known.

// Drop is one file that produced no contribution, and why.
type Drop struct {
	// Path is the file, as the provider's loader addresses it.
	Path string `json:"path"`

	// Kind is what it would have become. Recorded because the reader of a
	// drop is usually looking for the absence of a particular kind of thing.
	Kind string `json:"kind,omitempty"`

	// Reason is what went wrong, in the producer's words. It is the only
	// account of the failure that will ever exist — nothing else saw it.
	Reason string `json:"reason"`
}

// DropReporter is implemented by a provider that can say what it skipped.
//
// Optional, and deliberately not part of ContributionProvider: a provider with
// nothing to drop — one enumerating in-memory service definitions, say — has no
// question to answer here, and widening the contract would make every
// implementation answer it anyway.
type DropReporter interface {
	// Drops returns what the most recent Enumerate skipped.
	Drops() []Drop
}

// DropLog accumulates drops for a provider. Embed it and call Record.
//
// Guarded, because Enumerate is called from whatever goroutine is building a
// descriptor and a provider is shared. Reset at the start of each Enumerate, so
// what Drops returns describes the enumeration that just happened rather than
// every enumeration since the process started.
type DropLog struct {
	mu    sync.Mutex
	drops []Drop
}

// Record notes one skipped file.
func (l *DropLog) Record(path, kind, reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.drops = append(l.drops, Drop{Path: path, Kind: kind, Reason: reason})
}

// ResetDrops clears the log. Called at the top of Enumerate.
func (l *DropLog) ResetDrops() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.drops = nil
}

// Drops implements DropReporter.
func (l *DropLog) Drops() []Drop {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.drops) == 0 {
		return nil
	}
	out := make([]Drop, len(l.drops))
	copy(out, l.drops)
	return out
}
