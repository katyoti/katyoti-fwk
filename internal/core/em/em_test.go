// SPDX-License-Identifier: Apache-2.0

package em

import (
	"errors"
	"testing"
)

// --- Pure extension point registry tests ---

// Use distinct types so each test gets a unique PointID.
type testStrings struct{ V string }
type testInts struct{ V int }
type testOther struct{ V string }
type testOwnerA struct{ V string }
type testInfoA struct{ V string }
type testInfoB struct{ V int }
type testInfoAll struct{ V string }
type testInfoAll2 struct{ V int }
type testMismatch struct{ V string }

func TestRegisterPoint_And_Contribute(t *testing.T) {
	mgr := New()
	point := NewListPoint[testStrings]()

	if err := RegisterPoint(mgr, point); err != nil {
		t.Fatal(err)
	}
	if err := Contribute(mgr, testStrings{"hello"}); err != nil {
		t.Fatal(err)
	}
	if err := Contribute(mgr, testStrings{"world"}); err != nil {
		t.Fatal(err)
	}

	got, err := GetPoint[testStrings](mgr)
	if err != nil {
		t.Fatal(err)
	}
	entries := got.Entries()
	if len(entries) != 2 || entries[0].V != "hello" || entries[1].V != "world" {
		t.Fatalf("entries = %v, want [{hello} {world}]", entries)
	}
}

func TestContribute_PointNotFound(t *testing.T) {
	mgr := New()
	err := Contribute(mgr, testOther{"value"})
	if err == nil {
		t.Fatal("expected ErrPointNotFound")
	}
	var notFound ErrPointNotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("expected ErrPointNotFound, got %T: %v", err, err)
	}
}

func TestDuplicatePointID(t *testing.T) {
	mgr := New()
	p1 := NewListPoint[testStrings]()
	p2 := NewListPoint[testStrings]()

	if err := RegisterPoint(mgr, p1); err != nil {
		t.Fatal(err)
	}
	err := RegisterPoint(mgr, p2)
	if err == nil {
		t.Fatal("expected ErrDuplicatePointID")
	}
	var dupErr ErrDuplicatePointID
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected ErrDuplicatePointID, got %T: %v", err, err)
	}
}

func TestGetPoint_NotFound(t *testing.T) {
	mgr := New()
	_, err := GetPoint[testOther](mgr)
	if err == nil {
		t.Fatal("expected ErrPointNotFound")
	}
}

type testStrategy struct {
	Name string
}

func TestIndexedPoint_Integration(t *testing.T) {
	mgr := New()
	point := NewIndexedPoint[testStrategy](func(s testStrategy) string { return s.Name })

	if err := RegisterPoint(mgr, point); err != nil {
		t.Fatal(err)
	}
	if err := Contribute(mgr, testStrategy{Name: "mcp"}); err != nil {
		t.Fatal(err)
	}
	if err := Contribute(mgr, testStrategy{Name: "xskill"}); err != nil {
		t.Fatal(err)
	}

	raw, err := GetPoint[testStrategy](mgr)
	if err != nil {
		t.Fatal(err)
	}
	indexed := raw.(*IndexedPoint[testStrategy])
	if !indexed.Has("mcp") {
		t.Error("expected mcp strategy")
	}
	if !indexed.Has("xskill") {
		t.Error("expected xskill strategy")
	}
	if indexed.Has("unknown") {
		t.Error("unexpected unknown strategy")
	}
}

// --- SetCurrentExt + ownership tracking tests ---

func TestSetCurrentExt_Ownership(t *testing.T) {
	mgr := New()
	mgr.SetCurrentExt("ext-alpha")
	RegisterPoint(mgr, NewListPoint[testOwnerA]())
	Contribute(mgr, testOwnerA{"from-alpha"})

	mgr.SetCurrentExt("ext-beta")
	Contribute(mgr, testOwnerA{"from-beta"})
	mgr.SetCurrentExt("")

	info := mgr.PointInfo(PointID[testOwnerA]())
	if info == nil {
		t.Fatal("PointInfo = nil")
	}
	if info.Owner != "ext-alpha" {
		t.Fatalf("Owner = %q, want %q", info.Owner, "ext-alpha")
	}
	if len(info.Contributors) != 2 {
		t.Fatalf("Contributors = %v, want 2 entries", info.Contributors)
	}
	if info.Contributors[0] != "ext-alpha" || info.Contributors[1] != "ext-beta" {
		t.Fatalf("Contributors = %v", info.Contributors)
	}
}

func TestPointInfo(t *testing.T) {
	mgr := New()

	mgr.SetCurrentExt("ext-alpha")
	RegisterPoint(mgr, NewListPoint[testInfoA]())
	Contribute(mgr, testInfoA{"from-alpha"})

	mgr.SetCurrentExt("ext-beta")
	RegisterPoint(mgr, NewListPoint[testInfoB]())
	Contribute(mgr, testInfoA{"from-beta-1"})
	Contribute(mgr, testInfoA{"from-beta-2"})
	mgr.SetCurrentExt("")

	// testInfoA — owner, contributors
	info := mgr.PointInfo(PointID[testInfoA]())
	if info.ID != PointID[testInfoA]() {
		t.Fatalf("ID = %q", info.ID)
	}
	if info.Owner != "ext-alpha" {
		t.Fatalf("Owner = %q", info.Owner)
	}
	if len(info.Contributors) != 3 {
		t.Fatalf("Contributors = %v, want 3", info.Contributors)
	}

	// testInfoB — owner, no contributors
	info2 := mgr.PointInfo(PointID[testInfoB]())
	if info2.Owner != "ext-beta" {
		t.Fatalf("Owner = %q", info2.Owner)
	}
	if len(info2.Contributors) != 0 {
		t.Fatalf("Contributors = %v, want empty", info2.Contributors)
	}

	// Unknown returns nil.
	if mgr.PointInfo("unknown") != nil {
		t.Fatal("PointInfo(unknown) should be nil")
	}
}

func TestPointsInfo(t *testing.T) {
	mgr := New()

	mgr.SetCurrentExt("ext-all")
	RegisterPoint(mgr, NewListPoint[testInfoAll]())
	RegisterPoint(mgr, NewListPoint[testInfoAll2]())
	Contribute(mgr, testInfoAll{"item"})
	mgr.SetCurrentExt("")

	infos := mgr.PointsInfo()
	if len(infos) != 2 {
		t.Fatalf("PointsInfo() len = %d, want 2", len(infos))
	}

	byID := make(map[string]PointInfo)
	for _, pi := range infos {
		byID[pi.ID] = pi
	}

	a := byID[PointID[testInfoAll]()]
	if a.Owner != "ext-all" {
		t.Fatalf("Owner = %q", a.Owner)
	}
	if len(a.Contributors) != 1 || a.Contributors[0] != "ext-all" {
		t.Fatalf("Contributors = %v", a.Contributors)
	}

	b := byID[PointID[testInfoAll2]()]
	if b.Owner != "ext-all" {
		t.Fatalf("Owner = %q", b.Owner)
	}
}

func TestContribute_WrongType_IsPointNotFound(t *testing.T) {
	mgr := New()
	RegisterPoint(mgr, NewListPoint[testStrings]())

	// Contributing a different type results in PointNotFound (different PointID).
	err := Contribute(mgr, testInts{42})
	if err == nil {
		t.Fatal("expected error for unregistered type")
	}
	var notFound ErrPointNotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("expected ErrPointNotFound, got %T: %v", err, err)
	}
}

func TestNew_ReturnsEmptyEM(t *testing.T) {
	mgr := New()
	infos := mgr.PointsInfo()
	if len(infos) != 0 {
		t.Fatalf("new EM should have no points, got %d", len(infos))
	}
}

func TestPointID_Distinct(t *testing.T) {
	idStr := PointID[testStrings]()
	idInt := PointID[testInts]()
	if idStr == idInt {
		t.Fatalf("PointID should differ: %q vs %q", idStr, idInt)
	}
	// Same type yields same ID.
	if idStr != PointID[testStrings]() {
		t.Fatal("PointID should be stable")
	}
}
