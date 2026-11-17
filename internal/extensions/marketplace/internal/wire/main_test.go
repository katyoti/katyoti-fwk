// SPDX-License-Identifier: Apache-2.0

package wire

import (
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/testworld"
)

// TestMain runs the package's tests in a world of their own, the person's
// installation guarded (testworld).
func TestMain(m *testing.M) { os.Exit(testworld.Run(m)) }
