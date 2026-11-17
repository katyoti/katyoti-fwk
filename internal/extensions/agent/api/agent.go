// SPDX-License-Identifier: Apache-2.0

package api

// AgentRunRequest is the config struct for the agent-loop machine strategy.
// 100% JSON-serializable — this IS the checkpoint seed.
// Used by AgentMachineFactory.CreateMachine() to configure the agent loop.
// Can be passed directly (*AgentRunRequest) or as map[string]any (from TaskService).
type AgentRunRequest struct {
	// Name identifies this run (for logging and session recording).
	Name string `json:"name" yaml:"name" desc:"Run name"`

	// Prompt is the instruction text for the agent.
	Prompt string `json:"prompt" yaml:"body" desc:"Instruction text"`

	// ProfileName selects the LLM profile (e.g., "smart", "fast", "powerful").
	// The yaml key `capability` is a WIRE format used by ~80 shipped configs and
	// names an LLM PROFILE. It is neither a CWM capability nor an agent plugin;
	// the key stays for compatibility and the word is not reused elsewhere here.
	ProfileName string `json:"profileName,omitempty" yaml:"capability" desc:"LLM profile name"`

	// Plugins lists plugin references to activate for this run.
	// Each entry is a string "type:name" or a mapping {type: "...", name: "...", ...props}.
	Plugins []PluginRef `json:"plugins,omitempty" yaml:"plugins" desc:"Plugin references"`

	// Skills lists skill selectors — sugar that desugars into `skill:` plugin
	// refs (DesugarPluginLists). Forms: a name, a glob (`cwm-*`), `*` (all
	// advertised), or provider-scoped (`<provider>:/skill/<glob>`).
	Skills []string `json:"skills,omitempty" yaml:"skills" desc:"Skill selectors (name | glob | <provider>:/skill/<glob>); * = all advertised"`

	// Tools lists action/tool URI selectors — sugar that desugars into `tool:`
	// plugin refs (the catalog-driven tool proxy). Each resolved URI becomes a
	// native typed agent tool. Forms: `<provider>:<slot>/<method>`,
	// `<provider>:<slot>/*`, `<provider>:tasks/<glob>`, `*` (all advertised).
	Tools []string `json:"tools,omitempty" yaml:"tools" desc:"Tool/action URI selectors; * = all advertised"`

	// MaxTurns overrides the profile's max_turns (0 = use profile default).
	MaxTurns int `json:"maxTurns,omitempty" yaml:"max_turns" desc:"Maximum agent turns"`

	// MaxBudgetUSD is the cost ceiling: once the run's model calls have cost
	// this much (priced by the model's configured rates), no further call is
	// made and the run ends "budget_exceeded". 0 = unlimited.
	MaxBudgetUSD float64 `json:"maxBudgetUSD,omitempty" yaml:"max_budget_usd" desc:"Maximum cost in USD"`

	// Overrides are provider-specific config overrides applied on top of the profile.
	Overrides map[string]any `json:"overrides,omitempty" desc:"Provider-specific overrides"`

	// WorkspaceRoot is the workspace directory for file operations.
	WorkspaceRoot string `json:"workspaceRoot,omitempty" desc:"Workspace directory"`

	// Variables are resolved built-in variables (workspace, config_dir, etc.).
	Variables map[string]string `json:"variables,omitempty" desc:"Resolved built-in variables"`

	// Context holds additional key-value pairs available to plugins.
	Context map[string]any `json:"context,omitempty" desc:"Plugin context variables"`

	// InitialMessages provides pre-populated conversation history.
	// Used by fork mode to pass the parent's conversation context to the child.
	// When set, replaces the default "Please start the task." initial message.
	InitialMessages []Message `json:"initialMessages,omitempty" desc:"Pre-populated conversation (fork mode)"`

	// IsolationMode declares how the child's lifecycle relates to the parent:
	//   - "in_process" (default): child shares parent's context; parent cancel cascades
	//   - "context_isolated": child has an independent abort signal; parent cancel
	//     does NOT close the child's abort. Used for teammates.
	//   - "pane" / "remote": external isolation — handled by ExternalSpawner
	//     (not implemented here; see spawn-behavior-isolation-external)
	IsolationMode string `json:"isolationMode,omitempty" desc:"Isolation mode (in_process|context_isolated|pane|remote)"`

	// HistoryMode declares how parent conversation flows to this child:
	//   - "none" / "prompt_only" (default): no parent context — child uses task body
	//   - "full_fork": parent's InitialMessages copied (set by ForkManager)
	//   - "summary": pre-spawn summarizer call (NOT IMPLEMENTED — returns error)
	// In the current implementation "none" and "prompt_only" are equivalent runtime
	// behavior (no InitialMessages). The field is kept for explicit dimension
	// declaration and future evolution.
	HistoryMode string `json:"historyMode,omitempty" desc:"Conversation history mode (none|prompt_only|full_fork|summary)"`

	// Mode is the delegation mode for this agent run.
	// Determines which behavioral prompt is injected into the system message.
	// Values: worker (default), background, coordinator, team-leader, teammate,
	// fork, isolated, fork-isolated, chat, remote.
	//
	// `chat` switches the loop into interactive-chat semantics:
	//
	//   1. The `complete` tool is filtered from every plugin's
	//      `ToolDefs()` output — the LLM never sees it, never tries
	//      to call it.
	//   2. The keepalive OnDecision hook is auto-attached: a
	//      text-only turn loops back with a user-role nudge to call
	//      `ask-user`. The user owns session lifetime — closing the
	//      chat tab is the only termination signal.
	//   3. Idle-declared tools (ask-user) are exempt from the
	//      loop-safety repetition guard.
	//   4. The chat preset's UserComms default is `direct` (HumanGate
	//      prompts render inline).
	//
	// NOTE: the chat loop SEMANTICS (1-3 above) are no longer keyed off the mode
	// name — they are driven by Finish (below). `chat` is simply the preset whose
	// Finish default is `driven`.
	Mode string `json:"mode,omitempty" yaml:"mode" desc:"Delegation mode"`

	// Finish is the loop-lifecycle dimension — HOW this run terminates
	// (presets.FinishMode: "self" | "driven"). Empty resolves from the mode's
	// preset (chat → driven, else self), unless explicitly set at launch
	// (delegate `finish:` / TASK.md). It is the single switch the assembly reads
	// to pick the loop's terminator:
	//   - self:   `complete` exposed, no keepalive — the run ends itself.
	//   - driven: `complete` hidden + keepalive — ends only on external stop
	//             (parent stop-child hard-kill, or human quit).
	Finish string `json:"finish,omitempty" yaml:"finish" desc:"Loop termination: self (completes itself) | driven (external stop)"`

	// ParentRunID identifies the parent run that spawned this run via delegate
	// or spawn_teammate. Empty for top-level runs. Logged in the session_start
	// event so reports can reconstruct the delegation tree.
	ParentRunID string `json:"parentRunId,omitempty" desc:"Parent run ID for delegation chain"`

	// SessionID identifies the session this run records into (the
	// session folder name under SessionsDir). Copied from
	// TaskExecRequest.Runtime by the factory; used to place
	// session-scoped artifacts like llm-model-calls.log.
	SessionID string `json:"sessionId,omitempty" desc:"Owning session ID"`

	// TeammateName is this agent's name in a team (set when spawned via spawn_teammate).
	// Used by orchestration plugin to auto-subscribe to to.<name>.message topic.
	TeammateName string `json:"teammateName,omitempty" desc:"Teammate name for team-based messaging"`

	// TeamName is the team this teammate belongs to.
	// Used by orchestration plugin to auto-subscribe to group.<team>.message topic.
	TeamName string `json:"teamName,omitempty" desc:"Team name for group broadcast subscriptions"`

	// SpawnPermissions carries per-spawn tool access restrictions from the parent.
	// Nil means no per-spawn restrictions (full permissions subject to user profile).
	// Composes with the user-curated permission profile: this layer is parent-controlled,
	// the profile is user-controlled; both pass through the permission gate.
	SpawnPermissions *SpawnPermissions `json:"spawnPermissions,omitempty" desc:"Per-spawn tool access restrictions from parent"`

	// WorkspacePaths carries per-spawn path-level access restrictions.
	// Nil means no path filter (full workspace access).
	// Enforced via the workspace path hook (OnPreToolExec) before file tool dispatch.
	WorkspacePaths *WorkspacePaths `json:"workspacePaths,omitempty" desc:"Per-spawn path-level access restrictions"`

	// UserCommsInput is the resolved channel for ask_user calls,
	// computed by EffectiveUserCommsInput(presetDefault, override) at
	// task-dispatch time.
	//
	// Values: "none" | "direct" | "inbox" | "bubble_to_parent" |
	// "via_leader". Empty = resolver hasn't run (legacy programmatic
	// paths); downstream code treats it as "direct" for safety so
	// chat-style flows that bypass the dispatch helper still work.
	//
	// The auto-detection rule:
	//   - explicit override in TASK.md frontmatter wins over everything
	//   - else: the preset default for the resolved Mode (chat → InputDirect;
	//     system-spawned modes → InputInbox; teammate → InputViaLeader;
	//     remote → InputNone)
	UserCommsInput string `json:"userCommsInput,omitempty" yaml:"user_comms_input" desc:"Resolved channel for ask_user calls (none|direct|inbox|bubble_to_parent|via_leader)"`
}

// WorkspacePaths describes per-spawn path-level access restrictions enforced by
// the workspace path hook. Read-only tools (file_read, glob, grep) check
// against ReadPaths; write tools (file_write, file_edit) check against WritePaths.
//
// Path matching:
//   - Empty list (or nil) = no restriction (allow all)
//   - "*" or "**" entry = wildcard, allow all
//   - Otherwise: prefix match against the tool's path argument
//     (workspace_path / absolute_path / file_path / path)
type WorkspacePaths struct {
	ReadPaths  []string `json:"readPaths,omitempty"`
	WritePaths []string `json:"writePaths,omitempty"`

	// URLAllowlist gates fetch tools (`rest_api`, `web_fetch`, future MCP
	// fetch verbs). Closes UC-058's URL/SSRF half + threat-T2-critical.
	//
	// Semantics — symmetric to ReadPaths/WritePaths but operates on URL
	// hosts:
	//   - Empty list (or nil) = no restriction at the host level. (When
	//     combined with restrictive ReadPaths/WritePaths the fetch tools
	//     are denied at the per-tool gate; opt-in by setting URLAllowlist.)
	//   - Wildcard `*` = any host. Authors who genuinely want unrestricted
	//     fetch in a restrictive workspace declare this explicitly.
	//   - Exact host: `api.example.com` matches that host on any scheme.
	//   - Wildcard subdomain: `*.example.com` matches all subdomains of
	//     example.com (but NOT example.com itself).
	//   - Scheme constraint: `https://api.example.com` matches only that
	//     scheme + host combo.
	//
	// **Private-network deny is ALWAYS on** regardless of the allowlist,
	// per the OWASP SSRF cheatsheet. The denied ranges:
	//   - 127.0.0.0/8 (loopback), ::1
	//   - 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 (RFC 1918)
	//   - 169.254.0.0/16 (link-local; AWS/GCP metadata endpoint)
	//   - fc00::/7 (IPv6 unique local), fe80::/10 (IPv6 link-local)
	URLAllowlist []string `json:"urlAllowlist,omitempty"`
}

// SpawnPermissions describes per-spawn tool access restrictions enforced by the
// spawn permission hook. See internal/extensions/agent/internal/runtime/spawn_permission_hook.go.
//
// Fail-closed by default: a zero-value SpawnPermissions{} denies dangerous
// tools (bash, rest_api, web_fetch, file_write, file_edit, delegate,
// spawn_teammate) at OnPreToolExec. Set AllowAll: true to bypass the gate;
// the user-curated permission profile remains the next layer of enforcement.
type SpawnPermissions struct {
	// AllowedTools is the allow-list of tool names. Empty = use fail-closed
	// defaults (deny dangerous tools); ["*"] = allow all (subject to profile gate).
	AllowedTools []string `json:"allowedTools,omitempty"`

	// DisallowedTools is the deny-list. Takes precedence over AllowedTools.
	DisallowedTools []string `json:"disallowedTools,omitempty"`

	// PermissionMode controls the bubbling/restriction behavior.
	// Values: "default" (no bubbling), "bubble" (restricted tools route to parent),
	// "restricted" (only safe subset allowed).
	PermissionMode string `json:"permissionMode,omitempty"`

	// AllowAll opts the spawn out of the fail-closed dangerous-tool gate. Use
	// for top-level interactive user runs where the user-curated permission
	// profile is the intended gate. Spawn paths (delegate/workflow) should
	// leave this false and rely on mode-preset-derived restrictions instead.
	AllowAll bool `json:"allowAll,omitempty"`
}
