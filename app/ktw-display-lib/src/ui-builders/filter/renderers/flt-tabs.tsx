// SPDX-License-Identifier: Apache-2.0
// Tabs filter renderer — Mantine Tabs row, one Tab per option (+ allOption).
// Single-select discriminator over the dataset.

import { Tabs } from '@mantine/core'
import type { FieldOption } from '../../../schema/index.ts'
import { Controller, useFormContext } from 'react-hook-form'

export function TabsFilterRenderer({
  fieldName,
  allOption,
  options,
}: {
  fieldName: string
  allOption?: FieldOption<number | string>
  options: FieldOption<number | string>[]
}) {
  const { control } = useFormContext()

  const allOptions = [...(allOption ? [allOption] : []), ...options]

  return (
    <Controller
      key={`filter-${fieldName}`}
      name={fieldName}
      control={control}
      render={({ field }) => {
        const currentValue = String(field.value)

        const handleChange = (value: string | null) => {
          if (value === null) return
          // Find the original option to preserve the type (number or string).
          const option = allOptions.find((opt) => String(opt.value) === value)
          if (option) {
            field.onChange(option.value)
          }
        }

        return (
          <Tabs value={currentValue} onChange={handleChange}>
            <Tabs.List>
              {allOption && (
                <Tabs.Tab key={String(allOption.value)} value={String(allOption.value)}>
                  {allOption.label}
                </Tabs.Tab>
              )}
              {options.map((option) => (
                <Tabs.Tab key={String(option.value)} value={String(option.value)}>
                  {option.label}
                </Tabs.Tab>
              ))}
            </Tabs.List>
          </Tabs>
        )
      }}
    />
  )
}
