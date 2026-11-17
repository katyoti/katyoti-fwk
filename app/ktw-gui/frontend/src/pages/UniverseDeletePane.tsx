// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react'
import {
  Alert,
  Avatar,
  Badge,
  Box,
  Button,
  Container,
  Group,
  Paper,
  Stack,
  Text,
  ThemeIcon,
  Title,
} from '@mantine/core'
import { IconAlertCircle, IconAlertTriangle, IconArrowLeft, IconShieldLock } from '@tabler/icons-react'
import { useAppStore } from '../store'
import { orchestratorClient } from '../ktsp'
import { listUniversesWithStatus } from '../ktsp/universes'
import type { Universe } from '../types/universe'
import { useT } from '../i18n'

/**
 * Delete-universe view shown in the main pane (router state: delete/<name>).
 *
 * Confirms with the user, deactivates the universe (a running hub would hold
 * its folder open), then calls the orchestrator's delete-universe, which
 * archives the folder under <parent>/.archived/ and drops the registry entry.
 * Protected universes (admin) show a "cannot be deleted" message instead
 * of the action footer.
 */
interface UniverseDeletePaneProps {
  name: string
}

export function UniverseDeletePane({ name }: UniverseDeletePaneProps) {
  const t                = useT('universes')
  const setMainPaneRoute = useAppStore((s) => s.setMainPaneRoute)
  const displayedName    = useAppStore((s) => s.displayedUniverseName)
  const universes        = useAppStore((s) => s.universes)
  const setUniverses     = useAppStore((s) => s.setUniverses)
  const summary          = universes.find((u) => u.name === name)

  const [submitting, setSubmitting] = useState(false)
  const [error, setError]           = useState<string | null>(null)

  const goBack = () => {
    setMainPaneRoute(displayedName && displayedName !== name ? { kind: 'universe' } : { kind: 'welcome' })
  }

  const onConfirmDelete = async () => {
    if (!summary || summary.protected || submitting) return
    setSubmitting(true)
    setError(null)
    try {
      // Deleting a universe MOVES its folder, and a running hub holds that
      // folder open - on Windows the rename is refused outright. The GUI
      // activates a universe IMPLICITLY on any listing call, so merely having
      // looked at this one is enough to leave a hub running. Stop it first.
      //
      // Process lifecycle is the one thing KTSP cannot express, so this stays a
      // Wails binding. It is idempotent: a no-op when nothing is active.
      const w = window as unknown as {
        go?: { main?: { App?: { UniverseDeactivate?: (n: string) => Promise<{ error?: string }> } } }
      }
      const deactivate = w.go?.main?.App?.UniverseDeactivate
      if (deactivate) {
        const res = await deactivate(name)
        if (res?.error) {
          throw new Error(t('delete.errors.stopFailed', { error: res.error }))
        }
      }
      await orchestratorClient().deleteUniverse({ name })
      // Refresh list, return to welcome / displayed.
      setUniverses(await listUniversesWithStatus())
      // If the deleted universe was the one being displayed, fall back to welcome.
      if (displayedName === name) {
        setMainPaneRoute({ kind: 'welcome' })
      } else {
        setMainPaneRoute(displayedName ? { kind: 'universe' } : { kind: 'welcome' })
      }
    } catch (err) {
      setError(String(err))
    } finally {
      setSubmitting(false)
    }
  }

  if (!summary) {
    return (
      <Container size="md" py="xl">
        <Stack>
          <Group>
            <Button variant="subtle" color="gray" size="xs" leftSection={<IconArrowLeft size={14} />} onClick={goBack}>
              {t('common.back')}
            </Button>
          </Group>
          <Text c="dimmed">{t('delete.notFound', { name })}</Text>
        </Stack>
      </Container>
    )
  }

  return (
    <Container size="md" py="xl">
      <Stack gap="md">
        <Group gap="sm" align="center">
          <Button variant="subtle" color="gray" size="xs" leftSection={<IconArrowLeft size={14} />} onClick={goBack}>
            {t('common.back')}
          </Button>
          <Title order={3}>{t('delete.title')}</Title>
        </Group>

        <Paper withBorder p="lg" radius="md">
          <Stack gap="md">
            <Group gap="md" wrap="nowrap" align="center">
              <Avatar size={56} radius="md" color={summary.color} variant="light" style={{ fontSize: 18, fontWeight: 700 }}>
                {summary.icon}
              </Avatar>
              <div style={{ flex: 1 }}>
                <Title order={4}>{summary.displayName || summary.name}</Title>
                <Text size="sm" c="dimmed">{summary.name}</Text>
              </div>
              {summary.protected && (
                <Badge color="gray" leftSection={<IconShieldLock size={12} />}>{t('common.protected')}</Badge>
              )}
            </Group>

            {summary.protected ? (
              <Paper p="md" radius="md" withBorder bg="var(--mantine-color-gray-light)">
                <Group gap="sm" wrap="nowrap">
                  <ThemeIcon color="gray" variant="light" size={28}>
                    <IconShieldLock size={16} />
                  </ThemeIcon>
                  <div>
                    <Text fw={600} size="sm">{t('delete.protectedTitle')}</Text>
                    <Text size="xs" c="dimmed" mt={2}>
                      {t('delete.protectedBody')}
                    </Text>
                  </div>
                </Group>
              </Paper>
            ) : (
              <>
                <Paper p="md" radius="md" withBorder bg="var(--mantine-color-red-light)">
                  <Group gap="sm" wrap="nowrap" align="flex-start">
                    <ThemeIcon color="red" variant="light" size={28}>
                      <IconAlertTriangle size={16} />
                    </ThemeIcon>
                    <Box>
                      <Text fw={600} size="sm">{t('delete.archiveTitle')}</Text>
                      <Text size="xs" c="dimmed" mt={2}>
                        {t('delete.archiveBodyBefore')} <code>{summary.homePath}</code> {t('delete.archiveBodyMiddle')}{' '}
                        <code>{'.archived/'}</code> {t('delete.archiveBodyAfter')}
                      </Text>
                    </Box>
                  </Group>
                </Paper>

                {error && (
                  <Alert color="red" icon={<IconAlertCircle size={16} />} title={t('delete.errors.deleteFailed')}>
                    <Text size="xs">{error}</Text>
                  </Alert>
                )}

                <Group justify="flex-end" gap="sm">
                  <Button variant="default" onClick={goBack} disabled={submitting}>
                    {t('common.cancel')}
                  </Button>
                  <Button color="red" onClick={onConfirmDelete} loading={submitting}>
                    {t('delete.submit')}
                  </Button>
                </Group>
              </>
            )}
          </Stack>
        </Paper>
      </Stack>
    </Container>
  )
}
