// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// WorkspaceEventHandler receives every CWM workspace-scoped object event
// arriving on a universe's bus. The App layer forwards these to the frontend
// as Wails events so panels consuming CWM data (workspace tree, declared
// panels) can refresh when ANY process — CLI, agent run, a second GUI window,
// this one — mutates the workspace.
type WorkspaceEventHandler func(WorkspaceEvent)

// WorkspaceEvent is one CWM object event as the GUI forwards it: where it
// happened and WHICH object — its id, module, type and parent, read from the
// event's payload — so a panel decides whether it shows that object (its
// declared refresh scope) without re-reading anything. A move also carries
// where the object was: it left that module or parent. An event raised on an
// object (action "event": a source's source.sync) also carries its type's name
// and its payload, so a screen following that event (a progress list's
// per-source lines) reads it without asking anything.
type WorkspaceEvent struct {
	Universe   string `json:"universe"`
	Subject    string `json:"subject"`
	Object     string `json:"object,omitempty"`
	Module     string `json:"module,omitempty"`
	Type       string `json:"type,omitempty"`
	Parent     string `json:"parent,omitempty"`
	Action     string `json:"action,omitempty"     jsonschema:"The subject's last token: created | updated | deleted | moved | split | transitioned | ..."`
	FromModule string `json:"fromModule,omitempty" jsonschema:"A moved object's module before the move"`
	FromParent string `json:"fromParent,omitempty" jsonschema:"A moved object's parent before the move"`
	Event      string `json:"event,omitempty"      jsonschema:"A raised event's type name (source.sync); action is event"`
	Payload    string `json:"payload,omitempty"    jsonschema:"A raised event's payload as compact JSON - as CWM publishes it: cut at its note bound"`
}

// WorkspaceEventOf reads one bus message of the workspace family: the action
// is the subject's last token, the object from the payload's common keys (every
// CWM object event payload names object_id and module; most its type and
// parent; a raised event its type name and payload). An undecodable payload
// still yields the universe, subject and action — a panel scoped to the
// universe re-reads on it.
func WorkspaceEventOf(universe, subject string, data []byte) WorkspaceEvent {
	ev := WorkspaceEvent{Universe: universe, Subject: subject}
	if i := strings.LastIndexByte(subject, '.'); i >= 0 {
		ev.Action = subject[i+1:]
	}
	var p struct {
		ObjectID   string `json:"object_id"`
		Module     string `json:"module"`
		Type       string `json:"type"`
		Parent     string `json:"parent"`
		FromModule string `json:"from_module"`
		FromParent string `json:"from_parent"`
		Event      string `json:"event"`
		Summary    string `json:"summary"`
	}
	if len(data) > 0 && json.Unmarshal(data, &p) == nil {
		ev.Object, ev.Module, ev.Type, ev.Parent = p.ObjectID, p.Module, p.Type, p.Parent
		ev.FromModule, ev.FromParent = p.FromModule, p.FromParent
		ev.Event, ev.Payload = p.Event, p.Summary
	}
	return ev
}

// SetWorkspaceEventHandler installs the handler invoked for every
// workspace event on every active universe. Call once at App startup,
// BEFORE the first Activate — cores activated earlier won't have the
// watch installed retroactively.
func (m *Master) SetWorkspaceEventHandler(h WorkspaceEventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wsHandler = h
}

// installWorkspaceWatch subscribes the core's bus to the universe's
// workspace-scoped subject family:
//
//	ktw.<tenant>.universe.<name>.workspace.>
//
// One wildcard covers node + item + edge lifecycle subjects (see
// cwm/api/subjects.go builders). Plain Subscribe (not QueueSubscribe):
// every GUI instance watching the same universe receives every event
// independently, same co-work semantics as the terminalRouter.
//
// Subscription failure is non-fatal — CRUD still works through the
// revision-bump refresh; only external-write liveness is lost. Logged
// loudly so it's visible in dev runs.
func (m *Master) installWorkspaceWatch(core *ActiveCore) {
	pattern := fmt.Sprintf("ktw.%s.universe.%s.workspace.>", activeTenantID, core.Name)
	sub, err := core.MessageBus().Subscribe(context.Background(), pattern,
		func(_ context.Context, msg mbapi.Message) {
			m.mu.RLock()
			h := m.wsHandler
			m.mu.RUnlock()
			if h != nil {
				h(WorkspaceEventOf(core.Name, msg.Subject(), msg.Data()))
			}
		})
	if err != nil {
		log.Printf("[supervisor] workspace watch subscribe failed for %q (%s): %v — external-write live refresh disabled",
			core.Name, pattern, err)
		return
	}
	core.mu.Lock()
	core.wsSub = sub
	core.mu.Unlock()
}
