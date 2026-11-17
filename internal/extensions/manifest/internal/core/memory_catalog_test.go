// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

func TestMemoryCatalog_GetListNames(t *testing.T) {
	mA := &mbapi.Manifest{
		Provider:      "ktw-a",
		Contributions: []contribapi.Contribution{{Provider: "ktw-a", Type: "skill", ID: "s1"}},
	}
	mB := &mbapi.Manifest{
		Provider:      "ktw-b",
		Contributions: []contribapi.Contribution{{Provider: "ktw-b", Type: "task", ID: "t1"}},
	}
	cat := NewMemoryCatalog([]*mbapi.Manifest{mA, mB}, nil)

	ctx := context.Background()

	got, err := cat.Get(ctx, "ktw-a")
	if err != nil || got != mA {
		t.Errorf("Get(ktw-a) = %v, %v", got, err)
	}

	got, err = cat.Get(ctx, "missing")
	if !errors.Is(err, mbapi.ErrSidecarNotInCatalog) {
		t.Errorf("Get(missing) err = %v, want ErrSidecarNotInCatalog", err)
	}

	list, _ := cat.List(ctx)
	if len(list) != 2 {
		t.Errorf("List len = %d, want 2", len(list))
	}
	// Sorted by Provider
	if list[0].Provider != "ktw-a" || list[1].Provider != "ktw-b" {
		t.Errorf("List order = [%s %s], want [ktw-a ktw-b]", list[0].Provider, list[1].Provider)
	}

	names, _ := cat.Names(ctx)
	if len(names) != 2 || names[0] != "ktw-a" || names[1] != "ktw-b" {
		t.Errorf("Names = %v, want [ktw-a ktw-b]", names)
	}
}

func TestMemoryCatalog_ExtraMembersAppearInNames(t *testing.T) {
	mA := &mbapi.Manifest{Provider: "ktw-a"}
	cat := NewMemoryCatalog([]*mbapi.Manifest{mA}, []string{"ktw-b", "ktw-c"})

	names, _ := cat.Names(context.Background())
	if len(names) != 3 {
		t.Errorf("Names len = %d, want 3 (extras + loaded)", len(names))
	}
	// extras-first iteration order
	if names[0] != "ktw-b" || names[1] != "ktw-c" || names[2] != "ktw-a" {
		t.Errorf("Names order = %v, want [ktw-b ktw-c ktw-a] (extras-first)", names)
	}

	// extras don't accidentally satisfy Get
	_, err := cat.Get(context.Background(), "ktw-b")
	if !errors.Is(err, mbapi.ErrSidecarNotInCatalog) {
		t.Errorf("extra-only entry should error on Get; got %v", err)
	}
}

func TestMemoryCatalog_GetResources(t *testing.T) {
	cat := NewMemoryCatalog([]*mbapi.Manifest{
		{
			Provider: "ktw-x",
			Contributions: []contribapi.Contribution{
				{Provider: "ktw-x", Type: "skill", ID: "alpha"},
				{Provider: "ktw-x", Type: "task", ID: "beta"},
			},
		},
		{
			Provider: "ktw-y",
			Contributions: []contribapi.Contribution{
				{Provider: "ktw-y", Type: "skill", ID: "gamma"},
			},
		},
	}, nil)

	skills, _ := cat.GetContributionsByType(context.Background(), "skill")
	if len(skills) != 2 {
		t.Fatalf("got %d skill entries, want 2", len(skills))
	}
	// Sorted by (provider, id)
	if skills[0].Provider != "ktw-x" || skills[0].ID != "alpha" {
		t.Errorf("skills[0] = %+v, want ktw-x/alpha", skills[0])
	}
	if skills[1].Provider != "ktw-y" || skills[1].ID != "gamma" {
		t.Errorf("skills[1] = %+v, want ktw-y/gamma", skills[1])
	}

	tasks, _ := cat.GetContributionsByType(context.Background(), "task")
	if len(tasks) != 1 || tasks[0].ID != "beta" {
		t.Errorf("tasks = %+v", tasks)
	}

	unknown, _ := cat.GetContributionsByType(context.Background(), "no-such-type")
	if len(unknown) != 0 {
		t.Errorf("unknown type should return empty; got %+v", unknown)
	}
}

func TestMemoryCatalog_Replace(t *testing.T) {
	cat := NewMemoryCatalog([]*mbapi.Manifest{
		{Provider: "ktw-a"},
	}, nil)

	cat.Replace([]*mbapi.Manifest{
		{Provider: "ktw-z"},
	}, []string{"ktw-extra"})

	names, _ := cat.Names(context.Background())
	if len(names) != 2 {
		t.Errorf("Names len after Replace = %d, want 2", len(names))
	}
	// Old provider should be gone
	_, err := cat.Get(context.Background(), "ktw-a")
	if !errors.Is(err, mbapi.ErrSidecarNotInCatalog) {
		t.Errorf("ktw-a should be gone after Replace; got %v", err)
	}
}

func TestMemoryCatalog_NilManifestsSkipped(t *testing.T) {
	cat := NewMemoryCatalog([]*mbapi.Manifest{
		nil,
		{Provider: ""},
		{Provider: "ktw-real"},
	}, nil)

	names, _ := cat.Names(context.Background())
	if len(names) != 1 || names[0] != "ktw-real" {
		t.Errorf("Names = %v, want [ktw-real] (nil + empty-provider skipped)", names)
	}
}

func TestMemoryCatalog_Put_InsertNew(t *testing.T) {
	mA := &mbapi.Manifest{
		Provider:      "ktw-a",
		Contributions: []contribapi.Contribution{{Provider: "ktw-a", Type: "skill", ID: "s1"}},
	}
	cat := NewMemoryCatalog([]*mbapi.Manifest{mA}, nil)
	ctx := context.Background()

	mB := &mbapi.Manifest{
		Provider:      "ktw-b",
		Contributions: []contribapi.Contribution{{Provider: "ktw-b", Type: "task", ID: "t1"}},
	}
	cat.Put("ktw-b", mB)

	got, err := cat.Get(ctx, "ktw-b")
	if err != nil || got != mB {
		t.Fatalf("Get(ktw-b) after Put = %v, %v", got, err)
	}
	list, _ := cat.List(ctx)
	if len(list) != 2 {
		t.Errorf("List len = %d, want 2", len(list))
	}
	names, _ := cat.Names(ctx)
	if len(names) != 2 {
		t.Errorf("Names = %v, want 2 members", names)
	}
	tasks, _ := cat.GetContributionsByType(ctx, "task")
	if len(tasks) != 1 || tasks[0].ID != "t1" {
		t.Errorf("GetContributionsByType(task) = %v, want [t1]", tasks)
	}
}

func TestMemoryCatalog_Put_ReplaceExisting(t *testing.T) {
	mA1 := &mbapi.Manifest{
		Provider:      "ktw-a",
		Contributions: []contribapi.Contribution{{Provider: "ktw-a", Type: "skill", ID: "old"}},
	}
	cat := NewMemoryCatalog([]*mbapi.Manifest{mA1}, nil)
	ctx := context.Background()

	mA2 := &mbapi.Manifest{
		Provider:      "ktw-a",
		Contributions: []contribapi.Contribution{{Provider: "ktw-a", Type: "skill", ID: "new"}},
	}
	cat.Put("ktw-a", mA2)

	skills, _ := cat.GetContributionsByType(ctx, "skill")
	if len(skills) != 1 || skills[0].ID != "new" {
		t.Errorf("after Put-replace, skills = %v, want [new]", skills)
	}
	// Membership must NOT duplicate.
	names, _ := cat.Names(ctx)
	if len(names) != 1 || names[0] != "ktw-a" {
		t.Errorf("Names = %v, want [ktw-a] (no duplicate)", names)
	}
}

func TestMemoryCatalog_Put_DeclaredMemberNoDuplicate(t *testing.T) {
	// ktw-b is a declared member with no cached manifest yet.
	mA := &mbapi.Manifest{Provider: "ktw-a"}
	cat := NewMemoryCatalog([]*mbapi.Manifest{mA}, []string{"ktw-b"})
	ctx := context.Background()

	cat.Put("ktw-b", &mbapi.Manifest{Provider: "ktw-b"})

	names, _ := cat.Names(ctx)
	count := 0
	for _, n := range names {
		if n == "ktw-b" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("ktw-b appears %d times in Names %v, want exactly 1", count, names)
	}
}

func TestMemoryCatalog_Put_NilAndEmptyIgnored(t *testing.T) {
	cat := NewMemoryCatalog([]*mbapi.Manifest{{Provider: "ktw-a"}}, nil)
	cat.Put("", &mbapi.Manifest{Provider: "x"})
	cat.Put("ktw-b", nil)
	names, _ := cat.Names(context.Background())
	if len(names) != 1 || names[0] != "ktw-a" {
		t.Errorf("Names = %v, want [ktw-a] (empty/nil Put ignored)", names)
	}
}

func TestMemoryCatalog_Put_ConcurrentSafe(t *testing.T) {
	cat := NewMemoryCatalog(nil, nil)
	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			cat.Put("ktw-a", &mbapi.Manifest{Provider: "ktw-a"})
		}
		close(done)
	}()
	for i := 0; i < 200; i++ {
		_, _ = cat.List(ctx)
		_, _ = cat.Names(ctx)
	}
	<-done
}

func TestRefreshHook_Matches(t *testing.T) {
	cases := []struct {
		name    string
		hook    []string
		changed []string
		want    bool
	}{
		{"empty hook fires", nil, []string{"skill"}, true},
		{"empty changed fires", []string{"skill"}, nil, true},
		{"hook wildcard fires", []string{"*"}, []string{"task"}, true},
		{"changed wildcard fires", []string{"skill"}, []string{"*"}, true},
		{"intersection fires", []string{"node-type", "skill"}, []string{"skill"}, true},
		{"disjoint does not fire", []string{"node-type"}, []string{"skill"}, false},
	}
	for _, c := range cases {
		h := mbapi.RefreshHook{Kinds: c.hook}
		if got := h.Matches(c.changed); got != c.want {
			t.Errorf("%s: Matches(%v) with hook %v = %v, want %v", c.name, c.changed, c.hook, got, c.want)
		}
	}
}

// TestMemoryCatalog_SelfHeal_IndexRefreshDrivesBody proves the content-cache
// "self-heal on hash mismatch": there is no active mismatch-detection — because
// the body store is content-addressed by the index's BodyHash, refreshing the
// index (so the entry's BodyHash changes H1→H2) makes the next GetBody miss on
// H2 and fetch the new body. The stale H1 blob is never re-served.
func TestMemoryCatalog_SelfHeal_IndexRefreshDrivesBody(t *testing.T) {
	bodyA := []byte("version A")
	bodyB := []byte("version B")
	hA := HashBody(bodyA)
	hB := HashBody(bodyB)

	const uri = "ktw-x:/skill/s1"
	cat := NewMemoryCatalog([]*mbapi.Manifest{{
		Provider:      "ktw-x",
		Contributions: []contribapi.Contribution{{Provider: "ktw-x", Type: "skill", ID: "s1", Layers: contribapi.DataDescriptor(hA, 0)}},
	}}, nil)

	store := NewContentStore(t.TempDir())
	fetched := map[string]int{}
	fetch := func(_ context.Context, hash, _ string) ([]byte, error) {
		fetched[hash]++
		switch hash {
		case hA:
			return bodyA, nil
		case hB:
			return bodyB, nil
		}
		return nil, fmt.Errorf("unexpected hash %q", hash)
	}
	cat.SetContentLane(store, fetch)
	ctx := context.Background()

	got, err := cat.GetBody(ctx, uri)
	if err != nil || string(got) != "version A" {
		t.Fatalf("first GetBody = %q, %v; want version A", got, err)
	}

	// Index refresh: the SAME URI now addresses bodyB.
	cat.Put("ktw-x", &mbapi.Manifest{
		Provider:      "ktw-x",
		Contributions: []contribapi.Contribution{{Provider: "ktw-x", Type: "skill", ID: "s1", Layers: contribapi.DataDescriptor(hB, 0)}},
	})

	got, err = cat.GetBody(ctx, uri)
	if err != nil || string(got) != "version B" {
		t.Fatalf("self-heal GetBody = %q, %v; want version B", got, err)
	}
	if fetched[hB] != 1 {
		t.Errorf("hB fetched %d times, want exactly 1 (then content-store cached)", fetched[hB])
	}
	// A repeat read of the refreshed entry is a content-store hit (no new fetch).
	if _, err := cat.GetBody(ctx, uri); err != nil {
		t.Fatalf("repeat GetBody err: %v", err)
	}
	if fetched[hB] != 1 {
		t.Errorf("hB fetched %d times after repeat read, want still 1 (cached)", fetched[hB])
	}
}
