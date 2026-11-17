// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The GUI's per-view loaders went onto the host's one load (
// ): a view's part (a source panel, a resolved page tree, the
// request list, the Global panels, the task list, the objects tree) loads
// through useHostLoad / loadPart - one gate per universe shared with the page
// sections, the cold-start retry in one place, the error card with
// « Réessayer » - never its own cancelled-flag loader. They do not come back.
var (
	retryUntilFilled = regexp.MustCompile(`useRetryUntilFilled`)
	cancelledFlag    = regexp.MustCompile(`let cancelled\b`)
	bareDemandsRead  = regexp.MustCompile(`WorkspaceRunAction(As)?\([^)]*['"]demands['"]|WorkspaceRunAction(As)?\([^)]*SERVICE \+ 'demands'`)
)

func TestThePerViewLoadersStayGone(t *testing.T) {
	root := repoRoot(t)
	src := filepath.Join(root, "app", "ktw-gui", "frontend", "src")
	checked := 0
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !(strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".tsx")) {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		checked++
		rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
		if retryUntilFilled.Match(raw) || strings.HasSuffix(rel, "/hooks/useRetryUntilFilled.ts") {
			t.Errorf("%s names useRetryUntilFilled - the cold-start retry lives in the host's one load (useHostLoad)", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 200 {
		t.Fatalf("checked %d GUI files: the root is wrong", checked)
	}
	read := func(rel string) string {
		raw, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	for _, rel := range []string{"sections/workspace/RenderDefView.tsx", "sections/home/GlobalPanels.tsx"} {
		if m := cancelledFlag.FindString(read(rel)); m != "" {
			t.Errorf("%s holds a cancelled-flag loader (%q) - its parts load through useHostLoad", rel, m)
		}
	}
	requests := read("sections/workspace/RequestsView.tsx")
	if m := bareDemandsRead.FindString(requests); m != "" {
		t.Errorf("RequestsView reads planification's demands by a bare call (%q) - it reads them through the host's load (useHostLoad)", m)
	}
	if !strings.Contains(requests, "useHostLoad") {
		t.Error("RequestsView does not read its requests through the host's load (useHostLoad)")
	}
}
