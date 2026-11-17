// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// A member that declares read roots is probed once, then its record is exact:
// the roots and their fingerprint, taken from the earlier record BEFORE the
// probe (ReadsBeforeProbe), are what the new record keeps — so the next check
// finds nothing moved and spares a confirming probe. Without them the record
// says its roots were learnt by this probe (readsUnknown), and the watch
// re-probes.
func TestReadsBeforeProbe_AKnownRecordSparesTheConfirmingProbe(t *testing.T) {
	universe, global := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(universe, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(universe, "skills", "greet.md"), []byte("---\nname: greet\n---\nhi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := NewCache(t.TempDir()).ForUniverse(universe, global)
	exe := scopeBinary(t)
	m := makeManifest("ktw-x", "1", "sha256:x")
	m.ReadRoots = []mbapi.ReadRoot{{Anchor: mbapi.ReadRootUniverse, Path: "skills"}}

	// The first probe of the binary learns its roots: the record cannot vouch
	// for them, and the watch re-probes once.
	if err := cache.Save("ktw-x", m, exe, SourceDigests{}); err != nil {
		t.Fatal(err)
	}
	if moved, _ := cache.ReadsMoved("ktw-x", exe); !moved {
		t.Fatal("a record whose roots were learnt by its own probe must read as moved once")
	}

	// That confirming probe takes the roots and their fingerprint from the
	// record BEFORE probing: its record is exact, nothing moved since.
	digests := SourceDigests{}
	digests.ReadRoots, digests.Reads = cache.ReadsBeforeProbe("ktw-x", exe)
	if len(digests.ReadRoots) != 1 || digests.Reads == "" {
		t.Fatalf("ReadsBeforeProbe = %v, %q; want the record's root and its fingerprint", digests.ReadRoots, digests.Reads)
	}
	if err := cache.Save("ktw-x", m, exe, digests); err != nil {
		t.Fatal(err)
	}
	if moved, why := cache.ReadsMoved("ktw-x", exe); moved {
		t.Fatalf("a record taken with the earlier fingerprint reads as moved (%s): one probe too many", why)
	}

	// And an edit of a read folder is seen.
	if err := os.WriteFile(filepath.Join(universe, "skills", "bye.md"), []byte("---\nname: bye\n---\nbye\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if moved, _ := cache.ReadsMoved("ktw-x", exe); !moved {
		t.Fatal("a skill added under a read root must read as moved")
	}
}
