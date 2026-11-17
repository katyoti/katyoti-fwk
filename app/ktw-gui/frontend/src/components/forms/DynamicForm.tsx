// SPDX-License-Identifier: Apache-2.0
/**
 * DynamicForm — schema-driven form renderer using Mantine.
 * Takes JSON Schema + UI Schema (from Go formdef) and renders a full form.
 * No JSON Forms dependency — pure Mantine components.
 */
import { useState } from 'react'
import { Button, Group, Paper, Stack, Tabs, Text, Title } from '@mantine/core'
import { FieldRenderer } from './FieldRenderer'
import { useT } from '../../i18n'
import type {
  FormSchema, JsonSchema, JsonSchemaProperty,
  UISchemaElement, UISchemaControl, UISchemaLayout,
  UISchemaCategorization,
} from './types'

interface Props {
  schema: FormSchema
  onSubmit: (data: Record<string, any>) => void
  onCancel?: () => void
}

export function DynamicForm({ schema, onSubmit, onCancel }: Props) {
  const t = useT('components')
  const [data, setData] = useState<Record<string, any>>(() => ({ ...schema.data }))

  const handleChange = (path: string, value: any) => {
    setData(prev => {
      const next = { ...prev }
      setNestedValue(next, path, value)
      return next
    })
  }

  const handleSubmit = () => {
    onSubmit(data)
  }

  return (
    <Stack gap="md" p="md">
      {schema.title && (
        <Title order={4}>{schema.title}</Title>
      )}

      <FormBody
        jsonSchema={schema.jsonSchema}
        uiSchema={schema.uiSchema}
        data={data}
        onChange={handleChange}
      />

      <Group justify="flex-end" gap="sm" mt="md">
        {onCancel && (
          <Button variant="subtle" color="gray" onClick={onCancel}>
            {t('dynamicForm.cancel')}
          </Button>
        )}
        <Button onClick={handleSubmit}>
          {t('dynamicForm.submit')}
        </Button>
      </Group>
    </Stack>
  )
}

// ── Layout rendering ─────────────────────────────────────────────────────

interface FormBodyProps {
  jsonSchema: JsonSchema
  uiSchema: UISchemaElement
  data: Record<string, any>
  onChange: (path: string, value: any) => void
}

function FormBody({ jsonSchema, uiSchema, data, onChange }: FormBodyProps) {
  const t = useT('components')
  if (!uiSchema || !uiSchema.type) {
    // No UI schema — render all properties as vertical layout
    return <AutoLayout jsonSchema={jsonSchema} data={data} onChange={onChange} />
  }

  switch (uiSchema.type) {
    case 'Categorization':
      return <CategorizedLayout schema={uiSchema as UISchemaCategorization} jsonSchema={jsonSchema} data={data} onChange={onChange} />
    case 'VerticalLayout':
    case 'HorizontalLayout':
    case 'Group':
      return <LayoutRenderer schema={uiSchema as UISchemaLayout} jsonSchema={jsonSchema} data={data} onChange={onChange} />
    case 'Control':
      return <ControlRenderer schema={uiSchema as UISchemaControl} jsonSchema={jsonSchema} data={data} onChange={onChange} />
    default:
      return <Text size="xs" c="dimmed">{t('dynamicForm.unknownElement', { type: uiSchema.type })}</Text>
  }
}

// Tabbed layout (from Groups in formdef)
function CategorizedLayout({ schema, jsonSchema, data, onChange }: { schema: UISchemaCategorization } & Omit<FormBodyProps, 'uiSchema'>) {
  return (
    <Tabs defaultValue={schema.elements[0]?.label}>
      <Tabs.List>
        {schema.elements.map(cat => (
          <Tabs.Tab key={cat.label} value={cat.label}>{cat.label}</Tabs.Tab>
        ))}
      </Tabs.List>
      {schema.elements.map(cat => (
        <Tabs.Panel key={cat.label} value={cat.label} pt="md">
          <Stack gap="sm">
            {cat.elements.map((el, i) => (
              <FormBody key={i} jsonSchema={jsonSchema} uiSchema={el} data={data} onChange={onChange} />
            ))}
          </Stack>
        </Tabs.Panel>
      ))}
    </Tabs>
  )
}

// Vertical/Horizontal layout
function LayoutRenderer({ schema, jsonSchema, data, onChange }: { schema: UISchemaLayout } & Omit<FormBodyProps, 'uiSchema'>) {
  const Wrapper = schema.type === 'HorizontalLayout' ? Group : Stack
  const content = (
    <Wrapper gap="sm">
      {schema.elements.map((el, i) => (
        <FormBody key={i} jsonSchema={jsonSchema} uiSchema={el} data={data} onChange={onChange} />
      ))}
    </Wrapper>
  )

  if (schema.type === 'Group' && schema.label) {
    return (
      <Paper withBorder p="sm" radius="sm">
        <Text size="sm" fw={600} mb="xs">{schema.label}</Text>
        {content}
      </Paper>
    )
  }
  return content
}

// Single control → field renderer
function ControlRenderer({ schema, jsonSchema, data, onChange }: { schema: UISchemaControl } & Omit<FormBodyProps, 'uiSchema'>) {
  // Extract property key from scope: "#/properties/name" → "name"
  const path = schema.scope.replace('#/properties/', '').replace(/\//g, '.')
  const propSchema = resolveProperty(jsonSchema, schema.scope)
  if (!propSchema) return null

  const required = jsonSchema.required?.includes(path.split('.').pop()!) ?? false
  const value = getNestedValue(data, path)

  return (
    <FieldRenderer
      path={path}
      schema={propSchema}
      uiOptions={schema.options}
      label={schema.label || propSchema.title || path}
      required={required}
      value={value}
      onChange={(v) => onChange(path, v)}
    />
  )
}

// Auto-layout when no UI schema
function AutoLayout({ jsonSchema, data, onChange }: Omit<FormBodyProps, 'uiSchema'>) {
  const props = jsonSchema.properties || {}
  return (
    <Stack gap="sm">
      {Object.entries(props).map(([key, prop]) => (
        <FieldRenderer
          key={key}
          path={key}
          schema={prop}
          label={prop.title || key}
          required={jsonSchema.required?.includes(key) ?? false}
          value={data[key]}
          onChange={(v) => onChange(key, v)}
        />
      ))}
    </Stack>
  )
}

// ── Helpers ──────────────────────────────────────────────────────────────

function resolveProperty(schema: JsonSchema, scope: string): JsonSchemaProperty | undefined {
  // scope: "#/properties/name" or "#/properties/group/properties/field"
  const parts = scope.replace('#/', '').split('/')
  let current: any = schema
  for (const part of parts) {
    current = current?.[part]
    if (!current) return undefined
  }
  return current
}

function getNestedValue(obj: any, path: string): any {
  return path.split('.').reduce((o, k) => o?.[k], obj)
}

function setNestedValue(obj: any, path: string, value: any): void {
  const keys = path.split('.')
  let current = obj
  for (let i = 0; i < keys.length - 1; i++) {
    if (!(keys[i] in current)) current[keys[i]] = {}
    current = current[keys[i]]
  }
  current[keys[keys.length - 1]] = value
}
