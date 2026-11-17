// SPDX-License-Identifier: Apache-2.0
/**
 * InlineUIForm — in-process, themed form rendered DIRECTLY from a schema
 * AST (no iframe, no GetSchemaAST fetch). The schema-AST sibling of
 * FormWidgetPanel for the ask-user-data chat flow: the agent already sent
 * the canonical AST on the prompt event, so we build + render it here with
 * `@ktw/display-lib` and surface exactly two actions — Next / Cancel.
 *
 * No outer Card: the caller (ChatInputPanel) already provides the framing
 * — wrapping again would double-box (the issue NodeEditorTab also avoids).
 */
import { useMemo } from 'react'
import { Stack, Text, Title } from '@mantine/core'

import { buildUIForms, UIFormView } from '@ktw/display-lib/ui-builders/form/index.ts'
import { wireRegistries } from '@ktw/display-lib/registry-wire'
import type { Schema } from '@ktw/display-lib/schema'
import type { ActionMode, UIActionRef } from '@ktw/display-lib/ui-core/action/index.ts'
import { withFieldLabels } from './schema-labels'
import { useT } from '../../i18n'
import type { TFn } from '../../i18n'

// The display library's i18n is the GUI's, set up once at boot in the
// person's language (i18n/index.ts); only its registries are wired here.
wireRegistries()

// Exactly two buttons (replace the lib default Save/Cancel/Reset/Delete):
// Cancel + Next. Both 'item'-scoped so UIFormView's processor wires them
// to onCancel / onSubmit(value).
const ACTION_MODE: ActionMode = 'replace'
const nextCancel = (t: TFn): UIActionRef[] => [
  { type: 'ui-action', id: 'cancel', scope: 'item', label: t('form.cancel'), icon: 'IconX' },
  { type: 'ui-action', id: 'submit', scope: 'item', label: t('form.next'), icon: 'IconArrowRight' },
]

interface InlineUIFormProps {
  schema: Schema
  value?: Record<string, unknown>
  title?: string
  description?: string
  onSubmit: (value: Record<string, unknown>) => void
  onCancel: () => void
}

export function InlineUIForm({ schema, value, title, description, onSubmit, onCancel }: InlineUIFormProps) {
  const t = useT('chat')
  const form = useMemo(() => {
    try {
      // Derive labels from field names when the schema declares none
      // (LLM-authored shorthand schemas don't), so fields read
      // "Destination Type" not "destination_type".
      const labeled = withFieldLabels(schema)
      const forms = buildUIForms({ schema: labeled, actionRefs: nextCancel(t), actionMode: ACTION_MODE })
      return forms?.['default'] ?? (forms ? Object.values(forms)[0] : null)
    } catch {
      return null
    }
  }, [schema, t])

  const customTitle =
    title || description ? (
      <Stack gap={2}>
        {title && <Title order={4}>{title}</Title>}
        {description && <Text size="sm" c="dimmed">{description}</Text>}
      </Stack>
    ) : undefined

  if (!form) {
    return <Text size="sm" c="red">{t('form.invalidSchema')}</Text>
  }

  return (
    <UIFormView
      form={form}
      isNew
      customTitle={customTitle}
      // Pass the initial value only when present — an empty {} is truthy
      // and would short-circuit the schema's per-field defaults.
      item={value && Object.keys(value).length > 0 ? (value as any) : undefined}
      onSubmit={(v: Record<string, unknown>) => onSubmit(v)}
      onCancel={onCancel}
    />
  )
}
