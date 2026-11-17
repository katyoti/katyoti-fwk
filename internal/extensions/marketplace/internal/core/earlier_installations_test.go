// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// through the hub: the installations of this machine
// are listed with their size and last use and why one cannot be deleted; one is
// deleted only with the token of the plan that says what goes; the installation
// answering never is.
func TestTheHubListsTheInstallationsAndDeletesOneOnlyOnceAcknowledged(t *testing.T) {
	dir := t.TempDir()
	earlierWorld(t, dir)
	old := filepath.Join(dir, "old")
	write(t, filepath.Join(old, "root", "notes.txt"), "something of the old installation")
	var removed []string
	was := controlplane.RemoveShortcuts
	controlplane.RemoveShortcuts = func(name string) { removed = append(removed, name) }
	t.Cleanup(func() { controlplane.RemoveShortcuts = was })
	ctx := context.Background()
	s := &service{}

	list, err := s.EarlierInstallations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]mktapi.EarlierInstallation{}
	for _, i := range list.Installations {
		byName[i.Name] = i
	}
	if o := byName["old"]; o.Current || o.Reason != "" || o.Bytes == 0 || o.LastUsed == "" || o.Title != "old" {
		t.Fatalf("old = %+v, want deletable with its size and last use", o)
	}
	if n := byName["new"]; !n.Current || n.Reason != "current" {
		t.Fatalf("new = %+v, want the current one, refused as current", n)
	}

	// The current one is refused, from the plan on.
	if _, err := s.DeleteInstallation(ctx, mktapi.DeleteInstallationInput{Name: "new"}); err == nil {
		t.Fatal("the installation answering got a deletion plan")
	}

	// The plan touches nothing; a wrong token deletes nothing.
	plan, err := s.DeleteInstallation(ctx, mktapi.DeleteInstallationInput{Name: "old"})
	if err != nil || plan.Deleted || plan.Token == "" || len(plan.Folders) == 0 {
		t.Fatalf("plan = %+v (%v), want folders and a token, nothing deleted", plan, err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("the plan touched the installation: %v", err)
	}
	if _, err := s.DeleteInstallation(ctx, mktapi.DeleteInstallationInput{Name: "old", Ack: "old-wrong"}); err == nil || !strings.Contains(err.Error(), "not acknowledged") {
		t.Fatalf("a wrong token = %v, want refused", err)
	}
	if _, err := os.Stat(old); err != nil || len(removed) != 0 {
		t.Fatalf("a wrong token deleted something (%v, shortcuts %v)", err, removed)
	}

	// With the plan's token: its folder, shortcuts and registry entry go.
	done, err := s.DeleteInstallation(ctx, mktapi.DeleteInstallationInput{Name: "old", Ack: plan.Token})
	if err != nil || !done.Deleted {
		t.Fatalf("delete with the token = %+v (%v)", done, err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("its folder is still there (%v)", err)
	}
	if len(removed) != 1 || removed[0] != "old" {
		t.Fatalf("shortcuts removed %v, want old's", removed)
	}
	after, _ := s.EarlierInstallations(ctx)
	if len(after.Installations) != 1 || after.Installations[0].Name != "new" {
		t.Fatalf("after: %+v, want only the current one", after.Installations)
	}
}
