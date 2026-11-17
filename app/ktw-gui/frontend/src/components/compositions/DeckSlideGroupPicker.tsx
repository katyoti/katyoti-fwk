// SPDX-License-Identifier: Apache-2.0
// <DeckSlideGroupPicker> — « Insérer un groupe de slides » (modules/slidedeck/,
// Slide groups): the slide groups in reach, the project's first, each with
// its description and a preview of its slides in the deck's theme. Picking
// one hands its name to the host, which inserts the include slide after the
// slide shown (deck/settings.ts insertSlideAfter).
//
// The list comes from the universe (DeckSlideGroupList); the preview is drawn
// by the host (`preview`), so this component never imports the deck view.

import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Alert, Badge, Box, Button, Group, Loader, Modal, ScrollArea, Stack, Text, UnstyledButton } from '@mantine/core'
import type { main } from '../../../wailsjs/go/models'
import { DeckSlideGroupList } from '../../../wailsjs/go/main/App'
import { parseDeck } from '@ktw/display-lib/ui-builders/deck/parse.ts'
import { useT } from '../../i18n'

const PROJECT = 'project'

export interface DeckSlideGroupPickerProps {
  opened: boolean
  universe: string
  /** The slide shown (from 0), -1 when none: where the group goes. */
  slide: number
  /** Draws a group's slides as a deck (the host's deck view). */
  preview: (markdown: string) => ReactNode
  /** The frontmatter keys the preview keeps from the deck (theme, aspect). */
  look: { theme?: string; aspect?: string }
  onPick: (name: string) => void
  onClose: () => void
}

export function DeckSlideGroupPicker({ opened, universe, slide, preview, look, onPick, onClose }: DeckSlideGroupPickerProps) {
  const t = useT('workspace')
  const [groups, setGroups] = useState<main.DeckSlideGroupInfo[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [chosen, setChosen] = useState<string | null>(null)

  useEffect(() => {
    if (!opened) return
    let cancelled = false
    setGroups(null)
    setError(null)
    DeckSlideGroupList(universe)
      .then((g) => {
        if (cancelled) return
        setGroups(g ?? [])
        setChosen((c) => (c && g?.some((x) => x.name === c) ? c : g?.[0]?.name ?? null))
      })
      .catch((e) => { if (!cancelled) setError(e instanceof Error ? e.message : String(e)) })
    return () => { cancelled = true }
  }, [opened, universe])

  const group = groups?.find((g) => g.name === chosen)
  const sample = useMemo(() => {
    if (!group) return ''
    const fm = ['---', `title: ${JSON.stringify(group.name)}`]
    if (look.theme) fm.push(`theme: ${JSON.stringify(look.theme)}`)
    if (look.aspect) fm.push(`aspect: ${JSON.stringify(look.aspect)}`)
    return `${fm.join('\n')}\n---\n\n${group.slides.trim()}\n`
  }, [group, look.theme, look.aspect])
  const count = (g: main.DeckSlideGroupInfo) => parseDeck(g.slides).slides.length

  return (
    <Modal opened={opened} onClose={onClose} title={t('deckGroups.title')} size="80rem" centered>
      <Text size="sm" c="dimmed" mb="sm">{t('deckGroups.intro')}</Text>
      {error ? <Alert color="red" p="xs" mb="sm"><Text size="xs">{error}</Text></Alert> : null}
      {!groups && !error ? <Group gap="xs"><Loader size="xs" /><Text size="sm">{t('deckGroups.loading')}</Text></Group> : null}
      {groups && groups.length === 0 ? <Text size="sm">{t('deckGroups.none')}</Text> : null}
      {groups && groups.length > 0 ? (
        <Group align="stretch" gap="md" wrap="nowrap" style={{ height: '60vh' }}>
          <ScrollArea style={{ width: 300, flexShrink: 0 }} type="auto">
            <Stack gap={4}>
              {groups.map((g) => (
                <UnstyledButton
                  key={g.name}
                  onClick={() => setChosen(g.name)}
                  onDoubleClick={() => onPick(g.name)}
                  p="xs"
                  style={{
                    borderRadius: 6,
                    border: '1px solid var(--mantine-color-default-border)',
                    background: g.name === chosen ? 'var(--mantine-color-default-hover)' : undefined,
                  }}
                >
                  <Group gap={6} justify="space-between" wrap="nowrap">
                    <Text size="sm" fw={600} truncate>{g.name}</Text>
                    <Badge size="xs" variant="light" color={g.provider === PROJECT ? 'teal' : 'gray'}>
                      {g.provider === PROJECT ? t('deckGroups.project') : g.provider}
                    </Badge>
                  </Group>
                  <Text size="xs" c="dimmed" lineClamp={3}>{g.description || t('deckGroups.noDescription')}</Text>
                  <Text size="xs" c="dimmed" mt={2}>{t('deckGroups.slides', { count: count(g) })}</Text>
                </UnstyledButton>
              ))}
            </Stack>
          </ScrollArea>
          <Box style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
            {group ? <Box key={group.name} style={{ flex: 1, minHeight: 0 }}>{preview(sample)}</Box> : null}
          </Box>
        </Group>
      ) : null}
      <Group justify="flex-end" gap="xs" mt="md">
        <Button size="xs" variant="default" onClick={onClose}>{t('deckGroups.cancel')}</Button>
        <Button size="xs" disabled={!group} onClick={() => group && onPick(group.name)}>
          {slide >= 0 ? t('deckGroups.insertAfter', { n: slide + 1 }) : t('deckGroups.insertEnd')}
        </Button>
      </Group>
    </Modal>
  )
}
