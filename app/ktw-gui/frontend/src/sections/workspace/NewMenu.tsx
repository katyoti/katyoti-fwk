// SPDX-License-Identifier: Apache-2.0
/**
 * « + New »: the pane toolbar's button, where « + »
 * was. Pressed, it opens the cards « + » drew, of exactly what « + » offered - the types allowed under the
 * object and the aspects carrying a create task, each with its icon, its name
 * and one line, minus what the universe's view hides (offeredChoices, the same
 * list as the create dialog). A type opens its creation step; one with a create
 * task runs that task (an assistant or an action). Escape or a click outside
 * closes it. No box stands on the pane.
 */
import { useEffect, useState } from 'react'
import { Button, Card, Group, Loader, Popover, SimpleGrid, Stack, Text } from '@mantine/core'
import { useHotkeys } from '@mantine/hooks'
import { IconPlus } from '@tabler/icons-react'
import { WorkspaceObjectChildTypes } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { createTaskPage, offeredChoices } from './createChoices'
import { iconForType, colorForType, type DeclaredTypeStyle } from './type-style'
import { contribURI, useCT } from '../../i18n/contrib'
import { useT } from '../../i18n'

/** What picking a choice does: a create task runs, a type opens its creation. */
export type NewPick = { kind: 'task'; choice: main.WorkspaceNodeTypeChoice } | { kind: 'create'; type: string }

/** pickOf says what picking a choice does: a create task runs;
 *  a type opens its creation - one whose create_task answers a page as well,
 *  the dialog drawing that page or its plain form. */
export function pickOf(choice: main.WorkspaceNodeTypeChoice): NewPick {
  return choice.createTask && !createTaskPage(choice.createTask) ? { kind: 'task', choice } : { kind: 'create', type: choice.name }
}

export function NewMenu({ universeName, parentId, styles, allows, onPick }: {
  universeName: string
  parentId: string
  styles: Record<string, DeclaredTypeStyle>
  allows: (id: string) => boolean
  onPick: (pick: NewPick) => void
}) {
  const t = useT('workspace')
  const ct = useCT()
  const [choices, setChoices] = useState<main.WorkspaceNodeTypeChoice[] | null>(null)
  const load = () => {
    setChoices(null)
    WorkspaceObjectChildTypes(universeName, parentId)
      .then((c) => setChoices(c ?? []))
      .catch(() => setChoices([]))
  }
  const offered = offeredChoices(choices, undefined, allows)
  const [opened, setOpened] = useState(false)
  // Escape closes the cards wherever the focus is (a popover hears it only
  // from inside).
  useHotkeys(opened ? [['Escape', () => setOpened(false)]] : [])
  // The list is read each time the menu opens.
  useEffect(() => { if (opened) load() }, [opened]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Popover shadow="md" withinPortal position="bottom-end" closeOnEscape closeOnClickOutside trapFocus returnFocus width={560}
      opened={opened} onChange={setOpened}>
      <Popover.Target>
        <Button size="compact-xs" variant="subtle" leftSection={<IconPlus size={14} />} data-guide="object.new"
          aria-label={t('objectTab.newObject')} onClick={() => setOpened((o) => !o)}>
          {t('objectTab.newButton')}
        </Button>
      </Popover.Target>
      <Popover.Dropdown data-guide="object.new-menu" mah={460} style={{ overflowY: 'auto' }}>
        <Stack gap="sm">
          {offered === null ? (
            <Group justify="center" p="xs"><Loader size="xs" /></Group>
          ) : offered.length === 0 ? (
            // Nothing allowed here: say why and where to add instead, never a
            // fallback list of every type.
            <Stack gap={4} p="xs" data-guide="object.new-empty" maw={420}>
              <Text size="sm" fw={600}>{t('create.emptyTitle')}</Text>
              <Text size="xs" c="dimmed">{t('create.emptyBody')}</Text>
            </Stack>
          ) : null}
          {offered === null || offered.length === 0 ? null : (
            <SimpleGrid cols={2} spacing="xs">
              {offered.map((c) => (
                <NewCard key={c.name} data-guide="object.new-type" name={c.name}
                  label={ct(contribURI('object-type', c.name), 'label', c.label || c.name)}
                  description={ct(contribURI('object-type', c.name), 'description', c.description || '')}
                  Icon={iconForType(c.name, styles[c.name] ?? (c.icon ? { icon: c.icon } : undefined))}
                  color={colorForType(c.name, styles[c.name])} onClick={() => { setOpened(false); onPick(pickOf(c)) }} />
              ))}
            </SimpleGrid>
          )}
        </Stack>
      </Popover.Dropdown>
    </Popover>
  )
}

/** A card, as « + » drew them: its icon, its name and one line. */
function NewCard({ name, label, description, Icon, color, onClick, ...guide }: {
  name: string
  label: string
  description: string
  Icon: ReturnType<typeof iconForType>
  color: string
  onClick: () => void
  'data-guide': string
}) {
  return (
    <Card withBorder padding="sm" radius="md" {...guide} data-guide-name={name} style={{ cursor: 'pointer' }} onClick={onClick}>
      <Stack gap={4}>
        <Group gap="xs" wrap="nowrap">
          <Icon size={18} color={`var(--mantine-color-${color}-6)`} />
          <Text fw={600} size="sm" style={{ flex: 1, minWidth: 0 }} truncate>{label}</Text>
        </Group>
        {description && <Text size="xs" c="dimmed" lineClamp={2}>{description}</Text>}
      </Stack>
    </Card>
  )
}
