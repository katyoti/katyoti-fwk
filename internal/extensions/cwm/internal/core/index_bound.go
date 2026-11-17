// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"time"
)

// What the in-memory index may cost, and the trigger past which it would no
// longer be enough. The bound is measured, not guessed: the heap
// one object keeps in the index, retained after a full scan of this
// repository's own modules, plus a margin. TestMeasureIndexOnRealCorpus (one
// project) and TestTheJoinedIndexOfTwoProjectsStaysInItsBound (two projects of
// one repository, the store set) fail above it. A change that makes an object
// cost more in the index raises the bound on purpose, with its new measure.
//
// The trigger is only logged: the index one hub serves passing 1 GB of heap or
// a 5 s full scan. The index is never persisted as a cache file; the answer to
// the trigger is the universe database becoming the index.
const (
	// IndexHeapPerObjectBound is the heap, in bytes, one object may keep in
	// the index once scanned: measured 40,970 B per object over 9,260
	// objects (four fifths of it the full-text postings, ftIndex), bound with
	// a margin of about two fifths.
	IndexHeapPerObjectBound = 56 * 1024

	// IndexHeapTrigger is the heap of the index one hub serves at which the
	// index stops being enough.
	IndexHeapTrigger = 1 << 30

	// IndexScanTrigger is the full-scan time at which the index stops being
	// enough.
	IndexScanTrigger = 5 * time.Second
)

// IndexTrigger reads an index's heap and full-scan time against the trigger:
// one line naming each and whether it is passed.
func IndexTrigger(heap uint64, scan time.Duration) string {
	state := func(passed bool) string {
		if passed {
			return "PASSED"
		}
		return "under"
	}
	return fmt.Sprintf("index heap %d MB of %d MB - %s; full scan %s of %s - %s",
		heap>>20, IndexHeapTrigger>>20, state(heap > IndexHeapTrigger),
		scan.Round(time.Millisecond), IndexScanTrigger, state(scan > IndexScanTrigger))
}
