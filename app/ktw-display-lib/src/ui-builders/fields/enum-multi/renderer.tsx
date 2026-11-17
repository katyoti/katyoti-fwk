// SPDX-License-Identifier: Apache-2.0
// enum-multi content renderer — Mantine MultiSelect / TagsInput.
//
// Three modes; the renderer picks based on whether the array's items
// have an enum constraint and on the explicit `creatable` flag:
//
//   1. Bounded enum  — items have `enum: [...]` → Mantine MultiSelect
//                      (pick from a fixed list, no free entry).
//   2. Bounded but creatable — items have `enum: [...]` AND
//                      `creatable: true` on the content-type → still
//                      MultiSelect, but with `searchable` + a
//                      "+ Create '<value>'" affordance.
//   3. Free-entry tags — items have no enum → Mantine TagsInput
//                      (every keystroke + Enter becomes a tag; ideal
//                      for free-form labels like Dossier's parent_da).
//
// View + column paths share the same renderer: an empty array shows
// "—"; populated arrays render as a comma-separated value list (a
// chip-list view variant ships separately in US-S5 Tier 1).

import { Badge, Checkbox, CheckIcon, Group, Input, MultiSelect, SimpleGrid, Stack, TagsInput, Text } from '@mantine/core'
import { Icon } from '../../../ui-core/components/icons/Icon.tsx'
import { Controller, useFormContext } from 'react-hook-form'
import {
  columnStyles,
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import type { ArrayType } from '../../../schema/schema-types.ts'
import {
  hasRichDetail,
  makeRenderOption,
  resolveFieldOptions,
  RichOptionRow,
  toSelectData,
} from '../rich-options.tsx'
import type { EnumMultiContent } from './types.ts'

/** Pull the enum constraint off the array's ITEM type. `buildInfo` already
 *  unwraps the array and resolves any named/`$ref` item, so `info.type` IS the
 *  item type — the enum sits directly on it (`info.type.enum`), NOT under
 *  `.items`. (Reading `.items` here double-unwrapped and always returned null,
 *  which is why bounded `string[]+enum` fields fell through to free-entry
 *  tags.) Returns null when the item carries no enum. */
function readItemEnum(itemType: { enum?: unknown[] }): string[] | null {
  const en = itemType?.enum
  if (!Array.isArray(en) || en.length === 0) return null
  return en.map((v) => String(v))
}

function asArrayOfString(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.map((v) => String(v))
}

const view: ViewFieldRendererFactory<ArrayType, EnumMultiContent> = ({ info }) => {
  // Chip list (US-S5): one Badge per value, colour resolved from the
  // content-type's `color_by_value` map when present. Empty → "—".
  const colorByValue = (info.contentType as { color_by_value?: Record<string, string> })
    .color_by_value
  return (item: JsonObject) => {
    const arr = asArrayOfString(item[info.name])
    if (arr.length === 0) return <Text c="dimmed">—</Text>
    return (
      <Group gap={4} data-testid={'view.' + info.name}>
        {arr.map((v) => (
          <Badge key={v} size="sm" variant="light" color={colorByValue?.[v]}>
            {v}
          </Badge>
        ))}
      </Group>
    )
  }
}

const form: FormFieldRendererFactory<ArrayType, EnumMultiContent> = ({ def, info, texts }) => {
  // `info.type` is typed ArrayType but buildInfo resolves it to the item type
  // at runtime (the array is unwrapped) — so the enum sits directly on it.
  const enumValues = readItemEnum(info.type as unknown as { enum?: unknown[] })
  // Rich options (label + description + icon) declared on the content-type win;
  // otherwise fall back to the bare item `enum` keys. Either makes the field a
  // BOUNDED set; an empty result keeps the free-entry tags behaviour.
  const options = resolveFieldOptions(info.contentType, enumValues ?? [])
  const required = def.required ?? !info.type.optional
  const max = info.contentType.maxSelected || undefined
  const placeholder = info.contentType.placeholder
    || (options.length > 0
      ? texts.t('fields.selectValues', { defaultValue: 'Select values…' })
      : texts.t('fields.typeEnter', { defaultValue: 'Type and press Enter…' }))

  // Mode discriminator — bounded enum vs free-entry tags. Bounded-
  // creatable is still MultiSelect (the chevron stays), just with
  // `searchable` to make the typing affordance visible.
  const bounded = options.length > 0
  const creatable = info.contentType.creatable ?? !bounded
  const selectData = toSelectData(options)
  const renderOption = hasRichDetail(options) ? makeRenderOption(options) : undefined
  // Checkbox vs dropdown for a bounded enum: explicit `editAs` wins; else a
  // small fixed list (≤ 8) renders as a checkbox group (every option visible),
  // a larger one stays a searchable MultiSelect dropdown.
  // `editAs: cards`: a grid of option cards (two columns, one on a phone).
  const asCards = bounded && info.contentType.editAs === 'cards'
  const asCheckboxes =
    bounded && !asCards &&
    (info.contentType.editAs === 'checkboxes' ||
      (info.contentType.editAs !== 'multiselect' && selectData.length <= 8))

  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    return (
      <Controller
        key={info.id}
        name={info.name}
        control={control}
        rules={{
          validate: (value) => {
            if (!required) return true
            const arr = asArrayOfString(value)
            return arr.length > 0 || 'At least one value required'
          },
        }}
        render={({ field, fieldState }) => {
          const value = asArrayOfString(field.value)
          // Bounded enum, cards mode — every option a card ticked in place.
          if (asCards) {
            const locked = def.disabled === true || def.readOnly === true || isSubmitting
            const toggle = (v: string) => {
              const next = value.includes(v) ? value.filter((x) => x !== v) : [...value, v]
              field.onChange(options.filter((o) => next.includes(String(o.value))).map((o) => String(o.value)))
            }
            return (
              <Input.Wrapper
                label={info.label}
                description={info.description}
                error={fieldState.error?.message}
                required={required}
                data-testid={'input.' + info.name}
              >
                <SimpleGrid cols={{ base: 1, sm: 2 }} spacing={8} verticalSpacing={8} mt={info.label || info.description ? 'xs' : 0}>
                  {options.map((opt) => {
                    const v = String(opt.value)
                    const on = value.includes(v)
                    const off = locked || (!on && max !== undefined && value.length >= max)
                    return (
                      <OptionCard key={v} id={'input.' + info.name + '.' + v} on={on} disabled={off}
                        label={opt.label} description={opt.description} icon={opt.icon}
                        onToggle={() => { if (!off) toggle(v) }} onBlur={field.onBlur} />
                    )
                  })}
                </SimpleGrid>
              </Input.Wrapper>
            )
          }
          // Bounded enum, checkbox mode — list every option at once.
          if (asCheckboxes) {
            return (
              <Checkbox.Group
                label={info.label}
                description={info.description}
                value={value}
                onChange={(v) => field.onChange(v)}
                error={fieldState.error?.message}
                required={required}
                data-testid={'input.' + info.name}
              >
                <Stack gap="xs" mt="xs">
                  {options.map((opt) => (
                    <Checkbox
                      key={String(opt.value)}
                      value={String(opt.value)}
                      label={
                        opt.icon || opt.description ? (
                          <RichOptionRow
                            label={opt.label}
                            description={opt.description}
                            icon={opt.icon}
                            iconSize={18}
                          />
                        ) : (
                          opt.label
                        )
                      }
                      disabled={def.disabled === true || def.readOnly === true || isSubmitting}
                    />
                  ))}
                </Stack>
              </Checkbox.Group>
            )
          }
          if (bounded) {
            return (
              <MultiSelect
                label={info.label}
                description={info.description}
                placeholder={placeholder}
                data={selectData}
                renderOption={renderOption}
                value={value}
                onChange={(v) => field.onChange(v)}
                onBlur={field.onBlur}
                searchable={creatable}
                clearable
                hidePickedOptions
                maxValues={max}
                disabled={def.disabled === true || isSubmitting}
                readOnly={def.readOnly === true}
                error={fieldState.error?.message}
                required={required}
                data-testid={'input.' + info.name}
              />
            )
          }
          // Free-entry tags — every Enter adds a new tag.
          return (
            <TagsInput
              label={info.label}
              description={info.description}
              placeholder={placeholder}
              value={value}
              onChange={(v) => field.onChange(v)}
              onBlur={field.onBlur}
              maxTags={max}
              clearable
              disabled={def.disabled === true || isSubmitting}
              readOnly={def.readOnly === true}
              error={fieldState.error?.message}
              required={required}
              data-testid={'input.' + info.name}
            />
          )
        }}
      />
    )
  }
}

/** One option of an `editAs: cards` field: a checkbox, the option's
 *  icon, its label bold and its description muted; ticked, an accent
 *  border. Drawn on the body colour with the theme's border, readable in
 *  both themes. */
function OptionCard({ id, on, disabled, label, description, icon, onToggle, onBlur }: {
  id: string; on: boolean; disabled: boolean; label: string; description?: string; icon?: string
  onToggle: () => void; onBlur?: () => void
}) {
  const accent = 'var(--mantine-primary-color-filled)'
  return (
    <button type="button" role="checkbox" aria-checked={on} aria-disabled={disabled || undefined} disabled={disabled}
      data-testid={id} data-option-card={on ? 'on' : 'off'} onClick={onToggle} onBlur={onBlur}
      style={{
        display: 'flex', gap: 10, alignItems: 'center', width: '100%', textAlign: 'left', font: 'inherit', color: 'inherit',
        padding: '9px 10px', borderRadius: 9, background: 'var(--mantine-color-body)',
        border: `1px solid ${on ? accent : 'var(--mantine-color-default-border)'}`,
        boxShadow: on ? `0 0 0 1px ${accent} inset` : 'none',
        cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? 0.5 : 1,
      }}>
      <span aria-hidden style={{
        width: 18, height: 18, flex: 'none', borderRadius: 4, display: 'inline-grid', placeItems: 'center',
        border: `1px solid ${on ? accent : 'var(--mantine-color-default-border)'}`,
        background: on ? accent : 'transparent', color: 'var(--mantine-primary-color-contrast, #fff)',
      }}>{on ? <CheckIcon size={10} /> : null}</span>
      {icon ? <span aria-hidden data-option-icon={icon} style={{ display: 'inline-flex', flex: 'none' }}><Icon icon={icon} size={18} /></span> : null}
      <span style={{ display: 'flex', flexDirection: 'column', minWidth: 0, flex: 1 }}>
        <Text component="span" size="sm" fw={600}>{label}</Text>
        {description ? <Text component="span" size="xs" c="dimmed">{description}</Text> : null}
      </span>
    </button>
  )
}

const column: ColumnRendererFactory<ArrayType, EnumMultiContent> = ({ info }) => {
  return (item: JsonObject) => {
    const arr = asArrayOfString(item[info.name])
    if (arr.length === 0) return <Text c="dimmed" style={columnStyles}>—</Text>
    return (
      <Text size="sm" style={columnStyles} data-testid={'column.' + info.name}>
        {arr.join(', ')}
      </Text>
    )
  }
}

const EnumMultiRenderer: FieldRendererFactory<ArrayType, EnumMultiContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default EnumMultiRenderer
