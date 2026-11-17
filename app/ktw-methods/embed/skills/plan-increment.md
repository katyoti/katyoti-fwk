---
name: plan-increment
description: >-
  Plan the next increment — WSJF-rank the READY backlog cards, pull the
  top up to team capacity, and trigger just-in-time elaboration on each
  pulled card. Use when the user asks "what's next / plan the increment /
  pull from the backlog / what should we work on". NOT for adding new
  ideas to the backlog (use new-functionality), NOT for defining goals
  (define-goals), NOT for the per-use-case ISA articulation.
icon: "📐"
iconName: layout-board
hashtags: [increment, planning, wsjf, backlog, pull, capacity]
category: lifecycle
---

# Plan an increment (WSJF pull + JIT elaboration)

Operationalizes the lean-increment-flow doctrine
(`modules/method-isa/lean-increment-flow.md` §4–§6): take the backlog of
`ready` cards (each carrying `business_value` + `effort`), pull the highest-WSJF
ones up to team capacity into the increment, and **only then** elaborate them.

> **The rule:** you elaborate a card WHEN you pull it — never before. Pulling is
> the event that raises a card from a lean one-liner to a worked item (ACs,
> tasks). Cards left in the backlog stay one-liners. This is what keeps the plan
> cheap to re-order.

## Inputs the plan needs
- The **backlog** of cards, each with `business_value` (1–10) + `effort`
  (Fibonacci-ish). A card is a `feature` object; its stored ordering signal
  is its cost of delay (`cod`, capability `feature`). If a card lacks value
  or effort, get them first — WSJF needs both.
- **Team capacity** — the sum of effort points the team(s) can take this
  increment. Ask if unknown; don't guess.
- Any **dependencies** between cards (a card can't precede what it needs).

## Procedure

1. **Confirm cards are `ready`.** A card is pullable when it has value + effort
   and its dependencies are known. Cards still `idea` get shaped first
   (`new-functionality`) — not pulled.
2. **Rank by WSJF** = `business_value ÷ effort`, descending. (Full SAFe form is
   Cost-of-Delay ÷ job-size; start with value÷effort.)
3. **Pull to capacity.** Walk the ranked list top-down; pull each card whose
   dependencies are satisfied until the teams' capacity is filled. Mark pulled
   cards committed (e.g. `Status: executing` / lean-flow-state `committed`);
   leave the rest `ready`.
4. **Map to teams** (optional, if >1 stream) — assign pulled cards to teams
   within capacity (`build-team` wires the team).
5. **JIT-elaborate each pulled card** — NOW hand it to the `specifier` persona
   (or the relevant author skill) to write its ACs/tasks. Do this only for
   pulled cards.
6. **Report the increment**: the committed set, total effort vs capacity, what
   was left in the backlog and why (capacity / dependency).

## Done when
- The increment holds a capacity-bounded set of the highest-WSJF ready cards.
- Each pulled card is (or is being) elaborated; none of the un-pulled ones are.

## Don't
- Don't elaborate cards you didn't pull — that's the premature-elaboration churn
  this whole flow exists to prevent.
- Don't pull past capacity, or pull a card before its dependency.
- Don't plan increments beyond the current one — the next increment is planned
  when this one closes.
