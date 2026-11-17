// SPDX-License-Identifier: Apache-2.0
/**
 * GuideBrowser — the content of the guide tab and drawer (gt13).
 *
 * Recipes come from the admin universe's catalog (the shell recipes and the
 * welcome tour, provider ktw-hub) and from the catalog of the universe on
 * screen (its sidecars' recipes), deduplicated by URI. Search runs over name,
 * description and hashtags; *Suggested for you* is ranked here from what is
 * installed, what the person completed and what each recipe requires.
 *
 * Not shown: the "may be outdated" mark — the GUI has no binding that tells
 * the page its own version, so a recipe's `gui` range cannot be compared yet.
 */

import { useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  ActionIcon,
  Alert,
  Badge,
  Box,
  Button,
  CloseButton,
  Group,
  Loader,
  Paper,
  ScrollArea,
  Stack,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { IconBook2, IconCircleCheck, IconPlayerPlay, IconSearch, IconVideo } from '@tabler/icons-react'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { ADMIN_UNIVERSE } from '../ktsp'
import { useAppStore } from '../store'
import { useGuidePlayer, startRecipe } from './player'
import { completedHas, isCompleted, isTour, loadRecipes, nature, offeredIn, primaryAction, recipeText, SHELL_RECIPE_PROVIDER, type RecipeEntry, essentials, concepts, CONCEPTS_TOPIC } from './recipes'
import { useGuideT } from './i18n'
import { useAppLanguage } from '../i18n'
import { useDisplayLevel, useUniverseView } from '../hooks/useUniverseGui'

const LEVEL_RANK: Record<string, number> = { beginner: 0, regular: 1, expert: 2 }
const LEVEL_COLOR: Record<string, string> = { beginner: 'green', regular: 'blue', expert: 'grape' }
const NATURE_COLOR: Record<string, string> = { help: 'indigo', show: 'orange', explain: 'teal' }
const WELCOME_ID = 'welcome'

function norm(s: string): string {
  return s.toLowerCase().replace(/[-_]+/g, ' ').trim()
}

function matchesTopics(e: RecipeEntry, topics: string[]): boolean {
  if (topics.length === 0) return true
  // The help menu's « Concepts KaTyoti » keeps the concept documents.
  if (topics.includes(CONCEPTS_TOPIC)) return !!e.recipe.concept
  // Topics are English hashtags (the `?` buttons pass them): match the recipe's own.
  const tags = (e.recipe.hashtags ?? []).map(norm)
  const name = norm(e.recipe.name)
  return topics.map(norm).some((t) => name.includes(t) || tags.some((h) => h.includes(t) || t.includes(h)))
}

function matchesQuery(e: RecipeEntry, query: string, lang: string): boolean {
  const words = norm(query).split(/\s+/).filter(Boolean)
  if (words.length === 0) return true
  // The person searches in their language or in English: both are searched.
  const tx = recipeText(e, lang)
  const hay = norm([e.recipe.name, e.recipe.description ?? '', ...(e.recipe.hashtags ?? []),
    tx.name, tx.description, ...tx.hashtags].join(' '))
  return words.every((w) => hay.includes(w))
}

function providerLabel(p: string): string {
  return p === SHELL_RECIPE_PROVIDER ? 'KaTyoti' : p
}

/** Why a recipe is suggested: a `guide` text key and its values. */
interface Reason {
  key: string
  params?: Record<string, unknown>
}

interface Suggestion {
  entry: RecipeEntry
  reason: Reason
}

/** Not completed, every `requires` completed; the welcome tour first while it
 *  is not done, then beginner before regular before expert. */
function suggest(entries: RecipeEntry[], completed: Set<string>): Suggestion[] {
  const byId = new Map(entries.map((e) => [e.id, e]))
  const candidates = entries.filter(
    (e) => !isCompleted(completed, e) && (e.recipe.requires ?? []).every((r) => completedHas(completed, r)),
  )
  candidates.sort((a, b) => {
    const wa = a.id === WELCOME_ID ? 0 : 1
    const wb = b.id === WELCOME_ID ? 0 : 1
    if (wa !== wb) return wa - wb
    const la = LEVEL_RANK[a.recipe.level ?? 'regular'] ?? 1
    const lb = LEVEL_RANK[b.recipe.level ?? 'regular'] ?? 1
    if (la !== lb) return la - lb
    return a.recipe.name.localeCompare(b.recipe.name)
  })
  return candidates.slice(0, 3).map((entry) => {
    const req = entry.recipe.requires ?? []
    let reason: Reason
    if (entry.id === WELCOME_ID) reason = { key: 'browser.reasonWelcome' }
    else if (req.length > 0) reason = { key: 'browser.reasonNextAfter', params: { name: byId.get(req[req.length - 1])?.recipe.name ?? req[req.length - 1] } }
    else if (entry.recipe.level === 'beginner') reason = { key: 'browser.reasonFirstStep' }
    else reason = { key: 'browser.reasonReady' }
    return { entry, reason }
  })
}

function openVideo(url: string) {
  try {
    BrowserOpenURL(url)
  } catch {
    window.open(url, '_blank', 'noopener')
  }
}

export interface GuideBrowserProps {
  /** Narrow to recipes about these topics (the `?` buttons). */
  topics?: string[]
  onTopicsChange?: (topics: string[]) => void
  /** Called once a recipe starts (the drawer closes itself). */
  onStarted?: () => void
}

export function GuideBrowser({ topics = [], onTopicsChange, onStarted }: GuideBrowserProps) {
  const displayed          = useAppStore((s) => s.displayedUniverseName)
  const completed          = useGuidePlayer((s) => s.completed)
  const refreshCompletions = useGuidePlayer((s) => s.refreshCompletions)
  const t                  = useGuideT()

  const [loaded, setLoaded]   = useState<RecipeEntry[]>([])
  // A tagged universe offers its own guides and KaTyoti's usage ones.
  const view = useUniverseView(displayed)
  const entries = useMemo(() => offeredIn(loaded, view), [loaded, view])
  const [errors, setErrors]   = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [query, setQuery]     = useState('')
  const lang = useAppLanguage()

  useEffect(() => { void refreshCompletions() }, [refreshCompletions])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    const universes = displayed && displayed !== ADMIN_UNIVERSE ? [ADMIN_UNIVERSE, displayed] : [ADMIN_UNIVERSE]
    loadRecipes(universes)
      .then((r) => {
        if (cancelled) return
        setLoaded(r.entries)
        setErrors(r.errors)
      })
      .catch((e) => { if (!cancelled) setErrors([String(e)]) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [displayed])

  const filtered = useMemo(
    () => entries.filter((e) => matchesTopics(e, topics) && matchesQuery(e, query, lang)),
    [entries, topics, query],
  )
  const suggestions = useMemo(() => suggest(entries, completed), [entries, completed])
  // At Essentiel, the essentials of the wired sidecars come first.
  const level = useDisplayLevel()
  const essentialList = essentials(filtered, level)
  // The documents explaining KaTyoti come first, whatever the level.
  const conceptList = concepts(filtered)
  const tours = filtered.filter((e) => isTour(e.recipe))
  // Show me and Explain me: watched or read, never played step by step.
  const readings = filtered.filter((e) => !e.recipe.concept && (nature(e.recipe) === 'show' || nature(e.recipe) === 'explain'))
  const groups = useMemo(() => {
    const m = new Map<string, RecipeEntry[]>()
    for (const e of filtered) {
      if (nature(e.recipe) !== 'help' || isTour(e.recipe)) continue
      const list = m.get(e.provider) ?? []
      list.push(e)
      m.set(e.provider, list)
    }
    for (const list of m.values()) list.sort((a, b) => a.recipe.name.localeCompare(b.recipe.name))
    return [...m.entries()].sort(([a], [b]) => (a === SHELL_RECIPE_PROVIDER ? -1 : b === SHELL_RECIPE_PROVIDER ? 1 : a.localeCompare(b)))
  }, [filtered])

  const start = (uri: string) => {
    onStarted?.()
    void startRecipe(uri)
  }

  const showSuggested = !query.trim() && topics.length === 0 && suggestions.length > 0

  return (
    <ScrollArea h="100%" type="auto">
      <Stack gap="md" p="md" maw={820}>
        <TextInput
          placeholder={t('browser.searchPlaceholder')}
          aria-label={t('browser.searchLabel')}
          leftSection={<IconSearch size={14} />}
          value={query}
          onChange={(e) => setQuery(e.currentTarget.value)}
          rightSection={query ? <CloseButton size="sm" aria-label={t('browser.clearSearch')} onClick={() => setQuery('')} /> : null}
        />

        {topics.length > 0 && (
          <Group gap={6}>
            <Text size="xs" c="dimmed">{t('browser.about')}</Text>
            {topics.map((tp) => <Badge key={tp} size="sm" variant="light">{tp === CONCEPTS_TOPIC ? t('browser.concepts') : tp}</Badge>)}
            {onTopicsChange && (
              <Button size="compact-xs" variant="subtle" onClick={() => onTopicsChange([])}>{t('browser.showAll')}</Button>
            )}
          </Group>
        )}

        {loading && <Group gap="xs"><Loader size="xs" /><Text size="sm" c="dimmed">{t('browser.loading')}</Text></Group>}

        {errors.length > 0 && (
          <Alert color="orange" variant="light" title={t('browser.readErrors')}>
            {errors.map((e) => <Text key={e} size="xs">{e}</Text>)}
          </Alert>
        )}

        {conceptList.length > 0 && (
          <Section title={t('browser.concepts')}>
            {conceptList.map((e) => <RecipeRow key={e.uri} entry={e} completed={completed} onStart={start} />)}
          </Section>
        )}

        {essentialList.length > 0 && (
          <Section title={t('browser.essentials')}>
            {essentialList.map((e) => <RecipeRow key={e.uri} entry={e} completed={completed} onStart={start} />)}
          </Section>
        )}

        {showSuggested && (
          <Section title={t('browser.suggested')}>
            {suggestions.map((s) => (
              <RecipeRow key={s.entry.uri} entry={s.entry} reason={t(s.reason.key, s.reason.params)} completed={completed} onStart={start} />
            ))}
          </Section>
        )}

        {tours.length > 0 && (
          <Section title={t('browser.tours')}>
            {tours.map((e) => <RecipeRow key={e.uri} entry={e} completed={completed} onStart={start} />)}
          </Section>
        )}

        {readings.length > 0 && (
          <Section title={t('browser.readings')}>
            {readings.map((e) => <RecipeRow key={e.uri} entry={e} completed={completed} onStart={start} />)}
          </Section>
        )}

        {groups.map(([provider, list]) => (
          <Section key={provider} title={t('browser.recipesOf', { provider: providerLabel(provider) })}>
            {list.map((e) => <RecipeRow key={e.uri} entry={e} completed={completed} onStart={start} />)}
          </Section>
        ))}

        {!loading && filtered.length === 0 && (
          <Text size="sm" c="dimmed">{query ? t('browser.noMatchQuery', { query }) : t('browser.noMatch')}</Text>
        )}
      </Stack>
    </ScrollArea>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Box>
      <Title order={6} mb={6} c="dimmed" tt="uppercase" fz={11}>{title}</Title>
      <Stack gap={6}>{children}</Stack>
    </Box>
  )
}

function RecipeRow({
  entry,
  reason,
  completed,
  onStart,
}: {
  entry: RecipeEntry
  reason?: string
  completed: Set<string>
  onStart: (uri: string) => void
}) {
  const r = entry.recipe
  const done = isCompleted(completed, entry)
  const t = useGuideT()
  const tx = recipeText(entry, useAppLanguage())
  const n = nature(r)
  const first = primaryAction(n)
  const read = useGuidePlayer((s) => s.read)
  return (
    <Paper withBorder radius="md" p="sm">
      <Group wrap="nowrap" align="flex-start" gap="sm">
        <Box style={{ flex: 1, minWidth: 0 }}>
          <Group gap={6} wrap="nowrap">
            {done && (
              <Tooltip label={t('browser.completed')} withArrow>
                <IconCircleCheck size={16} color="var(--mantine-color-green-6)" aria-label={t('browser.completed')} style={{ flexShrink: 0 }} />
              </Tooltip>
            )}
            <Text size="sm" fw={600} truncate>{tx.name}</Text>
            {n && <Badge size="xs" variant="outline" color={NATURE_COLOR[n]}>{t(`kind.${n}`)}</Badge>}
            {r.level && <Badge size="xs" variant="light" color={LEVEL_COLOR[r.level] ?? 'gray'}>{t(`level.${r.level}`, { defaultValue: r.level })}</Badge>}
          </Group>
          {tx.description && <Text size="xs" c="dimmed" mt={2}>{tx.description}</Text>}
          {reason && <Text size="xs" c="blue" mt={2}>{reason}</Text>}
        </Box>
        <Group gap={4} wrap="nowrap">
          {r.video && first !== 'watch' && (
            <Tooltip label={t('browser.watchVideo')} withArrow>
              <ActionIcon variant="subtle" aria-label={t('browser.videoLabel', { name: tx.name })} onClick={() => openVideo(r.video!)}>
                <IconVideo size={16} />
              </ActionIcon>
            </Tooltip>
          )}
          {tx.body && first === 'watch' && (
            <Tooltip label={t('browser.read')} withArrow>
              <ActionIcon variant="subtle" aria-label={t('browser.readLabel', { name: tx.name })} onClick={() => read(entry)}>
                <IconBook2 size={16} />
              </ActionIcon>
            </Tooltip>
          )}
          {first === 'start' && (
            <Button size="compact-xs" leftSection={<IconPlayerPlay size={12} />} onClick={() => onStart(entry.uri)} aria-label={t('browser.startLabel', { name: tx.name })}>
              {t('browser.start')}
            </Button>
          )}
          {first === 'watch' && (
            <Button size="compact-xs" leftSection={<IconVideo size={12} />} onClick={() => openVideo(r.video!)} aria-label={t('browser.videoLabel', { name: tx.name })}>
              {t('browser.watch')}
            </Button>
          )}
          {first === 'read' && (
            <Button size="compact-xs" leftSection={<IconBook2 size={12} />} onClick={() => read(entry)} aria-label={t('browser.readLabel', { name: tx.name })}>
              {t('browser.read')}
            </Button>
          )}
        </Group>
      </Group>
    </Paper>
  )
}
