// SPDX-License-Identifier: Apache-2.0
/**
 * The guide player's state: which recipe plays, at which step, and what the
 * person has completed. The overlay (GuidePlayer.tsx) renders it; anything in
 * the GUI starts a recipe with `startRecipe(uri)`.
 *
 * Completions (modules/guide/concepts.md): the last check of a recipe passing
 * records `checked`; a tour played to its end records `seen`; skipping or
 * closing early records nothing.
 */

import { withFurther } from './further'
import { create } from 'zustand'
import { useAppStore } from '../store'
import { profileClient } from '../ktsp'
import type { LoadedRecipe } from './recipes'
import { fetchRecipe, isTour, nature, onceId, type RecipeEntry, type Translations } from './recipes'

export type PlayerStatus = 'idle' | 'loading' | 'playing' | 'error'

export type CompletionHow = 'checked' | 'seen' | 'skipped'

/** A step whose control could not be found — the author's report. */
export interface MissingControl {
  recipe: string
  step: number
  target: string
  at: string
}

interface GuidePlayerState {
  status: PlayerStatus
  uri: string | null
  recipe: LoadedRecipe | null
  /** The playing recipe's translations, when it has some. */
  translations: Translations | undefined
  step: number
  error: string | null
  /** Every missing control seen this session, newest last. */
  missing: MissingControl[]
  /** Recipe identities the person completed (from the profile). */
  completed: Set<string>
  completionsLoaded: boolean
  /** The Show me or Explain me open in the reader, if any. */
  reading: RecipeEntry | null
  /** The shown-once guides still to open at this start, and the one open. */
  onceQueue: RecipeEntry[]
  onceCurrent: RecipeEntry | null

  start: (uri: string) => Promise<void>
  goTo: (step: number) => void
  next: () => void
  back: () => void
  /** Leave the player. A tour closed on its last step counts as seen. */
  close: () => void
  reportMissing: (step: number, target: unknown) => void
  refreshCompletions: () => Promise<void>
  complete: (recipe: string, how: CompletionHow) => Promise<void>
  /** Open a Show me or an Explain me in the reader. */
  read: (entry: RecipeEntry) => void
  closeReading: () => void
  /** Open the shown-once guides of this start, one after the other. */
  runOnce: (queue: RecipeEntry[]) => void
}

function setOf(list: { recipe: string }[] | undefined): Set<string> {
  return new Set((list ?? []).map((c) => c.recipe))
}

export const useGuidePlayer = create<GuidePlayerState>((set, get) => ({
  status: 'idle',
  uri: null,
  recipe: null,
  translations: undefined,
  step: 0,
  error: null,
  missing: [],
  completed: new Set(),
  completionsLoaded: false,
  reading: null,
  onceQueue: [],
  onceCurrent: null,

  start: async (uri) => {
    set({ status: 'loading', uri, recipe: null, translations: undefined, step: 0, error: null })
    try {
      const entry = await fetchRecipe(uri, useAppStore.getState().displayedUniverseName)
      if (get().uri !== uri) return // another start won
      if (entry.recipe.steps.length === 0) throw new Error('the recipe has no steps')
      // « Pour aller plus loin » ends it when it names what to read next.
      set({ status: 'playing', recipe: withFurther(entry.recipe), translations: entry.translations, step: 0 })
    } catch (e) {
      if (get().uri !== uri) return
      set({ status: 'error', error: e instanceof Error ? e.message : String(e) })
    }
  },

  goTo: (step) => {
    const r = get().recipe
    if (!r) return
    set({ step: Math.max(0, Math.min(step, r.steps.length - 1)) })
  },

  next: () => {
    const { recipe, step, uri } = get()
    if (!recipe || !uri) return
    if (step < recipe.steps.length - 1) {
      set({ step: step + 1 })
      return
    }
    // Past the last step: a tour played to its end is seen. A recipe's
    // completion was recorded when its last check passed.
    if (isTour(recipe)) void get().complete(uri, 'seen')
    set({ status: 'idle', uri: null, recipe: null, step: 0 })
  },

  back: () => {
    const { step } = get()
    if (step > 0) set({ step: step - 1 })
  },

  close: () => {
    const was = get().uri
    set({ status: 'idle', uri: null, recipe: null, step: 0, error: null })
    nextOnce(was ?? undefined)
  },

  read: (entry) => set({ reading: entry }),

  closeReading: () => {
    const was = get().reading
    set({ reading: null })
    nextOnce(was?.uri)
  },

  runOnce: (queue) => {
    if (queue.length === 0) return
    const { onceCurrent, onceQueue } = get()
    if (onceCurrent) {
      // Another universe's start while one plays: its guides
      // follow, each once.
      const held = new Set([onceCurrent.uri, ...onceQueue.map((e) => e.uri)])
      set({ onceQueue: [...onceQueue, ...queue.filter((e) => !held.has(e.uri))] })
      return
    }
    set({ onceQueue: queue.slice(1), onceCurrent: queue[0] })
    openGuide(queue[0])
  },

  reportMissing: (step, target) => {
    const { uri, missing } = get()
    if (!uri) return
    const t = JSON.stringify(target)
    if (missing.some((m) => m.recipe === uri && m.step === step && m.target === t)) return
    console.warn('[guide] control not found', { recipe: uri, step, target })
    set({ missing: [...missing, { recipe: uri, step, target: t, at: new Date().toISOString() }] })
  },

  refreshCompletions: async () => {
    try {
      const out = await profileClient().guideGet()
      set({ completed: setOf(out?.completed), completionsLoaded: true })
    } catch (e) {
      console.warn('[guide] cannot read completions', e)
      set({ completionsLoaded: true })
    }
  },

  complete: async (recipe, how) => {
    try {
      const out = await profileClient().guideComplete({ recipe, how })
      set({ completed: setOf(out?.completed) })
    } catch (e) {
      console.warn('[guide] cannot record completion', recipe, how, e)
    }
  },
}))

/** Opens a guide by its nature: a Help me in the player, a Show me or an
 *  Explain me in the reader. */
export function openGuide(entry: RecipeEntry): void {
  if (nature(entry.recipe) === 'help') void startRecipe(entry.uri)
  else useGuidePlayer.getState().read(entry)
}

/** The shown-once guide just closed is seen; the next of the queue opens. */
function nextOnce(closedUri: string | undefined): void {
  const s = useGuidePlayer.getState()
  const cur = s.onceCurrent
  if (!cur || cur.uri !== closedUri) return
  void s.complete(onceId(cur), 'seen')
  const [next, ...rest] = s.onceQueue
  useGuidePlayer.setState({ onceCurrent: next ?? null, onceQueue: rest })
  if (next) openGuide(next)
}

/** Starts playing a recipe by its URI (`<provider>:/guide-recipe/<id>`). */
export function startRecipe(uri: string): Promise<void> {
  return useGuidePlayer.getState().start(uri)
}
