// SPDX-License-Identifier: Apache-2.0

package sm

import (
	"fmt"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktspendpoint "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/endpoint"
	ktspjsonrpc "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"
)

// ContainerFactory creates an in-process container instance.
type ContainerFactory func() ktsp.Container

// TransportConfig describes how to connect to a container.
// Implementations are sealed to the known transport types.
type TransportConfig interface {
	transportType() string // sealed discriminator
}

// ChannelTransport connects to an in-process container via Go channels.
type ChannelTransport struct {
	Factory ContainerFactory
}

func (ChannelTransport) transportType() string { return "channel" }

// StdioTransport connects to a subprocess container via stdin/stdout.
type StdioTransport struct {
	Command string   // executable path
	Args    []string // command-line arguments
}

func (StdioTransport) transportType() string { return "stdio" }

// WSTransport connects to a remote container via WebSocket.
type WSTransport struct {
	URL     string            // ws:// or wss:// endpoint
	Headers map[string]string // optional headers (e.g. auth tokens)
}

func (WSTransport) transportType() string { return "ws" }

// ContainerDescriptor describes a container that can be started.
type ContainerDescriptor struct {
	Name        string
	Description string
	Transport   TransportConfig // how to connect (ChannelTransport, StdioTransport, WSTransport)
	AutoStart   bool            // start automatically when session creates?
	Slots       []string        // expected slot names (for catalog-based slot resolution)
	SlotTypes   []ktsp.SlotType // slot types provided by this container (for type-based discovery)
}

// runningContainer tracks a started container.
type runningContainer struct {
	descriptor ContainerDescriptor
	container  ktsp.Container
	transport  ktsp.Transport
}

// ContainerManager holds a catalog of container descriptors and manages
// their lifecycle (start/stop). It is infrastructure — not a KTSP container itself.
type ContainerManager struct {
	mu      sync.RWMutex
	catalog map[string]ContainerDescriptor
	running map[string]*runningContainer
}

// NewContainerManager creates a new container manager.
func NewContainerManager() *ContainerManager {
	return &ContainerManager{
		catalog: make(map[string]ContainerDescriptor),
		running: make(map[string]*runningContainer),
	}
}

// Register adds a container descriptor to the catalog.
func (cm *ContainerManager) Register(desc ContainerDescriptor) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if desc.Name == "" {
		return fmt.Errorf("container descriptor must have a name")
	}
	if _, exists := cm.catalog[desc.Name]; exists {
		return fmt.Errorf("container %q already registered in catalog", desc.Name)
	}
	cm.catalog[desc.Name] = desc
	return nil
}

// Start starts a container by name from the catalog.
// Returns the container and transport pair for session registration.
func (cm *ContainerManager) Start(name string) (ktsp.Container, ktsp.Transport, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if rc, ok := cm.running[name]; ok {
		return rc.container, rc.transport, nil // already running
	}

	desc, ok := cm.catalog[name]
	if !ok {
		return nil, nil, fmt.Errorf("container %q not found in catalog", name)
	}

	switch t := desc.Transport.(type) {
	case ChannelTransport:
		if t.Factory == nil {
			return nil, nil, fmt.Errorf("container %q has no factory for channel transport", name)
		}
		container := t.Factory()
		transport := ktspendpoint.NewLocal(container)

		cm.running[name] = &runningContainer{
			descriptor: desc,
			container:  container,
			transport:  transport,
		}
		return container, transport, nil

	case StdioTransport:
		if t.Command == "" {
			return nil, nil, fmt.Errorf("container %q has no command for stdio transport", name)
		}
		transport, err := ktspjsonrpc.StartStdio(t.Command, t.Args...)
		if err != nil {
			return nil, nil, fmt.Errorf("start stdio container %q: %w", name, err)
		}

		cm.running[name] = &runningContainer{
			descriptor: desc,
			transport:  transport,
		}
		return nil, transport, nil

	case nil:
		return nil, nil, fmt.Errorf("container %q has no transport configured", name)

	default:
		return nil, nil, fmt.Errorf("unsupported transport type %T for container %q", t, name)
	}
}

// Stop stops a running container and closes its transport.
func (cm *ContainerManager) Stop(name string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	rc, ok := cm.running[name]
	if !ok {
		return fmt.Errorf("container %q is not running", name)
	}

	err := rc.transport.Close()
	delete(cm.running, name)
	return err
}

// StopAll stops all running containers.
func (cm *ContainerManager) StopAll() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var firstErr error
	for name, rc := range cm.running {
		if err := rc.transport.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(cm.running, name)
	}
	return firstErr
}

// IsRunning returns whether a container is currently running.
func (cm *ContainerManager) IsRunning(name string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	_, ok := cm.running[name]
	return ok
}

// Available returns all container descriptors in the catalog.
func (cm *ContainerManager) Available() []ContainerDescriptor {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	descs := make([]ContainerDescriptor, 0, len(cm.catalog))
	for _, d := range cm.catalog {
		descs = append(descs, d)
	}
	return descs
}

// AutoStartDescriptors returns descriptors marked for auto-start.
func (cm *ContainerManager) AutoStartDescriptors() []ContainerDescriptor {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var descs []ContainerDescriptor
	for _, d := range cm.catalog {
		if d.AutoStart {
			descs = append(descs, d)
		}
	}
	return descs
}

// FindBySlot searches the catalog for a container that provides the given slot.
// Uses the Slots field in the descriptor for catalog-based resolution.
func (cm *ContainerManager) FindBySlot(slotName string) (string, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, desc := range cm.catalog {
		for _, s := range desc.Slots {
			if s == slotName {
				return desc.Name, true
			}
		}
	}
	return "", false
}

// FindBySlotType searches the catalog for containers that provide slots of the given type.
// Returns the names of all matching containers.
func (cm *ContainerManager) FindBySlotType(slotType ktsp.SlotType) []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var names []string
	for _, desc := range cm.catalog {
		for _, st := range desc.SlotTypes {
			if st == slotType {
				names = append(names, desc.Name)
				break
			}
		}
	}
	return names
}

// Descriptor returns the descriptor for a named container.
func (cm *ContainerManager) Descriptor(name string) (ContainerDescriptor, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	desc, ok := cm.catalog[name]
	return desc, ok
}
