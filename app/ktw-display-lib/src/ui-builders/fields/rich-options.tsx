// SPDX-License-Identifier: Apache-2.0
// Rich enum-option helpers — shared by the single-value selects (`text`,
// `number`, `boolean`) and the multi-select (`enum-multi`).
//
// A pickable field can carry options on its content-type, in either the rich
// LIST form `options: [{ value, label, description?, icon? }]` (`icon` = a
// Tabler icon name) or the compact MAP form `options: { value: label }`. They
// reach the renderer via `info.contentType.options` once the schema-query
// bridge folds the Go `contentParams` back into the content envelope.
//
// Options are GENERIC over the value kind: a string field carries string-valued
// options, a number field number-valued, a boolean field boolean-valued. The
// Mantine Combobox only speaks strings, so `toSelectData` stringifies at the
// boundary and `coerceFromSelect` maps the picked string back to the original
// typed value. When no options are declared, callers fall back to the bare
// `enum` keys (value === label) — so existing plain enums are unchanged.

import { Group, Stack, Text } from '@mantine/core'
import type { ComboboxItem } from '@mantine/core'
import { Icon } from '../../ui-core/components/icons/Icon.tsx'
import type {
  ContentType,
  FieldOption,
  FieldOptionKey,
  FieldOptionList,
} from '../../schema/schema-contents.ts'
import { SchemaQuery } from '../../schema/schema-query.ts'

/**
 * Resolve the option list for a select/multi-select field. Rich (or map)
 * options declared on the content-type win; otherwise fall back to the bare
 * enum keys (value === label). Returns `[]` when neither is present (→ free
 * text / tags). The map form is normalized to a list by `getContentTypeOptions`.
 */
export function resolveFieldOptions<T extends FieldOptionKey = FieldOptionKey>(
  contentType: ContentType | undefined,
  fallbackKeys: T[],
): FieldOptionList<T> {
  const rich = SchemaQuery.getContentTypeOptions<T>(contentType).optionList
  if (rich && rich.length > 0) return rich
  return fallbackKeys.map((v) => ({ value: v, label: String(v) }))
}

/** Mantine `data` shape — value stringified at the Combobox boundary. */
export function toSelectData<T extends FieldOptionKey>(options: FieldOptionList<T>): ComboboxItem[] {
  return options.map((o) => ({ value: String(o.value), label: o.label }))
}

/** Map a picked Combobox string back to the option's original typed value
 *  (string / number / boolean). Returns `null` for an unknown / empty pick. */
export function coerceFromSelect<T extends FieldOptionKey>(
  options: FieldOptionList<T>,
  picked: string | null,
): T | null {
  if (picked == null) return null
  const found = options.find((o) => String(o.value) === picked)
  return found ? found.value : null
}

/** The option a select holds (`value` matched as a string), undefined
 *  when it holds none or one not listed. */
export function pickedOption<T extends FieldOptionKey>(options: FieldOptionList<T>, value: unknown): FieldOption<T> | undefined {
  if (value === undefined || value === null || value === '') return undefined
  return options.find((o) => String(o.value) === String(value))
}

/** True when any option carries a description → a select shows the picked
 *  one's under it. */
export function hasDescriptions<T extends FieldOptionKey>(options: FieldOptionList<T>): boolean {
  return options.some((o) => Boolean(o.description))
}

/** True when any option carries an icon or description → use the rich row. */
export function hasRichDetail<T extends FieldOptionKey>(options: FieldOptionList<T>): boolean {
  return options.some((o) => Boolean(o.icon) || Boolean(o.description))
}

/** The icon + label + description sub-text row, shared by the dropdown
 *  `renderOption` and the checkbox label. */
export function RichOptionRow({
  label,
  description,
  icon,
  iconSize = 20,
}: {
  label: string
  description?: string
  icon?: string
  iconSize?: number
}) {
  return (
    <Group gap="sm" wrap="nowrap">
      {icon ? <Icon icon={icon} size={iconSize} /> : null}
      <Stack gap={0}>
        <Text size="sm">{label}</Text>
        {description ? (
          <Text size="xs" c="dimmed">
            {description}
          </Text>
        ) : null}
      </Stack>
    </Group>
  )
}

/**
 * Build a Mantine `renderOption` that shows the icon + label + description for
 * each option. The Combobox passes `{ option: { value, label } }`; we look the
 * full FieldOption up by stringified value to recover the icon + description.
 */
export function makeRenderOption<T extends FieldOptionKey>(options: FieldOptionList<T>) {
  const byValue = new Map<string, FieldOption<T>>(options.map((o) => [String(o.value), o]))
  return ({ option }: { option: ComboboxItem }) => {
    const o = byValue.get(option.value)
    return <RichOptionRow label={option.label} description={o?.description} icon={o?.icon} />
  }
}
