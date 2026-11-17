// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person"
)

// QUARANTINED MEMORY IN « À FAIRE »: each quarantined folder
// is one question to the person - keep it as my memory here, or delete it -
// contributed to the inbox's one list (inboxapi.WaitingSource), answered by
// the two acts of the `agent-memory` service (ktw-agents:agent-memory/<verb>).
// Keeping writes only to the answering person's key in this universe: the
// process knows no other universe's data root, and another person's key would
// cross the wall.

// MemoryWaitingSourceName is the agent memory's source in « À faire ».
const MemoryWaitingSourceName = "agent-memory"

// AgentMemoryServiceDef declares the agent memory's acts for KTSP
// (`ktw-agents:agent-memory/<verb>`).
var AgentMemoryServiceDef = meta.ServiceDef{
	Name:    "agent-memory",
	Short:   "The agent memory written before the wall: list it, keep it as your memory in this universe, or delete it",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Quarantined": {Short: "The quarantined folders of agent memory waiting on the person", ReadOnly: true},
		"Assign":      {Short: "Keep a quarantined folder as the calling person's memory in this universe"},
		"Delete":      {Short: "Delete a quarantined folder of agent memory"},
	},
}

// QuarantinedMemoryInput asks for the quarantined folders.
type QuarantinedMemoryInput struct{}

// QuarantinedMemoryOutput lists them.
type QuarantinedMemoryOutput struct {
	Folders []QuarantinedFolder `json:"folders"`
}

// QuarantinedMemoryNameInput names one quarantined folder. It names no
// person: keeping is for the person who calls.
type QuarantinedMemoryNameInput struct {
	Name string `json:"name" jsonschema:"description=The quarantined folder's name (from quarantined)"`
}

// AgentMemoryService is the agent memory's acts.
type AgentMemoryService interface {
	Quarantined(ctx context.Context, in QuarantinedMemoryInput) (QuarantinedMemoryOutput, error)
	Assign(ctx context.Context, in QuarantinedMemoryNameInput) (render.Result, error)
	Delete(ctx context.Context, in QuarantinedMemoryNameInput) (render.Result, error)
}

// memoryService serves them over this universe's memory root.
type memoryService struct {
	// root is the memory root; place this universe's tenant and universe;
	// profile the universe's person (a call with no cause is theirs).
	root    func() string
	place   func() (string, string)
	profile func(ctx context.Context) string
}

var _ AgentMemoryService = memoryService{}

// newMemoryService is the service over the application's environment.
func newMemoryService() memoryService {
	return memoryService{root: memoryRoot, place: memoryPlace, profile: profileHandle}
}

// memoryRoot is <TenantDataDir>/agent-memory, "" when the universe has none.
func memoryRoot() string {
	env := application.Env()
	if env == nil || env.TenantDataDir() == "" {
		return ""
	}
	return filepath.Join(env.TenantDataDir(), "agent-memory")
}

// memoryPlace is this universe's tenant and universe.
func memoryPlace() (string, string) {
	env := application.Env()
	if env == nil {
		return "", ""
	}
	return cenv.ResolveTenantUniverse(env.UniverseDir())
}

// profileHandle is the universe's person: the profile's handle, "" when no
// profile is set (the placeholder is nobody).
func profileHandle(ctx context.Context) string {
	return profileHandleIn(ctx, filepath.Join(application.Env().GlobalHomeDir(), "config"))
}

// profileHandleIn reads the person from the ProfileService this process holds,
// else from Profile.yaml in dir - the global config layer the admin
// universe's ProfileService (ktw-univers) writes, read here as this process's
// PermissionService reads it. ktw-agents in a universe holds no ProfileService
// of its own, and asking the admin hub from a universe's sidecar could reach
// another admin universe than the one the person's profile was set in, or
// spawn one; a profile that cannot be read is logged.
func profileHandleIn(ctx context.Context, dir string) string {
	if svc, err := application.UseService[profileapi.ProfileService](ctx); err == nil {
		p, err := svc.Get(ctx)
		if err != nil {
			slog.Warn("agent memory: the profile cannot be read", "err", err)
			return ""
		}
		return p.Handle()
	}
	h := person.HandleIn(dir)
	if h == "" {
		slog.Info("agent memory: no profile set", "dir", dir)
	}
	return h
}

// caller is the person a keep or a delete is for: the call's cause's person,
// or the person an agent's session relays; a call with no cause is the
// universe's person's own. The quarantined folders came from the host's own
// folders - the universe's person's - so only that person keeps or deletes
// them: another person of a shared universe (a cause or a relay) is refused,
// as is an agent acting for no person and a universe with no profile.
func (s memoryService) caller(ctx context.Context) (string, error) {
	owner := s.profile(ctx)
	if owner == "" {
		return "", errors.New("agent memory: no person to keep it for (no profile set)")
	}
	person := owner
	if c, ok := cause.CauseFrom(ctx); ok && c.Kind != "" {
		person = runPerson(ctx)
		if person == "" {
			return "", errors.New("agent memory: only a person keeps or deletes quarantined memory; this call acts for none")
		}
	}
	if person != owner {
		return "", fmt.Errorf("agent memory: only %s keeps or deletes the memory quarantined from their folders; %s cannot", owner, person)
	}
	return person, nil
}

func (s memoryService) Quarantined(context.Context, QuarantinedMemoryInput) (QuarantinedMemoryOutput, error) {
	got, err := QuarantinedMemory(s.root())
	if got == nil {
		got = []QuarantinedFolder{}
	}
	return QuarantinedMemoryOutput{Folders: got}, err
}

func (s memoryService) Assign(ctx context.Context, in QuarantinedMemoryNameInput) (render.Result, error) {
	person, err := s.caller(ctx)
	if err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	tenant, universe := s.place()
	if err := AssignQuarantinedMemory(s.root(), in.Name, MemoryKey{Tenant: tenant, Universe: universe, Person: person}); err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	return render.Result{OK: true, Message: fmt.Sprintf("Kept as %s's agent memory in this universe.", person)}, nil
}

func (s memoryService) Delete(ctx context.Context, in QuarantinedMemoryNameInput) (render.Result, error) {
	if _, err := s.caller(ctx); err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	if err := DeleteQuarantinedMemory(s.root(), in.Name); err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	return render.Result{OK: true, Message: "Quarantined agent memory deleted."}, nil
}

// MemoryWaitingSource is the agent memory's contribution to « À faire ».
func MemoryWaitingSource() inboxapi.WaitingSource {
	return memoryWaitingSource(memoryRoot)
}

func memoryWaitingSource(root func() string) inboxapi.WaitingSource {
	return inboxapi.WaitingSource{
		Name: MemoryWaitingSourceName,
		Items: func(context.Context) ([]inboxapi.WaitingItem, error) {
			return memoryWaitingItems(root())
		},
		Controls: memoryControls,
	}
}

// memoryWaitingItems are the quarantined folders, one question each.
func memoryWaitingItems(root string) ([]inboxapi.WaitingItem, error) {
	folders, err := QuarantinedMemory(root)
	if err != nil {
		return nil, err
	}
	out := make([]inboxapi.WaitingItem, 0, len(folders))
	for _, f := range folders {
		out = append(out, inboxapi.WaitingItem{
			Key:    MemoryWaitingSourceName + ":" + f.Name,
			Kind:   inboxapi.KindChoose,
			Title:  "Agent memory written before the wall: keep it or delete it",
			Asks:   fmt.Sprintf("From %s: %d file(s), %s (%s). Keep it as your agent memory in this universe, or delete it.", f.Source, len(f.Files), sizeOf(f.Bytes), strings.Join(f.Files, ", ")),
			Module: "agent", About: "agent", // the module: « Ouvrir » opens the agent module, where the memory wall is described
			Since: f.Since, Ref: f.Name, AnswerableBy: "person",
		})
	}
	return out, nil
}

// sizeOf is a size in words.
func sizeOf(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", b)
}

// Element ids of a card's two buttons (the GUI walk finds them by id).
const (
	memoryAssignButton = "agent-memory-assign-"
	memoryDeleteButton = "agent-memory-delete-"
	memoryAssignURI    = "ktw-agents:agent-memory/assign"
	memoryDeleteURI    = "ktw-agents:agent-memory/delete"
)

// memoryControls are a card's two buttons: keep as my memory here, delete
// (confirmed).
func memoryControls(_ context.Context, it inboxapi.WaitingItem, lang string) []render.Node {
	name := it.Ref
	if name == "" {
		return nil
	}
	keep, del, sure := "Keep as my memory here", "Delete", "Delete this agent memory for good?"
	if lang == "fr" {
		keep, del, sure = "Garder comme ma mémoire ici", "Supprimer", "Supprimer définitivement cette mémoire d'agent ?"
	}
	args := map[string]any{"name": name}
	assign := render.RunURI(keep, memoryAssignURI, args)
	remove := render.RunURI(del, memoryDeleteURI, args)
	remove.Confirm, remove.Dangerous = sure, true
	return []render.Node{{"kind": "hbox", "gap": 6, "children": []any{
		render.Node{"kind": "button", "id": memoryAssignButton + name, "label": keep, "icon": "check", "size": "xs", "variant": "light", "action": assign},
		render.Node{"kind": "button", "id": memoryDeleteButton + name, "label": del, "icon": "trash", "size": "xs", "variant": "light", "color": "red", "action": remove},
	}}}
}
