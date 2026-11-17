// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/extensions/testharness/api"
)

// TestHarnessService manages process sessions and serves the HTTP API.
// Captures KTW_WORKSPACE and CWD at creation time so spawned processes get
// the same environment as the server.
type TestHarnessService struct {
	mu      sync.RWMutex
	session *ProcessSession // one active session (v1)
	counter int

	// Captured at server start — passed to every spawned process
	startupWorkspace string
	startupCWD       string
}

// NewTestHarnessService creates the service, capturing the current
// KTW_WORKSPACE and working directory for spawned processes.
func NewTestHarnessService() *TestHarnessService {
	cwd, _ := os.Getwd()
	return &TestHarnessService{
		startupWorkspace: os.Getenv("KTW_WORKSPACE"),
		startupCWD:       cwd,
	}
}

// Handler returns an http.Handler with all API routes.
func (s *TestHarnessService) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /run", s.handleRun)
	mux.HandleFunc("GET /output", s.handleOutput)
	mux.HandleFunc("POST /input", s.handleInput)
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("POST /kill", s.handleKill)
	mux.HandleFunc("GET /health", s.handleHealth)
	return mux
}

func (s *TestHarnessService) handleRun(w http.ResponseWriter, r *http.Request) {
	var req api.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request: %v", err)
		return
	}
	if req.Cmd == "" {
		httpError(w, http.StatusBadRequest, "cmd is required")
		return
	}

	// Default CWD to the server's startup directory
	if req.Dir == "" {
		req.Dir = s.startupCWD
	}

	// Inject captured KTW_WORKSPACE so spawned process uses the same config
	if s.startupWorkspace != "" {
		req.Env = append(req.Env, "KTW_WORKSPACE="+s.startupWorkspace)
	}

	// Kill existing session if any
	s.mu.Lock()
	if s.session != nil && s.session.Running() {
		_ = s.session.Kill()
	}
	s.counter++
	id := fmt.Sprintf("session-%d", s.counter)
	s.mu.Unlock()

	session, err := NewProcessSession(id, req)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "spawn failed: %v", err)
		return
	}

	s.mu.Lock()
	s.session = session
	s.mu.Unlock()

	writeJSON(w, api.RunResponse{
		SessionID: session.ID(),
		PID:       session.PID(),
	})
}

func (s *TestHarnessService) handleOutput(w http.ResponseWriter, r *http.Request) {
	session := s.getSession()
	if session == nil {
		httpError(w, http.StatusNotFound, "no active session")
		return
	}

	var since int64
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		since, _ = strconv.ParseInt(sinceStr, 10, 64)
	}

	lines := session.Output(since)
	if lines == nil {
		lines = []api.OutputLine{}
	}
	writeJSON(w, lines)
}

func (s *TestHarnessService) handleInput(w http.ResponseWriter, r *http.Request) {
	session := s.getSession()
	if session == nil {
		httpError(w, http.StatusNotFound, "no active session")
		return
	}

	var req api.InputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request: %v", err)
		return
	}

	if err := session.SendInput(req.Text); err != nil {
		httpError(w, http.StatusInternalServerError, "send input: %v", err)
		return
	}

	writeJSON(w, map[string]bool{"sent": true})
}

func (s *TestHarnessService) handleStatus(w http.ResponseWriter, r *http.Request) {
	session := s.getSession()
	if session == nil {
		httpError(w, http.StatusNotFound, "no active session")
		return
	}
	writeJSON(w, session.Status())
}

func (s *TestHarnessService) handleKill(w http.ResponseWriter, r *http.Request) {
	session := s.getSession()
	if session == nil {
		httpError(w, http.StatusNotFound, "no active session")
		return
	}

	if err := session.Kill(); err != nil {
		httpError(w, http.StatusInternalServerError, "kill: %v", err)
		return
	}

	writeJSON(w, map[string]bool{"killed": true})
}

func (s *TestHarnessService) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"status":   "ok",
		"cwd":      s.startupCWD,
		"ktw_home": s.startupWorkspace,
	})
}

func (s *TestHarnessService) getSession() *ProcessSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func httpError(w http.ResponseWriter, code int, format string, args ...any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": fmt.Sprintf(format, args...),
	})
}
