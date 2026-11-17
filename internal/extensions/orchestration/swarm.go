// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// SwarmManager spawns and tracks teammates within a team.
type SwarmManager struct {
	mu      sync.RWMutex
	bgMgr   taskapi.BackgroundTaskManager
	teamMgr *TeamManager
	names   map[string]bool // tracks used names across all teams
}

// NewSwarmManager creates a swarm manager.
func NewSwarmManager(bgMgr taskapi.BackgroundTaskManager, teamMgr *TeamManager) *SwarmManager {
	return &SwarmManager{
		bgMgr:   bgMgr,
		teamMgr: teamMgr,
		names:   make(map[string]bool),
	}
}

// SpawnConfig describes how to spawn a teammate.
type SpawnConfig struct {
	TeamName    string `json:"teamName"`
	TaskName    string `json:"taskName"`
	Prompt      string `json:"prompt,omitempty"`
	Name        string `json:"name,omitempty"` // auto-generated if empty
	Model       string `json:"model,omitempty"`
	Color       string `json:"color,omitempty"`
	ParentRunID string `json:"parentRunId,omitempty"` // run id of the leader spawning this teammate
}

// SpawnResult is returned after spawning a teammate.
type SpawnResult struct {
	TaskID string `json:"taskId"`
	Name   string `json:"name"`
	Team   string `json:"team"`
}

// SpawnTeammate creates a new teammate in a team and starts it as a background task.
func (sm *SwarmManager) SpawnTeammate(config SpawnConfig) (*SpawnResult, error) {
	if config.TaskName == "" && config.Prompt == "" {
		return nil, fmt.Errorf("spawn teammate needs a task_name or a prompt")
	}

	// Validate team exists
	team, err := sm.teamMgr.GetTeam(config.TeamName)
	if err != nil {
		return nil, fmt.Errorf("spawn teammate: %w", err)
	}

	// Generate unique name if not provided
	name := config.Name
	if name == "" {
		name = sm.GenerateUniqueName(config.TeamName)
	}

	sm.mu.Lock()
	sm.names[name] = true
	sm.mu.Unlock()

	// Auto-assign color if not provided
	color := config.Color
	if color == "" {
		color = autoColor(len(team.Members))
	}

	// Register as background task.
	// ConfigOverride carries the teammate's identity so the child's orchestration
	// plugin can auto-subscribe to to.<name>.message and group.<team>.message.
	// MaxDurationS comes from the teammate preset's scheduling dimension —
	// the bg manager rejects unbounded background spawns, and omitting it
	// silently broke EVERY spawn-teammate once that validation landed.
	maxDurationS, _ := presets.SchedulingFieldsFromMode(presets.ModeTeammate)
	request := taskapi.TaskRunRequest{
		Input: map[string]any{
			"prompt":     config.Prompt,
			"user_input": config.Prompt,
		},
		ParentRunID:  config.ParentRunID,
		MaxDurationS: maxDurationS,
		ConfigOverride: map[string]any{
			"teammateName": name,
			"teamName":     config.TeamName,
		},
	}
	// Prompt-only teammates: with no registered task_name, run an inline
	// `type: agent` task built from the prompt — so a leader can spawn ad-hoc
	// teammates without a pre-registered TASK.md. The orchestration plugin is
	// REQUIRED here: it provides send-message / check-messages and drives the
	// auto-subscribe to to.<name>.message that teammates depend on; `standard`
	// alone (workspace + task) would leave a teammate unable to communicate.
	if config.TaskName == "" && config.Prompt != "" {
		request.TaskDef = map[string]any{
			"name":        "teammate-" + name,
			"type":        "agent",
			"description": "Prompt-based teammate",
			"capability":  "fast",
			"plugins":     []any{"standard", "orchestration"},
			"body":        config.Prompt,
		}
	} else {
		request.TaskName = config.TaskName
	}

	taskID, err := sm.bgMgr.Register(config.ParentRunID, request)
	if err != nil {
		return nil, fmt.Errorf("spawn teammate %q: %w", name, err)
	}

	// Add to team
	member := TeamMember{
		AgentID:     taskID,
		Name:        name,
		Model:       config.Model,
		Color:       color,
		BackendType: "in-process",
		IsActive:    true,
		Mode:        "worker",
	}
	_ = sm.teamMgr.AddMember(config.TeamName, member)

	return &SpawnResult{
		TaskID: taskID,
		Name:   name,
		Team:   config.TeamName,
	}, nil
}

// GenerateUniqueName creates a unique teammate name using word slugs.
func (sm *SwarmManager) GenerateUniqueName(teamName string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for i := 0; i < 100; i++ {
		name := randomWordSlug()
		if !sm.names[name] {
			return name
		}
	}
	return fmt.Sprintf("agent-%d", rand.Intn(10000))
}

// KillTeammate stops a teammate by task ID and removes from team.
func (sm *SwarmManager) KillTeammate(teamName, taskID string) error {
	if err := sm.bgMgr.Kill(taskID); err != nil {
		return err
	}
	return sm.teamMgr.SetMemberActive(teamName, taskID, false)
}

var wordPool = []string{
	"alpha", "beta", "gamma", "delta", "echo", "fox", "golf", "hotel",
	"iris", "jade", "kilo", "lima", "mike", "nova", "oscar", "papa",
	"quest", "romeo", "sierra", "tango", "ultra", "victor", "whiskey",
	"xray", "yankee", "zulu", "amber", "blaze", "coral", "dawn",
}

func randomWordSlug() string {
	a := wordPool[rand.Intn(len(wordPool))]
	b := wordPool[rand.Intn(len(wordPool))]
	return strings.ToLower(a + "-" + b)
}

var colorPool = []string{
	"#e06c75", "#98c379", "#e5c07b", "#61afef", "#c678dd",
	"#56b6c2", "#be5046", "#d19a66", "#a0a1a7", "#7c8590",
}

func autoColor(index int) string {
	return colorPool[index%len(colorPool)]
}
