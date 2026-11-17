// SPDX-License-Identifier: Apache-2.0

package projectcmd

import "testing"

// A test command that exits 0 having run nothing proves nothing: a go test
// whose -run matches no test, a package with no test files, a bun test that
// passed none. Anything else is judged by its exit code alone.
func TestATestThatRanNoTestFails(t *testing.T) {
	for _, c := range []struct {
		check, out string
		fails      bool
	}{
		{"go test ./pkg/ -run TestX", "ok  \tgithub.com/x/pkg\t0.512s [no tests to run]\n", true},
		{"go test ./...", "?   \tgithub.com/x/a\t[no test files]\nok  \tgithub.com/x/b\t0.2s [no tests to run]\n", true},
		{"go test ./pkg/ -run TestX", "", true},
		{"go test ./pkg/ -run TestX", "ok  \tgithub.com/x/pkg\t1.204s\n", false},
		{"go test ./... -run TestX", "ok  \tgithub.com/x/a\t0.1s [no tests to run]\nok  \tgithub.com/x/b\t0.9s\n", false},
		{"bun test x", "\n 0 pass\n 0 fail\n", true},
		{"bun test x", "\n 0 pass\nRan 0 tests\n", true},
		{"bun test x", "\n 3 pass\n 0 fail\n", false},
		{"cd app/ui && bun test x", "\n 0 pass\n 0 fail\n", true},
		{"cd app/ui && bun test x", "\n 3 pass\n 0 fail\n", false},
		{"go test ./pkg/ -run TestX 2>&1", "ok  \tgithub.com/x/pkg\t0.5s [no tests to run]\n", true},
		{"go test ./pkg/ -run '^TestX_(A|B)$' -v 2>&1 | grep -c -- '--- PASS: TestX_' | grep -qx 2", "", false},
		{"go test ./pkg/ -run TestX > out.txt", "", false},
		{"go version", "go version go1.26 windows/amd64\n", false},
		{"make lint", "", false},
		{"out=$(go test -v -run TestX ./pkg/ 2>&1); echo \"$out\" | grep -q -- '--- PASS: TestX ('", "", false},
		// Only a pipe or a redirection outside quotes hands the output to another reader.
		{"go test ./pkg/ -run '^(TestA|TestB)$'", "ok  \tgithub.com/x/pkg\t0.5s [no tests to run]\n", true},
		{"go test ./pkg/ -run \"^(TestA|TestB)$\" 2>&1", "ok  \tgithub.com/x/pkg\t0.5s [no tests to run]\n", true},
		{"go test ./pkg/ -run '^(TestA|TestB)$'", "ok  \tgithub.com/x/pkg\t0.5s\n", false},
		{"go test ./pkg/ -run 'Test>X'", "ok  \tgithub.com/x/pkg\t0.5s [no tests to run]\n", true},
		{"go test ./pkg/ -run '^(TestA|TestB)$' -v | grep -c PASS", "", false},
		{"go test ./pkg/ -run '^(TestA|TestB)$' > out.txt", "", false},
		{"go test ./pkg/ -run TestX 2>err.txt", "", false},
		{"go test ./pkg/ -run $(echo TestA | tr A B)", "", false},
		{"go test ./pkg/ -run Test\\|X", "ok  \tgithub.com/x/pkg\t0.5s [no tests to run]\n", true},
	} {
		if got := RanNoTest(c.check, c.out) != ""; got != c.fails {
			t.Errorf("%q with %q: fails %v; want %v", c.check, c.out, got, c.fails)
		}
	}
}
