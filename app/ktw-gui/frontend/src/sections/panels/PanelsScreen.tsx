// SPDX-License-Identifier: Apache-2.0
/**
 * PanelsScreen — the panels screen of the approved display-panels mock-up
 * (modules/research-display-panels/display-panels-mockup.html):
 * on the left the list of open panels - each its title, `<id> · <type>` and
 * its state as a chip -, in the middle the selected panel drawn by its type
 * (DisplayPanel, variant `board`). The mock-up's « Simulate » column (its
 * demo events, the history slider, Reset) is not delivered. The list is the
 * panel server's (ktw-hub:panel-service/list), read again while the screen
 * is shown; each panel follows its own events live. Behind its display
 * switch: the ribbon opens it only while the switch is on. Its element
 * address is `screen:panels`.
 */

import { useEffect, useState } from 'react'
import { Badge, Flex, Paper, Stack, Text, UnstyledButton } from '@mantine/core'

import { useT } from '../../i18n'
import { panelClient } from '../../ktsp'
import { DisplayPanel } from './DisplayPanel'
import { panelListItems, selectedPanel, type PanelChip, type PanelListItem } from './panelList'

/** How often the list is read again while the screen is shown. */
const LIST_MS = 2000

/** The chip's colour by state, as the mock-up's: running orange, asking violet, complete green. */
const CHIP_COLOR: Record<PanelChip, string> = { running: 'orange', asking: 'violet', complete: 'teal' }

export function PanelsScreen({ universeName }: { universeName: string }) {
  const t = useT('components')
  const [items, setItems] = useState<PanelListItem[]>([])
  const [selected, setSelected] = useState<string | undefined>(undefined)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let gone = false
    const read = () => panelClient(universeName).list({}).then(
      (out) => {
        if (gone) return
        const next = panelListItems(out.panels)
        setItems(next)
        setSelected((cur) => selectedPanel(next, cur))
        setError(null)
      },
      (err) => { if (!gone) setError(String((err as Error)?.message ?? err)) },
    )
    void read()
    const timer = window.setInterval(() => { void read() }, LIST_MS)
    return () => { gone = true; window.clearInterval(timer) }
  }, [universeName])

  const current = items.find((i) => i.id === selected)

  return (
    <Flex p="md" gap={16} align={{ base: 'stretch', md: 'flex-start' }} direction={{ base: 'column', md: 'row' }} data-element="screen:panels">
      <Paper withBorder radius="md" py={6} component="nav" w={{ base: '100%', md: 240 }} style={{ flexShrink: 0 }} aria-label={t('displayPanel.list.aria')} data-guide="panels.list">
        {items.length === 0 && <Text size="sm" c="dimmed" px={14} py={9}>{error ?? t('displayPanel.list.empty')}</Text>}
        {items.map((it) => {
          const on = it.id === selected
          return (
            <UnstyledButton key={it.id} role="tab" aria-selected={on} onClick={() => setSelected(it.id)}
              data-panel={it.id} data-state={it.chip}
              style={{
                display: 'flex', flexDirection: 'column', gap: 2, width: '100%', padding: '9px 14px', textAlign: 'left',
                background: on ? 'var(--mantine-color-default-hover)' : undefined,
                boxShadow: on ? 'inset 3px 0 0 var(--mantine-primary-color-filled)' : undefined,
              }}>
              <Text fw={600} size="sm">{it.title ?? t('displayPanel.untitled')}</Text>
              <Text ff="monospace" size="11px" c="dimmed">{it.meta}</Text>
              <span><Badge variant="outline" color={CHIP_COLOR[it.chip]} size="sm" radius="xl" tt="none" ff="monospace">{t(`displayPanel.chip.${it.chip}`)}</Badge></span>
            </UnstyledButton>
          )
        })}
      </Paper>
      <Stack gap={0} style={{ minWidth: 0, flex: 1 }}>
        {current
          ? <DisplayPanel key={current.id} universeName={universeName} panelId={current.id} title={current.title} variant="board" />
          : <Paper withBorder radius="md" p="md" mih={360}><Text size="sm" c="dimmed">{t('displayPanel.list.none')}</Text></Paper>}
      </Stack>
    </Flex>
  )
}
