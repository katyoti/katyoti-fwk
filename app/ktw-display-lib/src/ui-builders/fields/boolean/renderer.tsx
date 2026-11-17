// SPDX-License-Identifier: Apache-2.0
// boolean content renderer — Mantine Checkbox / Switch / icon-flag
// (controlled by `editAs`). Flag preset table ported from the old
// `_ktw_workspace/old_ui_main/.../boolean.tsx` (cwm-v3 US-4 Tier 1,
// 2026-06-07).

import { ActionIcon, Checkbox, Group, Select, Stack, Switch, Text, Tooltip } from '@mantine/core'
import {
  IconBulb,
  IconCheck,
  IconShield,
  IconShieldLock,
  IconToggleLeft,
  IconToggleRight,
} from '@tabler/icons-react'
import { Controller, useFormContext } from 'react-hook-form'
import {
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import type { BooleanType } from '../../../schema/schema-types.ts'
import {
  coerceFromSelect,
  hasRichDetail,
  makeRenderOption,
  resolveFieldOptions,
  toSelectData,
} from '../rich-options.tsx'
import type { BooleanContent, BooleanFlagPreset } from './types.ts'

// FlagDef icons typed as `any` — Tabler icons are
// `ForwardRefExoticComponent` with their own prop union and don't
// cleanly satisfy a structural `ComponentType<{size?, color?}>`. The
// renderer only reads `size` + `color` props off them, so the loose
// type at the table is safe; the call site enforces the prop shape.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type IconLike = any
interface FlagDef {
  iconOn:   IconLike
  iconOff:  IconLike
  colorOn:  string
  colorOff: string
  labelOn:  string
  labelOff: string
}

/** Icon-pair presets for `editAs: 'flag'`. See `BooleanFlagPreset`
 *  godoc in `./types.ts`. Adding a preset = adding a row here +
 *  declaring it in the union type. */
const FLAG_PRESETS: Record<BooleanFlagPreset, FlagDef> = {
  protected: {
    iconOn:   IconShieldLock,
    iconOff:  IconShield,
    colorOn:  'orange',
    colorOff: 'green',
    labelOn:  'Protected',
    labelOff: 'Unprotected',
  },
  warning: {
    iconOn:   IconBulb,
    iconOff:  IconCheck,
    colorOn:  'orange',
    colorOff: 'green',
    labelOn:  'Warning',
    labelOff: 'OK',
  },
  checked: {
    iconOn:   IconCheck,
    iconOff:  IconShield,
    colorOn:  'green',
    colorOff: 'gray',
    labelOn:  'Checked',
    labelOff: 'Unchecked',
  },
  switch: {
    iconOn:   IconToggleRight,
    iconOff:  IconToggleLeft,
    colorOn:  'blue',
    colorOff: 'gray',
    labelOn:  'On',
    labelOff: 'Off',
  },
}

const view: ViewFieldRendererFactory<BooleanType, BooleanContent> = ({ info, texts }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null) return <Text c="dimmed">—</Text>
    // editAs:'flag' view → icon glyph with tooltip; no flag preset →
    // textual Yes/No (the historic shape).
    if (info.contentType.editAs === 'flag') {
      const preset = FLAG_PRESETS[info.contentType.flagIcon || 'switch']
      const on = value === true
      const Icon = on ? preset.iconOn : preset.iconOff
      const color = on ? preset.colorOn : preset.colorOff
      const label = on ? preset.labelOn : preset.labelOff
      return (
        <Tooltip label={label} withArrow>
          <Icon size={16} color={`var(--mantine-color-${color}-6)`} aria-label={label} data-testid={'view.' + info.name} />
        </Tooltip>
      )
    }
    const yesLabel = texts.t('boolean.yes', { defaultValue: 'Yes' })
    const noLabel = texts.t('boolean.no', { defaultValue: 'No' })
    return (
      <Text data-testid={'view.' + info.name}>{value ? yesLabel : noLabel}</Text>
    )
  }
}

const form: FormFieldRendererFactory<BooleanType, BooleanContent> = ({ def, info }) => {
  // A boolean field may declare a rich option set (e.g. a true/false pair with
  // explanatory labels + icons) → render as a Select; the picked value is
  // coerced back to a boolean. No options → the usual checkbox / switch / flag.
  const options = resolveFieldOptions<boolean>(info.contentType, [])
  const selectData = options.length > 0 ? toSelectData(options) : null
  const renderOption = hasRichDetail(options) ? makeRenderOption(options) : undefined
  return (_item, _isNew, isSubmitting) => {
    const { control } = useFormContext()
    const editAs = info.contentType.editAs || 'checkbox'
    const subLabel = info.contentType.subLabel

    if (selectData) {
      return (
        <Controller
          key={info.id}
          name={info.name}
          control={control}
          render={({ field, fieldState }) => (
            <Select
              value={field.value == null ? null : String(field.value)}
              onChange={(picked) => field.onChange(coerceFromSelect(options, picked))}
              onBlur={field.onBlur}
              label={info.label}
              description={info.description ?? subLabel}
              placeholder={def.placeholder}
              disabled={def.disabled === true || isSubmitting}
              readOnly={def.readOnly === true}
              error={fieldState.error?.message}
              data={selectData}
              renderOption={renderOption}
              allowDeselect={false}
              data-testid={'input.' + info.name}
            />
          )}
        />
      )
    }

    if (editAs === 'flag') {
      const preset = FLAG_PRESETS[info.contentType.flagIcon || 'switch']
      return (
        <Controller
          key={info.id}
          name={info.name}
          control={control}
          render={({ field, fieldState }) => {
            const on = field.value === true
            const Icon = on ? preset.iconOn : preset.iconOff
            const color = on ? preset.colorOn : preset.colorOff
            const stateLabel = on ? preset.labelOn : preset.labelOff
            return (
              <Group justify="space-between" align="center">
                <Stack gap={2}>
                  <Text size="sm" fw={500}>{info.label}</Text>
                  {subLabel && <Text size="xs" c="dimmed">{subLabel}</Text>}
                </Stack>
                <Tooltip label={stateLabel} withArrow>
                  <ActionIcon
                    variant="subtle"
                    color={color}
                    onClick={() => field.onChange(!on)}
                    onBlur={field.onBlur}
                    disabled={def.disabled === true || isSubmitting}
                    aria-label={stateLabel}
                    aria-pressed={on}
                    data-testid={'input.' + info.name}
                  >
                    <Icon size={18} />
                  </ActionIcon>
                </Tooltip>
                {fieldState.error && (
                  <Text size="xs" c="red">{fieldState.error.message}</Text>
                )}
              </Group>
            )
          }}
        />
      )
    }

    if (editAs === 'switch') {
      return (
        <Controller
          key={info.id}
          name={info.name}
          control={control}
          render={({ field, fieldState }) => (
            <Group justify="space-between">
              <div>
                <Text size="sm" fw={500}>{info.label}</Text>
                {subLabel && <Text size="xs" c="dimmed">{subLabel}</Text>}
              </div>
              <Switch
                checked={field.value === true}
                onChange={(e) => field.onChange(e.currentTarget.checked)}
                onBlur={field.onBlur}
                disabled={def.disabled === true || isSubmitting}
                readOnly={def.readOnly === true}
                error={fieldState.error?.message}
                data-testid={'input.' + info.name}
              />
            </Group>
          )}
        />
      )
    }

    return (
      <Controller
        key={info.id}
        name={info.name}
        control={control}
        render={({ field, fieldState }) => (
          <Checkbox
            checked={field.value === true}
            onChange={(e) => field.onChange(e.currentTarget.checked)}
            onBlur={field.onBlur}
            label={
              subLabel ? (
                <Stack gap={2}>
                  <Text size="sm">{info.label}</Text>
                  <Text size="xs" c="dimmed">{subLabel}</Text>
                </Stack>
              ) : (
                info.label
              )
            }
            disabled={def.disabled === true || isSubmitting}
            readOnly={def.readOnly === true}
            error={fieldState.error?.message}
            data-testid={'input.' + info.name}
          />
        )}
      />
    )
  }
}

const column: ColumnRendererFactory<BooleanType, BooleanContent> = ({ info, texts }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    const yesLabel = texts.t('boolean.yes', { defaultValue: 'Yes' })
    const noLabel = texts.t('boolean.no', { defaultValue: 'No' })
    if (value === undefined || value === null) return <Text c="dimmed">—</Text>
    return (
      <Text size="sm" data-testid={'column.' + info.name}>
        {value ? yesLabel : noLabel}
      </Text>
    )
  }
}

const BooleanRenderer: FieldRendererFactory<BooleanType, BooleanContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default BooleanRenderer
