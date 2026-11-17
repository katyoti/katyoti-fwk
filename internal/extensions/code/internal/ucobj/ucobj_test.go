// SPDX-License-Identifier: Apache-2.0

package ucobj

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// AC ids compare NUMERICALLY: AC10 after AC2, and a split child AC2.1
// directly after AC2. Lexicographic order put AC10 before AC2.
func TestCriteriaSortInReadingOrder(t *testing.T) {
	ordered := []string{"m.UC-1-AC1", "m.UC-1-AC2", "m.UC-1-AC2.1", "m.UC-1-AC3", "m.UC-1-AC10"}
	for i := 1; i < len(ordered); i++ {
		if !acIndexLess(ordered[i-1], ordered[i]) || acIndexLess(ordered[i], ordered[i-1]) {
			t.Errorf("%s should sort before %s", ordered[i-1], ordered[i])
		}
	}
}

// runWorld is a REAL CWM service on a temporary project carrying the shipped
// test-run type, with module `m` created: a run is recorded through the Object
// API exactly as in a running binary.
func runWorld(t *testing.T) (context.Context, cwmapi.CWMService, string) {
	t.Helper()
	root := t.TempDir()
	ctx, cwm := cwmapp.NewAt(t, root, cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":   {Data: []byte("name: module\ncapabilities: [container]\n")},
		"object-types/test-run.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/object-types/test-run.yaml")},
	}))
	cwmapp.Module(t, ctx, cwm, "m")
	return ctx, cwm, root
}

// A run is recorded as a NEW test-run object through CWM, never overwriting
// one; the latest by `ran_at` is what the use cases read back; two runs in one
// second are refused rather than one silently replacing the other.
func TestARunIsRecordedAndTheLatestIsRead(t *testing.T) {
	ctx, cwm, root := runWorld(t)
	first := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	if _, err := WriteRun(ctx, cwm, "m", Run{RanAt: first, LayerFilter: "L1", UCTotal: 1,
		Verdicts: map[string]gcapi.TestRunVerdict{"UC-001": {Verdict: "fail", ACs: map[string]gcapi.UCStatus{"UC-001-AC1": "fail"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteRun(ctx, cwm, "m", Run{RanAt: first, LayerFilter: "L1"}); !errors.Is(err, cwmapi.ErrAlreadyExists) {
		t.Fatalf("a second run in the same second: err = %v, want already-exists", err)
	}
	path, err := WriteRun(ctx, cwm, "m", Run{RanAt: first.Add(time.Hour), LayerFilter: "L1", UCTotal: 1,
		Verdicts: map[string]gcapi.TestRunVerdict{"UC-001": {Verdict: "pass", ACs: map[string]gcapi.UCStatus{"UC-001-AC1": "pass"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filepath.ToSlash(path), "test-run-20261001-100000"+objectFileSuffix) {
		t.Errorf("run path = %s", path)
	}
	// Through the service: the index knows both runs, and each reads back with
	// its verdicts.
	runs, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: "test-run"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs.Objects) != 2 {
		t.Fatalf("runs in the index = %d, want both kept", len(runs.Objects))
	}
	var objs []cwmapi.Object
	for _, r := range runs.Objects {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: r.ID})
		if err != nil {
			t.Fatal(err)
		}
		objs = append(objs, got.Object)
	}
	got, err := LatestVerdicts(objs)
	if err != nil {
		t.Fatal(err)
	}
	v := got["UC-001"]
	if v.Verdict != "pass" || v.ACs["UC-001-AC1"] != "pass" || !v.RanAt.Equal(first.Add(time.Hour)) {
		t.Fatalf("latest verdict = %+v", v)
	}
	// And from the files, as the projection reads them: the same answer.
	fromFiles, err := LoadVerdicts(filepath.Join(root, filepath.Dir(filepath.Dir(filepath.FromSlash(path)))), "m")
	if err != nil {
		t.Fatal(err)
	}
	if fromFiles["UC-001"].Verdict != "pass" {
		t.Fatalf("latest verdict from the files = %+v", fromFiles["UC-001"])
	}
}

// A run is held to its type's size budget (test-run: 32768 bytes) — the write
// goes through CWM, so an oversized record is refused, not written.
func TestAnOversizedRunIsRefusedByItsBudget(t *testing.T) {
	ctx, cwm, _ := runWorld(t)
	verdicts := map[string]gcapi.TestRunVerdict{}
	for i := 0; i < 400; i++ {
		id := fmt.Sprintf("UC-%03d", i)
		verdicts[id] = gcapi.TestRunVerdict{Verdict: "fail", Reason: strings.Repeat("x", 80),
			ACs: map[string]gcapi.UCStatus{id + "-AC1": "fail"}}
	}
	_, err := WriteRun(ctx, cwm, "m", Run{RanAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), LayerFilter: "L1",
		Verdicts: verdicts, UCTotal: len(verdicts)})
	var budget *cwmapi.SizeBudgetError
	if !errors.As(err, &budget) {
		t.Fatalf("an oversized run: err = %v, want a size-budget refusal", err)
	}
	runs, _ := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: "test-run"})
	if len(runs.Objects) != 0 {
		t.Fatalf("the refused run was written (%d runs)", len(runs.Objects))
	}
}

// No service, no record: there is no file-writing fallback.
func TestARunWithoutCWMIsRefused(t *testing.T) {
	if _, err := WriteRun(context.Background(), nil, "m", Run{RanAt: time.Now(), LayerFilter: "L1"}); err == nil {
		t.Fatal("a run was recorded without CWM")
	}
}
