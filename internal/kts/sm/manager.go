// SPDX-License-Identifier: Apache-2.0

package sm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/core/services"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/endpoint"
)

// sessionEntry holds a session with its router, journal, and scoped service container.
type sessionEntry struct {
	session *Session
	router  *Router
	journal *Journal
	scope   *services.ScopeContainer // session-scoped DI services
}

// Manager is the Session Manager — the architectural kernel.
// It is itself a KTSP container with slots for session lifecycle,
// container registry, routing, and journal aggregation.
// Each new session automatically registers the Manager as the "sm" container
// so that sm/session/*, sm/registry/*, sm/router/*, sm/journal/* are routable.
type Manager struct {
	*ktsp.Endpoint

	// smUUID is generated once at creation and is stable across all sessions.
	smUUID string

	cm *ContainerManager

	mu       sync.RWMutex
	sessions map[string]*sessionEntry
}

// NewManager creates a new Session Manager with the given ContainerManager.
func NewManager(cm *ContainerManager) *Manager {
	m := &Manager{
		Endpoint: ktsp.NewEndpoint(
			"session-manager",
			"Architectural kernel. Manages sessions, routes requests, aggregates events.",
			"1.0.0",
		),
		smUUID:   uuid.New().String(),
		cm:       cm,
		sessions: make(map[string]*sessionEntry),
	}
	m.registerSessionSlot()
	m.registerRegistrySlot()
	m.registerRouterSlot()
	m.registerJournalSlot()
	return m
}

// SMUUID returns the stable UUID assigned to this Manager for use as "sm" in sessions.
func (m *Manager) SMUUID() string { return m.smUUID }

// GetSession returns a session by ID.
func (m *Manager) GetSession(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	return entry.session, true
}

// GetRouter returns the router for a session.
func (m *Manager) GetRouter(sessionID string) (*Router, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.sessions[sessionID]
	if !ok {
		return nil, false
	}
	return entry.router, true
}

// GetScopeContainer returns the session-scoped DI container.
// Used to propagate session scope into task execution contexts.
func (m *Manager) GetScopeContainer(sessionID string) (*services.ScopeContainer, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.sessions[sessionID]
	if !ok {
		return nil, false
	}
	return entry.scope, true
}

// GetJournal returns the journal for a session.
func (m *Manager) GetJournal(sessionID string) (*Journal, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.sessions[sessionID]
	if !ok {
		return nil, false
	}
	return entry.journal, true
}

// registerSessionSlot creates the session lifecycle slot.
func (m *Manager) registerSessionSlot() {
	slot := ktsp.NewSlot("session", ktsp.DefaultInstance, "Session lifecycle management.", nil)

	// create — create a new session, auto-start containers
	slot.Add(ktsp.RawMethod("create", ktsp.MethodOpts{}, func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			ID string `json:"id"`
		}
		if params != nil {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, ktsp.ErrInvalidParams(err.Error())
			}
		}
		if p.ID == "" {
			p.ID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
		}

		m.mu.Lock()
		if _, exists := m.sessions[p.ID]; exists {
			m.mu.Unlock()
			return nil, ktsp.ErrInvalidParams(fmt.Sprintf("session %q already exists", p.ID))
		}

		sess := NewSession(p.ID)
		journal := NewJournal(p.ID)
		router := NewRouter(sess, m.cm)

		// Enter session scope — creates a ScopeContainer for session-scoped services.
		_, scopeContainer := services.SessionScope.EnterScope(ctx)

		m.sessions[p.ID] = &sessionEntry{
			session: sess,
			router:  router,
			journal: journal,
			scope:   scopeContainer,
		}
		m.mu.Unlock()

		// Self-register SM as "sm" container so sm/* paths are routable.
		smTransport := ktransport.NewLocal(m)
		_, _ = sess.Register(m.smUUID, "sm", m, smTransport)

		// Auto-start containers
		if m.cm != nil {
			for _, desc := range m.cm.AutoStartDescriptors() {
				container, transport, err := m.cm.Start(desc.Name)
				if err != nil {
					continue // log and skip, don't fail session creation
				}
				_, _ = sess.Register("", desc.Name, container, transport)
			}
		}

		return json.Marshal(map[string]any{
			"sessionId":  p.ID,
			"createdAt":  sess.CreatedAt.Format(time.RFC3339),
			"containers": sess.ContainerNames(),
		})
	}))

	// close — terminate a session and stop all containers
	slot.Add(ktsp.RawMethod("close", ktsp.MethodOpts{}, func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			SessionID string `json:"sessionId"`
		}
		if params != nil {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, ktsp.ErrInvalidParams(err.Error())
			}
		}

		m.mu.Lock()
		entry, ok := m.sessions[p.SessionID]
		if !ok {
			m.mu.Unlock()
			return nil, ktsp.ErrInvalidParams(fmt.Sprintf("session %q not found", p.SessionID))
		}
		delete(m.sessions, p.SessionID)
		m.mu.Unlock()

		entry.journal.Close()
		_ = entry.scope.Close() // dispose session-scoped services
		_ = entry.session.Close()

		return json.Marshal(map[string]string{"status": "closed"})
	}))

	// list — list active sessions
	slot.Add(ktsp.RawMethod("list", ktsp.MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		type sessionInfo struct {
			ID         string   `json:"id"`
			CreatedAt  string   `json:"createdAt"`
			Containers []string `json:"containers"`
		}
		infos := make([]sessionInfo, 0, len(m.sessions))
		for _, entry := range m.sessions {
			infos = append(infos, sessionInfo{
				ID:         entry.session.ID,
				CreatedAt:  entry.session.CreatedAt.Format(time.RFC3339),
				Containers: entry.session.ContainerNames(),
			})
		}
		return json.Marshal(infos)
	}))

	// info — get session status
	slot.Add(ktsp.RawMethod("info", ktsp.MethodOpts{}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			SessionID string `json:"sessionId"`
		}
		if params != nil {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, ktsp.ErrInvalidParams(err.Error())
			}
		}

		m.mu.RLock()
		entry, ok := m.sessions[p.SessionID]
		m.mu.RUnlock()
		if !ok {
			return nil, ktsp.ErrInvalidParams(fmt.Sprintf("session %q not found", p.SessionID))
		}

		return json.Marshal(map[string]any{
			"id":         entry.session.ID,
			"createdAt":  entry.session.CreatedAt.Format(time.RFC3339),
			"containers": entry.session.ContainerNames(),
			"closed":     entry.session.IsClosed(),
		})
	}))

	m.AddSlot(slot)
}

// registerRegistrySlot creates the container registry slot.
func (m *Manager) registerRegistrySlot() {
	slot := ktsp.NewSlot("registry", ktsp.DefaultInstance, "Container registration and discovery.", nil)

	// register — register a container in a session
	slot.Add(ktsp.RawMethod("register", ktsp.MethodOpts{}, func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			SessionID     string `json:"sessionId"`
			ContainerName string `json:"containerName"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ktsp.ErrInvalidParams(err.Error())
		}

		m.mu.RLock()
		entry, ok := m.sessions[p.SessionID]
		m.mu.RUnlock()
		if !ok {
			return nil, ktsp.ErrInvalidParams(fmt.Sprintf("session %q not found", p.SessionID))
		}

		// Path B: remote self-registration via WebSocket.
		// When a WS client calls registry/register, it is the container itself.
		if callerT, isWS := CallerTransport(ctx); isWS {
			remote, err := newRemoteContainer(ctx, p.ContainerName, callerT)
			if err != nil {
				return nil, ktsp.ErrInternalError(fmt.Sprintf("remote registration of %q: %v", p.ContainerName, err))
			}
			if _, err := entry.session.Register("", p.ContainerName, remote, callerT); err != nil {
				return nil, ktsp.ErrInternalError(err.Error())
			}
			return json.Marshal(map[string]string{"status": "registered", "container": p.ContainerName, "path": "remote"})
		}

		// Path A: in-process container via ContainerManager factory.
		container, transport, err := m.cm.Start(p.ContainerName)
		if err != nil {
			return nil, ktsp.ErrInternalError(fmt.Sprintf("failed to start container %q: %v", p.ContainerName, err))
		}

		if _, err := entry.session.Register("", p.ContainerName, container, transport); err != nil {
			return nil, ktsp.ErrInternalError(err.Error())
		}

		return json.Marshal(map[string]string{"status": "registered", "container": p.ContainerName, "path": "local"})
	}))

	// unregister — remove a container from a session
	slot.Add(ktsp.RawMethod("unregister", ktsp.MethodOpts{}, func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			SessionID     string `json:"sessionId"`
			ContainerName string `json:"containerName"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ktsp.ErrInvalidParams(err.Error())
		}

		m.mu.RLock()
		entry, ok := m.sessions[p.SessionID]
		m.mu.RUnlock()
		if !ok {
			return nil, ktsp.ErrInvalidParams(fmt.Sprintf("session %q not found", p.SessionID))
		}

		if err := entry.session.Unregister(p.ContainerName); err != nil {
			return nil, ktsp.ErrInternalError(err.Error())
		}

		_ = m.cm.Stop(p.ContainerName)

		return json.Marshal(map[string]string{"status": "unregistered", "container": p.ContainerName})
	}))

	// discover — list containers in a session, optionally filtered by slot type
	slot.Add(ktsp.RawMethod("discover", ktsp.MethodOpts{}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			SessionID string        `json:"sessionId"`
			SlotType  ktsp.SlotType `json:"slotType,omitempty"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ktsp.ErrInvalidParams(err.Error())
		}

		m.mu.RLock()
		entry, ok := m.sessions[p.SessionID]
		m.mu.RUnlock()
		if !ok {
			return nil, ktsp.ErrInvalidParams(fmt.Sprintf("session %q not found", p.SessionID))
		}

		names := entry.session.ContainerNames()
		var matches []ktsp.ContainerInfo
		for _, name := range names {
			info, err := entry.session.ContainerInfo(name)
			if err != nil {
				continue
			}
			if p.SlotType != "" {
				hasType := false
				for _, s := range info.Slots {
					if s.SlotType == p.SlotType {
						hasType = true
						break
					}
				}
				if !hasType {
					continue
				}
			}
			matches = append(matches, info)
		}

		return json.Marshal(matches)
	}))

	m.AddSlot(slot)
}

// registerRouterSlot creates the routing slot for cross-container dispatch.
func (m *Manager) registerRouterSlot() {
	slot := ktsp.NewSlot("router", ktsp.DefaultInstance, "Cross-container request routing.", nil)

	// send — route a request to a container via slot resolution
	slot.Add(ktsp.RawMethod("send", ktsp.MethodOpts{}, func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			SessionID string          `json:"sessionId"`
			Path      string          `json:"path"`
			Params    json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ktsp.ErrInvalidParams(err.Error())
		}

		m.mu.RLock()
		entry, ok := m.sessions[p.SessionID]
		m.mu.RUnlock()
		if !ok {
			return nil, ktsp.ErrInvalidParams(fmt.Sprintf("session %q not found", p.SessionID))
		}

		req, err := ktsp.NewRequest(nil, p.Path, p.Params)
		if err != nil {
			return nil, ktsp.ErrInvalidParams(err.Error())
		}

		resp, err := entry.router.Route(ctx, req)
		if err != nil {
			return nil, err
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}))

	m.AddSlot(slot)
}

// registerJournalSlot creates the journal aggregation slot.
func (m *Manager) registerJournalSlot() {
	slot := ktsp.NewSlot("journal", ktsp.DefaultInstance, "Session event aggregation.", nil)

	// Subscription tracking shared across methods via closure.
	var subsMu sync.Mutex
	type subEntry struct{ cancel context.CancelFunc }
	subs := make(map[string]*subEntry)
	var nextSubID uint64

	// emit — record an event in the session journal
	slot.Add(ktsp.RawMethod("emit", ktsp.MethodOpts{}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			SessionID     string          `json:"sessionId"`
			ContainerName string          `json:"containerName"`
			Type          string          `json:"type"`
			Data          json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ktsp.ErrInvalidParams(err.Error())
		}

		m.mu.RLock()
		entry, ok := m.sessions[p.SessionID]
		m.mu.RUnlock()
		if !ok {
			return nil, ktsp.ErrInvalidParams(fmt.Sprintf("session %q not found", p.SessionID))
		}

		seq := entry.journal.Emit(p.ContainerName, p.Type, p.Data)
		return json.Marshal(map[string]uint64{"seq": seq})
	}))

	// subscribe — stream journal events as KTSP notifications back to the caller.
	// Requires a WebSocket connection (CallerTransport must be set in ctx by WsServer).
	slot.Add(ktsp.RawMethod("subscribe", ktsp.MethodOpts{}, func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ktsp.ErrInvalidParams(err.Error())
		}

		callerT, ok := CallerTransport(ctx)
		if !ok {
			return nil, ktsp.ErrInvalidParams("journal/subscribe requires a WebSocket connection")
		}

		m.mu.RLock()
		entry, ok2 := m.sessions[p.SessionID]
		m.mu.RUnlock()
		if !ok2 {
			return nil, ktsp.ErrInvalidParams(fmt.Sprintf("session %q not found", p.SessionID))
		}

		// Derive subscription context from request context so the goroutine
		// exits automatically when the WebSocket connection closes.
		subCtx, cancel := context.WithCancel(ctx)

		subsMu.Lock()
		nextSubID++
		subID := fmt.Sprintf("sub-%d", nextSubID)
		subs[subID] = &subEntry{cancel: cancel}
		subsMu.Unlock()

		sub := entry.journal.Subscribe(256, nil)
		go func() {
			defer func() {
				cancel()
				entry.journal.Unsubscribe(sub)
				subsMu.Lock()
				delete(subs, subID)
				subsMu.Unlock()
			}()
			for {
				select {
				case event, ok := <-sub.Events:
					if !ok {
						return // journal closed (session ended)
					}
					notif, err := ktsp.NewNotification("journal/event", event)
					if err != nil {
						continue
					}
					if err := callerT.Notify(context.Background(), notif); err != nil {
						return // transport closed
					}
				case <-subCtx.Done():
					return
				}
			}
		}()

		return json.Marshal(map[string]string{
			"subscriptionId": subID,
			"sessionId":      p.SessionID,
			"status":         "subscribed",
		})
	}))

	// unsubscribe — cancel a journal subscription by ID.
	slot.Add(ktsp.RawMethod("unsubscribe", ktsp.MethodOpts{}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p struct {
			SubscriptionID string `json:"subscriptionId"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ktsp.ErrInvalidParams(err.Error())
		}

		subsMu.Lock()
		e, ok := subs[p.SubscriptionID]
		subsMu.Unlock()
		if !ok {
			return nil, ktsp.ErrInvalidParams(fmt.Sprintf("subscription %q not found", p.SubscriptionID))
		}
		e.cancel()

		return json.Marshal(map[string]string{
			"status":         "unsubscribed",
			"subscriptionId": p.SubscriptionID,
		})
	}))

	m.AddSlot(slot)
}
