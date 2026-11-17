---
name: request-file
description: "File work as a request in the module it concerns, from a person or from one module to another, and manage it. Load when work must be done or traced, or a module needs another's answer."
hashtags: [planification, requests, module, todo]
category: methodology
---

# request-file — work is a request in its module

Every piece of work is a **request** in the module it concerns, filed and changed through
`ktw-agents` verbs — never by writing files. The planner and a session working by hand
(`ktw-agents:/skill/module-next`) take requests from the same place, in the same order.
**File what a person asks for as it arrives**, finished work too (finished at once with
what was done): that is the trace.

## Before any command

The command line works whether the universe runs or not: a running hub (the GUI open)
is found and every verb goes to the process serving the workspace; none running, one
is spawned for the command (`--hub keep` leaves it running for the session's next
verbs; `katyoti universe stop <name>` ends it). Never kill a process. `--by` is who
acts, **always a JSON actor in single quotes, never a bare name** (`--by jdoe` is
refused: invalid JSON): a person `'{"id":"<handle>","kind":"person"}'` (their profile
handle, `katyoti profile`), a module `'{"id":"<m>","kind":"module"}'`, a session
`'{"id":"<session>","kind":"agent"}'`. `<who>`, `<session>` and `<person>` below stand
for that JSON.
Read back with `--out json`; a request's id is `<module>.RQ-<n>`.

## File

```
ktw-agents requests file --module <m> --kind <demand|build|fix|analyse|review|maintain|discover|question> \
  --priority <P0|P1|P2|P3> --by '<who asked>' --title "<one sentence: what they get>" \
  --details "<context; what done means; links>" \
  [--deliverables '[{"title":"<what is delivered>","check":"<a command or an action URI>"}]'] \
  [--depends-on <id,id>] [--about <object id>] [--on <a feature or goal that carries it>]
```
- **What a person asks is a demand or direct work**:
  - **direct work**: a bug fix, a label, a message, a small UI or doc fix — `--kind fix`, or
    `--kind build --depth quick`; it may add a test case, never a feature;
  - **a demand**: anything else a person asks — `--kind demand`, the default for a person
    filing with no `--kind`; it does no work itself: it reconciles into the goals, features,
    increments and open requests it changes, approved once by the person.
  - **Release work** is never filed by hand: it is known by its edges (it serves a feature an
    increment holds) and is filed when the increment runs. A module's or an agent's ask with no
    `--kind` is assessed at intake.
- **Define now, plan when run**: an increment, a release or a
  request that needs other requests and modules is defined - its goals, in the person's
  words - and planned only when a person runs it (`increments run`, `releases run`, Run in
  the planning view). Never file, link, split or order requests ahead for something nobody
  has run; work urgent enough to do now is a request of its own.
- The title is the outcome, in the asker's words, not the implementation.
- The module is the one whose code or documents change. Unsure: file it in the closest
  module and say so in its details; its analysis may move it (`requests split` with one
  part). A system module holds no to-dos, so never leave `--module` out. Several modules: file
  once in the module that leads it; its analysis splits it when it runs, never its filing.
- P0 now, P1 next, P2 later (the default), P3 someday.

**Already done**: file it, then
`ktw-agents plans finish --id <id> --by <session> --note "<what was done; where>"`.

## A module asks another

A module that needs a decision, an API or a change from another module files the request
**in the module that answers**, itself as the requester: `--by '{"id":"<asking module>","kind":"module"}'`,
`--kind question` for a question (`build|fix|analyse` for work, with `--deliverables` saying
how the asker will check it). The answering module takes it in `ktw-agents:/skill/module-next`
(step 2); the asker reads the answer the same way. Ask once:
`requests list --module <target> --requester <asking module>`.

One request per decision area. Its **details**, in this order:
1. **Context**: what the asking module builds and why, and the constraints it must respect.
2. **Questions**, numbered; each states the **proposal** and asks for **a decision, or the
   constraint that blocks it** (`3. Creation. Proposal: the first authoring verb creates the
   module. Who owns that step, and is an absent module a normal state?`).
3. **Meanwhile**: what the asking module does until answered (waits, or goes on with its
   proposal and adapts).
4. **Links**: the decision drafts, files and object ids concerned.

Then order the asker's waiting work after it: `requests depend --id <waiting> --on <new id>`.

## Change

- Assess (what intake records): `requests assess --id <id> --by <who> [--kind] [--priority] [--estimate <days>] [--serves <want id>]`
  — build, fix and maintain work serves a goal, a feature or a use case (`--serves`, or
  filed `--on` it); assessed without one, the answer proposes the wants it could serve and
  writes nothing.
- Its module brought up to date (the module-update step, skill `ktw-agents:/skill/module-update`):
  `requests module-update --id <id> --by <who> --changelog "<page>#<entry>" --checks <criterion ids>`
  (`--internal --reason "<why>"` instead of `--changelog` for a change no user sees); whether
  it may end: `requests check-module-updated --id <id>`. A request that changes a module ends
  only once it is up to date — or **a person** waives it, saying why:
  `requests waive --id <id> --note "<why>" --by <person>` (refused from an agent's session).
- Order after another: `requests depend --id <id> --on <other>` (a cycle is refused)
- Its steps, from its work template, are planned when it is filed (again:
  `work plan --request <id> --again [--no-intake] --by <who>`); changed before they run with
  `work edit|add|remove|move`; read with `work list --request <id>`
- Split across modules or into sub-features - its analysis does, when it runs:
  `requests split --id <id> --by <who> --note "<why>" --parts '[{"module":"<m>","title":"…"},{"sub":true,"title":"…"}]'`
- Drop: `plans finish --id <id> --outcome dropped --by <who> --note "<why>"`
- What is asked changed: reword it (`requests rescope --id <id> --title "…" --details "…" --note "<why>" --by <who>`);
  another open request carries its work (`requests supersede --id <id> --with <other> --note "<why>" --by <who>`);
  the two are one piece of work (`requests merge --id <id> --into <other> --note "<why>" --by <who>`, the other not started).
- Correct what it promised while it is open — a check that is wrong or that a later change
  broke: `requests revise --id <id> --by <who> --note "<why>" --deliverables '<the full list>'`;
  a deliverable whose check changed must be verified again; without `--deliverables` only the note
  is recorded (they are kept), `--deliverables '[]'` removes them all. Never file a duplicate for it.
- Run its checks now: `requests check --id <id> --by <who>` (each result answered; those
  that pass are recorded verified).
- **An agent tests first**: file each deliverable with the
  command that checks it whenever an agent can test it. One with no check is given to an
  agent's auto-test at the request's end — it writes the check, shows it can fail, records
  and runs it — or asked for: `requests auto-test --id <id> --by <who>`. Only a deliverable
  no agent can judge is the person's: file it with `by_hand` (the exact steps, and why).
- A deliverable no command can check is verified **by the person themselves**: approving
  the step that verifies it (an analyse request filed with one such deliverable has its
  accept step verify it; the request view says "verified when a person approves <step>"),
  or by hand with what they saw — in the request view, or typing
  `requests verify --id <id> --deliverable "<title>" --evidence "…" --by <person>`.
  An agent never verifies for a person: a verify from an agent's session is refused
  whatever `--by` says. Tell the person what to look at and where (the request view).

## Read

- A module's: `requests list --module <m> [--state idle|ready|blocked|split|done|dropped] [--requester <id>]`
- As trees: `requests demands --module <m> --view open|mine|history`
- One: `requests get --id <id>` (its plan: state, next step, trail; a question's answer)

## Rules

- Show the person the title, module, kind and priority before filing anything they did
  not dictate word for word.
- Never edit a request's file by hand.
- To take the next request: `ktw-agents:/skill/module-next`.
