// SPDX-License-Identifier: Apache-2.0
// The universe editor's solution action: a universe made before
// solutions - its Univers.yaml names none - is given an installed one through
// the orchestrator's set-universe-solution. The service decides what is
// installed and refuses the rest; these helpers only say when the action is
// offered and what to suggest.

/** The action is offered only to a universe that has no solution: moving one
 *  that has a solution to another version is the solution's update. */
export function offersSolutionChoice(summary: { solution?: string } | null | undefined): boolean {
  return !!summary && !(summary.solution ?? '').trim()
}

/** The solutions installed on this machine as the universes of the list show
 *  them (`<solution>@<version>`, each once, sorted) - the suggestions of the
 *  choice. The installation's own solution may not be among them: the field
 *  takes any name and the service answers. */
export function installedSolutionChoices(universes: ReadonlyArray<{ solution?: string }>): string[] {
  const seen = new Set<string>()
  for (const u of universes) {
    const label = (u.solution ?? '').trim()
    const [name, version] = label.split('@')
    if (name && version) seen.add(label)
  }
  return [...seen].sort()
}
