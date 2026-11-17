// SPDX-License-Identifier: Apache-2.0

package local

import (
	"log/slog"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
)

// defaultStamp keeps the default logger's UNSTAMPED base, so a process that
// boots CWM many times (a test binary boots it per test) stamps every line
// with the universe and the project id ONCE: each boot stamps from the base,
// never from the default the previous boot already stamped.
var defaultStamp struct {
	mu      sync.Mutex
	base    *slog.Logger // the default before CWM stamped it
	stamped *slog.Logger // the default CWM last installed
}

// stampDefaultLogger installs, as slog's default, the base logger stamped with
// universe and projectID. When the default is still the one CWM installed, the
// kept base is reused; when someone else replaced it since, that new default
// becomes the base.
func stampDefaultLogger(universe, projectID string) {
	defaultStamp.mu.Lock()
	defer defaultStamp.mu.Unlock()
	cur := slog.Default()
	if defaultStamp.base == nil || cur != defaultStamp.stamped {
		defaultStamp.base = cur
	}
	defaultStamp.stamped = appcore.StampLogger(defaultStamp.base, universe, projectID)
	slog.SetDefault(defaultStamp.stamped)
}
