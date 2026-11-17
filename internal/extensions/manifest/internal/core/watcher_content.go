// SPDX-License-Identifier: Apache-2.0

package core

import "time"

// watcher_content.go — noticing that a served folder changed.
//
// A generic host running someone's folder has the same executable whatever the
// folder holds, so nothing about the binary says its contributions moved. The
// digest of the folder does.
//
// The aggregator POLLS rather than subscribing to filesystem events, and that
// is the deliberate choice: digest comparison is the correctness mechanism, and
// a notification only reduces latency. A poller works with nothing else
// running, self-heals any missed signal, and behaves the same on every platform
// — whereas a filesystem watcher misses events under editors that save by
// rename, across network mounts, and inside containers, each silently. A
// watcher may be added later to shorten the wait; it could never replace this.
//
// What each round COMPARES AGAINST is the other half of the argument, and it is
// not a baseline taken when watching began. A watch-time baseline cannot see an
// edit made between the cache being written and the watch starting: the cache
// would hold the old content while the watcher believed the new content was
// what it had always been, and the edit would be invisible for as long as the
// process lived. The comparison is therefore against the digest recorded IN the
// cache — the same key `catalog refresh` and boot-time hydration already use.
//
// The aggregator computes these digests ITSELF, from roots the membership
// declares. It never asks the member whether anything changed: that would mean
// contacting a contributor to answer a discovery question, and would make the
// cost of noticing scale with the number of members and depend on every one of
// them being alive.

// ContentWatchInterval is how often a served folder is re-fingerprinted.
//
// Small enough that an author's edit converges while they are still looking at
// it; large enough that the cost is irrelevant, since a digest is a stat walk
// over a content folder rather than a read of it.
const ContentWatchInterval = 2 * time.Second
