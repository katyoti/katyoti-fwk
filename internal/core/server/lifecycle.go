// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// StartFunc is the server-specific function that starts listening.
// It must block until ctx is cancelled or the server exits.
type StartFunc func(ctx context.Context) error

// HealthFunc checks if the server described by info is healthy.
type HealthFunc func(info ServerInfo) bool

// StopFunc performs a graceful shutdown of the server described by info.
type StopFunc func(info ServerInfo) error

// Lifecycle manages the lifecycle of a named server process.
// It handles info file management, background process spawning,
// health checking, and graceful shutdown.
type Lifecycle struct {
	// Name identifies this server (used for info file and log file).
	Name string

	// SubCommand is the cobra command group name (e.g., "sm", "server").
	SubCommand string

	// Version is written to the info file.
	Version string

	// Host and Port for the server to listen on.
	Host string
	Port int

	// URL is the connection URL written to the info file (e.g., "ws://localhost:7770").
	URL string

	// FgArgs are the CLI args used to re-exec in foreground mode (e.g., ["sm", "start", "--fg"]).
	FgArgs []string

	// Start is the server-specific start function.
	Start StartFunc

	// Health checks if the server is alive. Defaults to TCPCheck if nil.
	Health HealthFunc

	// Stop performs graceful shutdown. If nil, sends SIGTERM to PID.
	Stop StopFunc
}

// StartForeground runs the server in the foreground with signal handling.
// Blocks until the server exits or a signal is received.
func (l *Lifecycle) StartForeground() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Write info file
	exePath, _ := os.Executable()
	info := ServerInfo{
		Name:    l.Name,
		PID:     os.Getpid(),
		URL:     l.URL,
		Host:    l.Host,
		Port:    l.Port,
		Started: time.Now(),
		Version: l.Version,
		Exe:     exePath,
	}
	if err := WriteInfo(info); err != nil {
		return fmt.Errorf("write info file: %w", err)
	}

	// Remove info file on exit
	defer RemoveInfo(l.Name)

	fmt.Printf("Starting %s on %s\n", l.Name, l.URL)
	return l.Start(ctx)
}

// StartBackgroundProcess spawns the server as a detached background process.
// Waits briefly and checks health before returning.
func (l *Lifecycle) StartBackgroundProcess() error {
	// Check if already running
	if l.isRunning() {
		fmt.Printf("%s is already running.\n", l.Name)
		return nil
	}

	logPath := l.logPath()
	pid, err := StartBackground(l.FgArgs, logPath)
	if err != nil {
		return err
	}

	fmt.Printf("Starting %s in background (PID %d)...\n", l.Name, pid)
	fmt.Printf("Log file: %s\n", logPath)

	time.Sleep(1500 * time.Millisecond)

	if l.checkHealth() {
		fmt.Printf("%s started successfully.\n", l.Name)
	} else {
		fmt.Printf("%s may not have started. Check the log file.\n", l.Name)
	}
	return nil
}

// DoStop stops the running server.
func (l *Lifecycle) DoStop() error {
	info, err := ReadInfo(l.Name)
	if err != nil {
		fmt.Printf("%s is not running.\n", l.Name)
		return nil
	}

	if !IsProcessAlive(info.PID) {
		_ = RemoveInfo(l.Name)
		fmt.Printf("%s is not running (stale info file cleaned).\n", l.Name)
		return nil
	}

	if l.Stop != nil {
		if err := l.Stop(info); err != nil {
			return fmt.Errorf("stop %s: %w", l.Name, err)
		}
	} else {
		// Default: kill process by PID
		p, err := os.FindProcess(info.PID)
		if err != nil {
			return fmt.Errorf("find process %d: %w", info.PID, err)
		}
		if err := p.Signal(syscall.SIGTERM); err != nil {
			// On Windows SIGTERM may not work; try Kill
			_ = p.Kill()
		}
	}

	fmt.Printf("%s stopped.\n", l.Name)
	return nil
}

// DoRestart stops the server (if running) and starts it again.
func (l *Lifecycle) DoRestart(fg bool) error {
	if l.isRunning() {
		if err := l.DoStop(); err != nil {
			return err
		}
		time.Sleep(1500 * time.Millisecond)
	}

	if fg {
		return l.StartForeground()
	}
	return l.StartBackgroundProcess()
}

// DoStatus prints the server status.
func (l *Lifecycle) DoStatus() error {
	info, err := ReadInfo(l.Name)
	if err != nil {
		fmt.Printf("%s is not running.\n", l.Name)
		return nil
	}

	if !IsProcessAlive(info.PID) {
		_ = RemoveInfo(l.Name)
		fmt.Printf("%s is not running (stale info file cleaned).\n", l.Name)
		return nil
	}

	healthy := l.checkHealth()
	status := "running"
	if !healthy {
		status = "running (not responding)"
	}

	fmt.Printf("Name:    %s\n", info.Name)
	fmt.Printf("Status:  %s\n", status)
	fmt.Printf("PID:     %d\n", info.PID)
	fmt.Printf("URL:     %s\n", info.URL)
	fmt.Printf("Port:    %d\n", info.Port)
	fmt.Printf("Started: %s\n", info.Started.Format(time.RFC3339))
	fmt.Printf("Version: %s\n", info.Version)
	return nil
}

func (l *Lifecycle) isRunning() bool {
	CleanStaleInfo(l.Name)
	return l.checkHealth()
}

func (l *Lifecycle) checkHealth() bool {
	if l.Health != nil {
		info, err := ReadInfo(l.Name)
		if err != nil {
			return false
		}
		return l.Health(info)
	}
	return TCPCheck(l.Host, l.Port, 2*time.Second)
}

func (l *Lifecycle) logPath() string {
	logsDir := filepath.Join(homeDir, "logs")
	_ = os.MkdirAll(logsDir, 0o755)
	return filepath.Join(logsDir, l.Name+".log")
}
