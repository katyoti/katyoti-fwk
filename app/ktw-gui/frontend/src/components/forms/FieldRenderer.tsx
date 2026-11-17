// SPDX-License-Identifier: Apache-2.0
/**
 * FieldRenderer — renders a single form field based on JSON Schema type.
 * Maps Go formdef types → Mantine components.
 */
import { useState } from 'react'
import {
  ActionIcon,
  Badge,
  Checkbox,
  Group,
  NumberInput,
  Paper,
  Radio,
  ScrollArea,
  Select,
  Stack,
  Table,
  Tabs,
  Text,
  TextInput,
  Textarea,
} from '@mantine/core'
import { DateInput, TimeInput } from '@mantine/dates'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import { Markdown } from '../Markdown'
import dayjs from 'dayjs'
import type { JsonSchemaProperty } from './types'
import { useT } from '../../i18n'

interface Props {
  path: string
  schema: JsonSchemaProperty
  uiOptions?: Record<string, any>
  label: string
  required: boolean
  value: any
  onChange: (value: any) => void
}

export function FieldRenderer({ path, schema, uiOptions, label, required, value, onChange }: Props) {
  const format = uiOptions?.format || schema.format
  const isMulti = uiOptions?.multi === true
  const readOnly = schema.readOnly === true

  // Enum (oneOf) → radio or select
  if (schema.oneOf && schema.oneOf.length > 0) {
    if (format === 'radio' || uiOptions?.format === 'radio') {
      return <RadioField schema={schema} uiOptions={uiOptions} label={label} required={required} value={value} onChange={onChange} />
    }
    return <SelectField schema={schema} label={label} required={required} value={value} onChange={onChange} readOnly={readOnly} />
  }

  // By format
  if (format === 'markdown') {
    return <MarkdownField label={label} required={required} value={value} onChange={onChange} readOnly={readOnly} />
  }
  if (format === 'date' || schema.format === 'date') {
    return <DateField label={label} required={required} value={value} onChange={onChange} readOnly={readOnly} />
  }
  if (format === 'time' || schema.format === 'time') {
    return <TimeField label={label} required={required} value={value} onChange={onChange} readOnly={readOnly} />
  }
  if (format === 'date-time' || schema.format === 'date-time') {
    return <DateTimeField label={label} required={required} value={value} onChange={onChange} readOnly={readOnly} />
  }
  if (format === 'table' && schema.type === 'array') {
    return <TableField schema={schema} label={label} value={value} onChange={onChange} readOnly={readOnly} />
  }

  // By type
  switch (schema.type) {
    case 'string':
      if (isMulti) {
        return (
          <Textarea
            label={label}
            required={required}
            value={value ?? ''}
            onChange={(e) => onChange(e.currentTarget.value || undefined)}
            placeholder={schema.description}
            minRows={4}
            maxRows={12}
            autosize
            readOnly={readOnly}
          />
        )
      }
      return (
        <TextInput
          label={label}
          required={required}
          value={value ?? ''}
          onChange={(e) => onChange(e.currentTarget.value || undefined)}
          placeholder={schema.description}
          readOnly={readOnly}
          maxLength={schema.maxLength}
        />
      )

    case 'integer':
    case 'number':
      return (
        <NumberInput
          label={label}
          required={required}
          value={value ?? ''}
          onChange={(v) => onChange(v === '' ? undefined : v)}
          placeholder={schema.description}
          min={schema.minimum}
          max={schema.maximum}
          readOnly={readOnly}
          allowDecimal={schema.type === 'number'}
        />
      )

    case 'boolean':
      return (
        <Checkbox
          label={label}
          checked={!!value}
          onChange={(e) => onChange(e.currentTarget.checked)}
          disabled={readOnly}
        />
      )

    case 'array':
      return <ArrayField schema={schema} label={label} value={value} onChange={onChange} readOnly={readOnly} />

    default:
      return <TextInput label={`${label} (${schema.type})`} value={String(value ?? '')} onChange={(e) => onChange(e.currentTarget.value)} />
  }
}

// ── Select (enum dropdown) ───────────────────────────────────────────────

function SelectField({ schema, label, required, value, onChange, readOnly }: {
  schema: JsonSchemaProperty; label: string; required: boolean; value: any; onChange: (v: any) => void; readOnly: boolean
}) {
  const data = (schema.oneOf || []).map(o => ({ value: o.const, label: o.title || o.const }))
  return (
    <Select
      label={label}
      required={required}
      value={value ?? null}
      onChange={(v) => onChange(v)}
      data={data}
      placeholder={schema.description || 'Select...'}
      readOnly={readOnly}
      clearable
    />
  )
}

// ── Radio with descriptions ──────────────────────────────────────────────

function RadioField({ schema, uiOptions, label, required, value, onChange }: {
  schema: JsonSchemaProperty; uiOptions?: Record<string, any>; label: string; required: boolean; value: any; onChange: (v: any) => void
}) {
  const optionDetails: Array<{ value: string; label: string; description?: string }> = uiOptions?.optionDetails || []
  const items = optionDetails.length > 0
    ? optionDetails
    : (schema.oneOf || []).map(o => ({ value: o.const, label: o.title || o.const, description: undefined as string | undefined }))

  return (
    <Radio.Group label={label} required={required} value={value ?? ''} onChange={onChange}>
      <Stack gap={6} mt={4}>
        {items.map((item) => {
          const selected = value === item.value
          return (
            <Paper
              key={item.value}
              withBorder
              p="xs"
              radius="sm"
              style={{
                cursor: 'pointer',
                borderColor: selected ? 'var(--mantine-color-blue-6)' : undefined,
                borderWidth: selected ? 2 : 1,
                background: selected ? 'var(--mantine-color-blue-light)' : undefined,
              }}
              onClick={() => onChange(item.value)}
            >
              <Group gap="xs" wrap="nowrap">
                <Radio value={item.value} size="xs" />
                <div>
                  <Text size="sm" fw={500}>{item.label}</Text>
                  {item.description && <Text size="xs" c="dimmed">{item.description}</Text>}
                </div>
              </Group>
            </Paper>
          )
        })}
      </Stack>
    </Radio.Group>
  )
}

// ── Markdown editor/preview ──────────────────────────────────────────────

function MarkdownField({ label, required, value, onChange, readOnly }: {
  label: string; required: boolean; value: any; onChange: (v: any) => void; readOnly: boolean
}) {
  const t = useT('components')
  const [tab, setTab] = useState<string>('edit')
  return (
    <Stack gap={4}>
      <Text size="sm" fw={500}>{label}{required && ' *'}</Text>
      <Tabs value={tab} onChange={(v) => setTab(v || 'edit')}>
        <Tabs.List>
          <Tabs.Tab value="edit">{t('fieldRenderer.edit')}</Tabs.Tab>
          <Tabs.Tab value="preview">{t('fieldRenderer.preview')}</Tabs.Tab>
        </Tabs.List>
        <Tabs.Panel value="edit" pt="xs">
          <Textarea
            value={value ?? ''}
            onChange={(e) => onChange(e.currentTarget.value || undefined)}
            minRows={6}
            maxRows={20}
            autosize
            readOnly={readOnly}
            styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 13 } }}
          />
        </Tabs.Panel>
        <Tabs.Panel value="preview" pt="xs">
          <Paper withBorder p="sm" radius="sm" mih={120}>
            {value ? (
              <Markdown text={value} />
            ) : (
              <Text size="sm" c="dimmed" fs="italic">{t('fieldRenderer.nothingToPreview')}</Text>
            )}
          </Paper>
        </Tabs.Panel>
      </Tabs>
    </Stack>
  )
}

// ── Date / Time / DateTime ───────────────────────────────────────────────

function DateField({ label, required, value, onChange, readOnly }: {
  label: string; required: boolean; value: any; onChange: (v: any) => void; readOnly: boolean
}) {
  const dateValue = value ? new Date(value) : null
  return (
    <DateInput
      label={label}
      required={required}
      value={dateValue}
      onChange={(d) => onChange(d ? dayjs(d).format('YYYY-MM-DD') : undefined)}
      readOnly={readOnly}
      clearable
    />
  )
}

function TimeField({ label, required, value, onChange, readOnly }: {
  label: string; required: boolean; value: any; onChange: (v: any) => void; readOnly: boolean
}) {
  return (
    <TimeInput
      label={label}
      required={required}
      value={value ?? ''}
      onChange={(e) => onChange(e.currentTarget.value || undefined)}
      readOnly={readOnly}
    />
  )
}

function DateTimeField({ label, required, value, onChange, readOnly }: {
  label: string; required: boolean; value: any; onChange: (v: any) => void; readOnly: boolean
}) {
  const t = useT('components')
  // Mantine doesn't have a combined DateTimePicker in core — use DateInput + TimeInput
  const dt = value ? dayjs(value) : null
  return (
    <Group gap="sm" grow>
      <DateInput
        label={label}
        required={required}
        value={dt?.toDate() ?? null}
        onChange={(d) => {
          if (!d) { onChange(undefined); return }
          const time = dt ? dt.format('HH:mm:ss') : '00:00:00'
          onChange(dayjs(d).format('YYYY-MM-DD') + 'T' + time)
        }}
        readOnly={readOnly}
        clearable
      />
      <TimeInput
        label={t('fieldRenderer.time')}
        value={dt?.format('HH:mm') ?? ''}
        onChange={(e) => {
          const date = dt ? dt.format('YYYY-MM-DD') : dayjs().format('YYYY-MM-DD')
          onChange(date + 'T' + (e.currentTarget.value || '00:00') + ':00')
        }}
        readOnly={readOnly}
      />
    </Group>
  )
}

// ── Table (array of objects) ─────────────────────────────────────────────

function TableField({ schema, label, value, onChange, readOnly }: {
  schema: JsonSchemaProperty; label: string; value: any; onChange: (v: any) => void; readOnly: boolean
}) {
  const rows: Record<string, any>[] = Array.isArray(value) ? value : []
  const itemProps = schema.items?.properties || {}
  const columns = Object.keys(itemProps)

  const updateCell = (rowIdx: number, col: string, val: string) => {
    const newRows = rows.map((row, i) => i === rowIdx ? { ...row, [col]: val } : row)
    onChange(newRows)
  }
  const addRow = () => {
    const empty: Record<string, any> = {}
    columns.forEach(c => empty[c] = '')
    onChange([...rows, empty])
  }
  const removeRow = (idx: number) => onChange(rows.filter((_, i) => i !== idx))

  return (
    <Stack gap={4}>
      <Text size="sm" fw={500}>{label}</Text>
      <ScrollArea>
        <Table striped highlightOnHover withTableBorder withColumnBorders>
          <Table.Thead>
            <Table.Tr>
              {columns.map(col => (
                <Table.Th key={col} style={{ fontSize: 12 }}>{itemProps[col]?.title || col}</Table.Th>
              ))}
              {!readOnly && <Table.Th w={40} />}
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {rows.map((row, ri) => (
              <Table.Tr key={ri}>
                {columns.map(col => (
                  <Table.Td key={col} p={4}>
                    <TextInput
                      size="xs"
                      value={row[col] ?? ''}
                      onChange={(e) => updateCell(ri, col, e.currentTarget.value)}
                      readOnly={readOnly}
                      variant="unstyled"
                    />
                  </Table.Td>
                ))}
                {!readOnly && (
                  <Table.Td p={4}>
                    <ActionIcon size="xs" variant="subtle" color="red" onClick={() => removeRow(ri)}>
                      <IconTrash size={12} />
                    </ActionIcon>
                  </Table.Td>
                )}
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </ScrollArea>
      {!readOnly && (
        <ActionIcon size="sm" variant="light" onClick={addRow}>
          <IconPlus size={14} />
        </ActionIcon>
      )}
    </Stack>
  )
}

// ── Array (generic) ──────────────────────────────────────────────────────

function ArrayField({ schema, label, value, onChange, readOnly }: {
  schema: JsonSchemaProperty; label: string; value: any; onChange: (v: any) => void; readOnly: boolean
}) {
  const items: any[] = Array.isArray(value) ? value : []
  return (
    <Stack gap={4}>
      <Text size="sm" fw={500}>{label}</Text>
      {items.map((item, i) => (
        <Group key={i} gap="xs">
          <TextInput
            size="xs"
            value={String(item ?? '')}
            onChange={(e) => {
              const next = [...items]
              next[i] = e.currentTarget.value
              onChange(next)
            }}
            readOnly={readOnly}
            style={{ flex: 1 }}
          />
          {!readOnly && (
            <ActionIcon size="xs" variant="subtle" color="red" onClick={() => onChange(items.filter((_, j) => j !== i))}>
              <IconTrash size={12} />
            </ActionIcon>
          )}
        </Group>
      ))}
      {!readOnly && (
        <ActionIcon size="sm" variant="light" onClick={() => onChange([...items, ''])}>
          <IconPlus size={14} />
        </ActionIcon>
      )}
    </Stack>
  )
}
