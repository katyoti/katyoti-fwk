// SPDX-License-Identifier: Apache-2.0

package file_write

import "testing"

func TestDefaultConfig_Defaults(t *testing.T) {
	c := DefaultConfig()
	if c.MaxBytes != 1<<20 {
		t.Errorf("MaxBytes = %d, want 1 MiB (1048576)", c.MaxBytes)
	}
}

func TestFileWriteConfig_LargerThanFileRead(t *testing.T) {
	// Sanity: the write cap should be larger than file_read's 256 KiB
	// read cap. Writes legitimately produce larger artifacts than
	// reads consume.
	c := DefaultConfig()
	if c.MaxBytes <= 262144 {
		t.Errorf("MaxBytes = %d, expected > 262144 (file_read's cap)", c.MaxBytes)
	}
}
