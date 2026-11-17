// SPDX-License-Identifier: Apache-2.0

package atomicio

import (
	"os"
	"testing"
)

// TestMain clears NoSyncEnv: the test lanes set it for every run
// (scripts/test-env.sh), and this package's tests are the ones
// that prove the sync and the durability step themselves.
func TestMain(m *testing.M) {
	_ = os.Unsetenv(NoSyncEnv)
	os.Exit(m.Run())
}

// TestTheSyncPathRunsUnderTheLanes fails when this package's tests would run
// with the syncs skipped - the lanes' switch leaking in.
func TestTheSyncPathRunsUnderTheLanes(t *testing.T) {
	if noSync() {
		t.Fatalf("%s is in force in atomicio's own tests: the sync path they prove is skipped", NoSyncEnv)
	}
	f, err := os.CreateTemp(t.TempDir(), "sync")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syncTemp(f); err != nil {
		t.Fatalf("syncTemp: %v", err)
	}
}
