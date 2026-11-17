// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"
)

// repinSettle is how long a universe's hub is given to reload a rewritten
// Univers.yaml (its watcher debounces 50ms) before a repinned member is
// restarted: the restart starts the spec the hub holds.
var repinSettle = time.Second

// restartRepinned restarts, in each universe whose pins moved (home → the
// members repinned), the repinned members that run, through the universe's
// hub, so they run the version they now pin.
// A universe that does not run is left alone: its members start on the new
// pin. A restart that fails is logged, never undoing the repin.
func (s *service) restartRepinned(ctx context.Context, byHome map[string][]string) {
	if len(byHome) == 0 {
		return
	}
	dial := s.dialHub
	if dial == nil {
		dial = dialUniverseHub
	}
	homes := make([]string, 0, len(byHome))
	for h := range byHome {
		homes = append(homes, h)
	}
	sort.Strings(homes)
	settled := false
	for _, home := range homes {
		hub, err := dial(ctx, home)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marketplace: %s: its hub could not be reached to restart %v: %v\n", home, byHome[home], err)
			continue
		}
		if hub == nil {
			continue
		}
		func() {
			defer hub.Close()
			running, err := hub.Running(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "marketplace: %s: its hub did not say which members run: %v\n", home, err)
				return
			}
			runs := map[string]bool{}
			for _, r := range running {
				runs[r] = true
			}
			for _, member := range byHome[home] {
				if !runs[member] || isSelf(home, member) {
					continue
				}
				if !settled {
					time.Sleep(repinSettle)
					settled = true
				}
				rctx, cancel := context.WithTimeout(ctx, restartWait)
				if _, err := hub.Restart(rctx, member); err != nil {
					fmt.Fprintf(os.Stderr, "marketplace: %s: restart %s on its new version: %v\n", home, member, err)
				}
				cancel()
			}
		}()
	}
}
