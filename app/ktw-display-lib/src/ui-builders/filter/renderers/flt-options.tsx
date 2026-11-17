// SPDX-License-Identifier: Apache-2.0
// Options filter renderer — Mantine Select dropdown with `allOption` at top.

import { Select } from '@mantine/core'
import type { FieldOption } from '../../../schema/index.ts'
import { Controller, useFormContext } from 'react-hook-form'

export function OptionFilterRenderer({
  fieldName,
  fieldLabel,
  allOption,
  options,
}: {
  fieldName: string
  fieldLabel: string
  allOption: FieldOption
  options: FieldOption[]
}) {
  const { control } = useFormContext()

  const selectOptions = [
    ...(allOption ? [{ value: String(allOption.value), label: allOption.label }] : []),
    ...options.map((option) => ({ value: String(option.value), label: option.label })),
  ]

  return (
    <Controller
      key={`filter-${fieldName}`}
      name={fieldName}
      control={control}
      render={({ field }) => (
        <Select
          {...field}
          label={fieldLabel}
          data={selectOptions}
          maxDropdownHeight={260}
          style={{
            maxWidth: 240,
            textTransform: 'capitalize',
          }}
          styles={{
            input: { textTransform: 'capitalize' },
            option: { textTransform: 'capitalize' },
          }}
        />
      )}
    />
  )
}
