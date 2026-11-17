// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react'
import { Button, Center, Stack, Text } from '@mantine/core'
import { IconPlayerPlay } from '@tabler/icons-react'
import { WorkspaceActionTask, WorkspaceRunAction } from '../../../wailsjs/go/main/App'
import { taskActionURI } from '../../hooks/taskActionURI'
import { useAppStore } from '../../store'
import { isTaskURI, type PanelEmpty } from './emptyState'

/**
 * A panel's empty state: its declared sentence and the button that
 * runs its action - a task opens its run form, a method runs and the panel
 * re-reads. `text` and `label` arrive in the person's language (the caller
 * reads them through the pack's translations).
 */
export function EmptyInvite({ empty, text, label, universeName, onDone }: {
  empty: PanelEmpty
  text: string
  label?: string
  universeName: string
  onDone?: () => void
}) {
  const openRunTask = useAppStore((s) => s.openRunTask)
  const [running, setRunning] = useState(false)
  const run = async () => {
    const uri = empty.action
    if (!uri) return
    if (isTaskURI(uri)) {
      const name = uri.split('/').pop() || uri
      // The task as the catalog lists it: its form, and its tags (a task
      // that takes no initial message starts at once).
      const task = await WorkspaceActionTask(universeName, taskActionURI(uri) || uri).catch(() => null)
      openRunTask({
        name: task?.task || uri, uri: task?.task || uri, taskName: name,
        title: task?.title || undefined, description: task?.description ?? '',
        provider: uri.split(':')[0],
        inputSchemaURI: task?.inputSchemaURI || undefined,
        tags: task?.tags ?? undefined,
      })
      return
    }
    setRunning(true)
    WorkspaceRunAction(universeName, uri, JSON.stringify(empty.input ?? {}))
      .finally(() => { setRunning(false); onDone?.() })
  }
  return (
    <Center p="xl" data-guide="panel.empty">
      <Stack align="center" gap="md" maw={480}>
        <Text size="sm" c="dimmed" ta="center">{text}</Text>
        {empty.action && label && (
          <Button leftSection={<IconPlayerPlay size={16} />} loading={running} onClick={run} data-guide="panel.empty.action">
            {label}
          </Button>
        )}
      </Stack>
    </Center>
  )
}
