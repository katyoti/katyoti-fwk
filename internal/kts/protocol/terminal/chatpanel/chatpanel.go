// SPDX-License-Identifier: Apache-2.0

// Package chatpanel makes a run's conversation its display panel of type
// `chat`: the agent's UI tools (display-ui-component
// patch-ui-component, complete-ui-component, ask-user-data) keep their names
// and their events, and the bridge turns each of those events into panel
// events (internal/core/panel) - the one answer and the one op set of the
// page DSL, by node id. The run's text messages - the agent's and the
// person's - are nodes of the panel too: the conversation
// node holds one node per message and per UI artifact, in the order they
// came, and a message the person sends on the panel's input is the run's
// next user turn (MessageOf).
//
// The bridge replace (the person's decision): each UI artifact keeps its
// value here; each JSON-Pointer patch the agent sends is applied to that
// value as the GUI's artifact store applies it, and becomes one panel
// `replace` op on the artifact's node - so no pointer crosses into the panel
// op set. A mount replaces the artifacts container with every artifact in
// mount order (the op set adds a node only through its parent), a
// completion replaces the artifact's node with its status complete.
//
// The bridge is plain state, not safe for concurrent use: its owner (the
// session's translator) holds it under its own lock.
package chatpanel

import (
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
)

// Type is the panel type a run's conversation is opened with.
const Type = "ktw-agents:/panel-type/chat"

// RootNode is the chat panel's root node; ArtifactsNode the conversation:
// the node holding one node per text message and per UI artifact, in the
// order they came (panel.ChatArtifactsNode).
const (
	RootNode      = "chat"
	ArtifactsNode = "chat:artifacts"
)

// Status of an artifact: streaming until it is completed.
const (
	StatusStreaming = "streaming"
	StatusComplete  = "complete"
)

// NodeID is the id of an artifact's node in the chat panel.
func NodeID(artifactID string) string { return "artifact:" + artifactID }

// MessageNodeID is the id of a text message's node in the chat panel.
func MessageNodeID(messageID string) string { return "message:" + messageID }

// PanelNodeID is the id of a display panel's node placed in the chat panel.
func PanelNodeID(panelID string) string { return "panel:" + panelID }

// Roles of a text message.
const (
	RoleAssistant = "assistant"
	RoleUser      = "user"
)

// Message is one text message of the conversation: who said it and what.
type Message struct {
	ID   string
	Role string
	Text string
}

// Node is the message's node in the chat panel: a text node carrying its
// role.
func (m *Message) Node() render.Node {
	return render.Node{"kind": "text", "id": MessageNodeID(m.ID), "content": m.Text, "role": m.Role}
}

// entry is one item of the conversation, in its order: a message, an
// artifact or a display panel placed in it.
type entry struct {
	message bool
	panel   bool
	id      string
}

// Placed is a display panel placed in the conversation.
type Placed struct {
	ID    string
	Title string
}

// Node is the placed panel's node: the page DSL's `panel` node, drawn bare -
// its page alone, where it came in the conversation (a question's card).
func (p *Placed) Node() render.Node {
	return render.PanelNode(p.ID, p.Title).With("id", PanelNodeID(p.ID)).With("bare", true)
}

// Artifact is one UI artifact as the chat shows it: what its mount said,
// with every patch applied since. Value is a schema-bound component's data
// (a deck's markdown), DSLTree a page's tree and Data its outer data.
type Artifact struct {
	ID        string         `json:"artifactId"`
	Kind      string         `json:"kind"`
	MediaType string         `json:"mediaType,omitempty"`
	Title     string         `json:"title,omitempty"`
	Schema    any            `json:"schema,omitempty"`
	Variant   string         `json:"variant,omitempty"`
	UI        map[string]any `json:"ui,omitempty"`
	Value     any            `json:"value,omitempty"`
	DSLTree   any            `json:"dslTree,omitempty"`
	Data      any            `json:"data,omitempty"`
	Mode      string         `json:"mode,omitempty"`
	Actions   any            `json:"actions,omitempty"`
	Status    string         `json:"status"`
}

// Node is the artifact's node in the chat panel: a card with the artifact's
// id and title, carrying the artifact whole under `artifact`.
func (a *Artifact) Node() render.Node {
	n := render.Node{"kind": "card", "id": NodeID(a.ID)}
	if a.Title != "" {
		n["title"] = a.Title
	}
	n["artifact"] = toJSON(a)
	return n
}

// Bridge is one run's chat panel: its track and the artifacts it shows.
type Bridge struct {
	track  panel.Track
	title  string
	owner  string
	order  []entry
	arts   map[string]*Artifact
	msgs   map[string]*Message
	placed map[string]*Placed
	Errors []error // the last call's ops that could not apply
}

// New is the chat panel id (a fresh one when empty) of a run's
// conversation, titled title and owned by owner (the run's id); it opens on
// the first artifact.
func New(id, title, owner string) *Bridge {
	return &Bridge{track: panel.NewTrack(id), title: title, owner: owner, arts: map[string]*Artifact{}, msgs: map[string]*Message{}, placed: map[string]*Placed{}}
}

// Own names the run owning the panel when it was made without one; the
// open carries it.
func (b *Bridge) Own(owner string) {
	if b.owner == "" {
		b.owner = owner
	}
}

// ID is the chat panel's id.
func (b *Bridge) ID() string { return b.track.ID }

// Artifact is the artifact id as the chat shows it now, nil when unknown.
func (b *Bridge) Artifact(id string) *Artifact { return b.arts[id] }

// Mount adds the artifact the event mounts and answers the panel events
// that show it: the panel's open first when it is not open yet, then the
// replace of the artifacts node. A mount of an artifact already shown
// answers nothing (the stream plugin may fire it again).
func (b *Bridge) Mount(e agentapi.UIArtifactMountEvent) ([]panel.Event, error) {
	b.Errors = nil
	if e.ArtifactID == "" {
		return nil, fmt.Errorf("chat panel: a mount names its artifact")
	}
	if _, ok := b.arts[e.ArtifactID]; ok {
		return nil, nil
	}
	a := &Artifact{
		ID: e.ArtifactID, Kind: e.Kind, MediaType: render.MediaTypeForKind(e.Kind), Title: e.Title,
		Schema: toJSON(e.Schema), Variant: e.Variant, UI: asMap(toJSON(e.UI)), Value: toJSON(e.Value),
		DSLTree: toJSON(e.DSLTree), Data: toJSON(e.Data), Mode: e.Mode, Actions: toJSON(e.Actions),
		Status: StatusStreaming,
	}
	out, err := b.open()
	if err != nil {
		return nil, err
	}
	b.arts[a.ID] = a
	b.order = append(b.order, entry{id: a.ID})
	ev, err := b.conversation()
	if err != nil {
		delete(b.arts, a.ID)
		b.order = b.order[:len(b.order)-1]
		return out, err
	}
	return append(out, ev), nil
}

// Message adds a text message of the conversation - the agent's or the
// person's - after what it shows, and answers the panel events that show
// it: the panel's open first when it is not open yet, then the replace of
// the conversation node. A message already shown with the same text
// answers nothing; with another text, its node is replaced.
func (b *Bridge) Message(role, id, text string) ([]panel.Event, error) {
	b.Errors = nil
	if id == "" || text == "" {
		return nil, fmt.Errorf("chat panel: a message names its id and carries its text")
	}
	if m, ok := b.msgs[id]; ok {
		if m.Text == text {
			return nil, nil
		}
		m.Text = text
		ev, err := b.track.Next(panel.Event{Kind: panel.KindOps, Ops: []render.Op{{
			Op: render.OpReplace, Target: MessageNodeID(id), Node: m.Node(),
		}}})
		if err != nil {
			return nil, err
		}
		return []panel.Event{ev}, nil
	}
	out, err := b.open()
	if err != nil {
		return nil, err
	}
	b.msgs[id] = &Message{ID: id, Role: role, Text: text}
	b.order = append(b.order, entry{message: true, id: id})
	ev, err := b.conversation()
	if err != nil {
		delete(b.msgs, id)
		b.order = b.order[:len(b.order)-1]
		return out, err
	}
	return append(out, ev), nil
}

// Panel places the display panel the event names in the conversation, after
// what it shows, and answers the panel events that show it:
// the open first when the chat panel is not open yet, then the replace of
// the conversation node. A panel already placed answers nothing (a replayed
// step may place it again).
func (b *Bridge) Panel(e agentapi.PanelShownEvent) ([]panel.Event, error) {
	b.Errors = nil
	if !panel.ValidID(e.PanelID) {
		return nil, fmt.Errorf("chat panel: %q is no panel id", e.PanelID)
	}
	if _, ok := b.placed[e.PanelID]; ok {
		return nil, nil
	}
	out, err := b.open()
	if err != nil {
		return nil, err
	}
	b.placed[e.PanelID] = &Placed{ID: e.PanelID, Title: e.Title}
	b.order = append(b.order, entry{panel: true, id: e.PanelID})
	ev, err := b.conversation()
	if err != nil {
		delete(b.placed, e.PanelID)
		b.order = b.order[:len(b.order)-1]
		return out, err
	}
	return append(out, ev), nil
}

// MessageOf is the text of a message the person sent on this panel's
// input - the run's next user turn -; ok false for any other input.
func (b *Bridge) MessageOf(in panel.Input) (string, bool) {
	if in.Panel != b.ID() || in.Kind != panel.InputMessage || in.Text == "" {
		return "", false
	}
	return in.Text, true
}

// open answers the panel's open when it is not open yet, nothing after.
func (b *Bridge) open() ([]panel.Event, error) {
	if b.track.State != "" {
		return nil, nil
	}
	ev, err := b.track.Next(panel.Event{Kind: panel.KindOpen, Open: &panel.Open{
		Type: Type, Title: b.title, Owner: b.owner,
		Answer: render.Answer{Tree: specOf(render.Node{"kind": "vbox", "id": RootNode, "children": []any{
			render.Node{"kind": "vbox", "id": ArtifactsNode, "children": []any{}},
		}})},
	}})
	if err != nil {
		return nil, err
	}
	return []panel.Event{ev}, nil
}

// conversation answers the replace of the conversation node with every
// message and artifact, in their order (the op set adds a node only
// through its parent).
func (b *Bridge) conversation() (panel.Event, error) {
	kids := make([]any, 0, len(b.order))
	for _, e := range b.order {
		if e.message {
			kids = append(kids, b.msgs[e.id].Node())
			continue
		}
		if e.panel {
			kids = append(kids, b.placed[e.id].Node())
			continue
		}
		kids = append(kids, b.arts[e.id].Node())
	}
	return b.track.Next(panel.Event{Kind: panel.KindOps, Ops: []render.Op{{
		Op: render.OpReplace, Target: ArtifactsNode,
		Node: render.Node{"kind": "vbox", "id": ArtifactsNode, "children": kids},
	}}})
}

// Patch applies the event's ops to its artifact and answers the one panel
// event replacing the artifact's node. A patch of an artifact the chat does
// not show answers nothing (as the GUI ignores it); an op that cannot apply
// is skipped and kept in Errors.
func (b *Bridge) Patch(e agentapi.UIArtifactPatchEvent) ([]panel.Event, error) {
	b.Errors = nil
	a, ok := b.arts[e.ArtifactID]
	if !ok {
		return nil, nil
	}
	raw, err := json.Marshal(e.Ops)
	if err != nil {
		return nil, fmt.Errorf("chat panel: patch %s: %w", e.ArtifactID, err)
	}
	var ops []terminal.UIPatchOp
	if err := json.Unmarshal(raw, &ops); err != nil {
		return nil, fmt.Errorf("chat panel: patch %s: %w", e.ArtifactID, err)
	}
	if len(ops) == 0 {
		return nil, nil
	}
	b.Errors = a.apply(ops)
	return b.replace(a)
}

// Complete marks the artifact complete and answers the panel event
// replacing its node; an artifact the chat does not show answers nothing.
func (b *Bridge) Complete(e agentapi.UIArtifactCompleteEvent) ([]panel.Event, error) {
	b.Errors = nil
	a, ok := b.arts[e.ArtifactID]
	if !ok || a.Status == StatusComplete {
		return nil, nil
	}
	a.Status = StatusComplete
	return b.replace(a)
}

func (b *Bridge) replace(a *Artifact) ([]panel.Event, error) {
	ev, err := b.track.Next(panel.Event{Kind: panel.KindOps, Ops: []render.Op{{
		Op: render.OpReplace, Target: NodeID(a.ID), Node: a.Node(),
	}}})
	if err != nil {
		return nil, err
	}
	return []panel.Event{ev}, nil
}

// apply applies the agent's ops to the artifact as the GUI's store does: a
// single replace of the root with no target is a def replace (each field the
// def carries replaces the artifact's, `data` a page's outer data or any
// other component's value); a deck's ops edit its markdown by slide; every
// other op patches a page's tree, else the component's value.
func (a *Artifact) apply(ops []terminal.UIPatchOp) []error {
	if len(ops) == 1 && ops[0].Kind == "replace" && ops[0].Path == "" && ops[0].Target == "" {
		v, err := decodeRaw(ops[0].Value)
		if err != nil {
			return []error{err}
		}
		if def, ok := v.(map[string]any); ok {
			a.replaceDef(def)
			return nil
		}
	}
	if a.Kind == "deck" {
		md, _ := a.Value.(string)
		if m, ok := a.Value.(map[string]any); ok {
			md, _ = m["markdown"].(string)
		}
		out, errs := editDeck(md, ops)
		a.Value = out
		return errs
	}
	if a.DSLTree != nil {
		tree, errs := applyOps(a.DSLTree, ops)
		a.DSLTree = tree
		return errs
	}
	v, errs := applyOps(a.Value, ops)
	a.Value = v
	return errs
}

func (a *Artifact) replaceDef(def map[string]any) {
	if k, ok := def["kind"].(string); ok {
		a.Kind = k
		a.MediaType = render.MediaTypeForKind(k)
	}
	if v, ok := def["schema"]; ok {
		a.Schema = v
	}
	if v, ok := def["variant"].(string); ok {
		a.Variant = v
	}
	if v, ok := def["ui"].(map[string]any); ok {
		a.UI = v
	}
	if v, ok := def["dslTree"]; ok {
		a.DSLTree = v
	}
	if v, ok := def["data"]; ok {
		if a.Kind == "page" {
			a.Data = v
		} else {
			a.Value = v
		}
	}
	if v, ok := def["mode"].(string); ok {
		a.Mode = v
	}
	if v, ok := def["actions"]; ok {
		a.Actions = v
	}
}

// specOf is an inline tree part.
func specOf(n render.Node) *render.Spec {
	s := render.InlineSpec(n)
	return &s
}

// toJSON turns any Go value into its plain JSON form (maps, slices,
// float64s), so the patches walk one shape whatever the publisher built.
func toJSON(v any) any {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if json.Unmarshal(raw, &out) != nil {
		return v
	}
	return out
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
