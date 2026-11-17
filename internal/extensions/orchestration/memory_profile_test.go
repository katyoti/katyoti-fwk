// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person/profiletest"
)

// placeholderProfiles is a universe with no profile set: Get gives John Doe.
type placeholderProfiles struct{ profileapi.ProfileService }

func (placeholderProfiles) Get(context.Context) (profileapi.Profile, error) {
	return profileapi.Placeholder(), nil
}

// A universe with no profile set refuses a keep and a delete: the profile
// service's placeholder (john-doe) is nobody, so no quarantined memory moves
// under its key and none is deleted.
func TestAUniverseWithNoProfileRefusesQuarantinedMemory(t *testing.T) {
	app := application.New(application.Config())
	application.PutService[profileapi.ProfileService](app, "default", placeholderProfiles{})
	ctx := application.WithApplication(context.Background(), app)

	if got := profileHandle(ctx); got != "" {
		t.Fatalf("the placeholder profile names a person: %q", got)
	}

	root := t.TempDir()
	svc := memoryService{
		root:    func() string { return root },
		place:   func() (string, string) { return "t1", "u1" },
		profile: profileHandle,
	}
	names := quarantineOf(t, root, map[string]string{memoryEntrypoint: "NOBODY-KEEPS"})
	in := QuarantinedMemoryNameInput{Name: names[0]}
	if r, err := svc.Assign(ctx, in); err != nil || r.OK || !strings.Contains(r.Message, "no profile set") {
		t.Errorf("a universe with no profile kept memory: %+v %v", r, err)
	}
	if r, err := svc.Delete(ctx, in); err != nil || r.OK || !strings.Contains(r.Message, "no profile set") {
		t.Errorf("a universe with no profile deleted memory: %+v %v", r, err)
	}
	if p := NewMemoryManager(root, MemoryKey{Tenant: "t1", Universe: "u1", Person: profileapi.Placeholder().ID}).LoadMemoryPrompt(); strings.Contains(p, "NOBODY-KEEPS") {
		t.Errorf("kept under the placeholder's key: %q", p)
	}
	if got, _ := QuarantinedMemory(root); len(got) != 1 {
		t.Fatalf("a refused act moved quarantined memory: %+v", got)
	}
}

// : ktw-agents in a universe holds no ProfileService of
// its own - ktw-univers serves it in the admin universe and writes the
// person's profile to Profile.yaml in the global config layer - so the
// universe's person is read from that file: a profile set there (as the GUI
// sets it, through the profile service's writer) names the person who keeps
// and deletes quarantined memory, never « no profile set ».
func TestAProcessWithNoProfileServiceReadsTheProfileTheAdminUniverseWrote(t *testing.T) {
	app := application.New(application.Config())
	ctx := application.WithApplication(context.Background(), app)
	dir := t.TempDir()

	if got := profileHandleIn(ctx, dir); got != "" {
		t.Fatalf("with no profile set, a person was named: %q", got)
	}
	profiletest.Set(t, dir, profileapi.UpdateInput{ID: "walk-person", FirstName: "Walk", LastName: "Person", Email: "walk.person@example.com"})
	if got := profileHandleIn(ctx, dir); got != "walk-person" {
		t.Fatalf("with no ProfileService in the process, the profile the admin universe wrote was not read: %q", got)
	}

	root := t.TempDir()
	svc := memoryService{root: func() string { return root }, place: func() (string, string) { return "t1", "u1" },
		profile: func(ctx context.Context) string { return profileHandleIn(ctx, dir) }}
	names := quarantineOf(t, root, map[string]string{memoryEntrypoint: "GONE"})
	if r, err := svc.Delete(ctx, QuarantinedMemoryNameInput{Name: names[0]}); err != nil || !r.OK {
		t.Fatalf("the universe's person could not delete quarantined memory: %+v %v", r, err)
	}
	if got, _ := QuarantinedMemory(root); len(got) != 0 {
		t.Fatalf("the deleted quarantined memory is still there: %+v", got)
	}
}
