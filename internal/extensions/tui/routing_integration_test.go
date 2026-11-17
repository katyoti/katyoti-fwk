// SPDX-License-Identifier: Apache-2.0

package tui_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/endpoint"
	ktspengine "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/engine"
	ktsm "github.com/katyoti/katyoti-fwk/internal/kts/sm"
)

// TestEndToEndRouting_AskUser_Text proves the full KTSP routing path:
// PrompterStub → Sender → RoutingTransport → SM → Router → TUI Container → response
func TestEndToEndRouting_AskUser_Text(t *testing.T) {
	// 1. Create a TUI container backed by a mock (no real Bubbletea)
	tuiContainer := newMockTUIContainer("hello from TUI")

	// 2. Create ContainerManager and register TUI
	cm := ktsm.NewContainerManager()
	if err := cm.Register(ktsm.ContainerDescriptor{
		Name:      "tui",
		Slots:     []string{"ui"},
		Transport: ktsm.ChannelTransport{Factory: func() ktsp.Container { return tuiContainer }},
	}); err != nil {
		t.Fatalf("register TUI descriptor: %v", err)
	}

	// 3. Create Session Manager and session
	sm := ktsm.NewManager(cm)
	ctx := context.Background()

	result, err := sm.Handle(ctx, "session", ktsp.DefaultInstance, "create", nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	var sessInfo struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(result, &sessInfo)

	// 4. Register TUI container in session
	regParams, _ := json.Marshal(map[string]any{
		"sessionId":     sessInfo.SessionID,
		"containerName": "tui",
	})
	if _, err := sm.Handle(ctx, "registry", ktsp.DefaultInstance, "register", regParams); err != nil {
		t.Fatalf("register TUI: %v", err)
	}

	// 5. Create the routing chain: RoutingTransport → Sender → PrompterStub
	smTransport := ktransport.NewLocal(sm)
	routingTransport := ktransport.NewRouting(smTransport, sessInfo.SessionID)
	sender := ktsp.NewRetrySender(routingTransport, ktsp.RetryPolicy{
		MaxRetries: 0, // no retries for tests
	})
	prompter := ktspengine.NewPrompterStub(sender, sessInfo.SessionID, "tui")

	// 6. Call PromptText — this should route through the full KTSP path
	answer, err := prompter.PromptText(ctx, "What is your name?")
	if err != nil {
		t.Fatalf("PromptText failed: %v", err)
	}
	if answer != "hello from TUI" {
		t.Errorf("expected 'hello from TUI', got %q", answer)
	}
}

// TestEndToEndRouting_AskUser_Select tests select prompt routing.
func TestEndToEndRouting_AskUser_Select(t *testing.T) {
	tuiContainer := newMockTUIContainer("option-b")

	cm := ktsm.NewContainerManager()
	cm.Register(ktsm.ContainerDescriptor{
		Name:      "tui",
		Slots:     []string{"ui"},
		Transport: ktsm.ChannelTransport{Factory: func() ktsp.Container { return tuiContainer }},
	})

	sm := ktsm.NewManager(cm)
	ctx := context.Background()

	result, _ := sm.Handle(ctx, "session", ktsp.DefaultInstance, "create", nil)
	var sessInfo struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(result, &sessInfo)

	regParams, _ := json.Marshal(map[string]any{
		"sessionId":     sessInfo.SessionID,
		"containerName": "tui",
	})
	sm.Handle(ctx, "registry", ktsp.DefaultInstance, "register", regParams)

	smTransport := ktransport.NewLocal(sm)
	routingTransport := ktransport.NewRouting(smTransport, sessInfo.SessionID)
	sender := ktsp.NewRetrySender(routingTransport, ktsp.RetryPolicy{})
	prompter := ktspengine.NewPrompterStub(sender, sessInfo.SessionID, "tui")

	selected, err := prompter.PromptSelect(ctx, "Pick one", []string{"option-a", "option-b"})
	if err != nil {
		t.Fatalf("PromptSelect failed: %v", err)
	}
	if selected != "option-b" {
		t.Errorf("expected 'option-b', got %q", selected)
	}
}

// TestEndToEndRouting_AskUser_Confirm tests confirm prompt routing.
func TestEndToEndRouting_AskUser_Confirm(t *testing.T) {
	tuiContainer := newMockTUIContainer("true")

	cm := ktsm.NewContainerManager()
	cm.Register(ktsm.ContainerDescriptor{
		Name:      "tui",
		Slots:     []string{"ui"},
		Transport: ktsm.ChannelTransport{Factory: func() ktsp.Container { return tuiContainer }},
	})

	sm := ktsm.NewManager(cm)
	ctx := context.Background()

	result, _ := sm.Handle(ctx, "session", ktsp.DefaultInstance, "create", nil)
	var sessInfo struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(result, &sessInfo)

	regParams, _ := json.Marshal(map[string]any{
		"sessionId":     sessInfo.SessionID,
		"containerName": "tui",
	})
	sm.Handle(ctx, "registry", ktsp.DefaultInstance, "register", regParams)

	smTransport := ktransport.NewLocal(sm)
	routingTransport := ktransport.NewRouting(smTransport, sessInfo.SessionID)
	sender := ktsp.NewRetrySender(routingTransport, ktsp.RetryPolicy{})
	prompter := ktspengine.NewPrompterStub(sender, sessInfo.SessionID, "tui")

	confirmed, err := prompter.PromptConfirm(ctx, "Are you sure?")
	if err != nil {
		t.Fatalf("PromptConfirm failed: %v", err)
	}
	if !confirmed {
		t.Error("expected confirmed=true")
	}
}

// TestEndToEndRouting_NoUIContainer tests that routing fails gracefully
// when no UI container is registered.
func TestEndToEndRouting_NoUIContainer(t *testing.T) {
	cm := ktsm.NewContainerManager()
	sm := ktsm.NewManager(cm)
	ctx := context.Background()

	result, _ := sm.Handle(ctx, "session", ktsp.DefaultInstance, "create", nil)
	var sessInfo struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(result, &sessInfo)

	smTransport := ktransport.NewLocal(sm)
	routingTransport := ktransport.NewRouting(smTransport, sessInfo.SessionID)
	sender := ktsp.NewRetrySender(routingTransport, ktsp.RetryPolicy{})
	prompter := ktspengine.NewPrompterStub(sender, sessInfo.SessionID, "tui")

	_, err := prompter.PromptText(ctx, "hello?")
	if err == nil {
		t.Fatal("expected error when no UI container is registered")
	}
}

// TestEndToEndRouting_ContextCancellation tests that context cancellation
// propagates through the full routing chain.
func TestEndToEndRouting_ContextCancellation(t *testing.T) {
	// Use a blocking container that never responds
	tuiContainer := newBlockingTUIContainer()

	cm := ktsm.NewContainerManager()
	cm.Register(ktsm.ContainerDescriptor{
		Name:      "tui",
		Slots:     []string{"ui"},
		Transport: ktsm.ChannelTransport{Factory: func() ktsp.Container { return tuiContainer }},
	})

	sm := ktsm.NewManager(cm)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, _ := sm.Handle(context.Background(), "session", ktsp.DefaultInstance, "create", nil)
	var sessInfo struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(result, &sessInfo)

	regParams, _ := json.Marshal(map[string]any{
		"sessionId":     sessInfo.SessionID,
		"containerName": "tui",
	})
	sm.Handle(context.Background(), "registry", ktsp.DefaultInstance, "register", regParams)

	smTransport := ktransport.NewLocal(sm)
	routingTransport := ktransport.NewRouting(smTransport, sessInfo.SessionID)
	sender := ktsp.NewRetrySender(routingTransport, ktsp.RetryPolicy{})
	prompter := ktspengine.NewPrompterStub(sender, sessInfo.SessionID, "tui")

	_, err := prompter.PromptText(ctx, "waiting forever...")
	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
}

// --- Mock TUI containers for testing ---

// mockTUIContainer simulates a TUI container that auto-responds to ask_user.
type mockTUIContainer struct {
	*ktsp.Endpoint
	autoResponse string
}

func newMockTUIContainer(autoResponse string) *mockTUIContainer {
	c := &mockTUIContainer{
		Endpoint: ktsp.NewEndpoint(
			"tui", "Mock TUI", "1.0.0",
		),
		autoResponse: autoResponse,
	}

	slot := ktsp.NewSlot("ui", ktsp.DefaultInstance, "Mock UI slot", nil)
	slot.Add(ktsp.RawMethod("ask_user", ktsp.MethodOpts{}, func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			Type string `json:"type"`
		}
		if params != nil {
			json.Unmarshal(params, &p)
		}
		if p.Type == "" {
			p.Type = "text"
		}

		switch p.Type {
		case "confirm":
			return json.Marshal(map[string]bool{"confirmed": c.autoResponse == "true"})
		case "select":
			return json.Marshal(map[string]string{"selected": c.autoResponse})
		default:
			return json.Marshal(map[string]string{"answer": c.autoResponse})
		}
	}))
	c.AddSlot(slot)
	return c
}

// blockingTUIContainer simulates a TUI that never responds (for timeout tests).
type blockingTUIContainer struct {
	*ktsp.Endpoint
}

func newBlockingTUIContainer() *blockingTUIContainer {
	c := &blockingTUIContainer{
		Endpoint: ktsp.NewEndpoint(
			"tui", "Blocking TUI", "1.0.0",
		),
	}

	slot := ktsp.NewSlot("ui", ktsp.DefaultInstance, "Blocking UI slot", nil)
	slot.Add(ktsp.RawMethod("ask_user", ktsp.MethodOpts{}, func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		// Block until context is cancelled
		<-ctx.Done()
		return nil, ctx.Err()
	}))
	c.AddSlot(slot)
	return c
}
