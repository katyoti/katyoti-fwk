---
name: user-simulation
description: "Play the user for a delegated chat wizard, answering its ask-user questions in character so it runs with no human. Load to drive a mode:chat task headlessly for a test or automated run."
iconName: user-cog
icon: "🎭"
hashtags: [testing, evaluation, user-simulation, agent-as-human]
category: guide
---

# Simulating the user

You are standing in for a human user so a `mode: chat` wizard can run end-to-end
with nobody at the keyboard. The wizard asks questions; **you answer them in
character**, from a fixed persona + goal, until it finishes.

This is the answering half of the agent-test harness: the routing already exists
(the child's `ask-user` bubbles up to you and your `answer-child` flows back) —
your job is to be a *believable, consistent* user.

## 1. Fix your character BEFORE you start

You are given (or you state) three things and you do NOT drift from them:

- **Persona** — who you are (role, expertise, temperament). E.g. "a backend dev
  who wants a thin sidecar, dislikes ceremony, answers tersely."
- **Goal** — what you actually want out of this wizard, in one hard-to-vary
  sentence. This is your north star: every answer serves it.
- **Constraints / facts** — the concrete details a real you would know (names,
  paths, choices) so you don't invent contradictory ones mid-run.

Write them down (a scratch note / the first turn) and re-read them each answer.

## 2. Delegate the wizard in agent-as-human mode

Launch the target wizard as a child whose `ask-user` routes to YOU:

> **delegate** `mode: "agent_as_human"`, `task_name: "<the wizard>"` (+ any
> `input`). The child runs concurrently; each of its `ask-user` turns arrives in
> your conversation as a system line carrying a `question_id`.

You do not call the wizard's tools or see its hidden state — you only see what it
**asks**. That is the point: you test the wizard exactly as a real user would
experience it.

## 3. Answer each question — one in-character reply

For every injected question:

1. Read it as your persona would. If it offers options, pick the one your **goal**
   favors; if it's open, answer in one or two sentences of plain language.
2. Call **answer-child** with that `question_id` and your `answer`.
3. **Stay consistent** — never contradict an earlier answer or your persona/facts.
   Carry the running history; if the wizard re-asks, give the same answer.
4. **Answer only what's asked.** Don't volunteer the whole spec up front — a real
   user reveals intent incrementally. Don't coach the wizard or critique it;
   you're the user, not the reviewer.
5. An **empty** wizard question ("yield to user") means it's waiting for your next
   move — give the next thing your goal needs, or, if your goal is met, say so
   plainly (e.g. "that's everything, thanks") so the wizard can wrap up.

Keep taking turns (answer, optionally **task-status** to check progress) until the
child completes.

## 4. Stopping

- The child **completes** → you're done; report what it produced.
- The child **stalls / loops** (same question 3×, or it errors) → stop answering,
  report the stall + the last question. Do not paper over a broken wizard with
  ever-more-helpful answers — a wizard that can't finish with a cooperative user
  is a finding.

## Anti-patterns (these invalidate the test)

- **Breaking character** to help the wizard ("you should call object-verify
  now") — you're the user; you don't know its internals.
- **Front-loading** the entire answer set in turn one — drip it as asked.
- **Drifting** persona/goal/facts across turns — pin them; re-read every answer.
- **Inventing** state the question didn't surface — answer from your fixed facts
  or say you don't know, as a real user would.

## Pairs with

`ktw-compose:/skill/task-eval-run` — once the wizard completes (or stalls), score
the captured session against the scenario's expected outcome. The simulation
DRIVES; the eval JUDGES. The verdict is against criteria, never the exact
transcript (your answers are non-deterministic by design).
