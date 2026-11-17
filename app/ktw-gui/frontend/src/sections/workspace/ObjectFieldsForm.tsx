// SPDX-License-Identifier: Apache-2.0
/**
 * ObjectFieldsForm — one capability's field-group, read and edited in place.
 *
 * A fields tab carries the capability's schema (a data-type URI) and its
 * stored values. Read mode lists them; Edit renders the same schema-driven
 * form the create flow used on nodes (`GetSchemaAST` → `buildUIForms` →
 * `UIFormView`, in-process), seeded with the stored values; Save writes that
 * ONE group through `WorkspaceObjectUpdate`, which merges it into the object —
 * the other groups are never sent, so they cannot be dropped.
 */
import { useEffect, useMemo, useState } from 'react'
import { Alert, Badge, Box, Button, Group, Loader, ScrollArea, Stack, Text, ActionIcon } from '@mantine/core'
import { IconAlertCircle, IconPencil, IconPlayerPlay } from '@tabler/icons-react'
import { GetSchemaAST, WorkspaceObjectUpdate } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { buildUIForms, UIFormView } from '@ktw/display-lib/ui-builders/form/index.ts'
import { initI18n } from '@ktw/display-lib/ui-core/i18n/index.ts'
import { wireRegistries } from '@ktw/display-lib/registry-wire'
import type { Schema } from '@ktw/display-lib/schema'
import { Markdown } from '../../components/Markdown'
import { GuiHostServices } from '../../components/compositions/hostServices'
import { useT } from '../../i18n'
import { errorText } from '../../i18n/errors'
import { ctSchema, useCT } from '../../i18n/contrib'
import { noteRevision, revisionOf } from './objectRevision'
import { DisplayPanel } from '../panels/DisplayPanel'
import { fieldView, type FieldView } from './fieldValue'

// The form layout resolves its labels through i18n and its field renderers
// through the registries: both must be wired before the first form renders.
void initI18n({ language: 'en' })
wireRegistries()

export interface ObjectFieldsFormProps {
  universeName: string
  objectId: string
  tab: main.WorkspaceObjectTab
  /** Called after a save, so the pane reloads the object. */
  onSaved: () => void
  result?: string
  onRunCheck?: () => Promise<void>
  /** The display panel a check run through the action surface reports each
   *  check into, drawn under the check. */
  checkPanel?: string
  /** Drawn as a section inside another view: no scroll area of its own. */
  embedded?: boolean
}

export function ObjectFieldsForm({ universeName, objectId, tab, onSaved, result, onRunCheck, checkPanel, embedded }: ObjectFieldsFormProps) {
  const capability = tab.capabilities?.[0] ?? ''
  const stored = ((tab.values ?? {})[capability] ?? {}) as Record<string, unknown>
  const groups = Object.entries(tab.values ?? {})
  const editable = !!tab.schema && !!capability

  const [editing, setEditing] = useState(false)
  const [fetched, setSchema] = useState<Schema | null>(null)
  const [schemaError, setSchemaError] = useState('')
  const [value, setValue] = useState<Record<string, unknown>>(stored)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const t = useT('workspace')
  const ct = useCT()
  // The schema with its field labels and descriptions in the GUI language
  // (the data-type's translations); re-made when the language changes.
  const schema = useMemo(() => (fetched ? ctSchema(ct, tab.schema, fetched) : null), [fetched, ct, tab.schema])

  // The schema is fetched with the section: read mode needs it too, for each
  // field's label and content type (a markdown field reads rendered).
  useEffect(() => {
    if (!tab.schema || fetched) return
    let cancelled = false
    GetSchemaAST(universeName, tab.schema, '*', '*')
      .then((r) => {
        if (cancelled) return
        if (r.error) setSchemaError(r.error)
        else if (r.ast) setSchema(r.ast as unknown as Schema)
        else setSchemaError(t('fields.noSchema'))
      })
      .catch((e) => { if (!cancelled) setSchemaError(String(e)) })
    return () => { cancelled = true }
  }, [tab.schema, universeName, fetched, t])

  // What the schema says about each field of the group: its label, and
  // whether its content is markdown.
  const fieldInfo = useMemo(() => {
    const out: Record<string, { label?: string; markdown: boolean }> = {}
    const root = schema ? (schema as unknown as { root?: string; types?: Record<string, { fields?: Record<string, { label?: string; content?: unknown }> }> }) : null
    const fields = root?.types?.[root.root ?? '']?.fields ?? {}
    for (const [k, f] of Object.entries(fields)) {
      const c = f?.content
      const name = typeof c === 'string' ? c : (c as { type?: string } | undefined)?.type
      out[k] = { label: f?.label, markdown: name === 'markdown' }
    }
    return out
  }, [schema])

  const form = useMemo(() => {
    if (!schema) return null
    try {
      return buildUIForms({ schema, actionRefs: [], actionMode: 'replace' })['default'] ?? null
    } catch (e) {
      setSchemaError(t('fields.buildFailed', { error: String(e) }))
      return null
    }
  }, [schema, t])

  const startEdit = () => { setValue(stored); setError(''); setEditing(true) }
  const save = async () => {
    setSaving(true)
    setError('')
    try {
      const r = await WorkspaceObjectUpdate(universeName, objectId, JSON.stringify({ [capability]: value }), '', revisionOf(objectId))
      noteRevision(objectId, r.revision)
      if (r.error) { setError(errorText(r)); return }
      setEditing(false)
      onSaved()
    } finally {
      setSaving(false)
    }
  }

  const body = (
        <Stack gap="sm">
          <Group justify="space-between" wrap="nowrap">
            <Group gap="xs">
              {tab.origin === 'object' && (
                <Text size="xs" c="dimmed">{t('fields.carriedByObject')}</Text>
              )}
              {onRunCheck && (
                <>
                  <ActionIcon variant="light" onClick={() => void onRunCheck()} aria-label={t('fields.runCheck')}>
                    <IconPlayerPlay size={14} />
                  </ActionIcon>
                  <Text size="xs">{t('fields.runCheck')}</Text>
                  {result && <Badge size="xs" variant="light">{result}</Badge>}
                </>
              )}
            </Group>
            {editable && !editing && (
              <Button size="xs" variant="light" leftSection={<IconPencil size={14} />} onClick={startEdit}>
                {t('objectTab.edit')}
              </Button>
            )}
          </Group>
          {onRunCheck && checkPanel && (
            <DisplayPanel key={checkPanel} universeName={universeName} panelId={checkPanel} variant="inline" title={t('fields.runCheck')} />
          )}

          {editing ? (
            <Stack gap="sm">
              {schemaError && (
                <Alert icon={<IconAlertCircle size={16} />} color="orange" variant="light" title={t('fields.cannotDraw')}>
                  {schemaError}
                </Alert>
              )}
              {!form && !schemaError && <Loader size="sm" />}
              {form && (
                <GuiHostServices universeName={universeName}>
                  <UIFormView
                    form={form}
                    item={stored}
                    isNew={false}
                    hideTitle
                    onChange={(v) => setValue(v as Record<string, unknown>)}
                  />
                </GuiHostServices>
              )}
              {error && (
                <Alert icon={<IconAlertCircle size={16} />} color="red" variant="light">{error}</Alert>
              )}
              <Group justify="flex-end" gap="sm">
                <Button variant="default" size="xs" onClick={() => setEditing(false)} disabled={saving}>{t('common.cancel')}</Button>
                <Button size="xs" onClick={() => void save()} loading={saving} disabled={!form}>{t('common.save')}</Button>
              </Group>
            </Stack>
          ) : (
            <>
              {groups.length === 0 && (
                <Text size="xs" c="dimmed">
                  {editable ? t('fields.noValuesEditable', { name: tab.name }) : t('fields.noValues', { name: tab.name })}
                </Text>
              )}
              {groups.map(([capID, fields]) => (
                <Stack key={capID} gap={4}>
                  {groups.length > 1 && <Text size="xs" fw={600}>{capID}</Text>}
                  {Object.entries(fields as Record<string, unknown>).map(([k, v]) => {
                    const info = capID === capability ? fieldInfo[k] : undefined
                    return (
                      <Group key={k} gap={6} align="flex-start" wrap="nowrap">
                        <Text size="xs" c="dimmed" style={{ minWidth: 140 }}>{info?.label || k}</Text>
                        {info?.markdown && typeof v === 'string' ? (
                          <Box style={{ minWidth: 0, flex: 1, fontSize: 'var(--mantine-font-size-xs)' }}><Markdown text={v} /></Box>
                        ) : (
                          <Box style={{ minWidth: 0, flex: 1 }}><FieldValueView view={fieldView(v)} /></Box>
                        )}
                      </Group>
                    )
                  })}
                </Stack>
              ))}
            </>
          )}
        </Stack>
  )
  if (embedded) return body
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="md">{body}</Box>
    </ScrollArea>
  )
}

/** A stored value as it reads: text, a list, a record's line, label/value rows - never JSON. */
function FieldValueView({ view }: { view: FieldView }) {
  switch (view.kind) {
    case 'text':
      return <Text size="xs" style={{ whiteSpace: 'pre-wrap' }}>{view.text}</Text>
    case 'record':
      return (
        <Text size="xs">
          {view.title}
          {view.facts.length > 0 && <Text span size="xs" c="dimmed">{' · ' + view.facts.join(' · ')}</Text>}
        </Text>
      )
    case 'list':
      if (view.items.length === 0) return <Text size="xs" c="dimmed">—</Text>
      return (
        <Stack gap={2} component="ul" style={{ margin: 0, paddingLeft: 16 }}>
          {view.items.map((it, i) => (
            <li key={i}><FieldValueView view={it} /></li>
          ))}
        </Stack>
      )
    case 'rows':
      return (
        <Stack gap={2}>
          {view.rows.map((r) => (
            <Group key={r.label} gap={6} align="flex-start" wrap="nowrap">
              <Text size="xs" c="dimmed" style={{ minWidth: 90 }}>{r.label}</Text>
              <Box style={{ minWidth: 0, flex: 1 }}><FieldValueView view={r.value} /></Box>
            </Group>
          ))}
        </Stack>
      )
  }
}
