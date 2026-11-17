// SPDX-License-Identifier: Apache-2.0

// Package orchestration provides team coordination, coordinator mode,
// and swarm spawning for multi-agent orchestration.
package orchestration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TeamFile is the persistent state of a team.
type TeamFile struct {
	// ID is the stable team identity (uuidv7) carried by the correlation
	// envelope on every team event. Teams remain ADDRESSED by Name (tool
	// surface, mailbox topics); ID exists for cross-lens correlation and
	// is deliberately session-agnostic (cross-session teams stay possible).
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	LeadAgentID   string `json:"leadAgentId"`
	LeadSessionID string `json:"leadSessionId,omitempty"`
	// State is the team lifecycle FSM:
	// forming → active (first member) → done | failed.
	State     string       `json:"state,omitempty"`
	Members   []TeamMember `json:"members"`
	CreatedAt time.Time    `json:"createdAt"`
}

// TeamMember tracks a member of a team.
type TeamMember struct {
	AgentID     string `json:"agentId"`
	Name        string `json:"name"`
	AgentType   string `json:"agentType,omitempty"`
	Model       string `json:"model,omitempty"`
	Color       string `json:"color,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	BackendType string `json:"backendType,omitempty"` // "in-process", "tmux", "iterm2"
	IsActive    bool   `json:"isActive"`
	Mode        string `json:"mode,omitempty"` // "worker", "coordinator"
	// State is the member lifecycle: working → done | left.
	State string `json:"state,omitempty"`
}

// TeamManager manages team lifecycle and persistence — and is the single
// emission chokepoint for the team-lens narrative: every mutation (create / join /
// leave / state change / completion) publishes a typed engine.TeamEvent on
// the flat `team.event` topic, journaled by the session recorder.
type TeamManager struct {
	mu      sync.RWMutex
	baseDir string
	teams   map[string]*TeamFile // in-memory cache

	// Event emission context — set once at plugin wire-up via
	// SetEventContext. bus nil = silent (tests / legacy callers).
	bus      engine.EventBus
	selfRun  string // the acting agent's runID (emitter + correlation.run_id)
	memberID string // the acting agent's persona, when it is itself a teammate
}

// SetEventBus wires the shared EventBus at plugin creation time (mirrors
// the mailbox bridge wiring).
func (m *TeamManager) SetEventBus(bus engine.EventBus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bus = bus
}

// SetActor records the acting agent's identity: its runID (becomes the
// events' emitter — the recorder's per-run filter keys on it) and its
// persona when it is itself a teammate. Called on the first turn (the
// runID lands in loop meta after plugin creation).
func (m *TeamManager) SetActor(runID, memberID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.selfRun = runID
	m.memberID = memberID
}

// EmitMessage records a message in the team-lens narrative (summary only —
// no full text; content rides the mailbox, the narrative records THAT
// communication happened). No-op when the team is unknown. Nil-receiver
// safe (plugins constructed without a TeamManager in tests).
func (m *TeamManager) EmitMessage(teamName, from, to, summary string) {
	if m == nil {
		return
	}
	m.mu.RLock()
	team := m.teams[teamName]
	m.mu.RUnlock()
	if team == nil {
		return
	}
	m.emit(engine.TeamActionMessage, team, from, map[string]any{
		"to":      to,
		"summary": summary,
	})
}

// emit publishes one TeamEvent. Caller passes the action, the team, the
// affected member (may differ from the ACTING member in m.memberID), and
// action-specific detail. Nil bus = no-op.
func (m *TeamManager) emit(action string, team *TeamFile, memberID string, detail map[string]any) {
	if m.bus == nil || team == nil {
		return
	}
	evt := &engine.TeamEvent{
		Action:   action,
		TeamID:   team.ID,
		TeamName: team.Name,
		State:    team.State,
		MemberID: memberID,
		Detail:   detail,
	}
	evt.Correlation = engine.Correlation{
		RunID:    m.selfRun,
		TeamID:   team.ID,
		MemberID: memberID,
	}
	evt.StampEvent(engine.EventTeamEvent, m.selfRun)
	m.bus.Publish(engine.EventTeamEvent, evt)
}

var teamNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,30}$`)

// NewTeamManager creates a team manager that persists teams as JSON files.
func NewTeamManager(baseDir string) *TeamManager {
	return &TeamManager{
		baseDir: baseDir,
		teams:   make(map[string]*TeamFile),
	}
}

// CreateTeam creates a new team. Returns error if name already exists.
func (m *TeamManager) CreateTeam(name, description, leadAgentID string) (*TeamFile, error) {
	sanitized := sanitizeTeamName(name)
	if !teamNamePattern.MatchString(sanitized) {
		return nil, fmt.Errorf("invalid team name %q", name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check collision
	if _, exists := m.teams[sanitized]; exists {
		sanitized = resolveCollision(sanitized, m.teams)
	}

	team := &TeamFile{
		ID:          teamID(),
		Name:        sanitized,
		Description: description,
		LeadAgentID: leadAgentID,
		State:       engine.TeamStateForming,
		Members:     []TeamMember{},
		CreatedAt:   time.Now(),
	}

	m.teams[sanitized] = team
	_ = m.persistTeam(team)
	m.emit(engine.TeamActionCreated, team, "", map[string]any{"description": description})
	return team, nil
}

// teamID generates the stable team identity (uuidv7 — time-sortable like
// session IDs). Falls back to a nanosecond form if generation fails.
func teamID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Sprintf("team-%d", time.Now().UnixNano())
	}
	return id.String()
}

// DeleteTeam removes a team. Fails if active members exist.
func (m *TeamManager) DeleteTeam(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, ok := m.teams[name]
	if !ok {
		return fmt.Errorf("team %q not found", name)
	}

	for _, member := range team.Members {
		if member.IsActive {
			return fmt.Errorf("cannot delete team %q: active member %q exists", name, member.Name)
		}
	}

	delete(m.teams, name)
	_ = os.Remove(m.teamFile(name))
	return nil
}

// GetTeam returns a team by name.
func (m *TeamManager) GetTeam(name string) (*TeamFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	team, ok := m.teams[name]
	if !ok {
		return nil, fmt.Errorf("team %q not found", name)
	}
	return team, nil
}

// ListTeams returns all teams.
func (m *TeamManager) ListTeams() []*TeamFile {
	m.mu.RLock()
	defer m.mu.RUnlock()

	teams := make([]*TeamFile, 0, len(m.teams))
	for _, t := range m.teams {
		teams = append(teams, t)
	}
	return teams
}

// AddMember adds a member to a team. Emits member-joined; the FIRST member
// also transitions the team forming → active (emitting state-changed).
func (m *TeamManager) AddMember(teamName string, member TeamMember) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, ok := m.teams[teamName]
	if !ok {
		return fmt.Errorf("team %q not found", teamName)
	}

	member.IsActive = true
	member.State = engine.MemberStateWorking
	team.Members = append(team.Members, member)
	first := len(team.Members) == 1
	if first && team.State == engine.TeamStateForming {
		team.State = engine.TeamStateActive
	}
	_ = m.persistTeam(team)
	m.emit(engine.TeamActionMemberJoined, team, member.Name, map[string]any{
		"agent_id": member.AgentID,
		"mode":     member.Mode,
	})
	if first && team.State == engine.TeamStateActive {
		m.emit(engine.TeamActionStateChange, team, "", map[string]any{"from": engine.TeamStateForming})
	}
	return nil
}

// CompleteTeam transitions the team to its terminal state and emits
// completed (or failed). Idempotent: a second call on a terminal team is a
// no-op. summary lands in the event detail for the team-lens narrative.
func (m *TeamManager) CompleteTeam(teamName string, failed bool, summary string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, ok := m.teams[teamName]
	if !ok {
		return fmt.Errorf("team %q not found", teamName)
	}
	if team.State == engine.TeamStateDone || team.State == engine.TeamStateFailed {
		return nil
	}
	action := engine.TeamActionCompleted
	team.State = engine.TeamStateDone
	if failed {
		action = engine.TeamActionFailed
		team.State = engine.TeamStateFailed
	}
	for i := range team.Members {
		if team.Members[i].State == engine.MemberStateWorking {
			team.Members[i].State = engine.MemberStateDone
			team.Members[i].IsActive = false
		}
	}
	_ = m.persistTeam(team)
	m.emit(action, team, "", map[string]any{"summary": summary})
	return nil
}

// RemoveMember removes a member from a team by agent ID.
func (m *TeamManager) RemoveMember(teamName, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, ok := m.teams[teamName]
	if !ok {
		return fmt.Errorf("team %q not found", teamName)
	}

	for i, member := range team.Members {
		if member.AgentID == agentID {
			team.Members = append(team.Members[:i], team.Members[i+1:]...)
			_ = m.persistTeam(team)
			m.emit(engine.TeamActionMemberLeft, team, member.Name, map[string]any{"agent_id": agentID})
			return nil
		}
	}
	return fmt.Errorf("member %q not found in team %q", agentID, teamName)
}

// SetMemberActive updates a member's active status.
func (m *TeamManager) SetMemberActive(teamName, agentID string, active bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, ok := m.teams[teamName]
	if !ok {
		return fmt.Errorf("team %q not found", teamName)
	}

	for i := range team.Members {
		if team.Members[i].AgentID == agentID {
			team.Members[i].IsActive = active
			if active {
				team.Members[i].State = engine.MemberStateWorking
			} else {
				team.Members[i].State = engine.MemberStateDone
			}
			_ = m.persistTeam(team)
			m.emit(engine.TeamActionMemberStateChange, team, team.Members[i].Name, map[string]any{
				"agent_id": agentID,
				"state":    team.Members[i].State,
			})
			return nil
		}
	}
	return fmt.Errorf("member %q not found in team %q", agentID, teamName)
}

func (m *TeamManager) persistTeam(team *TeamFile) error {
	if err := os.MkdirAll(m.baseDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(team, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.teamFile(team.Name), data, 0o644)
}

func (m *TeamManager) teamFile(name string) string {
	return filepath.Join(m.baseDir, name+".json")
}

func sanitizeTeamName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = regexp.MustCompile(`[^a-z0-9_-]`).ReplaceAllString(name, "-")
	if len(name) > 30 {
		name = name[:30]
	}
	return name
}

func resolveCollision(name string, existing map[string]*TeamFile) string {
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if _, exists := existing[candidate]; !exists {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", name, time.Now().UnixMilli()%10000)
}
