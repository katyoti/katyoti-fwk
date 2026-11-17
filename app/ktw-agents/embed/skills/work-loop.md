---
name: work-loop
description: "Work a module, an increment, a release or a stream — or, with none named, the whole workspace — until nothing is left for an agent: pull the next takeable to-do, work it, report it, without asking between to-dos. Several sessions can loop on the workspace at once: each takes its own work, never another's. Load when asked to loop on a module, empty it, or work the workspace through."
hashtags: [planification, todos, scheduler, worker, loop, workspace, concurrency]
category: methodology
---

# work-loop — work until nothing is left

`work-loop` is `ktw-agents:/skill/module-next` run as a loop: the same steps, the same
worker contract (`ktw-agents:/skill/work-todo`: **pull → do → report**), the same rules.
Read both first; this page only says what changes.

- **With a module**: the loop works that module's queue until it has no open request —
  none whose state is other than done or dropped: an `idle` (waiting) or a `split` one is
  still open (`requests list --module <m> --out json`).
- **With none**: the loop works the **whole workspace** — the next takeable to-do of any
  module, in the scheduler's order (priority, then age, then the step's position). Start
  several sessions this way (say five) and each loops on its own, concurrently, one to-do
  after the other.
- **With a scope** — `--increment <IN>`, `--release <RL>` or `--stream <SM>`: the loop
  works only the to-dos carrying that label (`increment:<IN>`, `release:<RL>`,
  `stream:<SM>` — planification labels each to-do with the increment, the release and the
  stream its request belongs to), across the modules they touch, in the scheduler's order:
  `todos queue --labels <level>:<id> --for agent` to see them, `todos pull --labels
  <level>:<id> --by <you>` to take the next. It stops when the scope holds only a person's
  step or what waits on one, **leaving it in the queue** (`todos queue --labels
  <level>:<id> --for person` says which). Several sessions on one scope never take the same
  to-do. A release holds increments and direct work only, never a goal or a feature
  directly: what an increment targets reaches its release through it. The work of an
  increment that one of the release's increments depends on, in any release or stream,
  carries the release's label too: `--release` works it before the gated work.
- **With a request** — `--request <RQ>`: the loop works only the to-dos that are steps of
  that request, in its own module: `todos queue --request <RQ> --for agent` to see them,
  `todos pull --request <RQ> --by <you>` to take the next. It stops like a scoped loop:
  when the request holds only a person's step or what waits on one, leaving it in the
  queue, or when the request has ended. This is the scope a worker started on one request
  works (Run in Claude Code on a request).

## Why sessions never collide

The scheduler guarantees it, whoever pulls:
- a to-do is held by one worker for its lease: a pull never hands out one another holds;
- **an agent holds one to-do at a time**: a pull by a session that holds one answers
  nothing and names it — report it or release it first;
- a request's next step waits on its previous one, and the queue never offers a to-do whose
  module a held step writes: a second session waits (the queue says why) rather than start
  work the first one's step changes.

So each session needs only **its own `--by`**: `{"id":"<session name>","kind":"agent"}`,
the same for every pull and report of that session, and different from every other
session's.

## Setup, once

- Keep a hub for the session's verbs (`--hub keep`, or `KTW_HUB=keep`).
- Name the session's actor once and use it everywhere.
- With a module: read where it stands, handle what it received and plan what is not
  planned (module-next §1–§3). With none: `ktw-agents todos queue --for agent --out json`
  shows the workspace's ready to-dos; questions and planning stay with the module-next
  sessions of each module.

## The loop

1. **Pick.** `ktw-agents todos queue [--module <m>] --for agent --out json`. Take the first
   takeable to-do unless another one is clearly better: one that unblocks other requests,
   a P0/P1 before a P2, a request already in progress before a new one. **Release work
   first**: within a priority, a to-do labelled `release:` (its request serves a feature an
   increment of an open release holds) before one that is not. Do not wait for a
   person to confirm. Say in one line which one you took and why not the next one.
2. **Work.** `ktw-agents todos pull --id <todo> --by <you>` — or, with no module, `todos pull
   --by <you>` takes the first takeable one. Then work it by work-todo: do the step, run its
   tests, update the docs, report it (`todos report`). Renew the lease on long steps.
   Another session was first (`nothing`, or the to-do is no longer ready): pick again.
   **Nothing takeable yet** while the scope still holds work — a step another worker
   holds, a module it is changing: wait for it, `todos pull [--module <m> | --labels
   <level>:<id> | --request <RQ>] --wait 30m --by <you>`. It answers the to-do as soon as
   one can be taken (you hold it), `ended: finished` once the scope holds no agent to-do
   ready, held or reported, or `ended: timeout` (pull with a wait again). Waiting costs no
   token: the command blocks.
3. **Settle.** When the report ends its request (module-next §5): run its checks, correct a
   wrong check (`requests revise`), and name the deliverables only a person can verify.
4. **Refresh.** New questions, answers and requests filed on the way join the next pick.
   Work found on the way is filed first (`ktw-agents:/skill/request-file`) in its module.
5. Back to 1.

Between to-dos, two lines: what was worked, what came out, what is next.

## When it stops

A scoped loop stops on the same conditions, read in its scope: nothing takeable carries
its label, or only a person's step does. Its stop summary gives the scope's state: its
increments done and open, the release's grade (`releases grade`).

- **Finished for agents**: `todos pull --wait` answers `ended: finished` — no agent to-do
  of the scope is ready, held or reported (with a module: the module has no open request).
  Work another worker holds is no stop: wait for it (§ The loop, 2).
- **Only a person can move what remains**: the rest waits on a person's step (validate,
  decide, provide), an inbox answer, a hand verification, or another module's request. A
  person's step that waits is no stop while another to-do is takeable. Never wait on a
  person: the wait ends `finished` when only their steps are left.
- **A failure you cannot fix**: the same step failed twice, or the fix needs a person's
  decision — report it failed with what was tried, and go on with the rest.
- **The person interrupts.**

**The stop summary, per session**: the to-dos it worked (and the requests they ended), what
it left and why, what is blocked and on what - each hold named as the pull's waiting line and
`planning check` name it: its K-number, its kind and who moves it (`K4 held by <worker>: …`,
`K5 outside by <party>: …`), never a to-do's raw wait. **What waits on the person is read live at
the stop** — `views person-queue` (« À faire ») and `inbox triage` at that moment — and the
summary lists only what they return, each item with the one action it asks and where the
person answers it (the Inbox or the request view); nothing waits when they are empty. Never
list an item from the session's own notes or an earlier summary: it may be done or answered
already.

## Rules

- One held to-do at a time; never hold one across a person's wait: report it first
  (`blocked` with the question when you are the one waiting).
- A person's to-do, verdict or verification is theirs: never report it, never verify for
  them; tell them where to answer.
- Never set a module `auto` to make the loop go: the loop is the session pulling by hand.
- Sessions loop side by side on one machine: a step tests what it touches, in the project's quick lane, never the whole suite - the full run is the final gate, one at a time (work-todo §3).
- Never work a to-do another worker holds; never pull a to-do whose step writes outside
  what it declares.
