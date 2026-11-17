// SPDX-License-Identifier: Apache-2.0
// A create page's controls: a set of choice cards - one
// chosen at a time, a radio group, setting a key of the page selection - and a
// labelled text field setting another, prefilled from the data and drawn only
// while the selection holds what its `when` names. A button's `requires`
// (interactive.tsx) keeps the page's primary action disabled until they are
// set.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, type ReactNode } from 'react'
import { Box, Select, Text, Textarea, TextInput, Tooltip } from '@mantine/core'
import { IconPointFilled } from '@tabler/icons-react'

import { tablerIcon } from '../../../ui-core/components/icons/index.ts'
import { useSelection } from '../page-context.tsx'
import type { UIChoiceCard, UIChoiceCardsNode, UITextFieldNode } from '../ui-page.interactive.ts'
import { monogramLetters } from './marks.ts'

function text(v: unknown): string {
  return v === undefined || v === null ? '' : typeof v === 'object' ? JSON.stringify(v) : String(v)
}

/** isSet: a selection value holds something - not absent, null, false nor blank. */
export function isSet(v: unknown): boolean {
  return v !== undefined && v !== null && v !== false && text(v).trim() !== ''
}

/** shownWhen says whether a `when` holds against the selection: `key=value`
 *  the key holds that value, `key` the key is set; none always holds. */
export function shownWhen(when: string | undefined, selection: Record<string, unknown>): boolean {
  if (!when) return true
  const eq = when.indexOf('=')
  if (eq < 0) return isSet(selection[when.trim()])
  return text(selection[when.slice(0, eq).trim()]) === when.slice(eq + 1).trim()
}

/** choiceSets are the keys a chosen card sets beside its own: each of its
 *  `sets` the person has not changed - unset, or still what the card chosen
 *  before (`previous`) set. */
export function choiceSets(o: UIChoiceCard, previous: UIChoiceCard | undefined, selection: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(o.sets ?? {})) {
    const cur = selection[k]
    if (!isSet(cur) || text(cur) === (previous?.sets?.[k] ?? '')) out[k] = v
  }
  return out
}

/** missingKeys are the keys of `requires` the selection does not set. */
export function missingKeys(requires: string[] | undefined, selection: Record<string, unknown>): string[] {
  return (requires ?? []).filter((k) => !isSet(selection[k]))
}

function Badge({ o }: { o: UIChoiceCard }) {
  const Icon = o.icon ? tablerIcon(o.icon, IconPointFilled) : undefined
  return (
    <Box bg={o.color ?? 'gray.6'} c="white" ff="monospace" fw={600} aria-hidden data-choice-badge={o.badge ?? ''}
      style={{ width: 32, height: 32, borderRadius: 8, display: 'inline-grid', placeItems: 'center', fontSize: 14, flex: 'none' }}>
      {Icon ? <Icon size={18} /> : (o.badge ?? monogramLetters(o.label ?? o.value))}
    </Box>
  )
}

export function ChoiceCardsRenderer({ node }: { node: UIChoiceCardsNode }): ReactNode {
  const { selection, select } = useSelection()
  const current = text(selection[node.key])
  const options = node.options ?? []
  return (
    <div {...dataAttrs(node)} role="radiogroup" aria-label={node.label ?? node.key} data-ktw-choice={node.key}
      data-element={`choice:${node.key}`}
      style={{ display: 'grid', gridTemplateColumns: `repeat(${node.cols ?? 2}, minmax(0, 1fr))`, gap: 10 }}>
      {options.map((o) => {
        const on = !o.disabled && current === o.value
        const card = (
          <button key={o.value} type="button" role="radio" aria-checked={on} aria-disabled={o.disabled || undefined}
            disabled={o.disabled} data-value={o.value} data-guide="choice-cards.option" data-guide-name={o.value}
            data-element={`choice:${node.key}:${o.value}`}
            onClick={() => {
              if (o.disabled) return
              const previous = options.find((p) => p.value === current)
              for (const [k, v] of Object.entries(choiceSets(o, previous, selection))) select(k, v)
              select(node.key, o.value)
            }}
            style={{
              font: 'inherit', textAlign: 'left', display: 'flex', gap: 10, alignItems: 'center', padding: '10px 12px',
              borderRadius: 8, cursor: o.disabled ? 'not-allowed' : 'pointer', opacity: o.disabled ? 0.55 : 1,
              border: `${on ? 2 : 1}px solid ${on ? 'var(--mantine-color-blue-filled)' : 'var(--mantine-color-default-border)'}`,
              background: on ? 'var(--mantine-color-blue-light)' : 'var(--mantine-color-body)', color: 'inherit',
            }}>
            <Badge o={o} />
            <span style={{ display: 'flex', flexDirection: 'column', minWidth: 0, flex: 1 }}>
              <span style={{ display: 'flex', gap: 6, alignItems: 'center', fontWeight: 600, fontSize: 14 }}>
                {o.label ?? o.value}
                {o.disabled && o.pill && (
                  <span data-choice-pill style={{ fontSize: 11, fontWeight: 500, padding: '1px 8px', borderRadius: 999,
                    background: 'var(--mantine-color-gray-light)', color: 'var(--mantine-color-dimmed)' }}>{o.pill}</span>
                )}
              </span>
              {o.description && <Text size="xs" c="dimmed" lineClamp={2}>{o.description}</Text>}
            </span>
          </button>
        )
        return o.tooltip ? <Tooltip key={o.value} label={o.tooltip} withArrow multiline w={260}><span style={{ display: 'grid' }}>{card}</span></Tooltip> : card
      })}
    </div>
  )
}

export function TextFieldRenderer({ node }: { node: UITextFieldNode }): ReactNode {
  const { selection, select } = useSelection()
  const v = selection[node.key]
  // Its starting value is the key's while the selection holds none.
  useEffect(() => {
    if (v === undefined && node.value) select(node.key, node.value)
  }, [node.key, node.value]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!shownWhen(node.when, selection)) return null
  // A field with its values given is a drop-down of them.
  if (node.options?.length) {
    return (
      <Select {...dataAttrs(node)} label={node.label} placeholder={node.placeholder} description={node.description}
        withAsterisk={node.required} value={v === undefined ? (node.value ?? null) : text(v) || null} data-ktw-field={node.key}
        data-element={`field:${node.key}`} allowDeselect={false}
        data={node.options.map((o) => ({ value: o.value, label: o.label || o.value }))}
        onChange={(x) => select(node.key, x ?? '')} />
    )
  }
  // A field given rows is multi-line: a text area growing with its text.
  if (node.rows) {
    return (
      <Textarea {...dataAttrs(node)} label={node.label} placeholder={node.placeholder} description={node.description}
        withAsterisk={node.required} value={v === undefined ? (node.value ?? '') : text(v)} data-ktw-field={node.key}
        data-element={`field:${node.key}`} autosize minRows={node.rows}
        onChange={(e) => select(node.key, e.currentTarget.value)} />
    )
  }
  return (
    <TextInput {...dataAttrs(node)} label={node.label} placeholder={node.placeholder} description={node.description}
      withAsterisk={node.required} value={v === undefined ? (node.value ?? '') : text(v)} data-ktw-field={node.key}
      data-element={`field:${node.key}`}
      onChange={(e) => select(node.key, e.currentTarget.value)} />
  )
}
