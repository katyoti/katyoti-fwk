// SPDX-License-Identifier: Apache-2.0

// Package core holds the Claude Code workers of this process: it starts each
// on the person's own subscription (the driver's guard), within the cap, and
// answers for them by id.
package core

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/internal/driver"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/internal/process"
)

// ClaudeCodeConfig is ClaudeCode.yaml.
type ClaudeCodeConfig struct {
	// ClaudePath is the claude binary; empty: claude.exe on PATH, else
	// ~/.local/bin. A .cmd or .bat is refused.
	ClaudePath string `yaml:"claude_path,omitempty" json:"claude_path,omitempty"`
	// GitBashPath is the Git Bash Claude Code runs its shell in on Windows;
	// empty: the usual install places.
	GitBashPath string `yaml:"git_bash_path,omitempty" json:"git_bash_path,omitempty"`
	// PermissionMode is a worker's mode when its start names none: auto, as
	// in the person's own sessions (decision).
	PermissionMode string `yaml:"permission_mode,omitempty" json:"permission_mode,omitempty"`
	// SettingSources are the settings a worker reads: the person's own rules
	// apply (decision).
	SettingSources []string `yaml:"setting_sources,omitempty" json:"setting_sources,omitempty"`
	// MaxWorkers is how many workers run at once (decision).
	MaxWorkers int `yaml:"max_workers,omitempty" json:"max_workers,omitempty"`
	// RemoteControl opens every worker to claude.ai/code and the Claude app
	// unless its start says otherwise.
	RemoteControl bool `yaml:"remote_control,omitempty" json:"remote_control,omitempty"`
}

// Defaults.
const (
	DefaultPermissionMode = api.PermissionModeAuto
	DefaultMaxWorkers     = 2
)

// DefaultSettingSources are the person's user, project and local settings.
var DefaultSettingSources = []string{"user", "project", "local"}

// Starter starts a claude process; tests replace it with the fake.
type Starter func(spec process.Spec) (driver.Conn, error)

// Service implements api.ClaudeCode.
type Service struct {
	cfg   ClaudeCodeConfig
	start Starter

	mu      sync.Mutex
	workers map[string]*driver.Worker
}

var _ api.ClaudeCode = (*Service)(nil)

// New is the service over cfg, starting real processes.
func New(cfg ClaudeCodeConfig) *Service {
	return NewWith(cfg, func(s process.Spec) (driver.Conn, error) { return process.Start(s) })
}

// NewWith is the service over cfg and a starter.
func NewWith(cfg ClaudeCodeConfig, start Starter) *Service {
	if cfg.PermissionMode == "" {
		cfg.PermissionMode = DefaultPermissionMode
	}
	if len(cfg.SettingSources) == 0 {
		cfg.SettingSources = DefaultSettingSources
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = DefaultMaxWorkers
	}
	return &Service{cfg: cfg, start: start, workers: map[string]*driver.Worker{}}
}

// Start starts a worker. Refused at the cap, or while the subscription's
// rate limit rejects requests.
func (s *Service) Start(ctx context.Context, in api.StartInput) (api.Worker, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return api.Worker{}, fmt.Errorf("claude: a worker starts on a prompt")
	}
	s.mu.Lock()
	live := 0
	for _, w := range s.workers {
		i := w.Info()
		if i.State.Ended() {
			continue
		}
		live++
		if i.RateLimit == "rejected" {
			s.mu.Unlock()
			return api.Worker{}, fmt.Errorf("claude: the subscription's rate limit rejects requests (worker %s); no new worker until it resets", i.ID)
		}
	}
	if live >= s.cfg.MaxWorkers {
		s.mu.Unlock()
		return api.Worker{}, fmt.Errorf("claude: %d workers already run (max_workers %d in ClaudeCode.yaml)", live, s.cfg.MaxWorkers)
	}
	s.mu.Unlock()

	bin, err := process.Find(s.cfg.ClaudePath)
	if err != nil {
		return api.Worker{}, err
	}
	mode := in.PermissionMode
	if mode == "" {
		mode = s.cfg.PermissionMode
	}
	args, err := process.Args(process.Options{Model: in.Model, PermissionMode: mode, AllowedTools: in.AllowedTools,
		DisallowedTools: in.DisallowedTools, AppendSystemPrompt: in.AppendSystemPrompt, MaxTurns: in.MaxTurns,
		MaxBudgetUSD: in.MaxBudgetUSD, Resume: in.Resume, Fork: in.Fork, SessionID: in.SessionID, Name: in.Name,
		SettingSources: s.cfg.SettingSources})
	if err != nil {
		return api.Worker{}, err
	}
	cwd := in.Cwd
	if cwd == "" {
		if cwd, err = os.Getwd(); err != nil {
			return api.Worker{}, err
		}
	}
	conn, err := s.start(process.Spec{Path: bin, Args: args, Env: process.Env(os.Environ(), process.GitBash(s.cfg.GitBashPath)), Dir: cwd})
	if err != nil {
		return api.Worker{}, err
	}
	w := driver.New(conn, api.Worker{Name: in.Name, Cwd: cwd, Model: in.Model})
	s.mu.Lock()
	s.workers[w.Info().ID] = w
	s.mu.Unlock()
	// The worker outlives the call that starts it.
	wctx := context.WithoutCancel(ctx)
	if !in.RemoteControl && !s.cfg.RemoteControl {
		if err := w.Start(wctx, in.Prompt); err != nil {
			return w.Info(), err
		}
		return w.Info(), nil
	}
	// Remote Control opens before the first prompt, so its page shows the
	// whole conversation; a refusal leaves the worker running without it.
	if err := w.Start(wctx, ""); err != nil {
		return w.Info(), err
	}
	name := in.RemoteControlName
	if name == "" {
		name = in.Name
	}
	if name == "" {
		name = "katyoti-" + w.Info().ID
	}
	_, _ = w.OpenRemote(wctx, name)
	if err := w.Send(in.Prompt); err != nil {
		return w.Info(), err
	}
	return w.Info(), nil
}

func (s *Service) worker(id string) (*driver.Worker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workers[id]
	if !ok {
		return nil, fmt.Errorf("claude: no worker %q", id)
	}
	return w, nil
}

// Send writes the next user message.
func (s *Service) Send(_ context.Context, in api.SendInput) (api.Worker, error) {
	w, err := s.worker(in.ID)
	if err != nil {
		return api.Worker{}, err
	}
	if err := w.Send(in.Prompt); err != nil {
		return w.Info(), err
	}
	return w.Info(), nil
}

// Answer answers the permission the worker waits on.
func (s *Service) Answer(_ context.Context, in api.AnswerInput) (api.Worker, error) {
	w, err := s.worker(in.ID)
	if err != nil {
		return api.Worker{}, err
	}
	if err := w.Answer(in); err != nil {
		return w.Info(), err
	}
	return w.Info(), nil
}

// Interrupt ends the current turn.
func (s *Service) Interrupt(ctx context.Context, in api.WorkerInput) (api.Worker, error) {
	w, err := s.worker(in.ID)
	if err != nil {
		return api.Worker{}, err
	}
	if err := w.Interrupt(ctx); err != nil {
		return w.Info(), err
	}
	return w.Info(), nil
}

// Stop ends the worker cleanly.
func (s *Service) Stop(_ context.Context, in api.WorkerInput) (api.Worker, error) {
	w, err := s.worker(in.ID)
	if err != nil {
		return api.Worker{}, err
	}
	w.Stop()
	return w.Info(), nil
}

// Get reads one worker.
func (s *Service) Get(_ context.Context, in api.WorkerInput) (api.Worker, error) {
	w, err := s.worker(in.ID)
	if err != nil {
		return api.Worker{}, err
	}
	return w.Info(), nil
}

// List reads the workers, the earliest started first.
func (s *Service) List(_ context.Context, in api.ListInput) (api.ListOutput, error) {
	s.mu.Lock()
	out := make([]api.Worker, 0, len(s.workers))
	for _, w := range s.workers {
		if i := w.Info(); in.All || !i.State.Ended() {
			out = append(out, i)
		}
	}
	s.mu.Unlock()
	sort.Slice(out, func(a, b int) bool { return out[a].StartedAt.Before(out[b].StartedAt) })
	return api.ListOutput{Workers: out}, nil
}

// Follow is a worker's events so far and from then on (the foreground run
// prints them).
func (s *Service) Follow(id string) ([]api.Event, <-chan api.Event, func(), error) {
	w, err := s.worker(id)
	if err != nil {
		return nil, nil, nil, err
	}
	past, ch, cancel := w.Follow()
	return past, ch, cancel, nil
}

// Close stops every worker (the process is ending).
func (s *Service) Close() error {
	s.mu.Lock()
	ws := make([]*driver.Worker, 0, len(s.workers))
	for _, w := range s.workers {
		ws = append(ws, w)
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, w := range ws {
		wg.Add(1)
		go func(w *driver.Worker) { defer wg.Done(); w.Stop() }(w)
	}
	wg.Wait()
	return nil
}
