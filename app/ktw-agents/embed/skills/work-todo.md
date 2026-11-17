---
name: work-todo
description: "Pull a to-do, do it, report its result: the one contract every worker follows, an outside session or the internal scheduler. Load to work a module's to-dos, or when given a to-do id."
hashtags: [planification, todos, scheduler, worker, contract, workflow]
category: methodology
---

# work-todo — pull a to-do, do it, report its result

A request is planned into **to-dos**, one per step of its work template, in its module's
queue, kept by the scheduler's store (a local runtime database, never committed: read it
with `todos list`, `get`, `search`, `history`). Every worker — the internal
scheduler, a person, a session outside KaTyoti — works them through the same contract:
**pull → do → report**. The worker never decides what comes after: it reports its
result, and planification records it on the request and releases, adds or reworks what
follows.

A to-do is a transient work ticket: it says which object to work on and which step; the
information lives on the objects. Never name a to-do in a request, a page or a commit:
what lasts goes on the object.

`--by` is who acts, **always a JSON actor in single quotes, never a bare name**
(`--by jdoe` is refused: invalid JSON): a session `'{"id":"<session name>","kind":"agent"}'`,
a person `'{"id":"<handle>","kind":"person"}'` (their profile handle, `katyoti profile`).
`<you>` below stands for that JSON. Use `--out json` when reading back.

## 1 — Pull one

```
ktw-agents todos pull --module <m> --by <you> --out json   # or --request, --priority, --group, --skill, --labels; --id <todo> for a given one
```
answers the first ready to-do of the queue for your filter — priority, then its rank
(lower first, unranked last), then age, then its position — held by you for a **lease** (15 minutes; `--lease-seconds`). Nothing ready:
`nothing` says why and `waiting` why each other ready one waits (a module another pulled
to-do writes, a module whose mode is off); say so and stop — or wait with `--wait 30m`
(at most 1h): the pull stays open until a to-do can be taken, and answers `ended: finished`
when no agent to-do of the filter is ready, held or reported, `ended: timeout` when the
wait passes. `--labels release:<R>` keeps
only the to-dos carrying every label given (comma-separated; labels are their owner's, the
scheduler gives them no meaning); `--prefer <module>` — the module you last worked — wins
a tie of priority and rank, never more. A person's step (validate,
challenge, decide, provide) is theirs; an agent never pulls it.

## 2 — Read it

The to-do: `target` (the **object** the step works on), `request`, `title`, `skill` (the
skill to follow — load it), `reads` / `writes` (the modules it may read / change), and
`work`, its owner's packet: `instructions` (what to do), `task` (the task that does the
step), `params` (with `answer` when a person decided before it, `feedback` when the work
is done again), `gate` (what done is on the object), `phase`, `refines` (a planning step:
its result may add steps).

**A step with a task and no skill is done by that task's instructions**: read them with
`ktw-agents tasks get --name <task> --out json` (its `body`) before doing anything — they
say what to report (a reconcile's delta and its operations, an apply's `applied`, …).
A step whose `work.doer` is `persona` names a **persona**, not a task, in `task` (a
recipe role the persona catalog carries: `reviewer`, `specifier`, `tester`, …) —
`tasks get` does not know it: read it with
`ktw-agents persona resolve --name <task> --out json` (its `skills` to load, its
`default_gate`, what it may write) and do the step as that persona.

Get the object's context first: `ktw-agents:/skill/node-context`.

## 3 — Do it

Follow the step's skill and instructions, on the object, within the modules it writes.
- Long work: renew the lease before it ends —
  `ktw-agents todos renew --id <todo> --by <you>`. A lease that passes makes the to-do
  anyone's again: your report is still taken while no one has pulled it again, refused
  once another worker has.
- Coherence before building: does it serve its parent goal, break no rule, contradict no
  sibling criterion? If not, report it blocked with the question that failed.
- Verify for real: a check that cannot fail proves nothing; never weaken a check to pass.
- Test what the step touches, in the project's quick lane (in KaTyoti: `bash scripts/test-affected.sh --paths <the files changed>`, the test cache kept); never the project's whole suite - several workers share the machine: the full run is the final gate, run once, one at a time.
- A **module-update** step (skill `ktw-agents:/skill/module-update`) — or the one step of a
  quick template or of maintain, which folds it in — brings the module up to date with the
  change: the feature and its use cases, the acceptance check that proves it, the pages,
  the change log naming the request; it records them with `requests module-update`, and
  in a git project commits them before reporting - the criterion file it added
  (`modules/<m>/use-cases/AC-*`) with its verdict: a report whose check or entry is
  untracked is not collected. A
  request that changes a module ends only so (the module-updated gate): a refused end
  plans the step again (`module-update (again)`), what is missing as its feedback.

## 4 — Report the result

```
ktw-agents todos report --id <todo> --by <you> --status success --summary "<what was done; how it was proven>"   [--details "<markdown: what changed and where, the tests run, what was decided and why>"]
```
`--summary` is **one line**: what a person reads first in the request's steps. When there
is more to say, it goes in `--details` (markdown), shown on demand behind *Details…* under
the summary — a summary written on several lines keeps its first line, the rest becomes
its details. Write the details for the person who checks the work: the changes and where,
the commits, the tests and checks run, the choices made and why.

**Every report's details end with a « Difficulties » section** - what slowed the step or
cost it time, one line each, starting with its kind, so they can be counted across steps:
`- <kind>: <one line>`, the kind one of `test` (hard to write, slow, flaky), `tool` (a
command broken or misleading), `lock` (a module lock, a claim), `permission`, `instruction`
(a step, skill or brief wrong or unclear), `retry` (redone, reported twice), `commit` (a
shared-tree clash, an index lock), `environment` (hub, build, installation), `other`. None:
write `- none`. Anything worth fixing goes also in `follow_ups` below, so it is filed.
`--more` carries, beside what was done, what planification acts on (JSON):
- `"add": [{"step":"…","task":"…","instructions":"…"}]` — steps the work needs after it,
  in order (a planning step refines the plan; `"doer":"person"` with `"interaction"` asks
  a person); what came after it waits on them. A go or a decision your work leaves to a
  person (a cutover, a migration, a release, an import) is such a step —
  `{"step":"go","doer":"person","interaction":"validate","instructions":"<what the go starts>"}`
  — never only a sentence in your details: a person sees it in « À faire »;
- `"follow_ups": [{"title":"…","details":"…","kind":"fix"}]` — other work found on the
  way, filed as requests of the module (`"module"` to file it in another), naming where
  it was found - a blocked or failed report's as well as a success's; the request ends
  without them. Add `"own": true` to one that is the
  request's own work done in a request of its own: linked to the request, it holds it
  open until it ends;
- `"choice": {"question":"…","options":[{"label":"…","means":"…","pros":["…"],"cons":["…"],"cost":"…","consequence":"…"},…],"proposal":{"option":"<a label>","why":"…"},"challenges":["…"]}`
  — the work ends on a choice a person makes (an analysis with options, a review that
  needs an arbitration): planification opens the decision in the request's module, linked
  to the request, and plans the person's decide step on it in place of the validation
  that followed (or after your step). At least two options, short labels, the proposal one
  of them; `context` defaults to your summary and details. Never ask the choice in your
  summary for a person to answer in an approval note.

Cannot finish:
- `--status fail --summary "<what failed>"` — it waits until someone retries it
  (`work retry`);
- `--status blocked --summary "<what stops it; what would lift it>"`;
- `todos release --id <todo> --by <you> --note "<why>"` — give it back, for anyone.

The answer says what followed (`note`): the step recorded on the request, the next to-dos
released, steps added, the request done — or blocked, saying why (a deliverable's check
failed). Then pull the next one.

## Inside KaTyoti

The internal scheduler is one more worker: for the modules in mode `auto`, it pulls their
auto to-dos and runs the task `work-todo` on each (`ktw-agents todos run --id <todo>`
does it for one to-do, in any mode). The task follows this contract with its verbs as
tools — `todos-get`, `todos-renew`, `todos-report`, `todos-release` — and the object's
context through its cwm and object-context tools. A run that ends without a report is
reported failed for it.

## Rules

- A report is final: it is collected the moment it is accepted. Never send one to try a
  field or learn a format — read the step's skill or task instead.

- One held to-do at a time; never hold one across a person's wait: report it first (`blocked` with your question if you are the one waiting). While a person's step waits, pick the next takeable to-do, from another request if need be.
- Never work a to-do you did not pull.
- Never edit a to-do's or a request's file by hand; every move goes through the verbs.
- A person's verdict, decision, answer or verification is theirs: never report their to-do
  nor verify for them, even with their `--by` (refused from an agent's session) — tell
  them where to answer: the Inbox, or the request view.
- New work found on the way is a follow-up, never done silently.
