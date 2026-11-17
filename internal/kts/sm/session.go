// SPDX-License-Identifier: Apache-2.0

package sm

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// Session represents an active session with registered containers.
// Containers are keyed by UUID. Name-based, slot-based, and slot-type-based
// indices enable fast resolution by any of these dimensions.
type Session struct {
	ID        string
	CreatedAt time.Time

	mu         sync.RWMutex
	containers map[string]*registeredContainer // uuid → container
	nameIndex  map[string]string               // name → uuid
	slotIndex  map[string]string               // slot name → uuid
	// slotTypeIndex maps a SlotType to an ordered list of container UUIDs that
	// provide it, in registration order. Used for multi-UI discovery.
	slotTypeIndex map[ktsp.SlotType][]string

	// primaryUserPrompter is the UUID of the container that currently
	// receives ask_user requests. Updated automatically on register/unregister.
	primaryUserPrompter string

	// regOrder records UUIDs in insertion order — used to determine fallback
	// primary when the current primary user_prompter unregisters.
	regOrder []string

	closed bool
}

// registeredContainer holds a container with its transport and metadata.
type registeredContainer struct {
	uuid      string
	name      string
	container ktsp.Container
	transport ktsp.Transport
}

// NewSession creates a new session with the given ID.
func NewSession(id string) *Session {
	return &Session{
		ID:            id,
		CreatedAt:     time.Now(),
		containers:    make(map[string]*registeredContainer),
		nameIndex:     make(map[string]string),
		slotIndex:     make(map[string]string),
		slotTypeIndex: make(map[ktsp.SlotType][]string),
	}
}

// Register adds a container to the session.
// If uuid is empty, a new UUID is generated.
// Returns the UUID assigned to the container.
func (s *Session) Register(id, name string, container ktsp.Container, transport ktsp.Transport) (string, error) {
	if id == "" {
		id = uuid.New().String()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return "", fmt.Errorf("session %s is closed", s.ID)
	}
	if _, exists := s.containers[id]; exists {
		return "", fmt.Errorf("container UUID %q already registered", id)
	}
	if _, exists := s.nameIndex[name]; exists {
		return "", fmt.Errorf("container name %q already registered", name)
	}

	rc := &registeredContainer{
		uuid:      id,
		name:      name,
		container: container,
		transport: transport,
	}
	s.containers[id] = rc
	s.nameIndex[name] = id
	s.regOrder = append(s.regOrder, id)

	// Build slot index and slot-type index from the container's slots.
	// For in-process containers, query Info() directly.
	// For remote/stdio containers (nil container), slots must be registered
	// separately via RegisterSlots.
	if container != nil {
		info := container.Info()
		for _, slot := range info.Slots {
			s.slotIndex[slot.Name] = id
			if slot.SlotType != "" {
				s.slotTypeIndex[slot.SlotType] = append(s.slotTypeIndex[slot.SlotType], id)
				if slot.SlotType == ktsp.SlotTypeUserPrompter {
					s.primaryUserPrompter = id
				}
			}
		}
	}

	return id, nil
}

// RegisterSlots adds slot names to the slot index for a container that was
// registered with a nil container (e.g. stdio/remote containers where the
// actual container runs in another process).
func (s *Session) RegisterSlots(containerName string, slots []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.nameIndex[containerName]
	if !ok {
		return
	}
	for _, slot := range slots {
		s.slotIndex[slot] = id
	}
}

// Unregister removes a container by name and cleans up all indices.
func (s *Session) Unregister(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.nameIndex[name]
	if !ok {
		return fmt.Errorf("container %q not found", name)
	}
	return s.unregisterByID(id)
}

// UnregisterByID removes a container by UUID.
func (s *Session) UnregisterByID(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unregisterByID(id)
}

// unregisterByID removes a container by UUID — must be called with mu held.
func (s *Session) unregisterByID(id string) error {
	rc, ok := s.containers[id]
	if !ok {
		return fmt.Errorf("container UUID %q not found", id)
	}

	// Remove slot entries owned by this container.
	for slot, uid := range s.slotIndex {
		if uid == id {
			delete(s.slotIndex, slot)
		}
	}

	// Remove slot-type entries.
	for slotType, uids := range s.slotTypeIndex {
		filtered := uids[:0]
		for _, uid := range uids {
			if uid != id {
				filtered = append(filtered, uid)
			}
		}
		if len(filtered) == 0 {
			delete(s.slotTypeIndex, slotType)
		} else {
			s.slotTypeIndex[slotType] = filtered
		}
	}

	// Update primary user_prompter if needed.
	if s.primaryUserPrompter == id {
		s.primaryUserPrompter = s.pickNextUserPrompter(id)
	}

	// Remove from reg order.
	filtered := s.regOrder[:0]
	for _, uid := range s.regOrder {
		if uid != id {
			filtered = append(filtered, uid)
		}
	}
	s.regOrder = filtered

	delete(s.nameIndex, rc.name)
	delete(s.containers, id)
	return nil
}

// pickNextUserPrompter returns the most-recently-registered user_prompter UUID
// that is not the given excluded UUID — must be called with mu held.
func (s *Session) pickNextUserPrompter(excludeID string) string {
	uids := s.slotTypeIndex[ktsp.SlotTypeUserPrompter]
	// Walk in reverse registration order to find the last-registered alternative.
	for i := len(s.regOrder) - 1; i >= 0; i-- {
		uid := s.regOrder[i]
		if uid == excludeID {
			continue
		}
		for _, u := range uids {
			if u == uid {
				return uid
			}
		}
	}
	return ""
}

// ResolveByID returns the transport for a container identified by UUID.
func (s *Session) ResolveByID(id string) (ktsp.Transport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rc, ok := s.containers[id]
	if !ok {
		return nil, fmt.Errorf("container UUID %q not found", id)
	}
	return rc.transport, nil
}

// ResolveByName returns the transport for a container identified by name.
func (s *Session) ResolveByName(name string) (ktsp.Transport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.nameIndex[name]
	if !ok {
		return nil, fmt.Errorf("container %q not found", name)
	}
	return s.containers[id].transport, nil
}

// ResolveSlot returns the transport for the container that owns the given slot.
// Returns the transport and the container name.
func (s *Session) ResolveSlot(slot string) (ktsp.Transport, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.slotIndex[slot]
	if !ok {
		return nil, "", fmt.Errorf("no container provides slot %q", slot)
	}
	rc := s.containers[id]
	return rc.transport, rc.name, nil
}

// FindBySlotType returns the UUIDs of all containers providing the given slot type.
func (s *Session) FindBySlotType(slotType ktsp.SlotType) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	uids := s.slotTypeIndex[slotType]
	if len(uids) == 0 {
		return nil
	}
	result := make([]string, len(uids))
	copy(result, uids)
	return result
}

// GetPrimaryUserPrompter returns the UUID of the primary user_prompter container.
// Returns an error if no user_prompter is registered.
func (s *Session) GetPrimaryUserPrompter() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.primaryUserPrompter == "" {
		return "", fmt.Errorf("no user_prompter registered in session %s", s.ID)
	}
	return s.primaryUserPrompter, nil
}

// ContainerNames returns all registered container names.
func (s *Session) ContainerNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.containers))
	for name := range s.nameIndex {
		names = append(names, name)
	}
	return names
}

// ContainerInfo returns info about a specific registered container by name.
func (s *Session) ContainerInfo(name string) (ktsp.ContainerInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.nameIndex[name]
	if !ok {
		return ktsp.ContainerInfo{}, fmt.Errorf("container %q not found", name)
	}
	rc := s.containers[id]
	if rc.container == nil {
		return ktsp.ContainerInfo{}, fmt.Errorf("container %q has no in-process instance", name)
	}
	return rc.container.Info(), nil
}

// Close shuts down all transports in the session.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	var firstErr error
	for _, rc := range s.containers {
		if err := rc.transport.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// IsClosed returns whether the session has been closed.
func (s *Session) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}
