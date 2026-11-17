// SPDX-License-Identifier: Apache-2.0
/**
 * The first guides a new universe can start with: the guides
 * of the sidecars picked for it, read from the universes the GUI reaches (the
 * new universe does not run yet), each once, by name. None is chosen by
 * default; the service refuses a guide of no chosen sidecar.
 */

export interface GuideChoice {
  value: string
  label: string
}

export interface KnownGuide {
  uri: string
  provider: string
  name: string
}

/** The guides offered for the sidecars picked, sorted by name. */
export function firstGuideChoices(guides: KnownGuide[], sidecars: string[]): GuideChoice[] {
  const picked = new Set(sidecars)
  const seen = new Set<string>()
  return guides
    .filter((g) => picked.has(g.provider) && !seen.has(g.uri) && (seen.add(g.uri), true))
    .map((g) => ({ value: g.uri, label: g.name || g.uri }))
    .sort((a, b) => a.label.localeCompare(b.label))
}

/** What the form sends: the guide chosen while it is still one of the
 *  choices (a sidecar unpicked takes its guides away), else none. */
export function chosenFirstGuide(chosen: string | null, choices: GuideChoice[]): string | undefined {
  return chosen && choices.some((c) => c.value === chosen) ? chosen : undefined
}
