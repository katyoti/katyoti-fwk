// SPDX-License-Identifier: Apache-2.0
/**
 * ConversionDialog — a project whose data is older than KaTyoti converts from
 * the GUI.
 *
 * It opens on the migrate dry run (cwm/migrate, dry_run) - while it reads the
 * project, past two seconds, it says so with the seconds elapsed -: the project, from
 * data version N to M, each step's title and the files it rewrites (« rien à
 * réécrire » when none), where git stands on those files, and that there is
 * no way back. Confirm runs cwm/migrate-commit lent a panel the dialog draws
 * (each step, the index rebuilt, the commit); the end says the version
 * reached, the files written and the commit - or, a commit refused by a hook,
 * the reason and that the person commits by hand. Confirm stays off while a
 * file the conversion writes has uncommitted edits (migrate-commit would
 * refuse it). The universe's views then read the project again - after a
 * failure too, files may already be rewritten; the dialog, drawn apart from
 * the header's mark, keeps its end until the person closes it. A dry run
 * that finds nothing waiting (converted meanwhile elsewhere) says the project
 * is up to date, offers no Confirm and has the views read the project again,
 * which drops a stale mark.
 */
import { useEffect, useState } from 'react'
import { Alert, Badge, Button, Code, Group, List, Loader, Modal, Stack, Text } from '@mantine/core'
import { IconAlertTriangle, IconCheck, IconGitCommit } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { dispatcherFor } from '../../ktsp'
import { inPanelDispatcherFor, type CallPanel } from '../../ktsp/inPanel'
import { DisplayPanel } from '../panels/DisplayPanel'
import { useT } from '../../i18n'
import {
  CONVERSION_CONFIRM,
  CONVERSION_DIALOG,
  MIGRATE,
  MIGRATE_COMMIT,
  canConfirm,
  conversionEnd,
  readingLine,
  stepRows,
  treeLine,
  upToDate,
  type ConversionEnd,
  type KnownProject,
  type MigrateAnswer,
} from './conversion'

interface Props {
  universe: string
  project: KnownProject
  opened: boolean
  onClose: () => void
}

const reason = (e: unknown) => (e instanceof Error ? e.message : String(e))

export function ConversionDialog({ universe, project, opened, onClose }: Props) {
  const t = useT('shell')
  const [dry, setDry] = useState<MigrateAnswer | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [running, setRunning] = useState(false)
  const [callPanel, setCallPanel] = useState<CallPanel | null>(null)
  const [end, setEnd] = useState<ConversionEnd | null>(null)
  const [elapsed, setElapsed] = useState(0)

  useEffect(() => {
    if (!opened) return
    setDry(null); setError(null); setEnd(null); setCallPanel(null); setRunning(false); setElapsed(0)
    dispatcherFor(universe)(MIGRATE, { dry_run: true })
      .then((out) => {
        setDry(out as MigrateAnswer)
        // Nothing waits: the mark was stale - the views read the project again.
        if (upToDate(out as MigrateAnswer)) useAppStore.setState((s) => ({ universeRevision: s.universeRevision + 1 }))
      })
      .catch((e) => setError(reason(e)))
  }, [opened, universe])

  // The seconds the dry run has been reading the project, while it runs.
  const reading = opened && !dry && !error
  useEffect(() => {
    if (!reading) return
    const started = Date.now()
    const tick = setInterval(() => setElapsed((Date.now() - started) / 1000), 1000)
    return () => clearInterval(tick)
  }, [reading])

  const confirm = async () => {
    if (!canConfirm(dry)) return
    setRunning(true); setError(null)
    try {
      const run = inPanelDispatcherFor(universe, setCallPanel, () => t('conversion.panelTitle', { from: dry?.from ?? 0, to: dry?.to ?? 0 }))
      const out = (await run(MIGRATE_COMMIT, {})) as MigrateAnswer
      setEnd(conversionEnd(out, !!dry?.tree?.repo))
    } catch (e) {
      setError(reason(e))
    } finally {
      setRunning(false)
      // Converted, or failed after rewriting some files: the universe's views
      // read the project again (the dialog, drawn apart from the mark, stays).
      useAppStore.setState((s) => ({ universeRevision: s.universeRevision + 1 }))
    }
  }

  const rows = stepRows(dry)
  const tree = treeLine(dry)
  const name = project.name || project.id || project.dir

  return (
    <Modal opened={opened} onClose={running ? () => undefined : onClose} title={t('conversion.title', { project: name })} size="lg" centered>
      <Stack gap="sm" data-element={CONVERSION_DIALOG}>
        {reading && (() => {
          const line = readingLine(elapsed)
          return (
            <Group justify="center" p="md" gap="xs" data-guide="conversion.reading">
              <Loader size="sm" />
              {line.kind === 'reading' && <Text size="sm" c="dimmed">{t('conversion.reading', { seconds: line.seconds })}</Text>}
            </Group>
          )
        })()}
        {dry && upToDate(dry) && (
          <Alert color="green" variant="light" icon={<IconCheck size={16} />} data-guide="conversion.up-to-date">
            <Text size="sm">{t('conversion.upToDate', { to: dry.to })}</Text>
          </Alert>
        )}
        {dry && !upToDate(dry) && (
          <>
            <Text size="sm" data-guide="conversion.versions">
              {t('conversion.versions', { from: dry.from, to: dry.to })}
            </Text>
            <List size="sm" spacing={4} data-guide="conversion.steps">
              {rows.map((r) => (
                <List.Item key={`${r.from}-${r.to}`} data-guide="conversion.step">
                  <Text size="sm" span fw={500}>{t('conversion.step', { from: r.from, to: r.to })}</Text>{' '}
                  <Text size="sm" span>{r.title}</Text>{' '}
                  {r.nothing
                    ? <Badge size="xs" variant="light" color="gray">{t('conversion.nothing')}</Badge>
                    : <Badge size="xs" variant="light" color="blue" title={[...r.changed, ...r.removed].join('\n')}>{t('conversion.files', { count: r.count })}</Badge>}
                </List.Item>
              ))}
            </List>
            {tree.kind === 'clean' && (
              <Text size="xs" c="dimmed" data-guide="conversion.tree" data-state="clean">{t('conversion.treeClean')}</Text>
            )}
            {tree.kind === 'no-repo' && (
              <Alert color="yellow" variant="light" icon={<IconAlertTriangle size={16} />} data-guide="conversion.tree" data-state="no-repo">
                <Text size="xs">{t('conversion.treeNoRepo')}</Text>
              </Alert>
            )}
            {tree.kind === 'dirty' && (
              <Alert color="red" variant="light" icon={<IconAlertTriangle size={16} />} data-guide="conversion.tree" data-state="dirty">
                <Text size="xs">{t('conversion.treeDirty', { count: tree.files.length })}</Text>
                <Code block>{tree.files.join('\n')}</Code>
              </Alert>
            )}
            {!end && (
              <Alert color="orange" variant="light" icon={<IconAlertTriangle size={16} />} data-guide="conversion.no-way-back">
                <Text size="xs">{t('conversion.noWayBack')}</Text>
              </Alert>
            )}
          </>
        )}
        {callPanel && (
          <DisplayPanel universeName={callPanel.universe} panelId={callPanel.panelId} variant="inline" title={callPanel.title} />
        )}
        {end && (
          <Alert
            color={end.kind === 'refused' ? 'orange' : 'green'}
            variant="light"
            icon={end.kind === 'committed' ? <IconGitCommit size={16} /> : end.kind === 'refused' ? <IconAlertTriangle size={16} /> : <IconCheck size={16} />}
            data-guide="conversion.end"
            data-state={end.kind}
          >
            <Text size="sm">{t('conversion.reached', { to: end.to, count: end.files })}</Text>
            {end.kind === 'committed' && <Text size="xs">{t('conversion.committed', { sha: end.sha.slice(0, 10) })}</Text>}
            {end.kind === 'no-repo' && <Text size="xs">{t('conversion.endNoRepo')}</Text>}
            {end.kind === 'refused' && (
              <>
                <Text size="xs">{t('conversion.refused')}</Text>
                <Code block>{end.reason}</Code>
              </>
            )}
          </Alert>
        )}
        {error && (
          <Alert color="red" variant="light" icon={<IconAlertTriangle size={16} />} data-guide="conversion.error">
            <Text size="xs">{error}</Text>
          </Alert>
        )}
        <Group justify="flex-end" gap="xs">
          <Button variant="default" size="xs" onClick={onClose} disabled={running}>
            {end || upToDate(dry) ? t('conversion.close') : t('project.cancel')}
          </Button>
          {!end && !upToDate(dry) && (
            <Button size="xs" color="orange" onClick={() => void confirm()} disabled={!canConfirm(dry)} loading={running} data-element={CONVERSION_CONFIRM}>
              {t('conversion.confirm')}
            </Button>
          )}
        </Group>
      </Stack>
    </Modal>
  )
}
