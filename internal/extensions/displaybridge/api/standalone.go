// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// Standalone is a display server started alone for one command: its own port
// and token, no hub, no universe (ktw-docs deck export). Stop ends it.
type Standalone struct {
	Origin string
	Token  string
	cmd    *exec.Cmd
	once   sync.Once
}

// StandaloneReadyTimeout is how long a started display server may take to
// answer /healthz: its first start on a machine waits for the antivirus to
// scan the large binary.
var StandaloneReadyTimeout = 90 * time.Second

// StartStandalone starts the display server at bin on a free loopback port
// with a fresh token (KTW_DISPLAY_PORT, KTW_DISPLAY_TOKEN; the rest of the
// environment, KTW_BROWSER included, is passed on) and waits until it
// answers. It fails as soon as the process exits, with what it printed.
func StartStandalone(ctx context.Context, bin string) (*Standalone, error) {
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("display server: no free port: %w", err)
	}
	var tok [16]byte
	if _, err := rand.Read(tok[:]); err != nil {
		return nil, err
	}
	s := &Standalone{Origin: "http://127.0.0.1:" + strconv.Itoa(port), Token: hex.EncodeToString(tok[:])}
	out := &lockedBuffer{}
	s.cmd = exec.Command(bin)
	publish.NoConsoleWindow(s.cmd) // a GUI or a hub may run the export: no window pops up
	s.cmd.Env = append(os.Environ(), "KTW_DISPLAY_PORT="+strconv.Itoa(port), "KTW_DISPLAY_TOKEN="+s.Token)
	s.cmd.Stdout, s.cmd.Stderr = out, out
	if err := s.cmd.Start(); err != nil {
		return nil, fmt.Errorf("display server %s did not start: %w", bin, err)
	}
	exited := make(chan error, 1)
	go func() { exited <- s.cmd.Wait() }()

	deadline := time.Now().Add(StandaloneReadyTimeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		select {
		case err := <-exited:
			return nil, fmt.Errorf("display server %s stopped at start (%v): %s", bin, err, strings.TrimSpace(tail(out.String(), 800)))
		case <-ctx.Done():
			s.Stop()
			return nil, ctx.Err()
		default:
		}
		if Reachable(ctx, client, s.Origin) == nil {
			return s, nil
		}
		if time.Now().After(deadline) {
			s.Stop()
			return nil, fmt.Errorf("display server %s started but did not answer on %s/healthz within %s: %s", bin, s.Origin, StandaloneReadyTimeout, strings.TrimSpace(tail(out.String(), 800)))
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// Stop ends the server; safe to call more than once.
func (s *Standalone) Stop() {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return
	}
	s.once.Do(func() { _ = s.cmd.Process.Kill() })
}

// Reachable checks that a display server answers /healthz at origin.
func Reachable(ctx context.Context, client *http.Client, origin string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(origin, "/")+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s/healthz answered %s", origin, resp.Status)
	}
	return nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func tail(s string, n int) string {
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

// lockedBuffer collects what the server prints while it is read for a message.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
