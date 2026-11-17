// SPDX-License-Identifier: Apache-2.0
// cod content renderer — a READ-ONLY, computed Cost of Delay (method-core).
//
// CoD = business_value + time_criticality + risk_opportunity. The
// prioritization signal that orders the backlog (replaces WSJF — no
// job-size divisor; effort is the DERIVED count of unfinished Use Cases,
// not a stored field). The renderer reads the three sibling primitive
// fields (names configurable via the content params) and computes live:
//   - form   → useWatch the three siblings → updates as the user edits them.
//   - view / column → reads them straight off the item.
//
// The editable inputs are the sibling `fibonacci` fields; this field only
// displays the result, so it binds no form control.

import { Badge, Group, Stack, Text, Tooltip } from '@mantine/core'
import { useFormContext, useWatch } from 'react-hook-form'
import {
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import type { NumberType } from '../../../schema/schema-types.ts'
import type { CodContent } from './types.ts'

interface Fields { value: string; criticality: string; risk: string }

function fieldsOf(c: CodContent): Fields {
  return {
    value:       c.value       ?? 'business_value',
    criticality: c.criticality ?? 'time_criticality',
    risk:        c.risk        ?? 'risk_opportunity',
  }
}

function num(v: unknown): number {
  const n = typeof v === 'number' ? v : typeof v === 'string' ? Number(v) : NaN
  return Number.isFinite(n) ? n : 0
}

interface Computed { bv: number; tc: number; ro: number; cod: number }

function compute(src: Record<string, unknown>, f: Fields): Computed {
  const bv = num(src[f.value]), tc = num(src[f.criticality]), ro = num(src[f.risk])
  return { bv, tc, ro, cod: bv + tc + ro }
}

function colorFor(cod: number): string {
  return cod >= 24 ? 'red' : cod >= 13 ? 'orange' : cod >= 6 ? 'blue' : 'gray'
}

function Display({ c, label, big }: { c: Computed; label?: string; big?: boolean }) {
  const badge =
    c.cod === 0 ? (
      <Tooltip label="Set value / criticality / risk to compute Cost of Delay" withArrow>
        <Badge size={big ? 'lg' : 'md'} variant="light" color="gray">—</Badge>
      </Tooltip>
    ) : (
      <Tooltip label={`Cost of Delay ${c.cod} (${c.bv}+${c.tc}+${c.ro})`} withArrow>
        <Badge size={big ? 'lg' : 'md'} variant="filled" color={colorFor(c.cod)}>
          {c.cod}
        </Badge>
      </Tooltip>
    )
  if (!big) return badge
  return (
    <Stack gap={2}>
      {label && <Text size="sm" fw={500}>{label}</Text>}
      <Group gap="xs" align="center">
        {badge}
        <Text size="xs" c="dimmed">CoD = {c.bv}+{c.tc}+{c.ro}</Text>
      </Group>
    </Stack>
  )
}

const view: ViewFieldRendererFactory<NumberType, CodContent> = ({ info }) => {
  const f = fieldsOf(info.contentType)
  return (item: JsonObject) => <Display c={compute(item, f)} />
}

const form: FormFieldRendererFactory<NumberType, CodContent> = ({ info }) => {
  const f = fieldsOf(info.contentType)
  return () => {
    // Live recompute as the sibling primitives change.
    const { control } = useFormContext()
    const watched = useWatch({ control, name: [f.value, f.criticality, f.risk] }) as unknown[]
    const src: Record<string, unknown> = {
      [f.value]: watched?.[0], [f.criticality]: watched?.[1], [f.risk]: watched?.[2],
    }
    return <Display c={compute(src, f)} label={info.label} big />
  }
}

const column: ColumnRendererFactory<NumberType, CodContent> = ({ info }) => {
  const f = fieldsOf(info.contentType)
  return (item: JsonObject) => <Display c={compute(item, f)} />
}

const CodRenderer: FieldRendererFactory<NumberType, CodContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default CodRenderer
