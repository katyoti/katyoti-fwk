// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A VIEW THE DELIVERY VIEW REPLACED DOES NOT COME BACK (
// F-072) - the backlog panel and the portfolio-planning views
// the Delivery view and the stream, release and increment panes replaced are
// deleted; their sources and builders are named here, and no Go file, panel
// declaration or GUI source may carry one again.
var retiredViews = []string{
	"ktw-agents:increments/backlog-page", // the backlog panel: each stream's lane carries its backlog
	"BacklogPage(",
	// portfolio planning's Streams, Streams and releases, Increments and What
	// increments hold tabs, and the stream's three portfolio panels: the
	// Delivery view's lanes and the stream, release and increment panes.
	"ktw-agents:portfolio/stream-delivery",
	"ktw-agents:portfolio/stream-increments",
	"ktw-agents:portfolio/stream-releases",
	"ktw-agents:portfolio/create-increment",
	"ktw-agents:portfolio/move-increment",
	"func incrementDetail(",
	"func incrementBoard(",
	"func streamsTree(",
	"func streamBoard(",
	// IN-062's display switch and the screens it kept: its
	// screens are the only ones - Open increments is one table, the Planned
	// delivery panel shows open requests, an increment's pane opens on them.
	"KTW_IN062_DISPLAY",
	"func in062On(",
	"In062Function",
	"in062-function",
	`when: "in062()"`,
	"func openIncrementsTab(",
	"func openCardOf(",
	"func panelActions(",
	"tab:increment-requests",
	// IN-035's display switch: the contribution
	// system review's screens are the only ones.
	"KTW_IN035_DISPLAY",
}

func TestRetiredDeliveryViewsDoNotComeBack(t *testing.T) {
	root := repoRoot(t)
	self := filepath.Join(root, "internal", "test", "arch", "retired_views_test.go")
	for _, dir := range []string{"internal", "app"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "dist", "testdata", ".git":
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".yaml", ".ts", ".tsx":
			default:
				return nil
			}
			if path == self {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, r := range retiredViews {
				if strings.Contains(string(b), r) {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s carries %q, a view the Delivery view replaced", rel, r)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
