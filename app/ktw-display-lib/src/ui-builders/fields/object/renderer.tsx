// SPDX-License-Identifier: Apache-2.0
// object content renderer — recursive sub-form / list editor for
// user-defined object types and arrays of them (US-S4).
//
// Replaces the JSON-textarea fallback for nested object body fields
// (e.g. CWM decision.alternatives: DecisionAlternative[],
// definition-of-done.criteria: DoDCriterion[]). Routed here by
// `info.ts` when a field's resolved type is an object the content
// catalog doesn't cover.
//
//   - single object  → a bordered sub-form: each inner field rendered
//     by its own dedicated renderer (recursive — reuses the factory).
//   - array of object → a list of those sub-forms with add / remove /
//     reorder, one card per row.
//
// Reuse mechanism: inner FORM fields bind to react-hook-form via
// ABSOLUTE dotted paths (`<arrayName>.<index>.<innerField>`) against
// the single root FormProvider, so every existing leaf renderer
// (text / number / enum / date / …) works unchanged. Inner VIEW
// fields read the sub-object directly (flat property access).

import { ActionIcon, Box, Button, Card, Group, Stack, Text } from '@mantine/core'
import {
  IconArrowDown,
  IconArrowUp,
  IconPlus,
  IconTrash,
} from '@tabler/icons-react'
import { useFieldArray, useFormContext, useFormState } from 'react-hook-form'
import {
  createHeaderRenderer,
  createViewRenderer,
  getFactory,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import { SchemaQuery } from '../../../schema/schema-query.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import type { ObjectType, Schema, SchemaType } from '../../../schema/schema-types.ts'
import type { ContentType } from '../../../schema/schema-contents.ts'
import type { UITexts } from '../../../ui-core/i18n/types.ts'
import type { CrudFieldInfo } from '../info.ts'
import type { UIFieldDef } from '../types.ts'

/** Humanize a field name for a default label: `author_email` → `Author email`. */
function humanize(name: string): string {
  const s = name.replace(/[_-]+/g, ' ').replace(/([a-z])([A-Z])/g, '$1 $2').trim()
  return s.charAt(0).toUpperCase() + s.slice(1)
}

/** Inner-field descriptor resolved once from the object type. */
export interface InnerField {
  name: string
  type: SchemaType
  label: string
}

export function innerFields(schema: Schema, objType: ObjectType): InnerField[] {
  let infos
  try {
    infos = SchemaQuery.getFieldInfos(schema, objType)
  } catch {
    return []
  }
  return infos.map((fi) => {
    const alias = (fi.fieldOptions as { alias?: string }).alias
    // The label the schema declares on the field (« Élément »), else its
    // name made readable.
    const declared = (fi.fieldType as { label?: unknown }).label
    return {
      name: fi.fieldName,
      type: fi.fieldType,
      label: typeof declared === 'string' && declared !== '' ? declared : humanize(alias ?? fi.fieldName),
    }
  })
}

// ── view ────────────────────────────────────────────────────────────────

const view: ViewFieldRendererFactory<SchemaType, ContentType> = ({ info, texts }) => {
  const objType = info.type as ObjectType
  const fields = innerFields(info.schema, objType)

  const renderOne = (sub: JsonObject, key: string) => (
    <Card key={key} withBorder padding="sm" radius="sm">
      <Stack gap={6}>
        {fields.map((f) => {
          // parentType = the object type → getInfo resolves f.name's
          // type + content against it. View reads the sub-object flat
          // (item[name]), so the plain field name is correct here.
          const fdef: UIFieldDef = { id: f.name, label: f.label }
          const render = createViewRenderer(info.schema, f.name, fdef, texts, objType)
          return <Box key={f.name}>{render(sub)}</Box>
        })}
      </Stack>
    </Card>
  )

  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null) return <Text c="dimmed">—</Text>
    if (info.isArray) {
      const rows = Array.isArray(value) ? value : []
      if (rows.length === 0) return <Text c="dimmed">(none)</Text>
      return <Stack gap="xs">{rows.map((r, i) => renderOne(r as JsonObject, String(i)))}</Stack>
    }
    return renderOne(value as JsonObject, info.name)
  }
}

// ── form ──────────────────────────────────────────────────────────────────

/** Render the inner fields of one object row at the given RHF path prefix. */
function InnerForm({
  info,
  texts,
  fields,
  pathPrefix,
  isSubmitting,
}: {
  info: CrudFieldInfo
  texts: UITexts
  fields: InnerField[]
  pathPrefix: string
  isSubmitting: boolean
}) {
  return (
    <Stack gap="sm">
      {fields.map((f) => {
        const dotted = `${pathPrefix}.${f.name}`
        // Resolve the leaf factory + info against the OBJECT type (plain
        // field name → correct type + content), then override the
        // binding identity to the absolute dotted RHF path so the leaf
        // renderer's `name={info.name}` writes into the nested form slot.
        const fdef: UIFieldDef = { id: dotted, label: f.label }
        const { factory, info: leafInfo } = getFactory(info.schema, f.name, fdef, info.type)
        const boundInfo = { ...leafInfo, id: dotted, name: dotted }
        const render = factory.form({ def: fdef, info: boundInfo, texts })
        return <Box key={f.name}>{render({}, false, isSubmitting)}</Box>
      })}
    </Stack>
  )
}

const form: FormFieldRendererFactory<SchemaType, ContentType> = ({ info, texts, def }) => {
  const objType = info.type as ObjectType
  const fields = innerFields(info.schema, objType)
  const disabled = (def as { disabled?: boolean }).disabled === true

  return (_item, _isNew, isSubmitting) => {
    if (!info.isArray) {
      // Single nested object — render its fields at <name>.<inner>.
      return (
        <Box>
          <Text size="sm" fw={500} mb={4}>{info.label}</Text>
          <Card withBorder padding="sm" radius="sm">
            <InnerForm
              info={info}
              texts={texts}
              fields={fields}
              pathPrefix={info.name}
              isSubmitting={isSubmitting}
            />
          </Card>
        </Box>
      )
    }
    return (
      <ArrayEditor
        info={info}
        texts={texts}
        fields={fields}
        disabled={disabled || isSubmitting}
        isSubmitting={isSubmitting}
      />
    )
  }
}

/** Array-of-objects list editor: add / remove / reorder rows, each a
 *  recursive InnerForm. Uses RHF useFieldArray on `info.name`. */
function ArrayEditor({
  info,
  texts,
  fields,
  disabled,
  isSubmitting,
}: {
  info: CrudFieldInfo
  texts: UITexts
  fields: InnerField[]
  disabled: boolean
  isSubmitting: boolean
}) {
  const { control } = useFormContext()
  const { fields: rows, append, remove, move } = useFieldArray({ control, name: info.name })
  // The list's own error - a host check on its line (« la raison est
  // requise ») is set on the list's name; a rule's lands on its root.
  const { errors } = useFormState({ control, name: info.name })
  const listError = info.name.split('.').reduce<any>((e, k) => e?.[k], errors)
  const errorText: string | undefined = listError?.message ?? listError?.root?.message

  const addRow = () => {
    const seed = SchemaQuery.getDefaultValue(info.schema, info.type) as JsonObject
    append(seed ?? {})
  }

  return (
    <Stack gap="xs" data-testid={'input.' + info.name}>
      <Group justify="space-between" align="center">
        <Box>
          <Text size="sm" fw={500}>{info.label}</Text>
          {info.description && <Text size="xs" c="dimmed">{info.description}</Text>}
        </Box>
        <Button
          size="compact-xs"
          variant="light"
          leftSection={<IconPlus size={12} />}
          onClick={addRow}
          disabled={disabled}
        >
          {texts.t('fields.add', { defaultValue: 'Add' })}
        </Button>
      </Group>

      {rows.length === 0 && (
        <Text size="xs" c="dimmed">{texts.t('fields.empty', { defaultValue: 'No entries yet — click « Add » to create one.' })}</Text>
      )}

      {errorText && <Text size="xs" c="red" role="alert" data-testid={'error.' + info.name}>{errorText}</Text>}

      {rows.map((row, index) => (
        <Card key={row.id} withBorder padding="sm" radius="sm">
          <Group justify="space-between" align="center" mb={6}>
            <Text size="xs" c="dimmed" fw={600}>#{index + 1}</Text>
            <Group gap={4}>
              <ActionIcon
                size="sm" variant="subtle" color="gray"
                disabled={disabled || index === 0}
                onClick={() => move(index, index - 1)}
                title={texts.t('fields.moveUp', { defaultValue: 'Move up' })}
              >
                <IconArrowUp size={14} />
              </ActionIcon>
              <ActionIcon
                size="sm" variant="subtle" color="gray"
                disabled={disabled || index === rows.length - 1}
                onClick={() => move(index, index + 1)}
                title={texts.t('fields.moveDown', { defaultValue: 'Move down' })}
              >
                <IconArrowDown size={14} />
              </ActionIcon>
              <ActionIcon
                size="sm" variant="subtle" color="red"
                disabled={disabled}
                onClick={() => remove(index)}
                title={texts.t('fields.remove', { defaultValue: 'Remove' })}
              >
                <IconTrash size={14} />
              </ActionIcon>
            </Group>
          </Group>
          <InnerForm
            info={info}
            texts={texts}
            fields={fields}
            pathPrefix={`${info.name}.${index}`}
            isSubmitting={isSubmitting}
          />
        </Card>
      ))}
    </Stack>
  )
}

// ── column ──────────────────────────────────────────────────────────────

const column: ColumnRendererFactory<SchemaType, ContentType> = ({ info }) => {
  return (item: JsonObject) => {
    const value = item[info.name]
    if (value === undefined || value === null) return <Text c="dimmed">—</Text>
    if (info.isArray) {
      const n = Array.isArray(value) ? value.length : 0
      return <Text size="sm" c="dimmed">{n} {n === 1 ? 'item' : 'items'}</Text>
    }
    return <Text size="sm" c="dimmed">{'{…}'}</Text>
  }
}

const ObjectRenderer: FieldRendererFactory<SchemaType, ContentType> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default ObjectRenderer
