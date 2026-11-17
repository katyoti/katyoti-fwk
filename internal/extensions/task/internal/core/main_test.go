// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"testing"
)

// TestMain points the application's data folder at a temporary one: a test
// that builds the task service through the application (NewTaskService) keeps
// its runs, results and journals there — never in the user's data folder,
// never read back by another test.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ktw-test-data-")
	if err != nil {
		panic(err)
	}
	os.Setenv("KTW_DATA", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
