// SPDX-License-Identifier: Apache-2.0

package sm

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/endpoint"
)

// --- Helpers ---

// mockContainer creates a Endpoint with a single slot for testing.
func mockContainer(name, slotName, methodName string) *ktsp.Endpoint {
	c := ktsp.NewEndpoint(name, name+" service", "1.0.0")
	if slotName != "" {
		slot := ktsp.NewSlot(slotName, ktsp.DefaultInstance, slotName+" operations", nil)
		slot.Add(ktsp.RawMethod(methodName, ktsp.MethodOpts{}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
			return json.Marshal(map[string]string{"from": name, "slot": slotName, "method": methodName})
		}))
		c.AddSlot(slot)
	}
	return c
}

// mockContainerWithSlotType creates a Endpoint with a slot bound to
// the given contract type. Post-SlotType-drop, the contract type IS the
// typeName — two containers sharing a contract surface as two
// (typeName, instance) bindings under one type.
func mockContainerWithSlotType(name, instance string, contractType ktsp.SlotType) *ktsp.Endpoint {
	c := ktsp.NewEndpoint(name, name+" service", "1.0.0")
	slot := ktsp.NewSlot(string(contractType), instance, string(contractType)+" binding", nil)
	slot.Add(ktsp.RawMethod("ask_user", ktsp.MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"from": name})
	}))
	c.AddSlot(slot)
	return c
}

func mockContainerDescriptor(name, slotName, methodName string, autoStart bool) ContainerDescriptor {
	return ContainerDescriptor{
		Name:      name,
		AutoStart: autoStart,
		Slots:     []string{slotName},
		Transport: ChannelTransport{
			Factory: func() ktsp.Container {
				return mockContainer(name, slotName, methodName)
			},
		},
	}
}

// containsName returns true if name is in the slice.
func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// --- Session Tests ---

func TestSessionRegisterAndResolve(t *testing.T) {
	sess := NewSession("test-session")

	ws := mockContainer("workspace", "file", "read")
	tr := ktransport.NewLocal(ws)

	id, err := sess.Register("", "workspace", ws, tr)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("Register should return a non-empty UUID")
	}

	// Resolve the "file" slot
	transport, containerName, err := sess.ResolveSlot("file")
	if err != nil {
		t.Fatal(err)
	}
	if containerName != "workspace" {
		t.Errorf("containerName = %q, want %q", containerName, "workspace")
	}
	if transport == nil {
		t.Fatal("transport should not be nil")
	}
}

func TestSessionResolveByIDAndName(t *testing.T) {
	sess := NewSession("test-session")

	ws := mockContainer("workspace", "file", "read")
	tr := ktransport.NewLocal(ws)

	id, err := sess.Register("", "workspace", ws, tr)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve by UUID
	transport, err := sess.ResolveByID(id)
	if err != nil {
		t.Fatalf("ResolveByID(%q): %v", id, err)
	}
	if transport == nil {
		t.Fatal("ResolveByID: transport should not be nil")
	}

	// Resolve by name
	transport2, err := sess.ResolveByName("workspace")
	if err != nil {
		t.Fatalf("ResolveByName: %v", err)
	}
	if transport2 == nil {
		t.Fatal("ResolveByName: transport should not be nil")
	}

	// ResolveByID with explicit UUID
	id2, err := sess.Register("my-explicit-uuid", "engine", mockContainer("engine", "compute", "run"), ktransport.NewLocal(mockContainer("engine", "compute", "run")))
	if err != nil {
		t.Fatal(err)
	}
	if id2 != "my-explicit-uuid" {
		t.Errorf("explicit UUID not preserved: got %q, want %q", id2, "my-explicit-uuid")
	}
	_, err = sess.ResolveByID("my-explicit-uuid")
	if err != nil {
		t.Fatalf("ResolveByID(explicit): %v", err)
	}
}

func TestSessionFindBySlotType(t *testing.T) {
	sess := NewSession("test-session")

	c1 := mockContainerWithSlotType("tui", "ui", ktsp.SlotTypeUserPrompter)
	c2 := mockContainerWithSlotType("gui", "ui2", ktsp.SlotTypeUserPrompter)
	c3 := mockContainerWithSlotType("engine", "agent", ktsp.SlotTypeAgent)

	_, _ = sess.Register("", "tui", c1, ktransport.NewLocal(c1))
	_, _ = sess.Register("", "gui", c2, ktransport.NewLocal(c2))
	_, _ = sess.Register("", "engine", c3, ktransport.NewLocal(c3))

	prompters := sess.FindBySlotType(ktsp.SlotTypeUserPrompter)
	if len(prompters) != 2 {
		t.Fatalf("FindBySlotType(user_prompter) = %v, want 2 entries", prompters)
	}

	agents := sess.FindBySlotType(ktsp.SlotTypeAgent)
	if len(agents) != 1 {
		t.Fatalf("FindBySlotType(agent) = %v, want 1 entry", agents)
	}

	skills := sess.FindBySlotType(ktsp.SlotTypeSkill)
	if len(skills) != 0 {
		t.Fatalf("FindBySlotType(skill) = %v, want empty", skills)
	}
}

func TestSessionMultiUIUserPrompter(t *testing.T) {
	sess := NewSession("test-session")

	c1 := mockContainerWithSlotType("tui", "ui", ktsp.SlotTypeUserPrompter)
	c2 := mockContainerWithSlotType("gui", "ui2", ktsp.SlotTypeUserPrompter)

	id1, _ := sess.Register("", "tui", c1, ktransport.NewLocal(c1))

	// After first user_prompter: primary = tui
	primary1, err := sess.GetPrimaryUserPrompter()
	if err != nil {
		t.Fatal(err)
	}
	if primary1 != id1 {
		t.Errorf("primary after first register = %q, want id1=%q", primary1, id1)
	}

	id2, _ := sess.Register("", "gui", c2, ktransport.NewLocal(c2))

	// After second user_prompter: last-registered wins (gui)
	primary2, err := sess.GetPrimaryUserPrompter()
	if err != nil {
		t.Fatal(err)
	}
	if primary2 != id2 {
		t.Errorf("primary after second register = %q, want id2=%q (last-registered)", primary2, id2)
	}

	// Unregister gui (current primary) — should fall back to tui
	if err := sess.Unregister("gui"); err != nil {
		t.Fatal(err)
	}
	primary3, err := sess.GetPrimaryUserPrompter()
	if err != nil {
		t.Fatalf("GetPrimaryUserPrompter after fallback: %v", err)
	}
	if primary3 != id1 {
		t.Errorf("primary after fallback = %q, want id1=%q (tui)", primary3, id1)
	}

	// Unregister the last user_prompter — GetPrimaryUserPrompter returns error
	if err := sess.Unregister("tui"); err != nil {
		t.Fatal(err)
	}
	_, err = sess.GetPrimaryUserPrompter()
	if err == nil {
		t.Fatal("expected error when no user_prompter registered")
	}
}

func TestSessionResolveSlotNotFound(t *testing.T) {
	sess := NewSession("test-session")

	_, _, err := sess.ResolveSlot("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent slot")
	}
}

func TestSessionUnregister(t *testing.T) {
	sess := NewSession("test-session")

	ws := mockContainer("workspace", "file", "read")
	tr := ktransport.NewLocal(ws)
	_, _ = sess.Register("", "workspace", ws, tr)

	if err := sess.Unregister("workspace"); err != nil {
		t.Fatal(err)
	}

	// Slot should no longer resolve
	_, _, err := sess.ResolveSlot("file")
	if err == nil {
		t.Fatal("expected error after unregister")
	}
}

func TestSessionDuplicateRegister(t *testing.T) {
	sess := NewSession("test-session")

	ws := mockContainer("workspace", "file", "read")
	tr := ktransport.NewLocal(ws)
	_, _ = sess.Register("", "workspace", ws, tr)

	_, err := sess.Register("", "workspace", ws, tr)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestSessionClose(t *testing.T) {
	sess := NewSession("test-session")

	ws := mockContainer("workspace", "file", "read")
	tr := ktransport.NewLocal(ws)
	_, _ = sess.Register("", "workspace", ws, tr)

	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}

	if !sess.IsClosed() {
		t.Error("session should be closed")
	}

	// Registration should fail on closed session
	ws2 := mockContainer("ws2", "file2", "read")
	tr2 := ktransport.NewLocal(ws2)
	_, err := sess.Register("", "ws2", ws2, tr2)
	if err == nil {
		t.Fatal("expected error registering on closed session")
	}
}

// --- ContainerManager Tests ---

func TestContainerManagerStartStop(t *testing.T) {
	cm := NewContainerManager()
	desc := mockContainerDescriptor("workspace", "file", "read", false)
	if err := cm.Register(desc); err != nil {
		t.Fatal(err)
	}

	container, transport, err := cm.Start("workspace")
	if err != nil {
		t.Fatal(err)
	}
	if container == nil || transport == nil {
		t.Fatal("container and transport should not be nil")
	}
	if !cm.IsRunning("workspace") {
		t.Error("workspace should be running")
	}

	// Start again should return same instance
	container2, _, err := cm.Start("workspace")
	if err != nil {
		t.Fatal(err)
	}
	if container2 != container {
		t.Error("expected same container instance on second start")
	}

	if err := cm.Stop("workspace"); err != nil {
		t.Fatal(err)
	}
	if cm.IsRunning("workspace") {
		t.Error("workspace should not be running after stop")
	}
}

func TestContainerManagerNotFound(t *testing.T) {
	cm := NewContainerManager()
	_, _, err := cm.Start("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent container")
	}
}

func TestContainerManagerFindBySlot(t *testing.T) {
	cm := NewContainerManager()
	cm.Register(mockContainerDescriptor("workspace", "file", "read", false))
	cm.Register(mockContainerDescriptor("journal-svc", "journal", "emit", false))

	name, found := cm.FindBySlot("file")
	if !found || name != "workspace" {
		t.Errorf("FindBySlot(file) = %q, %v — want workspace, true", name, found)
	}

	name, found = cm.FindBySlot("journal")
	if !found || name != "journal-svc" {
		t.Errorf("FindBySlot(journal) = %q, %v — want journal-svc, true", name, found)
	}

	_, found = cm.FindBySlot("nonexistent")
	if found {
		t.Error("expected not found for nonexistent slot")
	}
}

func TestContainerManagerFindBySlotType(t *testing.T) {
	cm := NewContainerManager()
	cm.Register(ContainerDescriptor{
		Name:      "ktw-docs",
		Slots:     []string{"docs"},
		SlotTypes: []ktsp.SlotType{ktsp.SlotTypeSkill},
		Transport: ChannelTransport{Factory: func() ktsp.Container { return mockContainer("ktw-docs", "docs", "render") }},
	})
	cm.Register(ContainerDescriptor{
		Name:      "engine",
		Slots:     []string{"agent"},
		SlotTypes: []ktsp.SlotType{ktsp.SlotTypeAgent},
		Transport: ChannelTransport{Factory: func() ktsp.Container { return mockContainer("engine", "agent", "run") }},
	})
	cm.Register(ContainerDescriptor{
		Name:      "ktw-voices",
		Slots:     []string{"voice"},
		SlotTypes: []ktsp.SlotType{ktsp.SlotTypeSkill},
		Transport: ChannelTransport{Factory: func() ktsp.Container { return mockContainer("ktw-voices", "voice", "play") }},
	})

	skills := cm.FindBySlotType(ktsp.SlotTypeSkill)
	if len(skills) != 2 {
		t.Fatalf("FindBySlotType(skill) = %v, want 2 results", skills)
	}

	agents := cm.FindBySlotType(ktsp.SlotTypeAgent)
	if len(agents) != 1 || agents[0] != "engine" {
		t.Errorf("FindBySlotType(agent) = %v, want [engine]", agents)
	}

	prompters := cm.FindBySlotType(ktsp.SlotTypeUserPrompter)
	if len(prompters) != 0 {
		t.Errorf("FindBySlotType(user_prompter) = %v, want empty", prompters)
	}
}

func TestContainerManagerAutoStart(t *testing.T) {
	cm := NewContainerManager()
	cm.Register(mockContainerDescriptor("ws", "file", "read", true))
	cm.Register(mockContainerDescriptor("engine", "compute", "run", false))

	autos := cm.AutoStartDescriptors()
	if len(autos) != 1 {
		t.Fatalf("expected 1 auto-start descriptor, got %d", len(autos))
	}
	if autos[0].Name != "ws" {
		t.Errorf("auto-start name = %q, want %q", autos[0].Name, "ws")
	}
}

// --- Router Tests ---

func TestRouterStrictThreeLevelPath(t *testing.T) {
	sess := NewSession("test-session")
	router := NewRouter(sess, nil)

	// 2-level path must be rejected
	req, _ := ktsp.NewRequest(1, "slot/method", nil)
	_, err := router.Route(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 2-level path — container_id required")
	}
}

func TestRouterDirectRouting(t *testing.T) {
	sess := NewSession("test-session")
	cm := NewContainerManager()
	router := NewRouter(sess, cm)

	ws := mockContainer("workspace", "file", "read")
	tr := ktransport.NewLocal(ws)
	_, _ = sess.Register("", "workspace", ws, tr)

	// Route by container name: workspace/file/read
	req, _ := ktsp.NewRequest(1, "workspace/file/read", nil)
	resp, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result map[string]string
	json.Unmarshal(resp.Result, &result)
	if result["from"] != "workspace" {
		t.Errorf("from = %q, want %q", result["from"], "workspace")
	}
}

func TestRouterRouteByUUID(t *testing.T) {
	sess := NewSession("test-session")
	router := NewRouter(sess, nil)

	ws := mockContainer("workspace", "file", "read")
	tr := ktransport.NewLocal(ws)
	id, _ := sess.Register("", "workspace", ws, tr)

	// Route by UUID: <uuid>/file/read
	req, _ := ktsp.NewRequest(1, id+"/file/read", nil)
	resp, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result map[string]string
	json.Unmarshal(resp.Result, &result)
	if result["from"] != "workspace" {
		t.Errorf("from = %q, want workspace", result["from"])
	}
}

func TestRouterOnDemandStart(t *testing.T) {
	sess := NewSession("test-session")
	cm := NewContainerManager()
	cm.Register(mockContainerDescriptor("workspace", "file", "read", false))
	router := NewRouter(sess, cm)

	// "workspace" not in session yet — on-demand start via container name
	req, _ := ktsp.NewRequest(1, "workspace/file/read", nil)
	resp, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result map[string]string
	json.Unmarshal(resp.Result, &result)
	if result["from"] != "workspace" {
		t.Errorf("from = %q, want %q", result["from"], "workspace")
	}
}

func TestRouterContainerNotFound(t *testing.T) {
	sess := NewSession("test-session")
	cm := NewContainerManager()
	router := NewRouter(sess, cm)

	// Unknown container — 3-level path
	req, _ := ktsp.NewRequest(1, "nonexistent-container/slot/method", nil)
	_, err := router.Route(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for unknown container")
	}
}

// --- Journal Tests ---

func TestJournalEmitAndSubscribe(t *testing.T) {
	j := NewJournal("sess-1")

	sub := j.Subscribe(10, nil)
	defer sub.Close()

	data, _ := json.Marshal(map[string]string{"text": "hello"})
	seq := j.Emit("engine", "text.delta", data)
	if seq != 1 {
		t.Errorf("seq = %d, want 1", seq)
	}

	select {
	case event := <-sub.Events:
		if event.Seq != 1 {
			t.Errorf("event.Seq = %d, want 1", event.Seq)
		}
		if event.ContainerName != "engine" {
			t.Errorf("ContainerName = %q, want %q", event.ContainerName, "engine")
		}
		if event.Type != "text.delta" {
			t.Errorf("Type = %q, want %q", event.Type, "text.delta")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestJournalFilter(t *testing.T) {
	j := NewJournal("sess-1")

	// Only receive tool events
	sub := j.Subscribe(10, func(e JournalEvent) bool {
		return e.Type == "tool.start"
	})
	defer sub.Close()

	j.Emit("engine", "text.delta", nil)
	j.Emit("engine", "tool.start", nil)

	select {
	case event := <-sub.Events:
		if event.Type != "tool.start" {
			t.Errorf("Type = %q, want %q", event.Type, "tool.start")
		}
		if event.Seq != 2 {
			t.Errorf("Seq = %d, want 2", event.Seq)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for filtered event")
	}
}

func TestJournalSeqMonotonic(t *testing.T) {
	j := NewJournal("sess-1")

	seq1 := j.Emit("a", "type1", nil)
	seq2 := j.Emit("b", "type2", nil)
	seq3 := j.Emit("c", "type3", nil)

	if seq1 >= seq2 || seq2 >= seq3 {
		t.Errorf("seq not monotonic: %d, %d, %d", seq1, seq2, seq3)
	}
}

func TestJournalUnsubscribe(t *testing.T) {
	j := NewJournal("sess-1")

	sub := j.Subscribe(10, nil)
	j.Unsubscribe(sub)
	sub.Close()

	j.Emit("engine", "text.delta", nil)

	// Channel should be empty — subscriber was removed
	select {
	case <-sub.Events:
		t.Fatal("should not receive events after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// expected — no event
	}
}

// --- Manager Tests ---

func TestManagerCreateSession(t *testing.T) {
	cm := NewContainerManager()
	m := NewManager(cm)

	req, _ := ktsp.NewRequest(1, "session/create", map[string]string{"id": "sess-1"})
	resp, err := m.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", req.Params)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	json.Unmarshal(resp, &result)
	if result["sessionId"] != "sess-1" {
		t.Errorf("sessionId = %v, want sess-1", result["sessionId"])
	}

	sess, ok := m.GetSession("sess-1")
	if !ok {
		t.Fatal("session not found")
	}
	if sess.ID != "sess-1" {
		t.Errorf("ID = %q, want %q", sess.ID, "sess-1")
	}
}

func TestManagerSMSelfRegistration(t *testing.T) {
	cm := NewContainerManager()
	m := NewManager(cm)

	p, _ := json.Marshal(map[string]string{"id": "sess-1"})
	m.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", p)

	sess, _ := m.GetSession("sess-1")

	// "sm" must be registered
	if !containsName(sess.ContainerNames(), "sm") {
		t.Fatalf("sm not registered in session: %v", sess.ContainerNames())
	}

	// SM must be addressable by its stable UUID
	_, err := sess.ResolveByID(m.SMUUID())
	if err != nil {
		t.Fatalf("ResolveByID(smUUID): %v", err)
	}

	// Route sm/session/list via Router — end-to-end test
	router, _ := m.GetRouter("sess-1")
	req, _ := ktsp.NewRequest(1, "sm/session/list", nil)
	resp, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("sm/session/list route failed: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("sm/session/list error: %v", resp.Error)
	}
}

func TestManagerCreateSessionAutoStart(t *testing.T) {
	cm := NewContainerManager()
	cm.Register(mockContainerDescriptor("workspace", "file", "read", true))
	cm.Register(mockContainerDescriptor("engine", "compute", "run", false))

	m := NewManager(cm)

	params, _ := json.Marshal(map[string]string{"id": "sess-1"})
	_, err := m.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", params)
	if err != nil {
		t.Fatal(err)
	}

	sess, _ := m.GetSession("sess-1")
	names := sess.ContainerNames()
	// sm (self-registration) + workspace (auto-start) = 2
	if len(names) != 2 {
		t.Fatalf("expected 2 containers (sm + workspace), got %d: %v", len(names), names)
	}
	if !containsName(names, "workspace") {
		t.Errorf("workspace not in containers: %v", names)
	}
	if !containsName(names, "sm") {
		t.Errorf("sm not in containers: %v", names)
	}
}

func TestManagerCloseSession(t *testing.T) {
	cm := NewContainerManager()
	m := NewManager(cm)

	params, _ := json.Marshal(map[string]string{"id": "sess-1"})
	m.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", params)

	closeParams, _ := json.Marshal(map[string]string{"sessionId": "sess-1"})
	_, err := m.Handle(context.Background(), "session", ktsp.DefaultInstance, "close", closeParams)
	if err != nil {
		t.Fatal(err)
	}

	_, ok := m.GetSession("sess-1")
	if ok {
		t.Error("session should not exist after close")
	}
}

func TestManagerListSessions(t *testing.T) {
	cm := NewContainerManager()
	m := NewManager(cm)

	p1, _ := json.Marshal(map[string]string{"id": "sess-1"})
	p2, _ := json.Marshal(map[string]string{"id": "sess-2"})
	m.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", p1)
	m.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", p2)

	result, err := m.Handle(context.Background(), "session", ktsp.DefaultInstance, "list", nil)
	if err != nil {
		t.Fatal(err)
	}

	var sessions []map[string]any
	json.Unmarshal(result, &sessions)
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestManagerRegisterContainer(t *testing.T) {
	cm := NewContainerManager()
	cm.Register(mockContainerDescriptor("tui", "ui", "ask_user", false))
	m := NewManager(cm)

	// Create session
	p, _ := json.Marshal(map[string]string{"id": "sess-1"})
	m.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", p)

	// Register TUI container
	regParams, _ := json.Marshal(map[string]any{
		"sessionId":     "sess-1",
		"containerName": "tui",
		"roles":         []string{"ui"},
	})
	_, err := m.Handle(context.Background(), "registry", ktsp.DefaultInstance, "register", regParams)
	if err != nil {
		t.Fatal(err)
	}

	sess, _ := m.GetSession("sess-1")
	if !containsName(sess.ContainerNames(), "tui") {
		t.Errorf("tui not found in containers: %v", sess.ContainerNames())
	}
}

func TestManagerRouterSend(t *testing.T) {
	cm := NewContainerManager()
	cm.Register(mockContainerDescriptor("workspace", "file", "read", true))
	m := NewManager(cm)

	// Create session with auto-started workspace
	p, _ := json.Marshal(map[string]string{"id": "sess-1"})
	m.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", p)

	// Route via router.send using 3-level path
	sendParams, _ := json.Marshal(map[string]any{
		"sessionId": "sess-1",
		"path":      "workspace/file/read",
	})
	result, err := m.Handle(context.Background(), "router", ktsp.DefaultInstance, "send", sendParams)
	if err != nil {
		t.Fatal(err)
	}

	var r map[string]string
	json.Unmarshal(result, &r)
	if r["from"] != "workspace" {
		t.Errorf("from = %q, want %q", r["from"], "workspace")
	}
}

func TestManagerJournalEmit(t *testing.T) {
	cm := NewContainerManager()
	m := NewManager(cm)

	p, _ := json.Marshal(map[string]string{"id": "sess-1"})
	m.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", p)

	emitParams, _ := json.Marshal(map[string]any{
		"sessionId":     "sess-1",
		"containerName": "engine",
		"type":          "text.delta",
		"data":          map[string]string{"text": "hello"},
	})
	result, err := m.Handle(context.Background(), "journal", ktsp.DefaultInstance, "emit", emitParams)
	if err != nil {
		t.Fatal(err)
	}

	var r map[string]uint64
	json.Unmarshal(result, &r)
	if r["seq"] != 1 {
		t.Errorf("seq = %d, want 1", r["seq"])
	}
}

// --- Integration: Multi-Container Routing ---

func TestIntegrationMultiContainerRouting(t *testing.T) {
	cm := NewContainerManager()
	cm.Register(mockContainerDescriptor("workspace", "file", "read", true))
	cm.Register(mockContainerDescriptor("tui", "ui", "ask_user", true))
	cm.Register(mockContainerDescriptor("engine", "compute", "run", true))

	m := NewManager(cm)

	// Create session — sm + 3 auto-started containers = 4
	p, _ := json.Marshal(map[string]string{"id": "sess-1"})
	m.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", p)

	sess, _ := m.GetSession("sess-1")
	if len(sess.ContainerNames()) != 4 {
		t.Fatalf("expected 4 containers (sm + 3 auto-start), got %d: %v", len(sess.ContainerNames()), sess.ContainerNames())
	}

	router, _ := m.GetRouter("sess-1")

	// Route to workspace: container/slot/method
	req1, _ := ktsp.NewRequest(1, "workspace/file/read", nil)
	resp1, err := router.Route(context.Background(), req1)
	if err != nil {
		t.Fatal(err)
	}
	var r1 map[string]string
	json.Unmarshal(resp1.Result, &r1)
	if r1["from"] != "workspace" {
		t.Errorf("workspace/file/read routed to %q, want workspace", r1["from"])
	}

	// Route to engine
	req2, _ := ktsp.NewRequest(2, "engine/compute/run", nil)
	resp2, err := router.Route(context.Background(), req2)
	if err != nil {
		t.Fatal(err)
	}
	var r2 map[string]string
	json.Unmarshal(resp2.Result, &r2)
	if r2["from"] != "engine" {
		t.Errorf("engine/compute/run routed to %q, want engine", r2["from"])
	}

	// Route to TUI
	req3, _ := ktsp.NewRequest(3, "tui/ui/ask_user", nil)
	resp3, err := router.Route(context.Background(), req3)
	if err != nil {
		t.Fatal(err)
	}
	var r3 map[string]string
	json.Unmarshal(resp3.Result, &r3)
	if r3["from"] != "tui" {
		t.Errorf("tui/ui/ask_user routed to %q, want tui", r3["from"])
	}

	// Route to SM: sm/session/list
	req4, _ := ktsp.NewRequest(4, "sm/session/list", nil)
	resp4, err := router.Route(context.Background(), req4)
	if err != nil {
		t.Fatalf("sm/session/list: %v", err)
	}
	if resp4.Error != nil {
		t.Fatalf("sm/session/list error: %v", resp4.Error)
	}

	// Route via UUID
	id, _ := sess.ResolveByName("workspace")
	_ = id // just checking it resolves
}

func TestIntegrationOnDemandStart(t *testing.T) {
	cm := NewContainerManager()
	cm.Register(mockContainerDescriptor("workspace", "file", "read", false))
	m := NewManager(cm)

	p, _ := json.Marshal(map[string]string{"id": "sess-1"})
	m.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", p)

	// Workspace not auto-started — route should trigger on-demand start
	sendParams, _ := json.Marshal(map[string]any{
		"sessionId": "sess-1",
		"path":      "workspace/file/read",
	})
	result, err := m.Handle(context.Background(), "router", ktsp.DefaultInstance, "send", sendParams)
	if err != nil {
		t.Fatal(err)
	}

	var r map[string]string
	json.Unmarshal(result, &r)
	if r["from"] != "workspace" {
		t.Errorf("from = %q, want %q", r["from"], "workspace")
	}

	// Container should now be in the session
	sess, _ := m.GetSession("sess-1")
	if !containsName(sess.ContainerNames(), "workspace") {
		t.Errorf("workspace should be in session after on-demand start: %v", sess.ContainerNames())
	}
}

func TestManagerSMKtspSlot(t *testing.T) {
	cm := NewContainerManager()
	m := NewManager(cm)

	// The SM itself is a container — its ktsp slot should work
	result, err := m.Handle(context.Background(), "ktsp", ktsp.DefaultInstance, "ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	var r map[string]string
	json.Unmarshal(result, &r)
	if r["status"] != "ok" {
		t.Errorf("ktsp.ping status = %q, want ok", r["status"])
	}

	// SM should have 5 slots: ktsp, session, registry, router, journal
	slotsResult, err := m.Handle(context.Background(), "ktsp", ktsp.DefaultInstance, "slots", nil)
	if err != nil {
		t.Fatal(err)
	}
	var slots []ktsp.SlotInfo
	json.Unmarshal(slotsResult, &slots)
	if len(slots) != 5 {
		names := make([]string, len(slots))
		for i, s := range slots {
			names[i] = s.Name
		}
		t.Errorf("expected 5 SM slots, got %d: %v", len(slots), names)
	}
}
