// SPDX-License-Identifier: Apache-2.0
/**
 * The object pane's core views — Information and Links — drawn as UI
 * extensions. Their panels are DECLARED (ktw-agents' `object-panels/`), so a
 * type or an object can hide or replace them like any other; they read the
 * object detail the pane already holds and call nothing themselves.
 *
 * Layout follows the method panels: uppercase dimmed section titles, a
 * the type's and capabilities' fields as sections, badges for what the object carries, cards for
 * links grouped by direction.
 */

import { objectTabId } from '../../elements'
import { useEffect, useState } from 'react'
import {
  ActionIcon, Alert, Badge, Box, Button, Card, Group, Loader, Modal, ScrollArea, Select, Stack, Text,
  Textarea, TextInput, ThemeIcon, Title, Tooltip,
} from '@mantine/core'
import { ItemCard } from '@ktw/display-lib/ui-core/components/other/index.ts'
import {
  IconAlertTriangle, IconArchive, IconArrowNarrowLeft, IconArrowNarrowRight, IconLinkOff, IconPencil, IconPlus, IconPuzzle, IconSearch, IconX,
} from '@tabler/icons-react'
import {
  WorkspaceObjectFind, WorkspaceObjectLinkAdd, WorkspaceObjectLinkDelete, WorkspaceObjectLinkKinds,
  WorkspaceObjectUpdateWithBody,
} from '../../../wailsjs/go/main/App'
import { Markdown } from '../../components/Markdown'
import { main } from '../../../wailsjs/go/models'
import { useAppStore } from '../../store'
import { openObjectTab } from './openObjectTab'
import { colorForType, iconForType } from './type-style'
import { useTypeStyles } from '../../hooks/useTypeStyles'
import { ObjectFieldsForm } from './ObjectFieldsForm'
import { ObjectComments } from './ObjectComments'
import { tablerIcon } from '@ktw/display-lib/ui-core/components/icons/index.ts'
import { useT } from '../../i18n'
import { errorText } from '../../i18n/errors'
import { contribURI, useCT } from '../../i18n/contrib'
import { ActionsPanel } from './ActionsPanel'
import { noteRevision, revisionOf } from './objectRevision'
import { visibleObjects } from './systemModules'
import { useDisplayLevel } from '../../hooks/useUniverseGui'
import { verifyObject } from './objectVerify'
import { ProjectMark } from './projectMark'
import { objectRef, projectOfRef } from './objectRef'

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <Title order={6} mb="xs" c="dimmed" tt="uppercase" style={{ letterSpacing: 0.5 }}>{children}</Title>
  )
}

/** CapabilityBadges — the capabilities an object carries, one badge each:
 *  icon and name, the description and where it comes from (its type, an
 *  aspect, this object) as the tooltip. Shown in the pane's header. */
export function CapabilityBadges({ detail, only }: {
  detail: main.WorkspaceObjectDetail
  /** Which of them to draw (paneHeader: what the type carries, or what is added). */
  only?: (infos: main.WorkspaceCapabilityInfo[]) => main.WorkspaceCapabilityInfo[]
}) {
  const all: main.WorkspaceCapabilityInfo[] = detail.capabilityInfo?.length
    ? detail.capabilityInfo
    : (detail.capabilities ?? []).map((id) => ({ id } as main.WorkspaceCapabilityInfo))
  const infos = only ? only(all) : all
  const t = useT('workspace')
  const ct = useCT()
  return (
    <>
      {infos.map((c) => {
        const Icon = tablerIcon(c.icon, IconPuzzle)
        const uri = contribURI('capability', c.id)
        const label = ct(uri, 'label', c.label || c.id)
        const description = c.description ? ct(uri, 'description', c.description) : ''
        const from = !c.source || c.source === 'type' ? t('core.capFromType')
          : c.source === 'object' ? t('core.capFromObject')
          : c.source === 'attached' ? t('core.capAttached')
          : t('core.capFromAspect', { aspect: ct(contribURI('aspect', c.source.replace(/^aspect:/, '')), 'label', c.source.replace(/^aspect:/, '')) })
        return (
          <Tooltip key={c.id} withArrow multiline w={320}
            label={<><Text size="xs" fw={600}>{label}</Text>
              {description && <Text size="xs">{description}</Text>}
              <Text size="xs" c="dimmed">{from}</Text></>}>
            <Badge size="xs" variant="light" color={c.source === 'object' ? 'grape' : 'teal'}
              leftSection={<Icon size={10} />} style={{ textTransform: 'none' }}>
              {label}
            </Badge>
          </Tooltip>
        )
      })}
    </>
  )
}

/** ObjectInformationView — the object beyond its core (the header shows and
 *  edits the core and lists its capabilities): its errors first, when it has
 *  any (what CWM found wrong when it read it), then the type's own fields with
 *  its body as the type declares it, each capability's fields that no panel
 *  covers, the policies that apply and the comments. */
export function ObjectInformationView({ detail, universeName }: { detail: main.WorkspaceObjectDetail; universeName: string }) {
  const bumpWorkspaceTree = useAppStore((s) => s.bumpWorkspaceTree)
  const [checkResult, setCheckResult] = useState<Record<string, string>>({})
  // A check runs as the long method it is: its panel drawn under the check.
  const [checkPanel, setCheckPanel] = useState<Record<string, string>>({})
  const sections = (detail.tabs ?? []).filter((t) => t.section)
  const policies = detail.policies ?? []
  const quick = (detail.quickActions ?? []).filter((a) => a.quick)
  const t = useT('workspace')
  const ct = useCT()
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Stack gap="lg" p="md">
        {/* The actions that matter most on this object, declared quick by its
            type or a capability: small buttons first. */}
        {quick.length > 0 && <ActionsPanel actions={quick} universeName={universeName} nodeId={objectRef(detail.id, detail.project)} layout="buttons" />}
        {(detail.errors?.length ?? 0) > 0 && (
          <Box>
            <SectionTitle>{t('core.errors', { count: detail.errors!.length })}</SectionTitle>
            <Stack gap={6}>
              {detail.errors!.map((d, i) => (
                <Alert key={`${d.code}-${i}`} color="red" variant="light" icon={<IconAlertTriangle size={14} />} title={d.label} p="xs">
                  {d.description && d.description !== d.label && <Text size="xs">{d.description}</Text>}
                  <Text size="xs" c="dimmed">{d.code}</Text>
                </Alert>
              ))}
            </Stack>
          </Box>
        )}

        {sections.map((tab) => {
          const capID = tab.capabilities?.[0] ?? ''
          return (
            <Box key={tab.id}>
              <SectionTitle>{tab.name || capID}</SectionTitle>
              <Card withBorder padding="sm" radius="sm">
                <Stack gap="md">
                  {(!tab.body || tab.schema) && (
                    <ObjectFieldsForm
                      embedded
                      universeName={universeName}
                      objectId={objectRef(detail.id, detail.project)}
                      tab={tab}
                      onSaved={bumpWorkspaceTree}
                      result={checkResult[capID]}
                      checkPanel={checkPanel[capID] || undefined}
                      onRunCheck={tab.verify
                        ? async () => {
                            const o = await verifyObject(universeName, objectRef(detail.id, detail.project))
                            setCheckPanel((m) => ({ ...m, [capID]: 'panel' in o ? o.panel : '' }))
                            setCheckResult((m) => ({ ...m, [capID]: 'text' in o ? o.text : '' }))
                          }
                        : undefined}
                    />
                  )}
                  {tab.body && (
                    <ObjectBodyField decl={tab.body} detail={detail} universeName={universeName} onSaved={bumpWorkspaceTree} />
                  )}
                </Stack>
              </Card>
            </Box>
          )
        })}

        {policies.length > 0 && (
          <Box>
            <SectionTitle>{t('core.policies', { count: policies.length })}</SectionTitle>
            <Group gap={6}>
              {policies.map((p) => (
                <Tooltip key={p.name} label={ct(contribURI('policy', p.name), 'description', p.description || p.name)} withArrow multiline w={320} disabled={!p.description}>
                  <Badge variant="outline" color="violet" style={{ textTransform: 'none' }}>
                    {ct(contribURI('policy', p.name), 'label', p.label)}{p.hasCheck ? ' ✓' : ''}
                  </Badge>
                </Tooltip>
              ))}
            </Group>
            <Text size="xs" c="dimmed" mt="xs">
              {t('core.policiesHint')}
            </Text>
          </Box>
        )}

        <Box>
          <SectionTitle>{t('core.comments')}</SectionTitle>
          <ObjectComments universeName={universeName} objectId={objectRef(detail.id, detail.project)} />
        </Box>
      </Stack>
    </ScrollArea>
  )
}

/** ObjectBodyField — the object's body as its type declares it: under the
 *  type's label, in the type's format (markdown rendered, text as written,
 *  html in a sandbox), edited here. A body a panel shows (a deck's slides) is
 *  named, not repeated. */
function ObjectBodyField({ decl, detail, universeName, onSaved }: {
  decl: main.WorkspaceObjectBody; detail: main.WorkspaceObjectDetail; universeName: string; onSaved: () => void
}) {
  const [draft, setDraft] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const t = useT('workspace')
  const body = detail.body ?? ''
  const panel = decl.panel ? (detail.tabs ?? []).find((t) => t.id === objectTabId(decl.panel!)) : undefined
  const save = async () => {
    if (draft === null) return
    setSaving(true); setError('')
    try {
      const id = objectRef(detail.id, detail.project)
      const r = await WorkspaceObjectUpdateWithBody(universeName, id, '', draft, revisionOf(id))
      noteRevision(id, r.revision)
      if (r.error) { setError(errorText(r)); return }
      setDraft(null)
      onSaved()
    } finally {
      setSaving(false)
    }
  }
  if (decl.panel) {
    return (
      <Group gap={6}>
        <Text size="xs" c="dimmed" style={{ minWidth: 140 }}>{decl.label}</Text>
        {/* The panel that shows the body, one click away. */}
        <Button size="xs" variant="light"
          onClick={() => openObjectTab(universeName, { id: objectRef(detail.id, detail.project), title: detail.title, panel: panel?.id ?? decl.panel ?? '' })}>
          {t('core.openPanel', { panel: panel?.name || decl.panel })}
        </Button>
      </Group>
    )
  }
  return (
    <Stack gap="xs">
      <Group justify="space-between" wrap="nowrap">
        <Tooltip label={decl.description} disabled={!decl.description} withArrow multiline w={320}>
          <Text size="xs" c="dimmed">{decl.label}</Text>
        </Tooltip>
        {draft === null && (
          <Button size="xs" variant="light" leftSection={<IconPencil size={14} />} onClick={() => { setError(''); setDraft(body) }}>
            {t('objectTab.edit')}
          </Button>
        )}
      </Group>
      {draft !== null ? (
        <Stack gap="xs">
          <Textarea autosize minRows={6} maxRows={30} value={draft} onChange={(e) => setDraft(e.currentTarget.value)}
            styles={{ input: { fontFamily: decl.format === 'text' ? undefined : 'var(--mantine-font-family-monospace)' } }} />
          {error && <Alert color="red" variant="light" icon={<IconAlertTriangle size={14} />} p="xs">{error}</Alert>}
          <Group justify="flex-end" gap="xs">
            <Button size="xs" variant="default" onClick={() => setDraft(null)} disabled={saving}>{t('common.cancel')}</Button>
            <Button size="xs" onClick={() => void save()} loading={saving}>{t('common.save')}</Button>
          </Group>
        </Stack>
      ) : body.trim() === '' ? (
        <Text size="xs" c="dimmed">{t('core.bodyEmpty', { label: decl.label.toLowerCase() })}</Text>
      ) : decl.format === 'text' ? (
        <Text size="sm" style={{ whiteSpace: 'pre-wrap' }}>{body}</Text>
      ) : decl.format === 'html' ? (
        <iframe title={decl.label} sandbox="" srcDoc={body} style={{ width: '100%', minHeight: 240, border: 0 }} />
      ) : (
        <Markdown text={body} />
      )}
    </Stack>
  )
}

function LinkCard({ l, universeName, project, onRemove }: {
  l: main.WorkspaceObjectLink; universeName: string; onRemove?: () => void
  /** The project holding the linking object: a target written bare is of it. */
  project?: string
}) {
  const styles = useTypeStyles(universeName)
  const Icon = l.targetType ? iconForType(l.targetType, styles[l.targetType]) : l.ended ? IconArchive : IconLinkOff
  const open = l.resolved ? () => openObjectTab(universeName, { id: objectRef(l.targetId, project), title: l.targetTitle }) : undefined
  const t = useT('workspace')
  const ct = useCT()
  // The link as a person reads it from this side, in the GUI language.
  const label = ct(contribURI('edge-type', l.kind), l.direction === 'out' ? 'label' : 'inverse_label', l.label)
  return (
    <ItemCard padding="xs" onClick={open} style={{ opacity: l.resolved ? 1 : 0.85 }}>
      <Group gap={8} wrap="nowrap">
        <Badge size="sm" variant="light" style={{ textTransform: 'none', flexShrink: 0 }}>{label}</Badge>
        <ThemeIcon size="sm" variant="light" color={l.targetType ? colorForType(l.targetType, styles[l.targetType]) : l.ended ? 'gray' : 'red'}>
          <Icon size={12} />
        </ThemeIcon>
        {l.resolved ? (
          <Text size="sm" fw={500} style={{ flex: 1, minWidth: 0 }} truncate>{l.targetTitle || l.targetId}</Text>
        ) : l.ended ? (
          // An archived object: a valid link to an end that has left the
          // project — shown as ended, never as a broken reference.
          <Tooltip label={t('core.endedTip')} withArrow>
            <Group gap={6} wrap="nowrap" style={{ flex: 1, minWidth: 0 }}>
              <Text size="sm" c="dimmed" ff="monospace" style={{ minWidth: 0 }} truncate>{l.targetId}</Text>
              <Badge size="xs" variant="light" color="gray" style={{ flexShrink: 0 }}>{t('core.ended')}</Badge>
            </Group>
          </Tooltip>
        ) : (
          // A path, a URI or a dangling id: shown, never hidden — hiding it
          // would hide a broken reference.
          <Tooltip label={t('core.notAnObject')} withArrow>
            <Text size="sm" c="dimmed" ff="monospace" style={{ flex: 1, minWidth: 0 }} truncate>{l.targetId}</Text>
          </Tooltip>
        )}
        {l.targetType && <Badge size="xs" variant="outline" color="gray" style={{ textTransform: 'none', flexShrink: 0 }}>{l.targetType}</Badge>}
        {onRemove && (
          <Tooltip label={t('core.removeLinkTip')} withArrow>
            <ActionIcon size="sm" variant="subtle" color="red" aria-label={t('core.removeLink')}
              onClick={(e) => { e.stopPropagation(); onRemove() }}>
              <IconX size={14} />
            </ActionIcon>
          </Tooltip>
        )}
      </Group>
    </ItemCard>
  )
}

/** ObjectLinksView — every incident edge, named from this object's side,
 *  outgoing first; an end that is not an object here is shown, not dropped.
 *  Outgoing links are this object's to author: "Add link" and each card's
 *  remove act on them; an incoming link belongs to the object it leaves. */
export function ObjectLinksView({ links, universeName, objectId }: {
  links: main.WorkspaceObjectLink[]; universeName: string; objectId?: string
}) {
  const bumpWorkspaceTree = useAppStore((s) => s.bumpWorkspaceTree)
  const [adding, setAdding] = useState(false)
  const [error, setError] = useState('')
  const t = useT('workspace')
  const out = links.filter((l) => l.direction === 'out')
  const inc = links.filter((l) => l.direction !== 'out')
  const remove = async (l: main.WorkspaceObjectLink) => {
    if (!objectId) return
    setError('')
    const r = await WorkspaceObjectLinkDelete(universeName, objectId, l.kind, l.targetId)
    if (r.error) { setError(errorText(r)); return }
    bumpWorkspaceTree()
  }
  const group = (title: string, Icon: typeof IconArrowNarrowRight, list: main.WorkspaceObjectLink[], removable: boolean) =>
    list.length > 0 && (
      <Box>
        <Group gap={6} mb="xs">
          <Icon size={14} color="var(--mantine-color-dimmed)" />
          <SectionTitle>{t('core.linkGroup', { title, count: list.length })}</SectionTitle>
        </Group>
        <Stack gap={6}>
          {list.map((l, i) => (
            <LinkCard key={`${l.kind}-${l.targetId}-${i}`} l={l} universeName={universeName} project={projectOfRef(objectId)}
              onRemove={removable && objectId ? () => void remove(l) : undefined} />
          ))}
        </Stack>
      </Box>
    )
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Stack gap="lg" p="md">
        {objectId && (
          <Group justify="flex-end">
            <Button size="xs" variant="light" leftSection={<IconPlus size={14} />} onClick={() => setAdding(true)} data-guide="links.add">
              {t('core.addLink')}
            </Button>
          </Group>
        )}
        {error && <Alert color="red" variant="light" icon={<IconAlertTriangle size={16} />}>{error}</Alert>}
        {links.length === 0 && <Text size="sm" c="dimmed">{t('core.noLinks')}</Text>}
        {group(t('core.outgoing'), IconArrowNarrowRight, out, true)}
        {group(t('core.incoming'), IconArrowNarrowLeft, inc, false)}
      </Stack>
      {objectId && (
        <AddLinkModal opened={adding} onClose={() => setAdding(false)} universeName={universeName} fromId={objectId}
          onAdded={() => { setAdding(false); bumpWorkspaceTree() }} />
      )}
    </ScrollArea>
  )
}

/** AddLinkModal — a kind from the declared edge types, a target found by
 *  search; EdgeCreate decides whether the link may exist and says why not. */
function AddLinkModal({ opened, onClose, universeName, fromId, onAdded }: {
  opened: boolean; onClose: () => void; universeName: string; fromId: string; onAdded: () => void
}) {
  const styles = useTypeStyles(universeName)
  const [kinds, setKinds] = useState<main.WorkspaceLinkKind[] | null>(null)
  const [kind, setKind] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [found, setFound] = useState<main.WorkspaceObject[]>([])
  const [searching, setSearching] = useState(false)
  const level = useDisplayLevel()
  const [target, setTarget] = useState<main.WorkspaceObject | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const t = useT('workspace')
  const ct = useCT()

  useEffect(() => {
    if (!opened) return
    setKind(null); setQuery(''); setFound([]); setTarget(null); setError('')
    WorkspaceObjectLinkKinds(universeName).then((k) => setKinds(k ?? [])).catch(() => setKinds([]))
  }, [opened, universeName])

  useEffect(() => {
    if (!opened || query.trim().length < 2) { setFound([]); return }
    let cancelled = false
    setSearching(true)
    const timer = setTimeout(() => {
      WorkspaceObjectFind(universeName, query.trim())
        .then((r) => { if (!cancelled) setFound(visibleObjects(r.objects ?? [], level).filter((o) => o.id !== fromId).slice(0, 12)) })
        .catch(() => { if (!cancelled) setFound([]) })
        .finally(() => { if (!cancelled) setSearching(false) })
    }, 250)
    return () => { cancelled = true; clearTimeout(timer) }
  }, [query, opened, universeName, fromId, level])

  const add = async () => {
    if (!kind || !target) return
    setSaving(true); setError('')
    try {
      // The target by its id as sent: a homonym of another
      // project is named, never taken for this one's.
      const r = await WorkspaceObjectLinkAdd(universeName, fromId, kind, objectRef(target.id, target.project))
      if (r.error) { setError(errorText(r)); return }
      onAdded()
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal opened={opened} onClose={onClose} centered title={<Text fw={600}>{t('core.addLinkTitle')}</Text>}>
      <Stack gap="sm">
        {kinds === null ? <Loader size="sm" /> : (
          <Select label={t('core.kind')} placeholder={t('core.kindPlaceholder')} searchable value={kind} onChange={setKind}
            data={kinds.map((k) => ({ value: k.name, label: ct(contribURI('edge-type', k.name), 'label', k.name) }))}
            description={(() => {
              const d = kinds.find((k) => k.name === kind)?.description
              return d ? ct(contribURI('edge-type', kind ?? ''), 'description', d) : d
            })()} />
        )}
        <TextInput label={t('core.target')} placeholder={t('core.targetPlaceholder')} leftSection={<IconSearch size={14} />} data-guide="links.target-search"
          value={query} onChange={(e) => { setQuery(e.currentTarget.value); setTarget(null) }}
          rightSection={searching ? <Loader size="xs" /> : undefined} />
        <Stack gap={4}>
          {found.map((o) => {
            const Icon = iconForType(o.type, styles[o.type])
            const picked = !!target && objectRef(target.id, target.project) === objectRef(o.id, o.project)
            return (
              <Card key={`${o.project ?? ''}:${o.id}`} withBorder padding="xs" radius="sm" style={{ cursor: 'pointer' }} data-guide="links.found" data-guide-name={o.id}
                bg={picked ? 'var(--mantine-color-blue-light)' : undefined} onClick={() => setTarget(o)}>
                <Group gap={8} wrap="nowrap">
                  <ThemeIcon size="sm" variant="light" color={colorForType(o.type, styles[o.type])}><Icon size={12} /></ThemeIcon>
                  <Text size="sm" style={{ flex: 1, minWidth: 0 }} truncate>{o.title || o.id}</Text>
                  {/* The project holding it, when the universe serves several. */}
                  <ProjectMark project={o.project} guide="search.project-mark" />
                  <Badge size="xs" variant="outline" color="gray" style={{ textTransform: 'none' }}>{o.type}</Badge>
                </Group>
              </Card>
            )
          })}
        </Stack>
        {error && <Alert color="red" variant="light" icon={<IconAlertTriangle size={16} />}>{error}</Alert>}
        <Group justify="flex-end" gap="sm">
          <Button variant="default" onClick={onClose} disabled={saving}>{t('common.cancel')}</Button>
          <Button onClick={() => void add()} loading={saving} disabled={!kind || !target}>{t('common.add')}</Button>
        </Group>
      </Stack>
    </Modal>
  )
}
