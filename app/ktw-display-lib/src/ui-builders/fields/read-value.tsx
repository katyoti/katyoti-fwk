// SPDX-License-Identifier: Apache-2.0
// How a closed read row (ui-form `edit: row`) draws its value when the
// field declares `read` (UIFieldDef.read): a worded template around the
// value, a tone pill with a muted note after it, a brand mark before it, a
// muted lead before it all. The editor behind the pencil stays the field's
// own; a field without `read` reads through its view renderer as before.

import { Badge, Box, Group, Text } from '@mantine/core'
import type { ReactNode } from 'react'

import { monogramLetters } from '../page/nodes/marks.ts'
import { valueAt } from './editing.ts'
import type { UIFieldRead, UIViewFieldRenderer } from './types.ts'

const empty = (v: unknown): boolean => v === undefined || v === null || v === ''

/** The value as a text: an enum value by its option label. */
function asText(v: unknown, options: Record<string, string> | undefined): string {
  if (empty(v)) return ''
  if (typeof v === 'object') return ''
  const s = String(v)
  return options?.[s] ?? s
}

/** relativeTime is a date said from now in the language given (« il y a
 *  5 min »), undefined for a value that is not a date. */
export function relativeTime(value: unknown, language = 'en', now: Date = new Date()): string | undefined {
  if (empty(value) || (typeof value !== 'string' && typeof value !== 'number' && !(value instanceof Date))) return undefined
  const at = new Date(value as string | number | Date)
  if (Number.isNaN(at.getTime())) return undefined
  const s = Math.round((at.getTime() - now.getTime()) / 1000)
  const units: [Intl.RelativeTimeFormatUnit, number][] = [['year', 31536000], ['month', 2592000], ['week', 604800], ['day', 86400], ['hour', 3600], ['minute', 60]]
  const rtf = new Intl.RelativeTimeFormat(language, { numeric: 'auto', style: 'short' })
  for (const [unit, size] of units) {
    if (Math.abs(s) >= size) return rtf.format(Math.round(s / size), unit)
  }
  return rtf.format(s, 'second')
}

/** readView wraps a read row's view renderer with the field's `read`
 *  drawing; the plain view stays for an empty value. */
export function readView<T>(read: UIFieldRead, view: UIViewFieldRenderer<T>, name: string, id: string,
  options: Record<string, string> | undefined, language: string | undefined): UIViewFieldRenderer<T> {
  return (item) => {
    const value = name.includes('.') ? valueAt(item, name) : (item as Record<string, unknown> | undefined)?.[name]
    let core: ReactNode
    if (read.pill) {
      const word = asText(read.pill.path ? valueAt(value, read.pill.path) : value, options)
      if (!word) return view(item)
      const tone = read.pill.tones?.[word] ?? 'gray'
      core = <Badge size="sm" variant="light" color={tone} tt="none" data-testid={`${id}.pill`} data-tone={tone}>{word}</Badge>
    } else if (read.template) {
      const text = asText(value, options)
      if (!text) return view(item)
      core = <Text size="sm" component="span">{read.template.split('{value}').join(text)}</Text>
    } else {
      if (empty(value)) return view(item)
      core = view(item)
    }
    const after = read.after ? afterText(read.after, value, language) : undefined
    const mark = read.mark ? markOf(read.mark, asText(value, options), id) : null
    return (
      <Group gap={6} wrap="wrap" align="center" data-testid={`${id}.read`}>
        {read.lead ? <Text size="sm" c="dimmed" component="span" data-testid={`${id}.lead`}>{read.lead}</Text> : null}
        {mark}
        <Box component="span" style={{ minWidth: 0 }}>{core}</Box>
        {after ? <Text size="sm" c="dimmed" component="span" title={after.title} data-testid={`${id}.after`}>{after.text}</Text> : null}
      </Group>
    )
  }
}

/** The muted note after the value: a part of it, said as a relative time
 *  (the absolute date its tooltip) or as it is. */
function afterText(after: NonNullable<UIFieldRead['after']>, value: unknown, language: string | undefined): { text: string; title?: string } | undefined {
  const v = valueAt(value, after.path)
  if (empty(v) || typeof v === 'object') return undefined
  if (after.as === 'relative-time') {
    const text = relativeTime(v, language)
    if (!text) return { text: String(v) }
    return { text: `${after.prefix ? after.prefix + ' ' : ''}${text}`, title: new Date(v as string).toLocaleString(language) }
  }
  return { text: `${after.prefix ? after.prefix + ' ' : ''}${String(v)}` }
}

/** The brand mark before the value: its letters on its colour, 22px. */
function markOf(mark: NonNullable<UIFieldRead['mark']>, valueText: string, id: string): ReactNode {
  const letters = mark.letters || monogramLetters(mark.text || valueText)
  if (!letters) return null
  return (
    <Box component="span" data-testid={`${id}.mark`} aria-hidden
      style={{ display: 'inline-grid', placeItems: 'center', width: 22, height: 22, borderRadius: 5, fontSize: 10, fontWeight: 700,
        color: 'white', background: mark.color || 'var(--mantine-color-gray-6)' }}>
      {letters}
    </Box>
  )
}
