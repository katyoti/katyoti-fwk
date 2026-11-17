# Review guide — goals versus state

**What it answers:** does what the module has built and is building serve
what it says it wants — and what serves nothing?

## Read

The module's goals (`goals/`), its features with their `serves` edges and
acceptance criteria and their verdicts, its open and finished requests
(`ktw-agents requests list --module <m>`), its decisions (open and applied)
and rules. Its code only to confirm a feature is realised.

## Judge

1. **Coverage** — for each goal: the features serving it, how many of their
   criteria pass; a goal with no feature, or only features with no passing
   criterion, is a gap.
2. **Drift** — a request, feature or decision working against a goal, or a
   feature marked realised whose criteria fail.
3. **Orphans** — features serving no goal, requests about nothing the module
   wants, decisions open with nothing deferred against them, rules with no
   reason.
4. **Progress** — what moved since the previous review of this type (its
   report, if there is one).

## Rate

`critical` when a goal the module exists for has no working feature or a
realised feature's criteria fail; `needs-attention` for gaps, drift or
orphans; `good` otherwise. Score: the share of the goals' criteria that pass
(0 to 100).

## Report

A table of goals (goal, state, features, criteria passing, gaps, drift risk),
then the gaps, the drift, the orphans, the progress since the previous
review, what to do next in order.
