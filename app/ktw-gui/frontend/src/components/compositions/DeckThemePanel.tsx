// SPDX-License-Identifier: Apache-2.0
/**
 * DeckThemePanel — a deck theme stored in the project (a `deck-theme`
 * object), shown as itself: its sample deck in its look, and a form for what
 * may be adjusted after an import — the description, the colours, the fonts
 * and the type scale. Variants, chrome and icons stay as imported.
 *
 * Saving rewrites the object's `deck-theme` capability (the theme's whole
 * declaration, with the edits); the preview is drawn again from the saved
 * theme — a project theme is resolved afresh each time it is published.
 */

import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Box,
  Button,
  ColorInput,
  Divider,
  Group,
  Loader,
  ScrollArea,
  Stack,
  Text,
  TextInput,
  Textarea,
  Title,
} from '@mantine/core'
import { DeckThemeObject, WorkspaceObjectUpdate } from '../../../wailsjs/go/main/App'
import { UIDeck } from './UIDeck'
import { noteRevision, revisionOf } from '../../sections/workspace/objectRevision'
import { useT } from '../../i18n'

/** The colour tokens the form edits; their meaning is the label
 *  `deckTheme.tokens.<token>` of the components namespace. */
const COLOURS: Array<[string]> = [
  ['bg'], ['fg'], ['heading'], ['muted'], ['accent'], ['accent-2'], ['surface'], ['border'], ['icon'],
]

/** The font and size tokens the form edits, with a placeholder. */
const TYPE: Array<[string, string]> = [
  ['heading-font', "'Proza Libre', Arial, sans-serif"],
  ['font', 'Arial, sans-serif'],
  ['size', '27px'],
  ['h1', '4em'],
  ['h2', '2.5em'],
]

interface Theme {
  name: string
  description?: string
  tokens?: Record<string, string>
  palette?: Array<{ name: string; value: string }>
  [key: string]: unknown
}

export function DeckThemePanel({ universeName, objectId }: { universeName: string; objectId: string }) {
  const t = useT('components')
  const [theme, setTheme] = useState<Theme | null>(null)
  const [draft, setDraft] = useState<Theme | null>(null)
  const [sample, setSample] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  // Bumped after a save: the preview mounts again and publishes the theme anew.
  const [drawn, setDrawn] = useState(0)

  useEffect(() => {
    let live = true
    DeckThemeObject(universeName, objectId)
      .then((r) => {
        if (!live) return
        if (r.error) {
          setError(r.error)
          return
        }
        const t = JSON.parse(r.theme) as Theme
        setTheme(t)
        setDraft(t)
        setSample(r.sample)
      })
      .catch((e) => live && setError(String(e)))
    return () => {
      live = false
    }
  }, [universeName, objectId])

  const swatches = useMemo(() => (theme?.palette ?? []).map((p) => p.value), [theme])
  const dirty = useMemo(() => JSON.stringify(theme) !== JSON.stringify(draft), [theme, draft])

  if (error) {
    return (
      <Alert color="red" m="md" title={t('deckTheme.unreadable')}>
        {error}
      </Alert>
    )
  }
  if (!theme || !draft) {
    return (
      <Group justify="center" p="xl">
        <Loader size="sm" />
      </Group>
    )
  }

  const token = (k: string) => draft.tokens?.[k] ?? ''
  const setToken = (k: string, v: string) => {
    const tokens = { ...(draft.tokens ?? {}) }
    if (v.trim() === '') delete tokens[k]
    else tokens[k] = v
    setDraft({ ...draft, tokens })
  }

  // Save writes the edits made here onto the theme as it is NOW (read
  // again): a change made meanwhile elsewhere (an agent, another pane) to
  // what this form does not edit is kept.
  async function save() {
    if (!draft || !theme) return
    setSaving(true)
    setSaveError('')
    try {
      const fresh = await DeckThemeObject(universeName, objectId)
      if (fresh.error) throw new Error(fresh.error)
      const base = JSON.parse(fresh.theme) as Theme
      const next: Theme = { ...base, tokens: { ...(base.tokens ?? {}) } }
      if ((draft.description ?? '') !== (theme.description ?? '')) next.description = draft.description
      for (const [k] of [...COLOURS, ...TYPE]) {
        const was = theme.tokens?.[k] ?? ''
        const now = draft.tokens?.[k] ?? ''
        if (was === now) continue
        if (now === '') delete next.tokens![k]
        else next.tokens![k] = now
      }
      const r = await WorkspaceObjectUpdate(universeName, objectId, JSON.stringify({ 'deck-theme': next }), '', revisionOf(objectId))
      noteRevision(objectId, r.revision)
      if (r.error) throw new Error(r.error)
      setTheme(next)
      setDraft(next)
      setDrawn((n) => n + 1)
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  // One sentence around the code a deck writes: split where {{code}} lands.
  const [usageBefore, usageAfter = ''] = t('deckTheme.usage', { code: '\u0001' }).split('\u0001')

  return (
    <Group align="stretch" gap={0} wrap="nowrap" style={{ height: '100%' }}>
      <Box style={{ flex: 1, minWidth: 0 }}>
        {sample ? (
          <UIDeck key={drawn} markdown={sample} universe={universeName} deckKey={`theme:${theme.name}`} fillHeight />
        ) : (
          <Text c="dimmed" p="md" size="sm">
            {t('deckTheme.noSample')}
          </Text>
        )}
      </Box>
      <Divider orientation="vertical" />
      <ScrollArea style={{ width: 320, flexShrink: 0 }} type="auto">
        <Stack gap="sm" p="md">
          <Title order={5}>{t('deckTheme.title', { name: theme.name })}</Title>
          <Text size="xs" c="dimmed">
            {usageBefore}<code>{`theme: ${theme.name}`}</code>{usageAfter}
          </Text>
          <Textarea
            label={t('deckTheme.description')}
            autosize
            minRows={2}
            value={draft.description ?? ''}
            onChange={(e) => setDraft({ ...draft, description: e.currentTarget.value })}
          />
          <Divider label={t('deckTheme.colours')} labelPosition="left" />
          {COLOURS.map(([k]) => (
            <ColorInput
              key={k}
              size="xs"
              label={t(`deckTheme.tokens.${k}`)}
              description={k}
              format="hex"
              swatches={swatches.length > 0 ? swatches : undefined}
              value={token(k)}
              onChange={(v) => setToken(k, v)}
            />
          ))}
          <Divider label={t('deckTheme.fonts')} labelPosition="left" />
          {TYPE.map(([k, placeholder]) => (
            <TextInput
              key={k}
              size="xs"
              label={t(`deckTheme.tokens.${k}`)}
              description={k}
              placeholder={placeholder}
              value={token(k)}
              onChange={(e) => setToken(k, e.currentTarget.value)}
            />
          ))}
          {saveError && (
            <Alert color="red" title={t('deckTheme.notSaved')} p="xs">
              {saveError}
            </Alert>
          )}
          <Group justify="flex-end" mt="sm">
            <Button variant="default" size="xs" disabled={!dirty || saving} onClick={() => setDraft(theme)}>
              {t('deckTheme.undo')}
            </Button>
            <Button size="xs" disabled={!dirty} loading={saving} onClick={() => void save()}>
              {t('deckTheme.save')}
            </Button>
          </Group>
        </Stack>
      </ScrollArea>
    </Group>
  )
}
