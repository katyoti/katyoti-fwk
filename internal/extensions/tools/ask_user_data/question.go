// SPDX-License-Identifier: Apache-2.0

package ask_user_data

import (
	"context"
	"errors"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// panelWire is the tool's reach to the universe's panel subjects: the
// bus's publish and subscribe as functions (rule) and the scope
// the subjects live in.
type panelWire struct {
	scope     panel.Scope
	publish   panel.Publish
	subscribe panel.Subscribe
}

// resolveWire finds the message bus and the universe; ok false when either
// is missing (programmatic callers, tests without a bus). A var so a test
// puts its own bus there.
var resolveWire = func(ctx context.Context) (panelWire, bool) {
	bus, err := application.UseService[mbapi.MessageBus](ctx)
	if err != nil || bus == nil {
		return panelWire{}, false
	}
	env := application.Env()
	if env == nil {
		return panelWire{}, false
	}
	tenant, universe := cenv.ResolveTenantUniverse(env.HomeDir())
	if tenant == "" || universe == "" {
		return panelWire{}, false
	}
	return wireOf(bus, panel.Scope{Tenant: tenant, Universe: universe}), true
}

// wireOf is the panel wire on bus in scope.
func wireOf(bus mbapi.MessageBus, scope panel.Scope) panelWire {
	return panelWire{
		scope: scope,
		publish: func(ctx context.Context, subject string, payload []byte) error {
			return bus.Publish(ctx, subject, payload)
		},
		subscribe: func(ctx context.Context, subject string, deliver func([]byte)) (func(), error) {
			sub, err := bus.Subscribe(ctx, subject, func(_ context.Context, m mbapi.Message) { deliver(m.Data()) })
			if err != nil {
				return nil, err
			}
			return func() { _ = sub.Unsubscribe() }, nil
		},
	}
}

// specOf is the question's form: the schema AST resolved for the mount
// (ui-form kept), inline.
func specOf(schema any) *render.Spec {
	if schema == nil {
		return nil
	}
	s := render.InlineSpec(schema)
	return &s
}

// answer is how the question ended: the submitted value, a cancel, or a
// text typed in the composer instead.
type answer struct {
	value     any
	cancelled bool
	typed     bool
	text      string
}

// summary is a short human label for the prompt-banner clear event.
func (a answer) summary() string {
	switch {
	case a.typed:
		return a.text
	case a.cancelled:
		return "(annulé)"
	default:
		return "(formulaire envoyé)"
	}
}

// typedText waits for a text the person types in the chat composer while
// the form asks: the session gate's next free-text answer. Nil when there
// is no gate.
func typedText(gate hgapi.HumanGate) func(ctx context.Context) (string, error) {
	if gate == nil {
		return nil
	}
	return func(ctx context.Context) (string, error) {
		resp, err := gate.Prompt(ctx, hgapi.PromptRequest{Type: "text"})
		if err != nil {
			return "", err
		}
		return resp.Answer, nil
	}
}

// awaitAnswer asks the question on the panel and waits for its answer on
// the panel's input subject (Handle.Question) - or for a text typed in the
// composer meanwhile (typed, when given), which ends the form as a cancel
// with that text. A typed wait that ends on its own (the screens detached)
// leaves the panel asking: the panel server keeps the question, so a screen
// coming back finds it and answers it there. An end of ctx ends the wait.
func awaitAnswer(ctx context.Context, h *panel.Handle, ask panel.Ask, subscribe panel.Subscribe, typed func(context.Context) (string, error)) (answer, error) {
	qctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		in  panel.Input
		err error
	}
	got := make(chan result, 1)
	go func() {
		in, err := h.Question(qctx, ask, subscribe)
		got <- result{in, err}
	}()
	var text chan string
	if typed != nil {
		text = make(chan string, 1)
		go func() {
			if t, err := typed(qctx); err == nil {
				text <- t
			}
		}()
	}
	select {
	case r := <-got:
		switch {
		case errors.Is(r.err, panel.ErrCancelled):
			return answer{cancelled: true}, nil
		case r.err != nil:
			return answer{}, r.err
		}
		return answer{value: r.in.Value}, nil
	case t := <-text:
		return answer{typed: true, text: t}, nil
	}
}

// status is how the question panel completes: ok when answered, cancelled
// otherwise.
func (a answer) status() string {
	if a.cancelled || a.typed {
		return "cancelled"
	}
	return "ok"
}

// closePanel completes the question panel once the wait ended, on a context
// of its own: the run's may be the one that ended.
func closePanel(h *panel.Handle, status string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = h.Complete(ctx, status, nil)
}
