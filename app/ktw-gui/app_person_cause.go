// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	profileremote "github.com/katyoti/katyoti-fwk/internal/extensions/profile/remote"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// THE PERSON AS THE CAUSE. Every call the GUI makes to a universe core goes
// out under the person using it — `{person, <profile handle>, 0}` — so the
// writes the core makes for that call (and the journal records of them) say
// who made them. The one place is universeSender: the sender it returns stamps
// the cause on each call's context when the context carries none yet.
//
// The handle is the profile's (served by the admin universe); it is read once
// and kept, re-read when the profile changes (the profile.changed event) and
// retried after a while when it could not be read. No profile set (the
// placeholder) is no person: such calls carry no cause, as before.

// personCause is the cached handle of the person using this GUI.
type personCause struct {
	mu      sync.Mutex
	handle  string
	loaded  bool
	tried   time.Time
	loading bool
}

// personRetry is how long a failed read of the profile waits before the next.
const personRetry = 30 * time.Second

// causedSender stamps the person on every call that carries no cause.
type causedSender struct {
	inner ktsp.Sender
	who   func(ctx context.Context) (cause.Cause, bool)
}

func (s causedSender) Call(ctx context.Context, path string, params any, opts ...ktsp.CallOption) (json.RawMessage, error) {
	return s.inner.Call(s.caused(ctx), path, params, opts...)
}

func (s causedSender) Notify(ctx context.Context, path string, params any, opts ...ktsp.CallOption) error {
	return s.inner.Notify(s.caused(ctx), path, params, opts...)
}

func (s causedSender) caused(ctx context.Context) context.Context {
	if _, ok := cause.CauseFrom(ctx); ok {
		return ctx
	}
	if c, ok := s.who(ctx); ok {
		return cause.WithCause(ctx, c)
	}
	return ctx
}

// withPersonCause wraps a universe core's sender so its calls carry the person.
func (a *App) withPersonCause(inner ktsp.Sender) ktsp.Sender {
	return causedSender{inner: inner, who: func(ctx context.Context) (cause.Cause, bool) {
		h := a.personHandle(ctx)
		if h == "" {
			return cause.Cause{}, false
		}
		return cause.Cause{Kind: cause.KindPerson, ID: h, Depth: 0}, true
	}}
}

// personHandle is the profile's handle, read through the admin core's RAW
// sender (not the caused one: reading who the person is cannot itself need
// to know who the person is), "" when no profile is set or it cannot be read.
func (a *App) personHandle(ctx context.Context) string {
	p := &a.person
	p.mu.Lock()
	if p.loaded || p.loading || (!p.tried.IsZero() && time.Since(p.tried) < personRetry) {
		h := p.handle
		p.mu.Unlock()
		return h
	}
	p.loading = true
	p.tried = time.Now()
	p.mu.Unlock()

	handle, ok := "", false
	if admin := a.adminRawSender(); admin != nil {
		rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if prof, err := profileremote.NewRemote(admin).Get(rctx); err == nil {
			handle, ok = handleOf(prof), true
		}
		cancel()
	}
	p.mu.Lock()
	p.loading = false
	if ok {
		p.handle, p.loaded = handle, true
	}
	p.mu.Unlock()
	return handle
}

// rememberPerson keeps a profile the GUI just read, so the next call needs
// no read of its own.
func (a *App) rememberPerson(prof profileapi.Profile) {
	a.person.mu.Lock()
	a.person.handle, a.person.loaded = handleOf(prof), true
	a.person.mu.Unlock()
}

// forgetPerson drops the cached handle: the profile changed.
func (a *App) forgetPerson() {
	a.person.mu.Lock()
	a.person.handle, a.person.loaded, a.person.tried = "", false, time.Time{}
	a.person.mu.Unlock()
}

// handleOf is the person's handle a profile gives: none for the placeholder.
func handleOf(p profileapi.Profile) string {
	if p.ID == "" || p.Source == profileapi.SourcePlaceholder {
		return ""
	}
	return p.ID
}

// adminRawSender is the admin core's sender as the master gives it, nil when
// the admin universe cannot be reached.
func (a *App) adminRawSender() ktsp.Sender {
	a.mu.Lock()
	master := a.master
	a.mu.Unlock()
	if master == nil {
		return nil
	}
	core, err := master.Activate(a.ctx, admin.UniverseName)
	if err != nil {
		return nil
	}
	return core.Sender()
}
