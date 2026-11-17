// SPDX-License-Identifier: Apache-2.0

package universe_test

import (
	"context"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	manifestuniverse "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/universe"
)

func hashed(m *mbapi.Manifest, hash string) *mbapi.Manifest { m.Hash = hash; return m }

// G15, the consumer half: correctness does not depend on event delivery. The
// view was built from ktw-methods@h1; the aggregator moved to h2 and the
// contributions-changed event was DROPPED (never delivered here). One
// reconnect check rebuilds the view from the digest, not from an event.
func TestResync_RepairsAViewThatMissedAnEvent(t *testing.T) {
	ctx := context.Background()
	local := manifest("ktw-agents", "workspace")
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{
		local,
		hashed(manifest("ktw-methods", "old"), "sha256:h1"),
		hashed(manifest("ktw-code", "untouched"), "sha256:c1"),
	}, nil)

	digests := func(context.Context) (map[string]string, error) {
		return map[string]string{"ktw-methods": "sha256:h2", "ktw-code": "sha256:c1"}, nil
	}
	fetched := 0
	fetchAll := func(context.Context) ([]*mbapi.Manifest, error) {
		fetched++
		return []*mbapi.Manifest{
			hashed(manifest("ktw-methods", "fresh"), "sha256:h2"),
			hashed(manifest("ktw-code", "untouched"), "sha256:c1"),
		}, nil
	}

	rebuilt, err := manifestuniverse.ResyncView(ctx, digests, fetchAll, cat, []string{"ktw-methods", "ktw-code"}, local, nil)
	if err != nil || !rebuilt {
		t.Fatalf("ResyncView = (%v, %v), want a rebuild: the view held h1 while the aggregator says h2", rebuilt, err)
	}
	if fetched != 1 {
		t.Fatalf("index fetched %d times, want exactly once", fetched)
	}
	if got := idsFor(t, cat, "ktw-methods"); len(got) != 1 || got[0] != "fresh" {
		t.Errorf("ktw-methods = %v, want [fresh] after the resync", got)
	}
	if _, err := cat.Get(ctx, "ktw-agents"); err != nil {
		t.Errorf("the local build must survive a resync: %v", err)
	}

	// A second reconnect with nothing moved fetches nothing.
	rebuilt, err = manifestuniverse.ResyncView(ctx, digests, fetchAll, cat, []string{"ktw-methods", "ktw-code"}, local, nil)
	if err != nil || rebuilt || fetched != 1 {
		t.Fatalf("identical digests: rebuilt=%v err=%v fetched=%d, want no rebuild and no fetch", rebuilt, err, fetched)
	}
}

// A digest the aggregator cannot give is not a reason to wipe or rebuild: the
// live view stays and the error is reported.
func TestResync_DigestFailureKeepsTheLiveView(t *testing.T) {
	ctx := context.Background()
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{hashed(manifest("ktw-code", "kept"), "sha256:c1")}, nil)
	digests := func(context.Context) (map[string]string, error) { return nil, context.DeadlineExceeded }
	fetchAll := func(context.Context) ([]*mbapi.Manifest, error) {
		t.Fatal("fetchAll must not run when the digest failed")
		return nil, nil
	}

	rebuilt, err := manifestuniverse.ResyncView(ctx, digests, fetchAll, cat, []string{"ktw-code"}, nil, nil)
	if err == nil || rebuilt {
		t.Fatalf("ResyncView = (%v, %v), want the error and no rebuild", rebuilt, err)
	}
	if got := idsFor(t, cat, "ktw-code"); len(got) != 1 || got[0] != "kept" {
		t.Errorf("ktw-code = %v, want [kept]", got)
	}
}
