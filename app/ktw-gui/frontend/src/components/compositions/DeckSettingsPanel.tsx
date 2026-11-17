// SPDX-License-Identifier: Apache-2.0
// <DeckSettingsPanel> — the deck's settings, and the settings of the slide
// shown, as a property panel docked beside the deck (modules/slidedeck/, the
// Deck component). Each change rewrites one key of the markdown
// (deck/settings.ts) and the deck re-renders on the same slide; the host
// saves on Save.
//
// Nothing is written down here: the settings, their values and descriptions
// come from the grammar's types, the layouts and themes from the universe
// (DeckEditorGrammar, fetched by the host), the variants from the chosen theme.

import { useEffect, useMemo, useState } from 'react'
import { Alert, Autocomplete, Button, ColorSwatch, Group, Loader, ScrollArea, Select, Stack, Tabs, Text, TextInput } from '@mantine/core'
import type { main } from '../../../wailsjs/go/models'
import { parseDeck } from '@ktw/display-lib/ui-builders/deck/parse.ts'
import { setDeckSetting, setSlideSetting } from '@ktw/display-lib/ui-builders/deck/settings.ts'
import { DEFAULT_THEME } from '@ktw/display-lib/ui-builders/deck/theme.ts'
import { useT } from '../../i18n'

interface LayoutSlot { name: string; required?: boolean }
interface Layout { name: string; description?: string; slots?: LayoutSlot[] }

const DEFAULT_LAYOUT = 'content'

export interface DeckSettingsPanelProps {
  /** The choices (DeckEditorGrammar); null while it loads. */
  grammar: main.DeckEditorGrammarResult | null
  grammarError?: string | null
  /** The deck being edited (the draft). */
  markdown: string
  onMarkdownChange: (markdown: string) => void
  /** The slide shown (index in the parsed deck), -1 when unknown. */
  slide: number
  /** Settings the host keeps elsewhere, shown read-only with why (a deck
   *  object's title is its name). */
  readOnly?: Record<string, string>
  saving?: boolean
  error?: string | null
  onSave: () => void
  onCancel: () => void
  /** « Insérer un groupe de slides »: opens the host's picker; the group goes
   *  after the slide shown. Absent where the host has no universe to list. */
  onInsertGroup?: () => void
}

/** A text setting: written to the markdown when the field is left. */
function TextSetting({ label, description, value, disabled, onCommit }: {
  label: string; description?: string; value: string; disabled?: boolean; onCommit: (v: string) => void
}) {
  const [v, setV] = useState(value)
  useEffect(() => setV(value), [value])
  return (
    <TextInput
      size="xs" label={label} description={description} value={v} disabled={disabled}
      onChange={(e) => setV(e.currentTarget.value)}
      onBlur={() => { if (v !== value) onCommit(v.trim()) }}
      onKeyDown={(e) => { if (e.key === 'Enter') (e.currentTarget as HTMLInputElement).blur() }}
    />
  )
}

/** A slide's background: a colour of the theme's palette (by name), or an
 *  image path. With a palette, a colour outside it is refused by the lint
 *  and not shown by the deck. */
function BackgroundSetting({ label, description, value, palette, onCommit }: {
  label: string; description?: string; value: string; palette: { name: string; value: string }[]; onCommit: (v: string) => void
}) {
  const tc = useT('components')
  const [v, setV] = useState(value)
  useEffect(() => setV(value), [value])
  const named = palette.find((p) => p.value.toLowerCase() === v.trim().toLowerCase())
  return (
    <Autocomplete
      size="xs" label={label} description={description} value={v} placeholder={tc('deckSettings.default')}
      data={palette.map((p) => p.value)}
      leftSection={named ? <ColorSwatch color={named.value} size={14} /> : undefined}
      renderOption={({ option }) => (
        <Group gap={6} wrap="nowrap"><ColorSwatch color={option.value} size={14} /><Text size="xs">{palette.find((p) => p.value === option.value)?.name} {option.value}</Text></Group>
      )}
      onChange={setV}
      onOptionSubmit={(o) => { setV(o); if (o !== value) onCommit(o) }}
      onBlur={() => { if (v.trim() !== value) onCommit(v.trim()) }}
      comboboxProps={{ withinPortal: true }}
    />
  )
}

export function DeckSettingsPanel({
  grammar, grammarError, markdown, onMarkdownChange, slide, readOnly = {}, saving, error, onSave, onCancel, onInsertGroup,
}: DeckSettingsPanelProps) {
  const [tab, setTab] = useState<string | null>('deck')
  const t = useT('workspace')
  const tc = useT('components')

  const deck = useMemo(() => parseDeck(markdown), [markdown])
  const settings = deck.settings as Record<string, unknown>
  const current = slide >= 0 ? deck.slides[slide] : undefined
  const slideSettings = current?.settings ?? {}
  const themeName = typeof settings.theme === 'string' && settings.theme ? settings.theme : DEFAULT_THEME
  const layoutName = slideSettings.layout || DEFAULT_LAYOUT
  const layouts = (grammar?.layouts ?? []) as Layout[]
  const theme = grammar?.themes?.find((t) => t.name === themeName)
  const layout = layouts.find((l) => l.name === layoutName)

  // A layout's required slots the slide leaves empty: the lint's finding,
  // shown before the user saves.
  const missing = useMemo(() => {
    if (!current || !layout) return []
    const filled = new Set(current.slots.filter((s) => s.markdown.trim() !== '').map((s) => s.name))
    return (layout.slots ?? []).filter((s) => s.required && !filled.has(s.name)).map((s) => s.name)
  }, [current, layout])

  const setDeck = (key: string, value: string | null) => onMarkdownChange(setDeckSetting(markdown, key, value ?? ''))
  const setSlide = (key: string, value: string | null) => {
    if (slide < 0) return
    let md = setSlideSetting(markdown, slide, key, value ?? '')
    // A variant belongs to its layout: a new layout drops a variant it lacks.
    if (key === 'layout' && slideSettings.variant && !theme?.variants?.[value || DEFAULT_LAYOUT]?.includes(slideSettings.variant)) {
      md = setSlideSetting(md, slide, 'variant', '')
    }
    onMarkdownChange(md)
  }

  const field = (f: main.DeckEditorField, scope: 'deck' | 'slide') => {
    const value = scope === 'deck'
      ? (typeof settings[f.name] === 'string' ? settings[f.name] as string : settings[f.name] == null ? '' : String(settings[f.name]))
      : (slideSettings[f.name] ?? '')
    const set = scope === 'deck' ? setDeck : setSlide
    const label = f.name.charAt(0).toUpperCase() + f.name.slice(1)
    if (scope === 'deck' && readOnly[f.name] !== undefined) {
      return <TextInput key={f.name} size="xs" label={label} description={readOnly[f.name]} value={value} disabled />
    }
    let data: { value: string; label: string }[] | null = null
    let description = f.description
    if (scope === 'deck' && f.name === 'theme') {
      data = (grammar?.themes ?? []).map((t) => ({ value: t.name, label: t.name }))
      description = theme?.description || f.description
    } else if (scope === 'slide' && f.name === 'layout') {
      data = layouts.map((l) => ({ value: l.name, label: l.name }))
      description = layout?.description || f.description
    } else if (scope === 'slide' && f.name === 'variant') {
      data = (theme?.variants?.[layoutName] ?? []).map((v) => ({ value: v, label: v }))
      if (data.length === 0) description = tc('deckSettings.noVariant', { theme: themeName, layout: layoutName })
    } else if (scope === 'slide' && f.name === 'background') {
      const palette = theme?.palette ?? []
      const hint = palette.length
        ? tc('deckSettings.backgroundHint', { theme: themeName })
        : f.description
      return <BackgroundSetting key={f.name} label={label} description={hint} value={value} palette={palette} onCommit={(v) => set(f.name, v)} />
    } else if (f.enum?.length) {
      data = f.enum.map((v) => ({ value: v, label: v }))
    }
    if (data) {
      return (
        <Select
          key={f.name} size="xs" label={label} description={description} data={data}
          value={value || null} placeholder={tc('deckSettings.default')} clearable searchable={data.length > 8}
          disabled={data.length === 0} onChange={(v) => set(f.name, v)}
          comboboxProps={{ withinPortal: true }}
        />
      )
    }
    return <TextSetting key={f.name} label={label} description={f.description} value={value} onCommit={(v) => set(f.name, v)} />
  }

  return (
    <Stack gap={0} h="100%" style={{ minHeight: 0 }}>
      <Tabs value={tab} onChange={setTab} style={{ display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
        <Tabs.List>
          <Tabs.Tab value="deck">{tc('deckSettings.deck')}</Tabs.Tab>
          <Tabs.Tab value="slide" disabled={!current}>{current ? tc('deckSettings.slideN', { n: slide + 1 }) : tc('deckSettings.slide')}</Tabs.Tab>
        </Tabs.List>
        <ScrollArea style={{ flex: 1 }} p="sm">
          {!grammar && !grammarError ? <Loader size="xs" /> : null}
          {grammarError ? <Alert color="yellow" p="xs" mb="xs"><Text size="xs">{grammarError}</Text></Alert> : null}
          <Tabs.Panel value="deck">
            <Stack gap="xs">{(grammar?.deck ?? []).map((f) => field(f, 'deck'))}</Stack>
          </Tabs.Panel>
          <Tabs.Panel value="slide">
            <Stack gap="xs">
              {missing.length ? (
                <Alert color="orange" p="xs">
                  <Text size="xs">{tc('deckSettings.missingSlots', { layout: layoutName, slots: missing.map((m) => `::${m}::`).join(', ') })}</Text>
                </Alert>
              ) : null}
              {(grammar?.slide ?? []).map((f) => field(f, 'slide'))}
            </Stack>
          </Tabs.Panel>
        </ScrollArea>
      </Tabs>
      {onInsertGroup ? (
        <Group px="sm" pt="xs" style={{ borderTop: '1px solid var(--mantine-color-default-border)' }}>
          <Button size="compact-xs" variant="light" fullWidth onClick={onInsertGroup}>{t('deckGroups.button')}</Button>
        </Group>
      ) : null}
      {error ? <Text size="xs" c="red" px="sm">{error}</Text> : null}
      <Group justify="flex-end" gap="xs" p="sm" style={{ borderTop: '1px solid var(--mantine-color-default-border)' }}>
        <Button size="compact-xs" variant="default" onClick={onCancel} disabled={saving}>{t('common.cancel')}</Button>
        <Button size="compact-xs" onClick={onSave} loading={saving}>{t('common.save')}</Button>
      </Group>
    </Stack>
  )
}
