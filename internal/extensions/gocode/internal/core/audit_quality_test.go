// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// testRunTools is a GoToolService whose go test returns the events it holds.
type testRunTools struct {
	dirTools
	run api.GoTestOutput
}

func (f *testRunTools) GoTest(context.Context, api.GoTestInput) (api.GoTestOutput, error) {
	return f.run, nil
}

func failingFinding(t *testing.T, run api.GoTestOutput) api.AuditFinding {
	t.Helper()
	tools := &testRunTools{dirTools: dirTools{dirs: map[string]string{}}, run: run}
	for _, f := range auditQuality(context.Background(), tools, checkScope{patterns: []string{"./..."}, dir: t.TempDir()}) {
		if f.FindingID == "quality-test-fail-1" {
			return f
		}
	}
	t.Fatalf("no quality-test-fail-1 finding for %+v", run)
	return api.AuditFinding{}
}

func testOut(pkg, test, line string) api.GoTestResult {
	return api.GoTestResult{Package: pkg, Test: test, Action: "output", Output: line + "\n"}
}

func testEv(pkg, test, action string) api.GoTestResult {
	return api.GoTestResult{Package: pkg, Test: test, Action: action}
}

// TestAQualityFindingNamesTheFailingTests: two failing tests in two packages
// are named in the finding, package and test, with the first lines they wrote.
func TestAQualityFindingNamesTheFailingTests(t *testing.T) {
	run := api.GoTestOutput{Passed: 1, Failed: 2, Results: []api.GoTestResult{
		testOut("example/a", "TestOne", "=== RUN   TestOne"),
		testOut("example/a", "TestOne", "    one_test.go:12: want 3, got 4"),
		testOut("example/a", "TestOne", "--- FAIL: TestOne (0.00s)"),
		testEv("example/a", "TestOne", "fail"),
		testEv("example/a", "TestFine", "pass"),
		testOut("example/a", "", "FAIL\texample/a\t0.1s"),
		testEv("example/a", "", "fail"),
		testOut("example/b", "TestTwo", "=== RUN   TestTwo"),
		testOut("example/b", "TestTwo", "    two_test.go:7: file not found"),
		testEv("example/b", "TestTwo", "fail"),
		testEv("example/b", "", "fail"),
	}}
	f := failingFinding(t, run)
	if f.Title != "2 test(s) failing" {
		t.Errorf("title = %q", f.Title)
	}
	for _, want := range []string{
		"passed=1 failed=2 skipped=0",
		"example/a TestOne", "one_test.go:12: want 3, got 4",
		"example/b TestTwo", "two_test.go:7: file not found",
	} {
		if !strings.Contains(f.Body, want) {
			t.Errorf("body misses %q:\n%s", want, f.Body)
		}
	}
	for _, framing := range []string{"=== RUN", "--- FAIL", "FAIL\texample/a"} {
		if strings.Contains(f.Body, framing) {
			t.Errorf("body keeps go test's framing %q:\n%s", framing, f.Body)
		}
	}
}

// TestAQualityFindingSaysTimeout: a package that hits go test's -timeout has
// no failing test event; the finding still names the package, the test that
// was running, and says it is a timeout.
func TestAQualityFindingSaysTimeout(t *testing.T) {
	run := api.GoTestOutput{Passed: 3, Results: []api.GoTestResult{
		testEv("example/slow", "TestQuick", "pass"),
		testOut("example/slow", "TestSlow", "=== RUN   TestSlow"),
		testOut("example/slow", "TestSlow", "panic: test timed out after 10m0s"),
		testOut("example/slow", "TestSlow", "\trunning tests:"),
		testOut("example/slow", "", "FAIL\texample/slow\t600.2s"),
		testEv("example/slow", "", "fail"),
	}}
	f := failingFinding(t, run)
	if f.Title != "1 package(s) failing (timeout)" {
		t.Errorf("title = %q", f.Title)
	}
	if want := "example/slow: timed out after 10m0s (running TestSlow)"; !strings.Contains(f.Body, want) {
		t.Errorf("body misses %q:\n%s", want, f.Body)
	}
}

// TestAQualityFindingNamesTheSubtestOnce: a failing subtest is named, not its
// parent that fails only because of it.
func TestAQualityFindingNamesTheSubtestOnce(t *testing.T) {
	run := api.GoTestOutput{Failed: 2, Results: []api.GoTestResult{
		testOut("example/a", "TestTable/case_2", "    table_test.go:20: bad"),
		testEv("example/a", "TestTable/case_2", "fail"),
		testEv("example/a", "TestTable", "fail"),
		testEv("example/a", "", "fail"),
	}}
	f := failingFinding(t, run)
	if !strings.Contains(f.Body, "example/a TestTable/case_2") {
		t.Errorf("body misses the subtest:\n%s", f.Body)
	}
	if strings.Contains(f.Body, "example/a TestTable\n") {
		t.Errorf("body names the parent too:\n%s", f.Body)
	}
}
