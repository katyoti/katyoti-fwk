---
name: module-update
description: "Bring a module up to date with the change a request made: the feature it serves and its use cases, the acceptance check that proves the change, the module pages updated in place, the change-log entry naming the request — and record it on the request. Load for a request's module-update step, or before ending a build, fix or maintain request."
hashtags: [planification, module, documentation, changelog, acceptance, workflow]
category: methodology
---

# module-update — the module says what is true once the change is made

A request whose work changes a module — its template is build, fix, maintain or
feature-build — ends with its module up to date. Its **module-update** step (after the
implementation, before the person's review or the verification; folded into the one
step of a quick template or of maintain) does it, and the request's end checks it: the
**module-updated** gate. You do the step; planification decides whether the request
may end.

`--by` is who acts, **always a JSON actor in single quotes**: a session
`'{"id":"<session name>","kind":"agent"}'`. `<you>` below stands for it.

## 1 — Read what changed

`ktw-agents requests get --id <request> --out json`: its title and details, the want it
**serves** (`serves`, or the object it is filed on when that is a feature, a use case or
a goal), its steps — what the implementation did (its result and details). Get the
want's context (`ktw-agents:/skill/node-context`): its use cases, its acceptance checks
(objects carrying `checkable`), the module's pages.

No want served: name it — `ktw-agents requests assess --id <request> --serves <want id> --by <you>`
— or, when none fits, file the feature it needs (an idea, in the owning module) and
serve it; never invent a link that is not true. **Direct work** — a fix, or a build at
depth quick, for a small change (a label, a message, a small UI or doc fix) — serves no
want and files no feature: skip step 2.1; add a test case when its behaviour changes,
else record why none can (`--no-check "<what changed and why no test sees it>"`).

## 2 — Update the module

1. **The feature and its use cases** say what the module now does: a new behaviour is a
   use case (or a criterion under one); a changed one is rewritten, in place.
2. **The acceptance check that proves the change**: a criterion under the want whose
   probe passes with the change and failed before it (a command — a `go test -run` — or
   an expression). Add one, or change the one that covers the behaviour. It stays: it is
   the change's non-regression. Run it once.
3. **The module pages** (`modules/<m>/*.md`) state what IS, rewritten in place: no
   history, no dates, no request number.
4. **The change log**: a change a user can see is one entry of the open release, in
   French, by what the user gets, with its Valeur and Maturité, **naming the requests it
   delivers** — added as data, never by editing the page:
   ```
   ktw-agents changes add --release <version> --title "<the gain>" --domain "<domain>" \
     --value major|notable|improvement|fix --text "<Ce que ça vous apporte… Le progrès…>" \
     --requests <request> --update "<its Dernière mise à jour line>"
   ktw-agents changes render --release <version> --write
   ```
   The render writes `docs/confluence-tew/changelog/<version>.md` from the entries.
   A change no user sees (a refactor, a test, an internal API) has no entry: say why.

## 3 — Record it on the request

```
ktw-agents requests module-update --id <request> --by <you> \
  --changelog <entry id> \
  --checks <criterion id>[,<criterion id>]
```
or, for a change no user sees: `--internal --reason "<why no user sees it>"`
instead of `--changelog`. The checks you name must be acceptance checks you added or
changed **since the request was filed**: the gate reads when each one last changed.
Check before you report: `ktw-agents requests check-module-updated --id <request> --out json`
answers `accept`, or what is still missing.

## 4 — Commit, then report

In a project kept in git, commit what the step wrote **before** reporting it, by explicit
pathspec: the criterion it added (`modules/<m>/use-cases/AC-*.cwm.yaml`, or wherever the
criterion was written - `git status --short modules/<m>` lists it), its verdict, the
change-log entry (`modules/sys-planification/.ktw/changes/CH-*`) and the rendered page, the
pages, the request file (to-dos are the scheduler's rows, never committed). The report is not collected while a check or the
entry the module update records is untracked: it names the file to commit.


`todos report` with what you updated (the use cases, the check and its probe, the pages,
the change-log entry). When the end is refused, the step comes back to you
(`module-update (again)`) with what is missing as its feedback: do just that.

## Rules

- Never weaken a check to make it pass; never name a check you did not change.
- The person's review comes after this step: leave the module in the state you would
  show them.
- Only a person waives a module update (`requests waive`, with why): an agent never does,
  and never asks for it to skip the work.
- A go or a decision the work leaves to a person (a cutover, an import, a release) is
  added as a person's step (`--more '{"add":[{"step":"go","doer":"person","interaction":"validate","instructions":"…"}]}'`),
  never left as a sentence in your details: only a step shows in their « À faire ».
