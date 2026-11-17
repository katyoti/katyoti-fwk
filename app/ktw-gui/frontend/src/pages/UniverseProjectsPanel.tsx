// SPDX-License-Identifier: Apache-2.0
/**
 * UniverseProjectsPanel — the projects attached to one universe.
 *
 * A central universe holds the shared configuration and sidecars; a PROJECT is a
 * folder that borrows them instead of carrying its own `.katyoti/`. Attaching is
 * how a repo joins a universe without being one.
 *
 * Attach / detach / list were KTSP-published and CLI-reachable long before this;
 * what was missing was the surface. The task that inherited this asked for
 * `Project*` Wails bindings — that predates the typed client, and adding them now
 * would rebuild the API surface a second time in Go. These are plain calls
 * through the generated orchestrator client: zero Go changes.
 *
 * A universe serves one project. Creating, attaching or detaching one in a
 * running universe makes the hub restart the member serving its objects, and
 * the panel waits until it serves the change (awaitServed); a
 * change recorded but not served is said, with the reason.
 */

import { useCallback, useEffect, useState } from 'react'
import { Alert, Badge, Box, Button, Group, Loader, Paper, Stack, Text, TextInput, Tooltip } from '@mantine/core'
import { IconAlertTriangle, IconFolderPlus, IconInfoCircle, IconPlus, IconUnlink } from '@tabler/icons-react'
import { dispatcherFor, orchestratorClient } from '../ktsp'
import { useT, type TFn } from '../i18n'
import { awaitServed } from '../sections/univers/projectMount'
import { notServedText } from '../sections/univers/serveWait'
import { CONVERSION_BADGE, PROJECTS_KNOWN, conversionMark, knownAt, type KnownProject } from '../sections/project/conversion'
import { canCreate, checkName, createNewProject, projectName, refusalKey, type NameCheck } from '../sections/univers/newProject'

interface Props {
  universe: string
}

export function UniverseProjectsPanel({ universe }: Props) {
  const t = useT('universes')
  const [projects, setProjects] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<string>('')
  const [error, setError] = useState<string | null>(null)
  // A new project from its name: the hub's check as it is typed.
  const [creating, setCreating] = useState(false)
  const [typed, setTyped] = useState('')
  const [check, setCheck] = useState<NameCheck | undefined>(undefined)
  // Where each project's data stands against this KaTyoti: a
  // project waiting for its conversion is marked; it converts from the
  // project panel once open.
  const [known, setKnown] = useState<KnownProject[] | undefined>(undefined)
  const tShell = useT('shell')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const out = await orchestratorClient().listProjects({ universe })
      setProjects(out.projects ?? [])
      setError(null)
      dispatcherFor(universe)(PROJECTS_KNOWN, {})
        .then((k) => setKnown((k as { projects?: KnownProject[] } | null)?.projects))
        .catch(() => setKnown(undefined))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [universe])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    const name = projectName(typed)
    if (!creating || name === '') {
      setCheck(undefined)
      return
    }
    let live = true
    const timer = setTimeout(() => {
      checkName(orchestratorClient(), name, universe)
        .then((c) => { if (live) setCheck(c) })
        .catch((err) => { if (live) setError(err instanceof Error ? err.message : String(err)) })
    }, 250)
    return () => { live = false; clearTimeout(timer) }
  }, [typed, creating, universe])

  const closeNew = () => {
    setCreating(false)
    setTyped('')
    setCheck(undefined)
  }

  const create = async () => {
    if (!canCreate(typed, check, busy !== '')) return
    setBusy('new')
    setError(null)
    try {
      const out = await createNewProject(orchestratorClient(), universe, typed)
      closeNew()
      await load()
      await awaitServed(universe, out, out.projectDir)
    } catch (err) {
      setError(notServedText(err, t))
    } finally {
      setBusy('')
    }
  }

  const checking = creating && projectName(typed) !== '' && check?.name !== projectName(typed)
  const refusal = check && check.name === projectName(typed) && check.refused ? (refusalKey(check) ? t(refusalKey(check)!) : check.refused) : ''

  const attach = async () => {
    // The folder PICKER stays a Wails binding — a native OS dialog is exactly
    // what KTSP cannot express. Registering the picked folder is a service call.
    const w = window as unknown as {
      go?: { main?: { App?: { PickWorkspaceFolder?: () => Promise<{ path?: string; error?: string }> } } }
    }
    const pick = w.go?.main?.App?.PickWorkspaceFolder
    if (!pick) {
      setError(t('projects.pickerUnavailable', { command: 'katyoti project attach --universe ' + universe }))
      return
    }
    const picked = await pick()
    if (!picked?.path) return // cancelled
    setBusy(picked.path)
    setError(null)
    try {
      const out = await orchestratorClient().attachProject({ universe, projectDir: picked.path })
      await load()
      await awaitServed(universe, out, out.projectDir)
    } catch (err) {
      setError(notServedText(err, t))
    } finally {
      setBusy('')
    }
  }

  const detach = async (dir: string) => {
    setBusy(dir)
    setError(null)
    try {
      // Detach takes the PROJECT, not the universe: a folder is attached to at
      // most one, so naming the universe would be redundant and could disagree.
      const out = await orchestratorClient().detachProject({ projectDir: dir })
      await load()
      // The detached project is served no more; the universe may serve others.
      await awaitServed(universe, out, { gone: dir })
    } catch (err) {
      setError(notServedText(err, t))
    } finally {
      setBusy('')
    }
  }

  return (
    <Stack gap="sm">
      <Group justify="space-between">
        <Text size="xs" c="dimmed">
          {t('projects.intro')}
        </Text>
        <Group gap="xs" wrap="nowrap">
          <Button size="xs" variant="light" leftSection={<IconPlus size={14} />} onClick={() => setCreating(true)} disabled={creating} data-element="action:universe/new-project">
            {t('projects.newProject')}
          </Button>
          <Button size="xs" leftSection={<IconFolderPlus size={14} />} onClick={() => void attach()} data-guide="projects.attach">
            {t('projects.attach')}
          </Button>
        </Group>
      </Group>

      {creating && (
        <Paper withBorder p="sm" radius="sm" data-element="section:universe/new-project">
          <Stack gap="xs">
            <Text size="xs" c="dimmed">{t('projects.newProjectHint')}</Text>
            <TextInput
              size="xs"
              label={t('projects.name')}
              placeholder={t('projects.namePlaceholder')}
              value={typed}
              onChange={(e) => setTyped(e.currentTarget.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') void create() }}
              error={refusal || undefined}
              autoFocus
              name="project-name"
            />
            {checking ? (
              <Text size="xs" c="dimmed">{t('projects.checking')}</Text>
            ) : check && check.name === projectName(typed) && !check.refused ? (
              <Text size="xs" c="dimmed" style={{ wordBreak: 'break-all' }}>{t('projects.willCreate', { folder: check.folder })}</Text>
            ) : null}
            <Group justify="flex-end" gap="xs">
              <Button size="xs" variant="subtle" onClick={closeNew}>{t('projects.cancel')}</Button>
              <Button size="xs" onClick={() => void create()} disabled={!canCreate(typed, check, busy !== '')} loading={busy === 'new'} data-element="action:universe/create-project">
                {t('projects.create')}
              </Button>
            </Group>
          </Stack>
        </Paper>
      )}

      {error && (
        <Alert color="red" variant="light" icon={<IconAlertTriangle size={16} />}>
          <Text size="xs">{error}</Text>
        </Alert>
      )}

      {loading ? (
        <Group justify="center" p="md"><Loader size="sm" /></Group>
      ) : projects.length === 0 ? (
        <Alert color="gray" variant="light" icon={<IconInfoCircle size={16} />}>
          <Text size="xs">
            {t('projects.empty')}
          </Text>
        </Alert>
      ) : (
        <Stack gap={4}>
          {projects.map((dir) => (
            <Group
              key={dir}
              justify="space-between"
              wrap="nowrap"
              p="xs"
              style={{ border: '1px solid var(--mantine-color-default-border)', borderRadius: 6 }}
            >
              <Box style={{ minWidth: 0 }}>
                <Tooltip label={dir} multiline w={420}>
                  <Text size="xs" truncate>{dir}</Text>
                </Tooltip>
              </Box>
              <ProjectConversion mark={conversionMark(knownAt(known, dir))} t={tShell} />
              <Tooltip label={t('projects.detachHint')}>
                <Button
                  size="xs"
                  variant="subtle"
                  color="red"
                  loading={busy === dir}
                  leftSection={<IconUnlink size={13} />}
                  onClick={() => void detach(dir)}
                >
                  {t('projects.detach')}
                </Button>
              </Tooltip>
            </Group>
          ))}
        </Stack>
      )}
    </Stack>
  )
}

/** A project's conversion mark in the list: waiting (open it to convert) or
 *  newer than this KaTyoti; nothing at its version. */
function ProjectConversion({ mark, t }: { mark: ReturnType<typeof conversionMark>; t: TFn }) {
  if (mark.kind === 'none') return null
  const waiting = mark.kind === 'waiting'
  return (
    <Tooltip label={t(waiting ? 'conversion.openToConvert' : 'conversion.newerHint', { from: mark.from })} multiline w={260}>
      <Badge size="xs" variant="light" color={waiting ? 'orange' : 'red'} data-element={CONVERSION_BADGE} data-state={mark.kind} style={{ flexShrink: 0 }}>
        {t(waiting ? 'conversion.waiting' : 'conversion.newer')}
      </Badge>
    </Tooltip>
  )
}
