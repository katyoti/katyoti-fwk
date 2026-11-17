// SPDX-License-Identifier: Apache-2.0

package stdio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/api"
)

// SubprocessTransport wraps a raw stdio Transport with subprocess lifecycle
// management. Close() closes stdin and waits for the process to exit.
type SubprocessTransport struct {
	api.Transport
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser // non-nil only for NewFromPipes — explicit close needed
}

// Start creates an exec.Cmd, starts it, and returns a Transport connected to
// the subprocess's stdin/stdout.
func Start(command string, args ...string) (*SubprocessTransport, error) {
	cmd := exec.Command(command, args...)
	publish.NoConsoleWindow(cmd) // a peer over pipes needs no window
	return StartCmd(cmd)
}

// StartCmd starts a pre-configured exec.Cmd and returns a Transport.
func StartCmd(cmd *exec.Cmd) (*SubprocessTransport, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdio: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdio: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("stdio: start %s: %w", cmd.Path, err)
	}

	stream := New(stdout, stdin)
	return &SubprocessTransport{Transport: stream, cmd: cmd, stdin: stdin}, nil
}

// NewFromPipes creates a Transport from raw reader/writer pipes.
// No subprocess is managed. Typically used in tests via io.Pipe().
func NewFromPipes(in io.ReadCloser, out io.WriteCloser) *SubprocessTransport {
	stream := New(in, out)
	return &SubprocessTransport{Transport: stream, stdin: out, stdout: in}
}

// Close shuts down the transport and waits for the subprocess to exit.
func (t *SubprocessTransport) Close() error {
	t.Transport.Close()
	if t.stdin != nil {
		t.stdin.Close()
	}
	if t.cmd != nil {
		return t.cmd.Wait()
	} else if t.stdout != nil {
		t.stdout.Close()
	}
	return nil
}

// Cmd returns the underlying subprocess command if one is being managed
// (nil for transports created via NewFromPipes). Exposes the process to
// callers that need to send signals (e.g., connmgr's SIGTERM-with-grace
// shutdown sequence) before invoking Close — Close itself does a
// cooperative stdin-EOF + cmd.Wait.
func (t *SubprocessTransport) Cmd() *exec.Cmd { return t.cmd }

// Serve is a convenience entry point for subprocess main() functions.
// It creates a stdio Transport on os.Stdin/os.Stdout, calls handler for
// each incoming message, and blocks until stdin closes.
func Serve(handler func(ctx context.Context, msg json.RawMessage)) error {
	stream := New(os.Stdin, os.Stdout)
	defer stream.Close()
	ch, err := stream.Receive(context.Background())
	if err != nil {
		return err
	}
	for msg := range ch {
		handler(context.Background(), msg)
	}
	return nil
}
