// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	commsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comms/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// StrategyDef is the typed strategy definition for the orchestration plugin.
var StrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"orchestration", "Team coordination, messaging, and permission tools",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "orchestration",
			CreateFunc:  orchPluginFactory,
		}, nil
	},
)

func orchPluginFactory(ctx context.Context, loop agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	taskSvc, _ := application.UseService[taskapi.TaskService](ctx)
	if taskSvc == nil {
		return nil, nil
	}

	cwd, _ := os.Getwd()
	teamsDir := cwd
	if teamsDir == "" {
		teamsDir = "."
	}

	// Resolve agent mode from context (set by factory from AgentRunRequest.Mode)
	mode := agentapi.AgentModeFrom(ctx)

	teamMgr := NewTeamManager(teamsDir + "/.teams")
	swarmMgr := NewSwarmManager(taskSvc.BackgroundManager(), teamMgr)
	coordMgr := NewCoordinatorManager()
	mailboxMgr := NewMailboxManager()
	// Activate EventBus bridge: when attached, SendMessage also publishes
	// on to.<name>.message / group.<team>.message topics so workflow nodes
	// and event-driven subscribers can observe agent-to-agent messages.
	if ch := engine.ChannelFrom(ctx); ch != nil {
		mailboxMgr.SetEventBus(ch.Bus())
		// Team-lens narrative:
		// the TeamManager emits typed team.event payloads at every mutation;
		// the actor identity is filled on the first turn (SetActor below).
		teamMgr.SetEventBus(ch.Bus())
	}
	permMgr := NewPermissionManager()

	memMgr := runMemory(ctx, cwd)

	op := newOrchPlugin(mode, loop, teamMgr, swarmMgr, coordMgr, mailboxMgr, permMgr, memMgr)
	// Resolve the comms service so this agent, as a parent, can drain + answer
	// its children's bubbled questions and block in await-child — all through the
	// one user-comms channel (Dimension 5) that non-agent tasks share.
	op.comms, _ = application.UseService[commsapi.Service](ctx)
	return []agentapi.AgentPlugin{op}, nil
}

// runMemory is the memory a run reads: kept under the universe's tenant data
// root, keyed by the run's tenant, universe and person. The
// memory written before the wall - the home folder's and the working folder's
// - is moved into the quarantine once per process, never joined again.
func runMemory(ctx context.Context, workDir string) *MemoryManager {
	root := memoryRoot()
	if root == "" {
		return NewMemoryManager("", MemoryKey{})
	}
	legacyOnce.Do(func() {
		home, _ := os.UserHomeDir()
		_, _ = QuarantineLegacyMemory(root, LegacyMemoryDirs(home, workDir), time.Now())
	})
	tenant, universe := memoryPlace()
	return NewMemoryManager(root, MemoryKey{Tenant: tenant, Universe: universe, Person: runPerson(ctx)})
}

var legacyOnce sync.Once

// runPerson is the person a run acts for: its cause's person, or the person an
// agent's session relays; "" when the run names none (it then reads no memory).
func runPerson(ctx context.Context) string {
	c, ok := cause.CauseFrom(ctx)
	if !ok {
		return ""
	}
	if profileapi.IsPerson(cwmapi.Actor{ID: c.ID, Kind: c.Kind}) {
		return c.ID
	}
	if c.Relay != nil {
		return c.Relay.Person
	}
	return ""
}

type orchPlugin struct {
	mode       string // resolved delegation mode (worker, coordinator, team-leader, etc.)
	loop       agentapi.LoopContext
	teamMgr    *TeamManager
	swarmMgr   *SwarmManager
	coordMgr   *CoordinatorManager
	mailboxMgr *MailboxManager
	permMgr    *PermissionManager
	memMgr     *MemoryManager
	// comms is the user-communication channel (Dimension 5). As a parent this
	// agent drains its children's bubbled questions (PendingQuestions), answers
	// them (Answer), blocks for their next event (Await), and stops driven
	// children (Stop) — all through the one service non-agent tasks share.
	comms      commsapi.Service
	subscribed bool // tracks whether auto-subscribe has run
}

func newOrchPlugin(mode string, loop agentapi.LoopContext, teamMgr *TeamManager, swarmMgr *SwarmManager, coordMgr *CoordinatorManager, mailboxMgr *MailboxManager, permMgr *PermissionManager, memMgr *MemoryManager) *orchPlugin {
	return &orchPlugin{mode: mode, loop: loop, teamMgr: teamMgr, swarmMgr: swarmMgr, coordMgr: coordMgr, mailboxMgr: mailboxMgr, permMgr: permMgr, memMgr: memMgr}
}

// parentRunID returns this agent's own run ID, used as parent_run_id when
// spawning teammates. Reads from loop meta where the agent machine stores it.
func (p *orchPlugin) parentRunID() string {
	if p.loop == nil {
		return ""
	}
	if v, ok := p.loop.GetMeta("_runId").(string); ok {
		return v
	}
	return ""
}

// senderIdentity returns the authoritative `From` for outbound messages. Reads
// `_teammate_name` from loop meta (set by the agent factory at spawn time, per
// tier-1.1 spawn-policies refactor); falls back to "coordinator" for parent
// agents that have no teammate identity. Closes UC-063 / sec-3 / threat-T3
// partial: the LLM cannot influence this value — the From field is not part of
// the send-message tool's input schema.
func (p *orchPlugin) senderIdentity() string {
	if p.loop != nil {
		if name, ok := p.loop.GetMeta("_teammate_name").(string); ok && name != "" {
			return name
		}
	}
	return "coordinator"
}

func (p *orchPlugin) Name() string { return "orchestration" }
func (p *orchPlugin) SystemMessage() string {
	// NB: the "answer delegated agents" guidance is NOT a standing block — it is
	// delivered contextually instead, so it never sits dead in a run that does
	// no agent-as-human delegation (the common case, e.g. the compose wizard).
	// The delegateAgentAsHuman tool result spells out the answer-child mechanics
	// at spawn time, and every bubbled question is injected (formatChildQuestions)
	// already carrying the "answer with the answer-child tool" instruction. So
	// the prompt carries only the mode prompt + memory.
	prompt := p.coordMgr.GetModePrompt(p.mode)
	memPrompt := p.memMgr.LoadMemoryPrompt()
	if memPrompt != "" {
		prompt += "\n" + memPrompt + "\n"
	}
	return prompt
}
func (p *orchPlugin) Stable() bool { return true }

func (p *orchPlugin) Hooks() agentapi.PluginHooks {
	return agentapi.PluginHooks{
		// At turn start: (1) auto-subscribe the mailbox (teammate identity),
		// (2) drain any bubbled child questions into the conversation so the
		// parent answers them (agent-as-human).
		OnTurnStart: func(ctx agentapi.LoopContext, turn int) error {
			if err := p.autoSubscribeTopics(ctx, turn); err != nil {
				return err
			}
			return p.drainChildQuestions(ctx, turn)
		},
		OnDecision: func(ctx agentapi.LoopContext, decision *agentapi.Decision) error {
			// Reserved for future message-drain on tool-round boundaries.
			return nil
		},
	}
}

// drainChildQuestions pulls bubbled ask-user questions from this agent's
// children and injects them as a system message so the parent (acting as the
// human) answers them via the answer-child tool.
func (p *orchPlugin) drainChildQuestions(ctx agentapi.LoopContext, _ int) error {
	if p.comms == nil {
		return nil
	}
	parentID := p.parentRunID()
	if parentID == "" {
		return nil
	}
	qs := p.comms.PendingQuestions(context.Background(), parentID)
	if len(qs) == 0 {
		return nil
	}
	ctx.PushMessage(agentapi.NewTextMessage(agentapi.RoleSystem, formatChildQuestions(qs)))
	return nil
}

func formatChildQuestions(qs []agentapi.ChildQuestion) string {
	out := "A delegated agent is waiting for YOUR answer (you are its user). " +
		"Answer each with the **answer-child** tool (question_id + answer):\n"
	for _, q := range qs {
		name := q.ChildName
		if name == "" {
			name = q.ChildTaskID
		}
		out += fmt.Sprintf("- [question_id: %s] %s asks: %s", q.ID, name, q.Question)
		if len(q.Options) > 0 {
			out += " (options: " + strings.Join(q.Options, ", ") + ")"
		}
		out += "\n"
	}
	return out
}

// autoSubscribeTopics subscribes the local mailbox to to.<self>.message and
// group.<team>.message on the first turn this run takes in this process — its
// first turn, or the first after a restart (a restored run's turn is saved:
// keyed on turn 1, a restored teammate heard nothing). Idempotent — safe to
// call again. Also fills the TeamManager's actor identity (runID lands in loop
// meta after plugin creation, so this is the earliest reliable point).
func (p *orchPlugin) autoSubscribeTopics(ctx agentapi.LoopContext, _ int) error {
	// Only act once per process, and only if this agent has a teammate identity.
	if p.subscribed {
		return nil
	}
	name, _ := ctx.GetMeta("_teammate_name").(string)
	team, _ := ctx.GetMeta("_team_name").(string)
	p.teamMgr.SetActor(p.parentRunID(), name)
	if name == "" {
		return nil
	}
	p.mailboxMgr.EnsureMailbox(name)
	// UC-064: Set the teammate's own team on the mailbox so receiveEvent's
	// cross-team check has something to compare against. Subscribe routes
	// incoming events; SetTeam tells the receiver which team it belongs to.
	// Teammates do NOT call SetCoordinatorMode(true); foreign-team messages
	// are dropped.
	p.mailboxMgr.SetTeam(team)
	p.mailboxMgr.Subscribe(name, team)
	p.subscribed = true
	return nil
}

func (p *orchPlugin) ToolDefs() []agentapi.ToolDef {
	// Use typed tool definitions — schemas auto-generated from Go structs in tools.go
	return orchToolDefs()
}

func (p *orchPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	switch name {
	// Team tools
	case "create-team":
		return p.handleCreateTeam(input)
	case "spawn-teammate":
		return p.handleSpawnTeammate(input)
	case "list-team":
		return p.handleListTeam(input)
	case "complete-team":
		return p.handleCompleteTeam(input)
	// Messaging tools
	case "send-message":
		return p.handleSendMessage(input)
	case "check-messages":
		return p.handleCheckMessages(input)
	// Permission tools
	case "request-permission":
		return p.handleRequestPermission(input)
	case "resolve-permission":
		return p.handleResolvePermission(input)
	case "list-permissions":
		return p.handleListPermissions()
	// Agent-as-human: answer a bubbled child question
	case "answer-child":
		return p.handleAnswerChild(ctx, input)
	case "await-child":
		return p.handleAwaitChild(ctx, input)
	case "stop-child":
		return p.handleStopChild(ctx, input)
	default:
		return nil, fmt.Errorf("unknown tool: %q", name)
	}
}

// handleAnswerChild delivers this agent's answer to a child's bubbled ask-user
// question: take the question from the broker, then Resume the suspended child
// via the task SuspensionManager with the answer shaped as the ask_user output.
func (p *orchPlugin) handleAnswerChild(ctx context.Context, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var params struct {
		QuestionID string `json:"question_id"`
		Answer     string `json:"answer"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if p.comms == nil {
		return nil, fmt.Errorf("answer-child: no comms service available")
	}
	res, err := p.comms.Answer(ctx, commsapi.AnswerRequest{QuestionID: params.QuestionID, Answer: params.Answer})
	if err != nil {
		return nil, err
	}
	if !res.Delivered {
		// Already answered / unknown id — a no-op. Say so AND point forward, so
		// the driver stops re-answering defensively (the "4 no-op retries" waste):
		// the next move is always await-child, never another answer for this id.
		return agentapi.StringResult(res.Detail + " — already handled, no action needed. Call await-child for the child's next question or its completion."), nil
	}
	// Surface the START of the answer (one line) — the question id alone is
	// noise; the answer is what makes the parent↔child exchange readable live —
	// plus the forward instruction so the driver awaits rather than re-answers.
	return agentapi.StringResult("answered: " + res.Detail + " — now call await-child for the next question or completion; do not re-answer this question_id."), nil
}

// handleStopChild hard-stops a driven child. This is the parent-controlled
// terminator for a finish:driven child — which never completes itself — so the
// parent's await-child can return and it can move on.
func (p *orchPlugin) handleStopChild(ctx context.Context, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var params StopChildInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if params.TaskID == "" {
		return agentapi.StringResult("stop-child: task_id required"), nil
	}
	if p.comms == nil {
		return nil, fmt.Errorf("stop-child: no comms service available")
	}
	if err := p.comms.Stop(ctx, commsapi.StopRequest{ChildTaskID: params.TaskID, Reason: params.Reason}); err != nil {
		return agentapi.StringResult(fmt.Sprintf("stop-child %s: %v", params.TaskID, err)), nil
	}
	reason := params.Reason
	if reason == "" {
		reason = "stopped by parent"
	}
	return agentapi.StringResult(fmt.Sprintf("stopped child %s — %s", params.TaskID, reason)), nil
}

// handleAwaitChild BLOCKS until a delegated child's next lifecycle event — it
// asked a question, completed, or failed — so an agent_as_human parent drives
// its child WITHOUT busy-polling task-status. The wait, the engine-Channel
// subscription, and the parent-correlation match all live in the comms service,
// shared verbatim with non-agent drivers (the simulator).
func (p *orchPlugin) handleAwaitChild(ctx context.Context, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var params AwaitChildInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &params); err != nil {
			return nil, fmt.Errorf("parse input: %w", err)
		}
	}
	if p.comms == nil {
		return agentapi.StringResult("await-child: no comms service in this run — use task-status instead."), nil
	}
	ev, err := p.comms.Await(ctx, commsapi.AwaitRequest{
		ChildTaskID: params.TaskID,
		TimeoutS:    params.TimeoutS,
		ParentRunID: p.parentRunID(),
	})
	var await *agentapi.ErrAwait
	if errors.As(err, &await) {
		return nil, err // the loop waits as a machine and calls again
	}
	if err != nil {
		return agentapi.StringResult("await-child: " + err.Error()), nil
	}
	// On a terminal outcome, point the parent at the child's session id (carried
	// on the task record, NOT the task id) so it can session-read the child's run.
	if ev.Kind == "completed" || ev.Kind == "failed" {
		if params.TaskID != "" {
			return agentapi.StringResult(fmt.Sprintf(
				"%s — call task-status(%q) to get the child's session_id, then session-read THAT to inspect/score its run.",
				ev.Kind, params.TaskID)), nil
		}
	}
	return agentapi.StringResult(ev.Kind), nil
}

// --- Team handlers ---

func (p *orchPlugin) handleCreateTeam(input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var params struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	team, err := p.teamMgr.CreateTeam(params.Name, params.Description, "")
	if err != nil {
		return nil, err
	}
	// Configure the mailbox's team so group broadcasts land on group.<team>.message.
	// UC-064: also enable coordinator mode so this agent (the team-leader)
	// accepts incoming messages from any team — coordinators must receive
	// escalations from their team's teammates AND peer-coordinator messages
	// from other teams. Teammates leave coordinator mode off; their mailbox
	// drops cross-team events.
	if p.mailboxMgr != nil {
		p.mailboxMgr.SetTeam(team.Name)
		p.mailboxMgr.SetCoordinatorMode(true)
	}
	return agentapi.StringResult(fmt.Sprintf("Team %q created.", team.Name)), nil
}

func (p *orchPlugin) handleSpawnTeammate(input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var params struct {
		Team     string `json:"team"`
		TaskName string `json:"task_name"`
		Prompt   string `json:"prompt"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	result, err := p.swarmMgr.SpawnTeammate(SpawnConfig{
		TeamName:    params.Team,
		TaskName:    params.TaskName,
		Prompt:      params.Prompt,
		Name:        params.Name,
		ParentRunID: p.parentRunID(),
	})
	if err != nil {
		return nil, err
	}
	// Ensure mailbox for the new teammate
	p.mailboxMgr.EnsureMailbox(result.Name)
	data, _ := json.Marshal(result)
	return agentapi.StringResult(string(data)), nil
}

func (p *orchPlugin) handleListTeam(input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var params struct {
		Team string `json:"team"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	team, err := p.teamMgr.GetTeam(params.Team)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(team)
	return agentapi.StringResult(string(data)), nil
}

func (p *orchPlugin) handleCompleteTeam(input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var params struct {
		Team    string `json:"team"`
		Failed  bool   `json:"failed"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if err := p.teamMgr.CompleteTeam(params.Team, params.Failed, params.Summary); err != nil {
		return nil, err
	}
	outcome := "completed"
	if params.Failed {
		outcome = "failed"
	}
	return agentapi.StringResult(fmt.Sprintf("Team %q %s.", params.Team, outcome)), nil
}

// --- Messaging handlers ---

func (p *orchPlugin) handleSendMessage(input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var params struct {
		To      string `json:"to"`
		Message string `json:"message"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	result, err := p.mailboxMgr.SendMessage(p.senderIdentity(), params.To, params.Message, params.Summary, "")
	if err != nil {
		return nil, err
	}
	// Team-lens narrative: record THAT a message happened (summary only —
	// the content rides the mailbox/journal transcript, not the narrative).
	if team, _ := p.loop.GetMeta("_team_name").(string); team != "" {
		p.teamMgr.EmitMessage(team, p.senderIdentity(), params.To, params.Summary)
	} else if p.mailboxMgr != nil {
		if team := p.mailboxMgr.Team(); team != "" {
			p.teamMgr.EmitMessage(team, p.senderIdentity(), params.To, params.Summary)
		}
	}
	data, _ := json.Marshal(result)
	return agentapi.StringResult(string(data)), nil
}

func (p *orchPlugin) handleCheckMessages(input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var params struct {
		AgentName string `json:"agent_name"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	messages := p.mailboxMgr.ReadUnread(params.AgentName)
	p.mailboxMgr.MarkAsRead(params.AgentName)
	if len(messages) == 0 {
		return agentapi.StringResult("No unread messages."), nil
	}
	data, _ := json.Marshal(messages)
	return agentapi.StringResult(string(data)), nil
}

// --- Permission handlers ---

func (p *orchPlugin) handleRequestPermission(input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var params struct {
		ToolName string `json:"tool_name"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	id := p.permMgr.CreateRequest("", "worker", params.ToolName, nil)
	return agentapi.StringResult(fmt.Sprintf("Permission requested (id: %s). Waiting for leader approval.", id)), nil
}

func (p *orchPlugin) handleResolvePermission(input json.RawMessage) (*agentapi.ToolCallResult, error) {
	var params struct {
		RequestID string `json:"request_id"`
		Decision  string `json:"decision"`
		Feedback  string `json:"feedback"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if err := p.permMgr.ResolveRequest(params.RequestID, params.Decision, "leader", params.Feedback); err != nil {
		return nil, err
	}
	return agentapi.StringResult(fmt.Sprintf("Permission %s: %s", params.RequestID, params.Decision)), nil
}

func (p *orchPlugin) handleListPermissions() (*agentapi.ToolCallResult, error) {
	pending := p.permMgr.ListPending()
	if len(pending) == 0 {
		return agentapi.StringResult("No pending permission requests."), nil
	}
	data, _ := json.Marshal(pending)
	return agentapi.StringResult(string(data)), nil
}
