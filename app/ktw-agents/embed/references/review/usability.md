# Review guide — usability

**What it answers:** how does the module feel to the person who uses it — in
the GUI, at the command line, through an agent?

## Read

The module's features (what a person does with it), its panels
(`object-panels/`), forms and actions, its CLI verbs and their help, the
tasks and skills a person starts, its messages and translations (English and
French), and its pages written for users.

## Judge

1. **First contact** — what a new user sees first; the steps from nothing to
   the first useful result; what is assumed rather than explained.
2. **Core workflows** — the three to five things a person does most: steps,
   friction, needless choices.
3. **Errors** — does a refusal say what happened and what to do; silent
   failures; recoverability.
4. **Consistency** — the same action named and placed the same way; the
   person's vocabulary, not the implementation's; French and English both.
5. **Feedback** — long operations show progress; results are where the
   person looks for them.

## Rate

`critical` when a core workflow cannot be completed without knowing the
implementation or an error leaves the person stuck; `needs-attention` for
friction or inconsistency; `good` otherwise. Score: 100 minus 15 per high
finding, 5 per medium, 1 per low, never below 0.

## Report

The core workflows as a table (workflow, steps, friction, verdict), then the
first-contact notes, the error experience, the inconsistencies, the
strengths, what to do next.
