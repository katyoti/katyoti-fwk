// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The PATH a new process gets is the registry's, in order, then what only
// this process added: a program just installed (its folder before WindowsApps
// in the user's PATH) comes before the Store's python.exe stub this process
// saw first.
func TestFreshPath_AnInstalledProgramComesBeforeAnOlderEntry(t *testing.T) {
	sep := string(os.PathListSeparator)
	machine := filepath.FromSlash("/machine/system32")
	py := filepath.FromSlash("/user/Programs/Python312")
	apps := filepath.FromSlash("/user/WindowsApps")
	own := filepath.FromSlash("/process/only")
	current := strings.Join([]string{machine, apps, own}, sep)
	got := registryFirst([]string{machine, py + sep + apps}, current)
	if want := strings.Join([]string{machine, py, apps, own}, sep); got != want {
		t.Errorf("PATH\n got %s\nwant %s", got, want)
	}
	if got := registryFirst(nil, current); got != current {
		t.Errorf("no registry: the PATH should stay %s, got %s", current, got)
	}
}

// On the machine running the test, FreshPath keeps every entry of the
// current PATH (nothing this process added is lost).
func TestFreshPath_KeepsTheCurrentEntries(t *testing.T) {
	cur := os.Getenv("PATH")
	fresh := FreshPath(cur)
	for _, p := range filepath.SplitList(cur) {
		if strings.TrimSpace(p) != "" && !strings.Contains(strings.ToLower(fresh), strings.ToLower(filepath.Clean(strings.TrimSpace(p)))) && !strings.Contains(strings.ToLower(fresh), strings.ToLower(strings.TrimSpace(p))) {
			t.Errorf("FreshPath lost %s", p)
		}
	}
}
