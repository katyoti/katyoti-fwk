// SPDX-License-Identifier: Apache-2.0
// Search filter renderer — TextInput with search icon, controlled by RHF.

import { TextInput } from '@mantine/core'
import { IconSearch } from '@tabler/icons-react'
import { Controller, useFormContext } from 'react-hook-form'

export function SearchFilterRenderer({ fieldName, fieldLabel }: { fieldName: string; fieldLabel: string }) {
  const { control } = useFormContext()

  return (
    <Controller
      key={`filter-${fieldName}`}
      name={fieldName}
      control={control}
      render={({ field }) => (
        <TextInput
          {...field}
          label={fieldLabel}
          key={`filter-${fieldName}`}
          placeholder={fieldLabel}
          leftSection={<IconSearch size={16} />}
          style={{ width: '100%' }}
        />
      )}
    />
  )
}
