---
name: review-dimension
description: "Perform one review type on one object (a module most of the time) by its guide: read what the guide says to read, judge what it says to judge, and answer a review result — a rating, a report and findings that a later review recognises. Load when a review-type's runner names this skill."
hashtags: [reviews, review, quality]
category: analysis
---

# review-dimension — one review type, by its guide

A review type that an agent performs names this skill as its runner and gives
its **guide** (`input.guide`, a reference URI such as
`ktw-agents:/reference/review/documentation.md`). The guide says what to read,
what to judge and how to rate; this skill says how every review runs and what
it answers. You run in a review's run: no question to a person, no file
written — the reviews service files what you answer.

## 1 — Read the object and the guide

- The object reviewed (`object`): read it with the cwm tools. For a module:
  its object (`modules/<m>/objects/<m>.cwm.yaml`: its `code_packages`, its
  parent), its pages (`modules/<m>/*.md`), its goals, features and their
  acceptance criteria, its decisions and rules (`modules/<m>/goals/`,
  `features/`, `decisions/`, `rules/`), its open requests
  (`ktw-agents requests list --module <m>`), and its code when the guide asks
  for it.
- The guide (`input.guide`): read it whole, and follow its process.
- The previous review of the same type, if the module has one (a `review`
  under it with the same `type`): read its findings, so that a problem still
  there is reported with the same location and key.

## 2 — Judge

Follow the guide's process on what you read. Every finding is something you
saw: name where (a file and its lines, a page and its section, an object id)
and why it matters. Do not invent one to fill a section; a dimension with
nothing wrong is rated `good` with an empty list.

## 3 — Answer the review result

Call complete with:
- `rating` — `good`, `needs-attention` or `critical`, by the guide's rating
  rule (and `score`, 0 to 100, only when the guide defines one);
- `summary` — the rating in two or three sentences;
- `report` — the guide's output format, in markdown: what was looked at, what
  was found, the strengths, what to do next;
- `findings` — each with `severity` (`critical`, `high`, `medium`, `low`,
  `info`), `statement` (one sentence), `location`, `recommendation`,
  `evidence`, and `key`: the rule or subject in a few stable words (the same
  words every time the same problem is found — a later review matches a
  finding by type, location and key).
