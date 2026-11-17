// SPDX-License-Identifier: Apache-2.0
// Harness React shell — the iframe's main UI.
//
// Reads `__KTW_RUNTIME_CONFIG__` (schema, initial value, mode, page metadata),
// walks the schema's root.fields, and dispatches each via the factory
// orchestrator. Renders a Mantine card with title + description + form +
// (optional) submit button + (optional) outcome panel. Surfaces ready /
// change / submit events back to the host via postMessage.
//
// When a field has no wired renderer (S7r2 progress / stub manifests),
// renders an inline notice and continues — no broken iframe.

import {
  Anchor,
  Box,
  Button,
  Card,
  Code,
  Group,
  Notification,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import { FormProvider, useForm } from 'react-hook-form'
import { useEffect, useMemo, useState } from 'react'
import {
  createFieldRenderer,
  type CrudFieldInfo,
  type UIFieldDef,
} from '@ktw/display-lib/ui-builders/fields/index.ts'
import { useTexts, type UITexts } from '@ktw/display-lib/ui-core/i18n/index.ts'
import { SchemaQuery } from '@ktw/display-lib/schema'
import type { ObjectType, Schema } from '@ktw/display-lib/schema'
import { makeEmit, resolveWidgetId, type WidgetMessage } from './postmessage.ts'

export interface HarnessConfig {
  schema: Schema
  initialValue: Record<string, unknown>
  mode: string
  pageTitle: string
  description: string
  showSchemaPane: boolean
  showOutcome: boolean
  widgetName: string
  /** Optional DSL tree — set ONLY by the `/widget/page` route. The
   *  UIPageHarness reads this; other harnesses ignore it. When set, the
   *  `schema` field is unused (a placeholder schema may be passed for
   *  type-shape compatibility). Wired by display-dsl-framework S5. */
  dslTree?: unknown
  /** Optional outer data tree for `dataField` bindings inside the
   *  dslTree (page kind only). Wired by display-package-restructure
   *  Phase 3 (2026-05-06). */
  data?: unknown
  /** When true, the harness renders MINIMAL chrome (no Card wrapper, no
   *  page title, no schema pane, no outcome panel, no full-viewport bg).
   *  Set by widget routes when the host has injected schema/value/dslTree
   *  via URL params — i.e. the iframe is hosted inside another tab/panel
   *  and shouldn't apply its own page chrome. Default: false (dev demo
   *  mode shows full chrome). Wired by display-dsl-framework S9 polish. */
  embedded?: boolean
}

export function Harness({ config }: { config: HarnessConfig }): React.ReactElement {
  const widgetId = resolveWidgetId(config.widgetName)
  const emit = useMemo(
    () => makeEmit({ widget: config.widgetName, widgetId }),
    [config.widgetName, widgetId],
  )

  // Resolve root object type once.
  const rootType = useMemo<ObjectType | null>(() => {
    try {
      const t = SchemaQuery.getType(config.schema, config.schema.root)
      return SchemaQuery.isObject(t) ? t : null
    } catch {
      return null
    }
  }, [config.schema])

  const formMethods = useForm({ defaultValues: config.initialValue, mode: 'onChange' })
  const [outcomeEvents, setOutcomeEvents] = useState<Array<{ ts: string; label: string; payload: unknown }>>([])
  const texts = useTexts('core')

  // Emit 'ready' on mount.
  useEffect(() => {
    emit({ type: 'ready', ts: Date.now() })
  }, [emit])

  // Subscribe to form value changes.
  useEffect(() => {
    const sub = formMethods.watch((value) => {
      emit({ type: 'change', value })
      if (config.showOutcome) {
        appendOutcome(setOutcomeEvents, 'change', value)
      }
    })
    return () => sub.unsubscribe()
  }, [formMethods, emit, config.showOutcome])

  const onSubmit = (data: Record<string, unknown>): void => {
    emit({ type: 'submit', value: data })
    if (config.showOutcome) {
      appendOutcome(setOutcomeEvents, 'submit', data)
    }
  }

  const layoutCols = config.showSchemaPane ? '2fr 1fr' : '1fr'

  return (
    <Box
      style={{
        display: 'grid',
        gridTemplateColumns: layoutCols,
        gap: 24,
        alignItems: 'start',
        padding: 24,
        background: 'var(--mantine-color-gray-0)',
        minHeight: '100vh',
      }}
    >
      <Card withBorder shadow="sm" padding="lg" radius="md">
        <Title order={3} mb={4}>
          {config.pageTitle}
        </Title>
        {config.description && (
          <Text size="sm" c="dimmed" mb="md">
            {config.description}
          </Text>
        )}
        {!rootType ? (
          <Notification color="yellow" withCloseButton={false} title="Cannot render">
            Root type is not an Object with fields. Walker only handles object roots in v1.
          </Notification>
        ) : (
          <FormProvider {...formMethods}>
            <form onSubmit={formMethods.handleSubmit(onSubmit)} data-testid="form.root">
              <Stack gap="sm">
                {Object.entries(rootType.fields ?? {}).map(([fieldName, fieldRef]) => (
                  <FieldSlot
                    key={fieldName}
                    schema={config.schema}
                    fieldName={fieldName}
                    fieldRef={fieldRef}
                    texts={texts}
                  />
                ))}
              </Stack>
              <Group justify="flex-start" mt="md">
                <Button type="submit" data-testid="action.submit">
                  {texts.t('buttons.submit', { defaultValue: 'Submit' })}
                </Button>
              </Group>
            </form>
          </FormProvider>
        )}
        {config.showOutcome && (
          <OutcomePanel events={outcomeEvents} />
        )}
      </Card>

      {config.showSchemaPane && (
        <Card withBorder padding="md" radius="md">
          <Text fw={600} size="sm" mb="xs">
            Source schema
          </Text>
          <Code
            block
            data-testid="case.schema"
            style={{ fontSize: 11, maxHeight: 480, overflow: 'auto' }}
          >
            {JSON.stringify(config.schema, null, 2)}
          </Code>
          <Text size="xs" c="dimmed" mt="xs">
            ↑ Schema literal. Form on the left rendered from it via the shared registry.
          </Text>
        </Card>
      )}
    </Box>
  )
}

function FieldSlot({
  schema,
  fieldName,
  fieldRef,
  texts,
}: {
  schema: Schema
  fieldName: string
  fieldRef: unknown
  texts: UITexts
}): React.ReactElement {
  const def: UIFieldDef = useMemo(() => {
    const ref = fieldRef as { label?: string; description?: string; default?: unknown }
    return {
      id: fieldName,
      name: fieldName,
      label: ref.label ?? fieldName,
      description: ref.description,
      defaultValue: ref.default as never,
    }
  }, [fieldName, fieldRef])

  let renderer: ReturnType<typeof createFieldRenderer> | null = null
  let info: CrudFieldInfo | null = null
  let error: string | null = null
  try {
    renderer = createFieldRenderer(schema, fieldName, def, texts)
    // We don't need info here at the slot — the renderer already closed over it
  } catch (err) {
    error = (err as Error).message
  }

  return (
    <Box data-testid={'field.' + fieldName}>
      {error ? (
        <Notification color="gray" withCloseButton={false} title={`field: ${fieldName}`}>
          <Text size="sm">{error}</Text>
          <Text size="xs" c="dimmed">
            See <Anchor href={`/test/api/cases`}>/test/api/cases</Anchor> for the wired catalog.
          </Text>
        </Notification>
      ) : (
        renderer && renderer({}, /*isNew*/ true, /*isSubmitting*/ false)
      )}
    </Box>
  )
}

function OutcomePanel({
  events,
}: {
  events: Array<{ ts: string; label: string; payload: unknown }>
}): React.ReactElement {
  if (events.length === 0) {
    return (
      <Box mt="md" p="sm" style={{ background: 'var(--mantine-color-gray-1)', borderRadius: 4 }}>
        <Text size="xs" c="dimmed" data-testid="case.outcome">
          No events yet. Edit a field, or click Submit.
        </Text>
      </Box>
    )
  }
  return (
    <Box
      mt="md"
      p="sm"
      data-testid="case.outcome"
      style={{
        background: 'var(--mantine-color-gray-0)',
        borderLeft: '3px solid var(--mantine-color-blue-6)',
        borderRadius: 4,
        fontFamily: 'ui-monospace, SFMono-Regular, monospace',
        fontSize: 12,
        maxHeight: 200,
        overflow: 'auto',
      }}
    >
      {events.map((e, i) => (
        <Box key={i} mb="xs">
          <Text size="xs" component="span" c="dimmed">
            [{e.ts}]
          </Text>{' '}
          <Text size="xs" component="span" fw={600}>
            {e.label}
          </Text>
          <Code block style={{ fontSize: 11, marginTop: 2 }}>
            {JSON.stringify(e.payload, null, 2)}
          </Code>
        </Box>
      ))}
    </Box>
  )
}

function appendOutcome(
  setEvents: React.Dispatch<React.SetStateAction<Array<{ ts: string; label: string; payload: unknown }>>>,
  label: string,
  payload: unknown,
): void {
  const ts = new Date().toISOString().slice(11, 19)
  setEvents((prev) => [{ ts, label, payload }, ...prev].slice(0, 5))
}
