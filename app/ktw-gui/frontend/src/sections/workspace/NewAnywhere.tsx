// SPDX-License-Identifier: Apache-2.0
/**
 * « + New » anywhere: the mod+shift+N shortcut and the new-project dialog the
 * left panel's « … » opens (« Ajouter un projet… »); the panel's head
 * draws no « + » - an object's « + Nouveau » creates under it. With an object
 * open in the Object tab - the place selected in the tree - the shortcut opens
 * the create dialog under it; at the universe's level it opens the
 * new-project dialog offering the project types. The hub makes and
 * attaches the project (create-project), CWM gives it its project
 * object (the default type), and a type chosen otherwise is set through CWM's
 * retype verb. A type whose create_task answers a page (a method URI) shows
 * that form in the dialog: its submit is an action of the page host, which
 * switches the universe to the project it answers. A project
 * already in its own repository is linked instead (the project's
 * attach): one repository is one project, never a project inside another.
 */
import { useState } from 'react'
import { Alert, Button, Divider, Group, Modal, Select, Stack, Text, TextInput } from '@mantine/core'
import { useHotkeys } from '@mantine/hooks'
import { IconFolderPlus } from '@tabler/icons-react'
import { useTabsStore } from '../../store/tabsSlice'
import { openObjectId } from './treeSelection'
import { ObjectCreateModal } from './ObjectCreateModal'
import { newTargetFor, projectTypes, DEFAULT_PROJECT_TYPE } from './newTarget'
import { createTaskPage, newProjectPick } from './newProjectPick'
import { PageHost } from '../../components/compositions/PageHost'
import type { UIPageNode } from '@ktw/display-lib/ui-builders/page/index.ts'
import { useTypeStyles } from '../../hooks/useTypeStyles'
import { contribURI, useCT } from '../../i18n/contrib'
import { useT } from '../../i18n'
import { orchestratorClient, dispatcherFor } from '../../ktsp'
import { createNewProject, projectName } from '../univers/newProject'
import { folderPicker, linkExistingProject } from '../univers/linkProject'
import { awaitServed } from '../univers/projectMount'
import { notServedText } from '../univers/serveWait'
import { RunTaskInUniverse } from '../../../wailsjs/go/main/App'
import { useAppStore } from '../../store'
import { useUniverseGui } from '../../hooks/useUniverseGui'

/** NewAnywhere owns mod+shift+N and the create and new-project dialogs; the
 *  new-project dialog is also opened by the panel's « … » (newProject). */
export function NewAnywhere({ universeName, newProject, onNewProject }: {
  universeName: string; newProject: boolean; onNewProject: (open: boolean) => void
}) {
  const ct = useCT()
  const selected = useTabsStore((s) => openObjectId(s.getForUniverse(universeName)))
  const styles = useTypeStyles(universeName)
  const { hidden } = useUniverseGui(universeName)
  const [creatingUnder, setCreatingUnder] = useState<string | null>(null)
  const open = () => {
    const target = newTargetFor(selected)
    if (target.kind === 'object') setCreatingUnder(target.parent)
    else onNewProject(true)
  }
  useHotkeys([['mod+shift+N', open]])
  return (
    <>
      <ObjectCreateModal opened={creatingUnder !== null} onClose={() => setCreatingUnder(null)} universeName={universeName}
        parentId={creatingUnder ?? ''} parentTitle={creatingUnder ?? ''} />
      <NewProjectModal opened={newProject} onClose={() => onNewProject(false)} universeName={universeName}
        types={projectTypes(styles, (id) => !hidden(id))} label={(type) => ct(contribURI('object-type', type), 'label', styles[type]?.label ?? type)}
        taskOf={(type) => styles[type]?.createTask} />
    </>
  )
}

function NewProjectModal({ opened, onClose, universeName, types, label, taskOf }: {
  opened: boolean; onClose: () => void; universeName: string; types: string[]; label: (type: string) => string
  taskOf: (type: string) => string | undefined
}) {
  const t = useT('shell')
  const tUniverses = useT('universes')
  const [name, setName] = useState('')
  const [type, setType] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [linking, setLinking] = useState(false)
  const [linkError, setLinkError] = useState('')
  const chosen = type ?? types[0] ?? DEFAULT_PROJECT_TYPE
  // A type created by its form: the form replaces the name.
  const page = createTaskPage(taskOf(chosen))
  const create = async () => {
    setBusy(true)
    setError('')
    try {
      const pick = newProjectPick(chosen, label(chosen), name, taskOf(chosen))
      if (pick.kind === 'page') return
      if (pick.kind === 'task') {
        useAppStore.getState().pushToast({ message: t('project.taskStarted', { type: label(chosen) }), color: 'blue' })
        await RunTaskInUniverse(universeName, pick.task, pick.input)
      } else {
        await createNewProject(orchestratorClient(), universeName, name)
        if (pick.retype) await retypeWhenReady(universeName, projectName(name), pick.retype)
      }
      onClose()
    } catch (e) {
      setError(String((e as Error)?.message ?? e))
    } finally {
      setBusy(false)
    }
  }
  // « Link an existing repository »: its folder picked, the hub
  // attaches it, and the dialog closes once the universe serves it.
  const link = async () => {
    const pick = folderPicker()
    if (!pick) {
      setLinkError(tUniverses('projects.pickerUnavailable', { command: 'katyoti project attach --universe ' + universeName }))
      return
    }
    setLinking(true)
    setLinkError('')
    try {
      const out = await linkExistingProject(orchestratorClient(), universeName, pick)
      if (!out) return
      await awaitServed(universeName, out, out.projectDir)
      onClose()
    } catch (e) {
      setLinkError(notServedText(e, tUniverses))
    } finally {
      setLinking(false)
    }
  }
  return (
    <Modal opened={opened} onClose={onClose} title={<Text fw={600}>{t('project.newProject')}</Text>} centered size={page ? 'lg' : 'sm'}>
      <Stack gap="sm" data-guide="project.new-project">
        <Select label={t('project.projectType')} data={types.map((ty) => ({ value: ty, label: label(ty) }))} value={chosen}
          onChange={setType} allowDeselect={false} comboboxProps={{ withinPortal: true }} />
        {page ? (
          // The form is found by its section (data-section="new-project-form").
          <PageHost universeName={universeName} node={{ kind: 'box', id: 'new-project-form', children: [], load: { method: page }, placeholder: { kind: 'skeleton', count: 4 } } as UIPageNode} />
        ) : (
          <>
            <TextInput label={t('project.projectName')} value={name} onChange={(e) => setName(e.currentTarget.value)} data-autofocus />
            {error && <Alert color="red" variant="light">{error}</Alert>}
            <Group justify="flex-end" gap="xs">
              <Button size="xs" variant="default" onClick={onClose} disabled={busy}>{t('project.cancel')}</Button>
              <Button size="xs" onClick={() => void create()} loading={busy} disabled={projectName(name) === ''}>{t('project.create')}</Button>
            </Group>
          </>
        )}
        <Divider label={t('project.orLink')} labelPosition="center" />
        <Group justify="center">
          <Button size="xs" variant="light" leftSection={<IconFolderPlus size={14} />} data-guide="project.link-existing"
            onClick={() => void link()} loading={linking} disabled={busy}>
            {t('project.linkExisting')}
          </Button>
        </Group>
        {linkError && <Alert color="red" variant="light">{linkError}</Alert>}
      </Stack>
    </Modal>
  )
}

// retypeWhenReady sets the new project's type once CWM has made its project
// object (the attach starts it): asked a few times while it is being made.
async function retypeWhenReady(universe: string, id: string, type: string): Promise<void> {
  let last: unknown
  for (let i = 0; i < 10; i++) {
    try {
      await dispatcherFor(universe)('ktw-agents:cwm/object-retype', { id, type })
      return
    } catch (e) {
      last = e
      await new Promise((r) => setTimeout(r, 1000))
    }
  }
  throw last
}
