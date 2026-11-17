---
name: demo-delegate-team-tree
type: workflow
description: "Coordinator forks two teams; each team's 3 workers (simple fast-LLM tasks) report a one-line finding, the team lead CENTRALIZES them into a synthesis, then the two leads SHARE their result over a direct comm channel (send-message). Fully LLM-driven, kept cheap with the fast model and one-line tasks."
iconName: sitemap
icon: "🌲"
hashtags: [demo, delegate]
category: demo
config:
  # Fully LLM-driven, minimal-token delegation demo:
  #   coordinator → 2 teams → each team: 3 worker LLM tasks (one-line findings)
  #   gathered by a LEAD that centralizes them, then the two leads exchange
  #   their synthesis over a DIRECT comm channel (send-message → the dashed 💬
  #   link in the Team tab). Every node is a real `fast` LLM task with a terse
  #   one-line prompt to keep the token cost low. Under `--dry-run` every LLM
  #   task is substituted with a simulator, so the topology runs token-free.
  # NOTE: no `_mode: coordinator` here — coordinator mode restricts children to
  # a SafeToolSubset that EXCLUDES send-message, which would block the
  # lead-to-lead comm channel. Plain (default) mode lets the leads message.
  steps:
    - name: teams
      kind: parallel
      on_branch_error: collect
      branches:

        # ── Team A: analysis ──────────────────────────────────────────
        - name: team-a
          mode: worker
          task:
            type: workflow
            steps:
              - name: gather
                kind: parallel
                on_branch_error: collect
                branches:
                  - name: analyst-1
                    mode: worker
                    task:
                      type: task
                      capability: fast
                      # Restrict the toolset to ONLY complete: a worker can't go
                      # off investigating the real codebase (bash/grep/file-read)
                      # and loop — it's forced to write one line and finish.
                      spawnPermissions:
                        allowedTools: [complete]
                      prompt: |
                        You are **analyst-1** on a code-analysis team. In ONE
                        short sentence, report a finding about the project's
                        CORE MODULE. Reply with one short MADE-UP sentence — do NOT use any
                        tool to investigate — then call `complete` with it.
                  - name: analyst-2
                    mode: worker
                    task:
                      type: task
                      capability: fast
                      # Restrict the toolset to ONLY complete: a worker can't go
                      # off investigating the real codebase (bash/grep/file-read)
                      # and loop — it's forced to write one line and finish.
                      spawnPermissions:
                        allowedTools: [complete]
                      prompt: |
                        You are **analyst-2** on a code-analysis team. In ONE
                        short sentence, report a finding about the project's
                        EXTENSIONS. Reply with one short MADE-UP sentence — do NOT use any
                        tool to investigate — then call `complete` with it.
                  - name: analyst-3
                    mode: worker
                    task:
                      type: task
                      capability: fast
                      # Restrict the toolset to ONLY complete: a worker can't go
                      # off investigating the real codebase (bash/grep/file-read)
                      # and loop — it's forced to write one line and finish.
                      spawnPermissions:
                        allowedTools: [complete]
                      prompt: |
                        You are **analyst-3** on a code-analysis team. In ONE
                        short sentence, report a finding about the project's
                        TEST COVERAGE. Reply with one short MADE-UP sentence — do NOT use any
                        tool to investigate — then call `complete` with it.
              - name: team-a-lead
                task:
                  type: task
                  capability: fast
                  # teammateName gives this lead a stable messaging identity so a
                  # cross-team send shows as `team-a-lead → team-b-lead`; the
                  # orchestration plugin provides send-message (and mirrors it
                  # onto the to.<name>.message bus topic the GUI renders).
                  teammateName: team-a-lead
                  plugins: [orchestration]
                  # Force the reliable path: the lead can ONLY send-message and
                  # complete — it can't delegate/investigate/loop. Combined with
                  # the prompt, it synthesizes, sends to the other lead, finishes.
                  spawnPermissions:
                    allowedTools: [send-message, complete]
                  prompt: |
                    You are the **Team-A lead** (analysis). You CENTRALIZE your
                    three analysts' findings:
                    - analyst-1: {{ .finding_1 }}
                    - analyst-2: {{ .finding_2 }}
                    - analyst-3: {{ .finding_3 }}

                    Say "starting team-a-lead". Synthesize the three findings into
                    ONE short sentence summarizing the state of the code.

                    Then SHARE it with the other team: call **send-message** with
                    `to: "team-b-lead"`, `message: "<your one-line synthesis>"`,
                    `summary: "Team A analysis → Team B"`. This is the direct
                    lead-to-lead channel.

                    Then call `complete` with your sentence.
                input:
                  finding_1: "${results.gather['analyst-1']}"
                  finding_2: "${results.gather['analyst-2']}"
                  finding_3: "${results.gather['analyst-3']}"
              - kind: return
                value: "${results['team-a-lead']}"

        # ── Team B: documentation ─────────────────────────────────────
        - name: team-b
          mode: worker
          task:
            type: workflow
            steps:
              - name: gather
                kind: parallel
                on_branch_error: collect
                branches:
                  - name: writer-1
                    mode: worker
                    task:
                      type: task
                      capability: fast
                      # Restrict the toolset to ONLY complete: a worker can't go
                      # off investigating the real codebase (bash/grep/file-read)
                      # and loop — it's forced to write one line and finish.
                      spawnPermissions:
                        allowedTools: [complete]
                      prompt: |
                        You are **writer-1** on a docs team. In ONE short
                        sentence, say what you drafted for the API REFERENCE.
                        Reply with just that sentence, then call `complete`
                        with it.
                  - name: writer-2
                    mode: worker
                    task:
                      type: task
                      capability: fast
                      # Restrict the toolset to ONLY complete: a worker can't go
                      # off investigating the real codebase (bash/grep/file-read)
                      # and loop — it's forced to write one line and finish.
                      spawnPermissions:
                        allowedTools: [complete]
                      prompt: |
                        You are **writer-2** on a docs team. In ONE short
                        sentence, say what you drafted for the USER GUIDE.
                        Reply with just that sentence, then call `complete`
                        with it.
                  - name: writer-3
                    mode: worker
                    task:
                      type: task
                      capability: fast
                      # Restrict the toolset to ONLY complete: a worker can't go
                      # off investigating the real codebase (bash/grep/file-read)
                      # and loop — it's forced to write one line and finish.
                      spawnPermissions:
                        allowedTools: [complete]
                      prompt: |
                        You are **writer-3** on a docs team. In ONE short
                        sentence, say what you drafted for the CHANGELOG.
                        Reply with just that sentence, then call `complete`
                        with it.
              - name: team-b-lead
                task:
                  type: task
                  capability: fast
                  teammateName: team-b-lead
                  plugins: [orchestration]
                  spawnPermissions:
                    allowedTools: [send-message, complete]
                  prompt: |
                    You are the **Team-B lead** (documentation). You CENTRALIZE
                    your three writers' drafts:
                    - writer-1: {{ .draft_1 }}
                    - writer-2: {{ .draft_2 }}
                    - writer-3: {{ .draft_3 }}

                    Say "starting team-b-lead". Synthesize the three drafts into
                    ONE short sentence describing what documentation is now ready.

                    Then SHARE it with the other team: call **send-message** with
                    `to: "team-a-lead"`, `message: "<your one-line synthesis>"`,
                    `summary: "Team B docs → Team A"`. This is the direct
                    lead-to-lead channel.

                    Then call `complete` with your sentence.
                input:
                  draft_1: "${results.gather['writer-1']}"
                  draft_2: "${results.gather['writer-2']}"
                  draft_3: "${results.gather['writer-3']}"
              - kind: return
                value: "${results['team-b-lead']}"

    - kind: return
      value: "${results.teams}"
---

# Demo: delegate-team-tree

**Pattern**: a coordinator that forks two LLM-led teams. Every node is a real
**`fast` LLM task** kept cheap with a one-line prompt. Each team's three workers
report a one-line finding; the team **lead centralizes** them into a synthesis;
then the two leads **share** their result with each other over a **direct comm
channel** (`send-message`). A 3-level, two-branch delegation tree with genuine
cross-team communication.

```
coordinator root (workflow)
├── team-a            (nested workflow)
│   ├── gather        (parallel)
│   │   ├── analyst-1 (fast LLM — one-line finding)
│   │   ├── analyst-2 (fast LLM — one-line finding)
│   │   └── analyst-3 (fast LLM — one-line finding)
│   └── team-a-lead   (fast LLM — centralizes the 3 findings)
└── team-b            (nested workflow)
    ├── gather        (parallel)
    │   ├── writer-1  (fast LLM — one-line draft note)
    │   ├── writer-2  (fast LLM — one-line draft note)
    │   └── writer-3  (fast LLM — one-line draft note)
    └── team-b-lead   (fast LLM — centralizes the 3 notes)

team-a-lead  ⇄  team-b-lead   (direct comm channel — each lead `send-message`s
                               its synthesis to the other; mailbox messaging)
```

**What to watch in the journal / GUI Team tab**:
- The two teams fan out concurrently under the coordinator root.
- Inside each team, three `child.<worker>.started/done` events fire roughly
  simultaneously (the `gather` parallel), then the lead's child session opens
  once all three reports are in.
- Each lead's `input` binding pulls its three workers' one-line outputs
  (`${results.gather['analyst-1']}` …) into its prompt — the **centralization**.
- A **direct lead-to-lead link** appears between `team-a-lead` and
  `team-b-lead` (dashed 💬 edge) once a lead calls `send-message` — click it to
  read what the channel carried. This is the mailbox channel (`to.<name>.message`),
  not a parent→child delegation.
- `workflow.<id>.done` fires once both leads have called `complete`.

**Cost**: minimal — 8 `fast` LLM tasks (6 one-line workers + 2 leads), each a
single short turn (the leads add one `send-message` call). Roughly ~6–10K
tokens total (~$0.01) with the fast model. Under `--dry-run` every LLM task is
substituted with a simulator, so the topology runs token-free.
