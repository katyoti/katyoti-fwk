// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"
)

// SidecarClient manages a lazy-spawned KTSP subprocess for a sidecar binary.
// The subprocess is started on the first call and cached for subsequent use.
// Thread-safe.
type SidecarClient struct {
	binaryPath string
	mu         sync.Mutex
	transport  *ktransport.Session
	sender     ktsp.Sender
}

// NewSidecarClient creates a client for the given sidecar binary path.
// The subprocess is NOT started until Sender() is called.
func NewSidecarClient(binaryPath string) *SidecarClient {
	return &SidecarClient{binaryPath: binaryPath}
}

// Sender returns a KTSP Sender connected to the sidecar subprocess.
// Spawns the subprocess lazily on the first call. Subsequent calls return
// the cached sender. The subprocess runs `<binary> sc ktsp stdio`.
func (c *SidecarClient) Sender() (ktsp.Sender, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sender != nil {
		return c.sender, nil
	}
	transport, err := ktransport.StartStdio(c.binaryPath, "sc", "ktsp", "stdio")
	if err != nil {
		return nil, err
	}
	c.transport = transport
	c.sender = ktsp.NewSender(transport)
	return c.sender, nil
}

// Close shuts down the sidecar subprocess if it's running.
func (c *SidecarClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.transport != nil {
		err := c.transport.Close()
		c.transport = nil
		c.sender = nil
		return err
	}
	return nil
}
