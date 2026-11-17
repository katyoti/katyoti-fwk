---
name: test-scenario-case
description: "Run, replay and write end-to-end scenario cases: a case in its own universe and project, stages driven through the hub, checkpointed. Load to prove a whole flow on a real project."
advertised: false
hashtags: [testing, scenario, end-to-end, autonomous, checkpoint, regression]
category: guide
---

# Scenario test cases (`ktw-compose scenario`)

The method for what no other one reaches: a whole flow, over hours, on a real
project, in a universe of its own. Each step goes through the universe's hub, the
way the GUI does it, so a case drives every sidecar and checks what a person would
see. A person's answers come from a simulated user. Every stage that passes is
saved, so a failure costs one stage, not the whole run.

Use it when the question is "does the platform carry this from start to finish?".
For one task or wizard, use the `test-scenario` task. For a screen, use
`ktw-compose:/skill/test-gui`. For code, use `ktw-compose:/skill/test-go`.

The reference case is `test_files/scenarios/demo-showcase`: it installs
ktw-demo's sign-off capability in a universe of its own and proves every
mechanism a pack gives — the pane's tabs, the page's acts, a refused write, a
gated move, verify and lint, a pack's own hook point, an out-of-scope gate
and its grant, the French labels — with no LLM. Copy its shape for a new
capability or panel.

## 1. The loop

```bash
ktw-compose scenario list                                    # the cases (test_files/scenarios)
ktw-compose scenario install <case> --root <root> --bin <bin>  # its universe, members, project, git; checkpoint `installed`
ktw-compose scenario run <case> --root <root> [--to <stage>]   # stages after the last that passed
ktw-compose scenario status <case> --root <root>               # passed, checkpoints, running, last report
```

When a stage fails, the run stops, the universe keeps running, and the
command prints how to replay:

1. Read the stage's report, `<root>/<case>/runs/<time>-<stage>.md`: every step, every reply, every check.
2. Look around while the universe still runs:
   - `scenario call <case> <provider:slot/Method> '<json>'` calls any method through the hub, e.g. `ktw-agents:plans/Get`, `ktw-agents:tasks/Runs`, `ktw-agents:cwm/ObjectList`.
   - `scenario action <case> <action-uri> '<json>'` runs an action.
   Then read the session of a failed run.
3. Fix the cause. If it is code, change it, write a test that fails without the fix, then build into `<bin>`. If it is the case, edit `test-case.yaml`.
4. Replay the stage: `scenario run <case> --root <root> --from <stage> --refresh-bin`. This copies the new binaries, restores the checkpoint taken before `<stage>`, and runs again from there. Drop `--refresh-bin` when no code changed.
5. When it passes, continue with `scenario run`, which picks up after the last stage that passed.

Pass JSON arguments through the Bash tool, single-quoted. PowerShell 5.1 mangles
them.

After changing the tool itself (`internal/extensions/scenario`), run `make
scenario-smoke`. It builds the binaries and drives every verb on the smoke case
(`test_files/scenarios/smoke`), with no LLM, in a few minutes. Set
`KTW_TEST_ROOT` to keep its runtime folder; otherwise it uses a temporary one.

## 2. What runs where

The runtime folder of a case is `<root>/<case>/`. Root is `--root`, else
`$KTW_TEST_ROOT`, else `~/katyoti-tests`. Keep every case outside the repository.

| Folder | Holds |
|---|---|
| `bin/` | the case's own binaries, copied from `--bin` (`refresh-bin` copies them again) |
| `universe/` | universe `scn-<case>`; its `data_dir` points at `data/` |
| `project/` | the attached project; a git repository with one commit per passed stage, so `git log -p` shows what each stage changed |
| `data/` | the universe's state: sessions, runs |
| `state.json` | stages passed, values saved, answers given, last failure |
| `checkpoints/` | `installed`, then one per passed stage (universe, project, data and state; not the binaries) |
| `machine/` | the case's sandbox: its registry, admin universe, control plane, caches |
| `runs/` | one report per stage run |

The universe runs headless: its own `ktw-hub serve`, with members joining as usual
(in the background — on Windows they open no console window; `status` shows
them, `stop` ends them). `start` and `stop` control it; `checkpoint` and
`restore` snapshot or put back the whole state. CWM has one writer per
workspace: a GUI joins the case's hub instead of opening the project itself
(§4), and is closed before the case runs again.

**A case is a sandbox.** Every verb enters the case's sandbox itself: its universe
registry, admin, control plane and caches live under `<root>/<case>/machine/`.
Nothing of your own installation is read, written or stopped, and your own GUI can
stay open. Around every verb a leak check watches your installation's machine
files (registries, `.env`, control plane, caches, hub lockfiles). A verb that
touched one fails with `LEAK:` and the paths. Set nothing by hand: no
`KTW_KATYOTI_ROOT`, `KTW_DATA` or `KTW_CACHE`. A case installed before sandboxes
existed is adopted by its next verb, with its stages and checkpoints kept.

## 3. Writing a case

`ktw-compose scenario new <case>` scaffolds `test_files/scenarios/<case>/`:

```yaml
name: <case>                 # universe scn-<case>
title: …
description: |               # the intent, from the user's side; what "done" means
universe:
  sidecars: [ktw-agents, ktw-methods]
  no_welcome: true           # its GUI opens no first guide nor « Quoi de neuf » by itself
  copy:                      # into the universe at install, in order
    - {from: config, to: config, select: ["*.yaml"]}            # a folder, filtered
    - {from: ~/.config/katyoti/.env, to: .env, optional: true}  # one file; keys stay out of the case
project: {from: project, git: true}
user: user.md                # the person the simulated user plays
stages:
  - id: s1-…
    title: …
    timeout_min: 30          # the stage fails past it
    max_agent_runs: 12       # the spend cap: the stage fails when more agent runs started
    steps: […]
    expect: […]
```

**`copy`**
- `from` is relative to the case folder; it can also be absolute or `~/…`.
- `to` is relative to the universe's home (`config/`, `.env`, `tasks/`) and cannot leave it.
- A file goes to `to`, or into `to` when it ends with `/`.
- A folder copies its files, filtered by `select` and then `exclude`. A glob without `/` matches file names; one with `/` matches the relative path. A folder that selects nothing is an error.
- `optional: true` skips a source that is missing.
- After the copies, install writes each member's bundled defaults (`sc config set`). A file the case copied is kept.
- A case gives its universe its LLM profiles this way, so agents can run, and takes the keys from the machine, never from the case.

**Steps** (exactly one kind each):

| Step | Does |
|---|---|
| `call: {uri, input, save, may_fail}` | a method through the hub; `save: x` keeps the reply as `${saved.x.<path>}`; `may_fail: true` reports a refusal and goes on — a move a gate refuses first (asking a person), then `answer_inbox` until it happened |
| `action: {uri, input, save}` | an action URI, through ktw-agents' `action/Run` |
| `answer_inbox: {until, max, timeout_min}` | the simulated user answers each request the Inbox has waiting, as `user.md`, reading the object it is about; it repeats until the `until` check passes |
| `wait: {until, timeout_min}` | waits for a check, answering nothing |
| `run: {cmd, dir, exit}` | a shell command in the project |
| `find: {uri, input, items, where, save}` | calls a method, takes the list at `items` in its answer, saves the first element matching `where` (case-insensitive substrings) |
| `each: {uri, input, items, where, steps}` | the same listing; runs `steps` once per element, which read it as `${item.<path>}` |
| `open: <address>` | a GUI element by its address (`object/<id>/<tab>`, `screen/<screen>/<section>`, `home/<entry>`, `rail/<entry>`; the case's universe when it names none): checked when the case loads, as the GUI reads it; a headless run records it, a GUI-driven one opens it |

**Checks**, used in `expect` and `until`:
- `objects: {type, module, name, state, min, max}`;
- `plans: {module, state, min, max}`;
- `file: <path in the project>`;
- `cmd: {cmd, exit}`;
- `call: {uri, input, contains, excludes}`: the reply contains `contains` and not `excludes` (a member that must be absent).

Inputs read `${saved.…}`, `${item.…}` (inside an `each`) and `${case.name|universe|project}`. Objects the platform created have ids the case cannot know: reach them with `find` / `each` (the release named 1.0, every feature it targets), never by a hard-coded id.

**Rules for a good case**
- Start each stage from what a person would do: file a request, accept a goal. Then let the platform work.
- Check outcomes a person would see (objects, plan states, files, `bun test`), not internal traces.
- Write `user.md` like a real person: what they want, their preferences, the facts, and how often they challenge. A vague persona gives vague runs.
- To answer without an LLM, script the person in `user.md`'s front matter:
  ```yaml
  ---
  answers:
    - {when: colour, decision: approved, note: "Blue, the colour of the logo."}
    - {decision: rejected, note: "Not yet: the tests first."}   # no when: any question
  ---
  ```
  Each answer goes, once, to the first pending request whose title or description contains its `when`. No agent runs. A request no answer left matches fails the step.
- Cap every stage that runs agents (`max_agent_runs`, `timeout_min`). The cap counts the runs the stage made start (by a person, the planner, an agent or a workflow). The universe's own runs are not counted: the planner machine, started again at each restart, and the tasks it keeps alive.
- Set the universe up with files (`copy`), never with a call whose effect lives only in the running universe (resuming a paused task group, say). A checkpoint keeps files: after a restore, such a switch is back to what the files say, and the replayed stage waits for nothing.
- Wait on the outcome, not on a sign of progress. An `until` that can hold before the work is finished (`bun test` passes as soon as the first tests do) ends the wait mid-work. Wait for the request's plan to be done, then check the files and commands in `expect`.
- Take objects along their real workflow (a goal starts in `idea`, is proposed, reviewed, then accepted by a person). A case that jumps a state fails on the transition, not on what it means to test.
- With no LLM, every act is a `call` and every check reads what a person sees: the pane's tabs (`ktw-agents:object-pane/Detail` `contains: "panel:<name>"`), the moves offered (`object-pane/TransitionOptions`), a gate's checklist (`ktw-agents:gates/Check`), and a refused write as an `expect` `call` whose answer `contains` the reason.

## 4. Looking at it in the GUI

A person double-clicks `open-gui.cmd` (`open-gui.sh`) in the case's runtime
folder, written at install: it runs `scenario start` (the case's own hub and
members) and opens ktw-gui on `scn-<case>`, which joins that hub. Give the person
its path after a run. `ktw-compose scenario gui <case>` writes it again and prints
the steps for an agent:
1. `scenario start <case>`.
2. Run `wails dev` in `app/ktw-gui`, with `KTW_NO_WELCOME=1` unless the case
   sets `universe.no_welcome` or the test is about the welcome screens.
3. Open `http://localhost:34115` with a browser automation tool.
4. Activate "Scenario <case>" from the ribbon.

Then take screenshots as evidence (see `ktw-compose:/skill/test-gui`).

## 5. Evidence

A run's evidence is its folder:
- the stage reports;
- the checkpoints;
- the project's git history;
- the sessions under `data/`.

To report, cite the report path and the verdict of each check, and keep the
folder.
