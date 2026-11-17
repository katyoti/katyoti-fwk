---
name: module-next
description: "Work a module step by step: plan its requests into to-dos, pull the next one from the scheduler's queue, work it and report its result. Load when asked what is next on a module."
hashtags: [planification, requests, todos, scheduler, module, next, workflow]
category: methodology
---

# module-next — a module, step by step

A module's work is its **requests** (CWM objects: what is asked, its deliverables, its
plan and the trace of each step it went through). Planification plans each request into
**to-dos** — one per step of its work template — in the module's queue, kept by the
scheduler's store (a local runtime database, never committed: read it with `todos list`,
`get`, `search`, `history`). A to-do is a transient work ticket: the target object, the
step, its state; once its result is collected it is archived done and pruned later. What lasts is written on the request; never point to a to-do from anything.

A session working a module takes its to-dos from the same queue, in the same order, as
every other worker: priority, then age, then the step's position, never a step whose
module another pulled step writes. It works each one through the worker contract
(`ktw-agents:/skill/work-todo`): **pull → do → report** — a report is a one-line
`--summary` and, when there is more, its `--details` (markdown). Planification reads the result
and decides what follows.

A module's scheduling mode says who may take its to-dos: `manual` (people and sessions
pull them — the default), `auto` (the internal scheduler runs its auto to-dos too), `off`
(nothing is handed out). `ktw-agents todos modes` lists them;
`todos mode --module <m> --mode manual|auto|off --by <you>` sets one.

Without a module: `ktw-agents requests demands --view open` lists the modules with open
requests; ask which one.

## Before any command

The command line works whether the universe runs or not. A running hub (the GUI open
on the universe) is found and every verb goes to the process serving the workspace;
none running, one is spawned for the command — `--hub keep` (or `KTW_HUB=keep`) leaves
it running for the session's next verbs, `katyoti universe stop <name>` ends it. A
method of another sidecar runs through the same door: `ktw-agents action run --uri
<provider>:<slot>/<method> --input '{…}'` (ktw-methods' goals and decisions, a dossier's
steps), never through that sidecar's own command line. Never kill a process. Use
`--out json` when reading back. `--by` is who acts, **always a JSON actor in single
quotes, never a bare name** (`--by jdoe` is refused: invalid JSON): a person
`'{"id":"<handle>","kind":"person"}'` (their profile handle, `katyoti profile`), a module
`'{"id":"<m>","kind":"module"}'`, a session `'{"id":"<session name>","kind":"agent"}'`.
`<you>`, `<session>` and `<person>` below stand for that JSON.

## 1 — Where the module stands

```
ktw-agents requests channel --module <m> --out json            # In, Out, the answers it received
ktw-agents views work --module <m>                              # its to-dos by state; each request's rollup
ktw-agents todos queue --module <m> --out json                  # the ready ones in pull order, each takeable or why it waits
ktw-agents requests demands --module <m> --view open --out json # its requests as trees
```
Show what is pulled, what is ready, what waits and on what (`waits`), what failed or is
blocked (with its summary), and where each request stands (`work rollup --request <id>`:
`new` (not planned), `planned`, `in-progress`, `waiting` (a person's step), `blocked`,
`done`, `dropped`).

## 2 — What the module received

- **Questions to answer**: its open requests of kind `question`. Per question, the decision,
  or the blocking constraint and what would lift it:
  `ktw-agents requests answer --id <id> --by <session> --text "1. … 2. … 3. …"` (a person or an
  agent answers; the module is where the question was filed).
  A decision that changes the module's design is also recorded with its decisions and
  linked in the answer; work the answer creates is filed in the module and named in it.
- **Requests from other modules**: open requests whose requester is a module; they are
  planned below like the others.
- **Answers to its own questions**: `ktw-agents requests list --requester <m> --state done`;
  for each one not acted on, `requests get --id <id>` and act on the answer.
- **What it awaits**: `ktw-agents waits list --module <m> --out json` — each request it
  filed in another module, with its deadline (the module's `wait_days`, 7 by default),
  removed once that request ends. One `overdue`: chase it (`requests get --id <on>`: where
  it stands, then ask the module or the person who can unblock it), or give up
  (`waits clear --id <wait>`) and say what the module does without it.

## 3 — Plan what is not planned

A request is planned when it is filed. One with no to-dos (its rollup `new`):
```
ktw-agents work plan --request <id> --by <session>
```
expands its work template into to-dos — the intake first (route, assess, order; it may make
the request quick, and the plan follows). Planning runs nothing. Read the plan
(`work list --request <id>`); when the work needs other steps, change it before they run:
`work edit`, `work add`, `work remove` (with why), `work move`. A challenge or an approval
added after the analysis (`--after <analyse>`) runs in its preparation, unless `--phase` says
otherwise: it reads the module only and runs while a dependency is built. A failed or blocked to-do
goes back to the queue with `work retry --id <todo>`.

## 4 — The next to-do

```
ktw-agents todos queue --module <m> --for agent --out json
```
gives the ready to-dos in the order they are pulled, each takeable or why it waits.
A screen feature's GUI tests batched at its increment's end hold nothing built after it:
a dependency on a realized feature whose only missing proof is a GUI test listed in a
request of its increment is met, and that feature's verify round waits « its GUI test runs
in the batch of <request> » until the batch has run it - never report the round blocked
for it. A feature that changes no screen in a module that draws one (a code removal, a
guard) carries the `no-screen` tag (`cwm object-add-capability`), so the GUI-test gate does
not hold it.
**Release work first**: within a priority, a to-do labelled `release:` (its request serves a
feature an increment of an open release holds) goes before one that is not.
Present the first takeable one — its step and title, its request (`requests get --id
<request>`: title, kind, priority, requester, deliverables), its work (`todos get --id`:
instructions, skill or task, target) — with the two after it, a recommendation, and **one line on
why not the others**. Let the person confirm or pick another. Then work it with
`ktw-agents:/skill/work-todo`:
```
ktw-agents todos pull --module <m> --by <session> --out json     # or --id <todo> for the one chosen
```
Nothing ready: the answer says why (`nothing`, `waiting`); say so and stop.

A person's to-do (validate, challenge, decide, provide) is theirs: it is asked in their
inbox, where they answer it — or in the request view. Tell the person what it asks and
where to answer it, and take the next one. When you tell the person what waits on them,
read it live — `views person-queue` and `inbox triage` at that moment — and list only what
they return, with its action; never from your notes. An agent never reports a person's to-do, not
even a verdict or an answer the person gives in the session: a report with a person's
`--by` from an agent's session is refused whatever `--by` says. Approving a step that
verifies a deliverable verifies it (the request view says which).

## 5 — When a request ends

Its last to-do collected ends it: its deliverables' checks run, its node moves, it is done
— or blocked, saying why (`work rollup --request <id>`). Each step's result stays on the
request (its plan's `steps`). A check that is wrong is corrected with `requests revise`
(never a duplicate request); one with no check is an agent's first — the request's end
plans an auto-test step, or `requests auto-test --id <id>` asks for one — and only one
marked `by_hand` (what a person alone can judge) is theirs — approving the step that
verifies it, or verifying it by hand in the request view: tell them what to look at and
where; never `requests verify` for them. Needs another module: ask it (`ktw-agents:/skill/request-file`,
"A module asks another"), then `requests depend` on it.
When the module carries method objects (goals, features with probes), show that the gap
moved: grade the module before and after (`ktw-methods:/skill/method-status`).

Then back to step 4.

## Rules

- One held to-do at a time; never hold one across a person's wait: report it first (`blocked` with your question if you are the one waiting). While a person's step waits, pick the next takeable to-do, from another request if need be.
- A to-do another worker holds is theirs; never work it.
- A person's verdict, decision, answer or verification is theirs: never report their to-do,
  never verify for them — tell them where to answer (the Inbox, the request view).
- Never edit a request's file or a to-do's file by hand; every change goes through the verbs.
- Never name a to-do in a request, a page or a commit message: write what lasts on the request.
- New work asked on the way: file it first (`ktw-agents:/skill/request-file`), then decide.
