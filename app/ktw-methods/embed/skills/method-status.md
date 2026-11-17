---
name: method-status
description: "Grade a module by running its checks: OK, KO or UNPROVEN per object, a rollup, progress and the blocking frontier. Load for where a module stands, before pulling its next piece."
hashtags: [method, measurement, gap, status]
category: evaluation
---

# method-status — where are we, and can we prove it

Covers three of the method's six functions at **L0**: **ObserveActual** (read the
objects), **ComputeGap** (roll up), **Measure** (run the checks). Doctrine:
`modules/method/concepts.md` §3.

## The one inviolable rule

> **Only a run produces a verdict. You may never author one.**

An `OK` you did not obtain by executing a command and seeing it succeed is a
**fabricated fact** — the single failure this whole method exists to prevent. If you
cannot run it, the answer is `UNPROVEN`, never `OK`. Reasoning about whether something
"is probably fine" is not measurement.

## Procedure

1. **Locate the objects** — every `*.cwm.yaml` and `*.cwm.md` (frontmatter)
   under `modules/<name>/`: the module object in `objects/`, its `goal`s in
   `goals/`, its `feature`s in `features/`, the `acceptance-criterion`
   objects under them (`<id>-AC<n>`, in the folder their type declares), its
   `decision`s in `decisions/`, its `rule`s in `rules/`.
   `source modules/module/probes/_objects.sh` and call `object_files` to list
   them (the one object glob every probe uses).
   No module given → do every **declared bundle**: a folder under `modules/` is
   one iff it holds an object whose id is the folder name and whose type
   carries the `module` capability. `bash modules/module/probes/_bundles.sh` and
   call `bundles` to list them.
2. **Read each object**: `id`, `type`, `parent`, `name`,
   `capabilities.want.reachability`,
   `capabilities.checkable.{kind,probe,timeout,note,blocked_by}`, its `serves` edges.
3. **Build the tree** from `parent` (an acceptance criterion sits under its
   feature, directly or under one of its sub-features).
4. **Evaluate each object** (see the verdict table below).
5. **Roll up** (see the rollup rule).
6. **Report** in the output format below.

## Evaluating one object

| `checkable.kind` | What you do | Verdict |
|---|---|---|
| `command` | **Run the probe from the repo root**, stopped after 10 minutes — or after the `timeout` its checkable declares (`30m`, at most `1h`), and then say it ran on that extended bound. Exit 0 → `OK`; non-zero or stopped → `KO` | OK / KO |
| `command` whose `timeout` is no duration, or above an hour | do not run it on a bound nobody declared | `UNPROVEN` · `run-failed` |
| `command` whose probe is `bash modules/<name>/probes/<x>.sh` | same — the probes live beside the objects | OK / KO |
| `command` but the command itself cannot execute (tool missing, path gone) | do not guess | `UNPROVEN` · `run-failed` |
| `expression` | L0 does not evaluate expressions | `UNPROVEN` · `no-probe` |
| `manual` | a human claim never auto-passes | `UNPROVEN` · `no-probe` |
| `none` | no probe declared — report `blocked_by` | `UNPROVEN` · `no-probe` |
| *(no `checkable`, and NO children)* — a leaf want with nothing to check | nothing to measure | `UNPROVEN` · `no-criteria` |
| *(no `checkable`, but HAS children)* — a container | its verdict **is the rollup** of its descendants | green / not-green (never `no-criteria`) |

**`run-failed` is not `KO`.** "The check could not run" and "the check ran and failed"
are different facts. Never collapse them.

## Grading a MILESTONE

A `milestone` is a **named set of members**, not a thing with its own state. So:

> **A release stores no grade. Its grade is the rollup over its members.**

1. Its members: its `judged-by` targets when there are any (the members that
   judge delivery), else every object its `takes` edges name, with the
   requests whose own `targets` edge names it.
   `python3 modules/module/probes/_grade.py --release <id>` does this
   (`ktw-agents releases grade --id <id>` through the service).
2. Grade each member as usual.
3. Roll up with the SAME rule below.

**A member id that resolves to no object is a `KO`, never a skip.** Dropping it
makes the milestone silently smaller, and a rollup over 4 of 5 members reads
greener than the truth.

**Never read or write a status/progress field on a milestone.** If you find one,
report it — that is the shape the method deliberately superseded
(`concepts.md` §2, "this is a scar").

## The rollup rule

> A parent is **green only when every descendant is `OK`.**

`KO` **and** `UNPROVEN` both block green — so "no criteria" and "not yet probed" can
never masquerade as success. Also report:

- **progress** = `OK / total` (steering signal — never a substitute for green);
- **frontier** = the specific objects blocking green.

A **goal** is always asymptotic (`want.reachability: asymptotic`): it never reads
green even at 100% — report its distance (the features that serve it, and theirs),
not a green. That is not a bug; it is what a goal is. A **feature** is always
`closable`: it is green when every acceptance criterion under it (directly or
through its sub-features) is `OK`. The ladder is AC → feature → goal: "use
case" names a small feature or a test scenario, never an object to grade.

## Probe hygiene — before you accept a green

A convenient green is the most dangerous output this skill can produce. For every `OK`,
ask: **could this probe have failed?**

- Prefer **"nothing is wrong"** over **"something is right"**.
  `test -z "$(list-the-bad)"` is trustworthy; `list-the-things | grep -q .` is not — it
  passes as soon as *one* item exists, regardless of the rest.
- If a probe would pass on an empty or irrelevant repo, it is **untrusted**.
- **Beware vacuous passes from test runners.** `go test -run <name>` exits **0** when the
  filter matches nothing ("no tests to run"). A probe that names a test which does not
  exist therefore reports OK forever. Always assert the test **exists** as well as passes:
  `grep -rq "func TestX" . && go test ./... -run TestX`.
- When a green looks too easy, **inspect what the command actually matched** before
  accepting it.

Mark such a probe `untrusted`, **do not count it toward green**, and say so. *(This rule
was earned: a `git ls-files … | grep -q .` probe reported OK while 10 of 14 objects were
untracked.)*

## Output format

```
module <name> — <GREEN | NOT GREEN>   (progress N/M)

OK        <id>    <command that proved it>
KO        <id>    <command> → <what failed>
UNPROVEN  <id>    <reason>  (<blocked_by, if any>)

frontier: [<ids blocking green>]
```

Keep it scannable. State the **command** next to every `OK` — that is the evidence, and
a reader must be able to re-run it.

## What this skill must NOT do

- **Never** write a verdict you did not obtain from a run.
- **Never** edit a probe to make it pass. Weakening a bar to get green is the exact
  dishonesty the method forbids; definitions are versioned so that stays visible
  (`concepts.md` §6). If a probe is wrong, say so and stop — do not silently retune it.
- **Never** report `UNPROVEN` as green, or fold it into progress as if passing.
- **Never** author or modify objects. This skill **reads and runs**; authoring goals,
  features and acceptance criteria belongs to `define-goals`, `new-functionality` and
  `isa`, choosing the next piece to `module-next`.

## See also

- `modules/method/concepts.md` §3 — the measurement discipline and the verdict set.
- `modules/method/functions/compute-gap.md` · `functions/measure.md` — the contracts.
- `modules/cwm/` — the first real object set to grade (its goals, features and their criteria).
- `modules/module/` — the bundle rules and `probes/_bundles.sh`.
- the project releases in `sys-planification` — selections; `module-merge` is the first.

## This skill needs no CWM service

It reads YAML **files** and runs shell probes. That is deliberate: it is the
method's specified L0, not a stopgap. It keeps working while the CWM storage
refactor is in flight, and it would keep working if CWM were switched off
entirely — which is also what makes it a trustworthy check ON that refactor.
