// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// hydrate_tie_break_test.go — the stat's one gap at hydration (blind audit
// M2, OD-2): a member whose binary carries a whole-second stamp and the same
// size as its record cannot be told fresh or stale by the stat, so the
// recorded provider digests break the tie on a probe. The probe that breaks
// it is the one saved when they moved — one probe, never two. A binary with
// a sub-second stamp is decided by the stat and never probed while fresh.

// digestProbe stands in for the manifest probe: the member publishes one
// immutable provider whose digest is the binary's CONTENT, so two builds of
// one size and one stamp answer two digests, as two builds would.
func digestProbe(t *testing.T) *atomic.Int32 {
	t.Helper()
	var probes atomic.Int32
	prev := probeManifest
	probeManifest = func(binary, _ string, _ bool) (*mbapi.Manifest, error) {
		probes.Add(1)
		name := strings.TrimSuffix(filepath.Base(binary), ".exe")
		content, err := os.ReadFile(binary)
		if err != nil {
			return nil, err
		}
		return &mbapi.Manifest{
			Provider: name, Version: "1", Hash: name + "-" + string(content),
			Providers: []mbapi.ProviderInfo{{Name: name, Type: "skill", Digest: "sha256:" + string(content)}},
		}, nil
	}
	t.Cleanup(func() { probeManifest = prev })
	return &probes
}

func TestHydrate_WholeSecondStampBreaksTheTieWithProviderDigests(t *testing.T) {
	probes := digestProbe(t)
	cache := manifestlocal.NewCache(t.TempDir())
	bin := t.TempDir()
	saveHubRecord(t, cache, bin)
	exe := filepath.Join(bin, "ktw-a.exe")
	if err := os.WriteFile(exe, []byte("build-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	whole := time.Date(2026, 9, 26, 12, 0, 0, 0, time.Local) // a FAT stamp, or a tarball's
	if err := os.Chtimes(exe, whole, whole); err != nil {
		t.Fatal(err)
	}
	sidecars := []appcore.SidecarSpec{{Name: "ktw-a", Binary: exe}}

	// Cold: one probe, the record is written.
	hydrateCache(cache, sidecars, io.Discard)
	if n := probes.Load(); n != 1 {
		t.Fatalf("cold hydrate: %d probes, want 1", n)
	}

	// Same binary, same whole-second stamp: the stat is undecided, the tie
	// is broken by a probe whose digests match — fresh, nothing rewritten.
	probes.Store(0)
	var errs strings.Builder
	hydrated, probed, _ := hydrateCache(cache, sidecars, &errs)
	if hydrated != 1 || probed != 0 {
		t.Fatalf("unchanged binary under an undecided stat: hydrated=%d probed=%d, want 1/0 (%s)", hydrated, probed, errs.String())
	}
	if n := probes.Load(); n != 1 {
		t.Fatalf("the tie-break must cost one probe, ran %d", n)
	}

	// Rebuilt: same size, same whole-second stamp, other bytes. The stat says
	// nothing; the digest moved; the record is rewritten from THAT probe.
	if err := os.WriteFile(exe, []byte("build-2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(exe, whole, whole); err != nil {
		t.Fatal(err)
	}
	probes.Store(0)
	errs.Reset()
	hydrated, probed, _ = hydrateCache(cache, sidecars, &errs)
	if hydrated != 0 || probed != 1 {
		t.Fatalf("same size, same second, other content: hydrated=%d probed=%d, want 0/1", hydrated, probed)
	}
	if n := probes.Load(); n != 1 {
		t.Fatalf("a moved digest must cost ONE probe (the tie-break's, then saved), ran %d", n)
	}
	if !strings.Contains(errs.String(), "stat undecided") || !strings.Contains(errs.String(), "sha256:build-1 → sha256:build-2") {
		t.Fatalf("the hub must say why it re-probed; said: %q", errs.String())
	}
	m, _, err := cache.Load("ktw-a", exe)
	if err != nil || m == nil || m.Hash != "ktw-a-build-2" {
		t.Fatalf("the record after the tie-break is not the new build: m=%v err=%v", m, err)
	}

	// A sub-second stamp is decided by the stat: a fresh record costs no
	// probe at all.
	fine := whole.Add(300 * time.Millisecond)
	if err := os.Chtimes(exe, fine, fine); err != nil {
		t.Fatal(err)
	}
	hydrateCache(cache, sidecars, io.Discard) // the stamp moved: re-probed once, recorded
	probes.Store(0)
	hydrated, probed, _ = hydrateCache(cache, sidecars, io.Discard)
	if hydrated != 1 || probed != 0 || probes.Load() != 0 {
		t.Fatalf("a decided stat must not probe: hydrated=%d probed=%d probes=%d", hydrated, probed, probes.Load())
	}
}
