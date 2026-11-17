// SPDX-License-Identifier: Apache-2.0
// source-set content renderer - the sources an object reads
// from, chosen in any form or action: one picker per contributed source
// connector, the connectors listed by the method the field names, every
// method called through the host's callMethod. Its value is the
// items picked, each with its connector. The library stays generic: it knows
// no connector, only the method URIs it is given. With no host that calls
// methods the form shows the value only.

import { useEffect, useState } from 'react'
import { Badge, Group, Stack, Text } from '@mantine/core'
import { Controller, useFormContext } from 'react-hook-form'
import {
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import type { ArrayType } from '../../../schema/schema-types.ts'
import { useHostServices } from '../../../ui-core/host/index.tsx'
import { Picker } from '../../page/nodes/picker.tsx'
import type { UIPickerItem, UIPickerNode } from '../../page/ui-page.interactive.ts'
import type { SourceConnector, SourceSetContent, SourceSetItem } from './types.ts'

// =================================================================================================
// The value - pure, tested apart from rendering
// =================================================================================================

/** asSourceSet reads a field value as items picked (anything else: none). */
export function asSourceSet(value: unknown): SourceSetItem[] {
  if (!Array.isArray(value)) return []
  return value.filter((v): v is SourceSetItem =>
    !!v && typeof v === 'object' && typeof (v as SourceSetItem).key === 'string' && typeof (v as SourceSetItem).connector === 'string')
}

/** ofConnector is the items of one connector, as its picker holds them. */
export function ofConnector(value: SourceSetItem[], connector: string): UIPickerItem[] {
  return value.filter((v) => v.connector === connector).map(({ connector: _c, ...item }) => item)
}

/** withConnector replaces one connector's items in the value, the others
 *  kept in place. */
export function withConnector(value: SourceSetItem[], connector: string, items: UIPickerItem[]): SourceSetItem[] {
  return [...value.filter((v) => v.connector !== connector), ...items.map((i) => ({ ...i, connector }))]
}

/** connectorLabel is how a group names its connector: the label its
 *  connector declares, its name only when none is known. */
export function connectorLabel(connector: string, connectors?: SourceConnector[] | null): string {
  return connectors?.find((c) => c.name === connector)?.label || connector
}

/** byConnector groups the value for display, in the order connectors appear. */
export function byConnector(value: SourceSetItem[]): Array<{ connector: string; items: SourceSetItem[] }> {
  const out: Array<{ connector: string; items: SourceSetItem[] }> = []
  for (const v of value) {
    const g = out.find((x) => x.connector === v.connector)
    if (g) g.items.push(v)
    else out.push({ connector: v.connector, items: [v] })
  }
  return out
}

// =================================================================================================
// Renderers
// =================================================================================================

function SourceSetList({ value, testId, connectors }: { value: unknown; testId?: string; connectors?: SourceConnector[] | null }) {
  const groups = byConnector(asSourceSet(value))
  if (groups.length === 0) return <Text c="dimmed">—</Text>
  return (
    <Stack gap={4} data-testid={testId}>
      {groups.map((g) => (
        <Group key={g.connector} gap={4}>
          <Text size="xs" c="dimmed">{connectorLabel(g.connector, connectors)}</Text>
          {g.items.map((i) => <Badge key={i.key} size="sm" variant="light">{i.title}</Badge>)}
        </Group>
      ))}
    </Stack>
  )
}

/** useConnectors lists the connectors through the host - the method
 *  `connectors` names, with `object` when given: null while asked or with no
 *  host that calls methods (the source-search node's `pickers` too). */
export function useConnectors(content?: Pick<SourceSetContent, 'connectors' | 'object'> | { connectors?: string; object?: string }): { connectors: SourceConnector[] | null; failed: string; call: boolean } {
  const host = useHostServices()
  const [connectors, setConnectors] = useState<SourceConnector[] | null>(null)
  const [failed, setFailed] = useState('')
  const call = host.callMethod
  useEffect(() => {
    if (!call || !content?.connectors) return
    let live = true
    call(content.connectors, content.object ? { object: content.object } : {})
      .then((out) => { if (live) setConnectors((out as { connectors?: SourceConnector[] } | undefined)?.connectors ?? []) })
      .catch((e) => { if (live) setFailed(String(e)) })
    return () => { live = false }
  }, [call, content?.connectors, content?.object])
  return { connectors, failed, call: !!call }
}

/** SourceSetView is the read view: each connector's items under its declared
 *  label. */
function SourceSetView({ value, testId, content }: { value: unknown; testId?: string; content?: SourceSetContent }) {
  const { connectors } = useConnectors(content)
  return <SourceSetList value={value} testId={testId} connectors={connectors} />
}

/** connectorNode is the picker a connector gets in the field: its search
 *  and its browse where it declares them - a browse-only
 *  connector is browsed, one with neither holds what was proposed. */
export function connectorNode(c: SourceConnector, object?: string): Omit<UIPickerNode, 'kind'> {
  return { id: c.name, search: c.search, browse: c.browse, object, args: c.args, layout: c.layout, badges: c.badges, status: c.status, derived: c.derived }
}

/** SourceSetPickers lists the connectors through the host and draws one
 *  picker per connector, the value shared by all. */
export function SourceSetPickers({ content, value, onChange, disabled }: {
  content: SourceSetContent
  value: SourceSetItem[]
  onChange: (next: SourceSetItem[]) => void
  disabled?: boolean
}) {
  const { connectors, failed, call } = useConnectors(content)
  if (!call) return <SourceSetList value={value} />
  if (failed) return <Text size="sm" c="red">{failed}</Text>
  if (!connectors) return <Text size="sm" c="dimmed">…</Text>
  return (
    <Stack gap="sm">
      {connectors.map((c) => (
        <Stack key={c.name} gap={4} data-guide="source-set.connector" data-guide-name={c.name}>
          <Text size="sm" fw={600}>{c.label ?? c.name}</Text>
          <Picker
            node={connectorNode(c, content.object)}
            selection={ofConnector(value, c.name)}
            onSelection={(items) => onChange(withConnector(value, c.name, items))}
            disabled={disabled}
          />
        </Stack>
      ))}
    </Stack>
  )
}

const view: ViewFieldRendererFactory<ArrayType, SourceSetContent> = ({ info }) => {
  return (item: JsonObject) => <SourceSetView value={item[info.name]} testId={'view.' + info.name} content={info.contentType} />
}

const form: FormFieldRendererFactory<ArrayType, SourceSetContent> = ({ def, info }) => {
  const required = def.required ?? !info.type.optional
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    return (
      <Controller
        key={info.id}
        name={info.name}
        control={control}
        rules={required ? { validate: (v) => asSourceSet(v).length > 0 || `${info.label} is required` } : undefined}
        render={({ field, fieldState }) => (
          <Stack gap={4} data-testid={'input.' + info.name}>
            <Text size="sm" fw={500}>{info.label}</Text>
            {info.description && <Text size="xs" c="dimmed">{info.description}</Text>}
            {info.contentType.note && <Text size="xs" c="dimmed" data-guide="source-set.note">{info.contentType.note}</Text>}
            <SourceSetPickers
              content={info.contentType}
              value={asSourceSet(field.value)}
              onChange={(next) => field.onChange(next)}
              disabled={def.disabled === true || isSubmitting}
            />
            {fieldState.error?.message && <Text size="xs" c="red">{fieldState.error.message}</Text>}
          </Stack>
        )}
      />
    )
  }
}

const column: ColumnRendererFactory<ArrayType, SourceSetContent> = ({ info }) => {
  return (item: JsonObject) => <Text size="sm">{asSourceSet(item[info.name]).map((i) => i.title).join(', ')}</Text>
}

const SourceSetRenderer: FieldRendererFactory<ArrayType, SourceSetContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default SourceSetRenderer
