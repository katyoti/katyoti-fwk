// SPDX-License-Identifier: Apache-2.0

package room

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
)

// detachGraceDefault is how long a pending prompt keeps waiting after
// the LAST UI client disconnects before giving up with
// ErrUserDetached. Long enough to survive a page refresh / transport
// reconnect; short enough that an abandoned question moves to the
// inbox promptly.
const detachGraceDefault = 10 * time.Second

// HumanGate implements hgapi.HumanGate by routing prompts through the terminal
// protocol. When a prompt is requested, the caller blocks until an "answer"
// command arrives from a connected terminal client — or, when every client
// has disconnected for longer than the grace period, returns
// hgapi.ErrUserDetached so the caller can fall over to an async channel
// (the ask-user tool re-routes the question to the workspace inbox).
type HumanGate struct {
	mu      sync.Mutex
	pending chan string

	detachGrace time.Duration
	clients     int
	// detached is closed when the gate has had zero clients for the
	// grace period. Replaced with a fresh open channel when a client
	// (re)connects. Pending waits select on the snapshot they grabbed
	// at entry; a re-attach during the wait swaps the channel so the
	// old one never closes — exactly the "keep waiting" semantic.
	detached chan struct{}
	timer    *time.Timer

	// panelInput sends the person's input to a display panel - its input
	// subject on the universe's bus (SetPanelInput). A ui_event for an
	// artifact that is a panel (the ask-user-data question) goes there,
	// where the run waiting on the panel takes it (DeliverPanelInput).
	panelInput func(ctx context.Context, in panel.Input) error
}

func NewHumanGate() *HumanGate {
	return &HumanGate{
		pending:     make(chan string, 1),
		detachGrace: detachGraceDefault,
		detached:    make(chan struct{}),
	}
}

// SetDetachGrace overrides the disconnect grace period. Tests use
// millisecond values to avoid slow sleeps.
func (g *HumanGate) SetDetachGrace(d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.detachGrace = d
}

// SetAttachedClients updates the connected-client count. The Room
// calls this on every client register/unregister. Transition to zero
// arms the grace timer; any return above zero disarms it and resets
// the detach signal so in-flight and future waits keep working.
func (g *HumanGate) SetAttachedClients(n int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.clients = n
	if n > 0 {
		if g.timer != nil {
			g.timer.Stop()
			g.timer = nil
		}
		// Reset the signal if a previous grace expiry closed it.
		select {
		case <-g.detached:
			g.detached = make(chan struct{})
		default:
		}
		return
	}
	if g.timer != nil {
		return // grace timer already armed
	}
	detachedSnapshot := g.detached
	g.timer = time.AfterFunc(g.detachGrace, func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.timer = nil
		if g.clients > 0 {
			return // re-attached during the race with the timer
		}
		select {
		case <-detachedSnapshot:
		default:
			close(detachedSnapshot)
		}
	})
}

// DeliverAnswer unblocks the pending prompt with the given answer.
func (g *HumanGate) DeliverAnswer(answer string) {
	select {
	case g.pending <- answer:
	default:
	}
}

func (g *HumanGate) Prompt(ctx context.Context, in hgapi.PromptRequest) (hgapi.PromptResponse, error) {
	answer, err := g.waitForAnswer(ctx)
	if err != nil {
		return hgapi.PromptResponse{}, err
	}

	switch in.Type {
	case "confirm":
		return hgapi.PromptResponse{
			Confirmed: answer == "yes" || answer == "true" || answer == "y",
		}, nil
	case "form":
		var data map[string]any
		if json.Unmarshal([]byte(answer), &data) != nil {
			data = map[string]any{"_raw": answer}
		}
		return hgapi.PromptResponse{Data: data}, nil
	default: // text, select
		return hgapi.PromptResponse{Answer: answer}, nil
	}
}

func (g *HumanGate) PromptPermission(ctx context.Context, in hgapi.PromptPermissionRequest) (hgapi.PromptPermissionResponse, error) {
	answer, err := g.waitForAnswer(ctx)
	if err != nil {
		return hgapi.PromptPermissionResponse{}, err
	}
	switch answer {
	case "allow_once":
		return hgapi.PromptPermissionResponse{Decision: hgapi.PermissionDecisionAllowOnce}, nil
	case "allow_always":
		return hgapi.PromptPermissionResponse{Decision: hgapi.PermissionDecisionAllowAlways, SelectedRule: in.ToolName + ":**"}, nil
	case "deny_always":
		return hgapi.PromptPermissionResponse{Decision: hgapi.PermissionDecisionDenyAlways, SelectedRule: in.ToolName + ":**"}, nil
	default:
		return hgapi.PromptPermissionResponse{Decision: hgapi.PermissionDecisionAllowOnce}, nil
	}
}

func (g *HumanGate) Capabilities() []string {
	return []string{"text", "select", "confirm", "form"}
}

// SetPanelInput gives the gate the way to a display panel's input subject:
// the session's panel wire. Nil leaves a ui_event for a panel undelivered.
func (g *HumanGate) SetPanelInput(send func(ctx context.Context, in panel.Input) error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.panelInput = send
}

// DeliverPanelInput sends a terminal ui_event (submit/cancel) for an
// artifact that is a display panel - the question ask-user-data opened - to
// that panel's input, where the run waiting on the panel takes it. It
// returns true when it went out; false for any other event or artifact, or
// with no panel wire, so the transport falls back to the envelope-as-answer
// path (Mode A of the CRUD action wiring).
func (g *HumanGate) DeliverPanelInput(ctx context.Context, artifactID, event string, value json.RawMessage) bool {
	if !panel.ValidID(artifactID) {
		return false
	}
	in := panel.Input{Panel: artifactID}
	switch event {
	case "submit":
		in.Kind = panel.InputSubmit
		if len(value) > 0 {
			var v any
			if json.Unmarshal(value, &v) != nil {
				return false
			}
			in.Value = v
		}
	case "cancel":
		in.Kind = panel.InputCancel
	default:
		return false
	}
	g.mu.Lock()
	send := g.panelInput
	g.mu.Unlock()
	if send == nil {
		return false
	}
	if err := send(ctx, in); err != nil {
		slog.Warn("room: a panel input did not go out", "panel", artifactID, "err", err)
		return false
	}
	return true
}

func (g *HumanGate) waitForAnswer(ctx context.Context) (string, error) {
	// Snapshot the detach channel at entry: a re-attach during the
	// wait swaps the channel on the gate, so the snapshot stays open
	// and this wait keeps going — only an expiry that happens while
	// we wait fires the fall-over.
	g.mu.Lock()
	detached := g.detached
	g.mu.Unlock()
	select {
	case answer := <-g.pending:
		return answer, nil
	case <-detached:
		return "", hgapi.ErrUserDetached
	case <-ctx.Done():
		return "", fmt.Errorf("prompt cancelled: %w", ctx.Err())
	}
}
