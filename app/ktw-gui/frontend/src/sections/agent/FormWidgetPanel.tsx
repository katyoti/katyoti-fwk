// SPDX-License-Identifier: Apache-2.0
/**
 * FormWidgetPanel — the universal task-run surface (per ms-schema-driven-ui
 * US-S1 D4). Renders a header card (task name / icon / description) on
 * top, the `@ktw/display-lib` form widget below.
 *
 * Behaviour:
 *  1. Mount → fetch the AST for `schemaURI` via Wails `GetSchemaAST`.
 *  2. Substrate returns the canonical Schema AST; pass it to
 *     `<UIFormView form={...} ... />` via the lib's `buildUIForms`.
 *  3. User fills the form. Run button enables when client-side
 *     validation passes (zero fields ⇒ enabled immediately).
 *  4. Submit → caller's `onSubmit(value)` (RunTaskPanel JSON-stringifies
 *     and feeds the existing `RunTaskInUniverse` path).
 *  5. Cancel → caller's `onCancel`.
 *
 * Per D3-LIBRARY: no iframe, no postMessage. Direct React embed using
 * the lib that already powers ktw-display's iframe runtime — one
 * renderer, two delivery modes.
 *
 * Error surfacing: if the AST fetch fails (schema URI not registered,
 * transport error, decode error), the substrate's actual message
 * surfaces inline above the form area so the user sees what to fix.
 */

import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Box,
  Button,
  Card,
  Center,
  Group,
  Loader,
  ScrollArea,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import { tablerIcon } from '@ktw/display-lib/ui-core/components/icons/index.ts'
import { IconAlertCircle, IconPlayerPlay, IconWand } from '@tabler/icons-react'

import { buildUIForms, UIFormView } from '@ktw/display-lib/ui-builders/form/index.ts'
import { wireRegistries } from '@ktw/display-lib/registry-wire'
import type { Schema } from '@ktw/display-lib/schema'
import type { ActionMode, UIActionRef } from '@ktw/display-lib/ui-core/action/index.ts'

import { GetSchemaAST } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { TaskIdentity } from './TaskIdentity'
import { formItem } from '../../hooks/formInitial'
import { useT } from '../../i18n'
import { declaredActions, entryTitle } from './entryForm'

// The display library's i18n is the GUI's, set up once at boot in the
// person's language (i18n/index.ts); only its registries are wired here.
wireRegistries()

interface FormWidgetPanelProps {
  /** Active universe (used to route the GetSchemaAST call). */
  universeName: string
  /** Canonical schema URI to render — e.g.
   *  `ktw-demo:/data-type/task/hello-world/input`. */
  schemaURI: string
  /** Which ui-form variant to render. Defaults to `'default'`, which
   *  the lib's `buildUIForms` synthesises for every schema (even when
   *  no `ui-form` extension is declared). Callers asking for a named
   *  variant get the matching extension. */
  formName?: string
  /** Optional header card metadata. When absent the panel renders the
   *  form alone (useful for embedded contexts). */
  header?: {
    title: string
    description?: string
    /** Tabler icon name (without the `Icon` prefix), per the same
     *  convention TaskGalleryPanel uses for task tiles. */
    iconName?: string
    /** Identity lines (Provider / Name / URI) shown under the description. */
    provider?: string
    name?: string
    uri?: string
  }
  /** Action set forwarded to `buildUIForms`. Combined with `actionMode`
   *  to control which buttons render in the form's button row.
   *  - `actionMode: 'replace'` + actions  → exactly those buttons.
   *  - `actionMode: 'replace'` + `[]`     → no buttons (preview mode).
   *  - `actionMode: 'append'` (default)   → these on top of the form's
   *    schema-declared actions; when neither side declares any, the
   *    lib falls back to its standard `defaultFormActions`
   *    (Save / Cancel / Reset / Delete).
   *
   *  Task launcher passes `[Cancel, Run]` with `replace`; Type Browser
   *  previews pass `[]` with `replace` for a chrome-free read of the
   *  form. */
  actionRefs?: UIActionRef[]
  actionMode?: ActionMode
  /** Called with the form's submitted value when validation passes.
   *  Wired to the `submit` action id. No-op when no submit action
   *  surfaces in `actionRefs` / form-declared actions. */
  onSubmit?: (value: Record<string, unknown>) => void
  /** Called when the user presses cancel. Wired to the `cancel`
   *  action id; same no-op rule applies. */
  onCancel?: () => void
  /** When true, the form is disabled and the Run button shows a
   *  loading spinner (the host already kicked off the run). */
  submitting?: boolean
  /** Server-side validation findings from
   *  `SchemaService.ValidateAgainst` (set by the host after a rejected
   *  submit). Each violation maps to a form field via its `path`
   *  (root-type prefix stripped) and surfaces inline under that field;
   *  the full list also renders in an alert above the form so
   *  unmappable paths are never silently dropped. */
  serverViolations?: { severity?: string; code?: string; message: string; path?: string }[]
  /** Values the form opens with - only those of fields it has (a create
   *  task's `parent`). */
  initial?: Record<string, unknown>
  /** The entry form of a create task opened from « + Nouveau », shown in a
   *  dialog (TaskEntryDialog): a header with its line and the form's title
   *  (its group's), no task name nor URI, the form's declared button labels,
   *  no card of its own. */
  entry?: { trail: string }
}

/** Resolve a Tabler icon name to its component. Falls back to a generic
 *  wand for unknown names (the form widget is, after all, a wand). */
function resolveTablerIcon(name?: string) {
  return tablerIcon(name, IconWand)
}

export function FormWidgetPanel({
  universeName,
  schemaURI,
  formName = 'default',
  header,
  actionRefs,
  actionMode,
  onSubmit,
  onCancel,
  // On the field-form path RHF manages the Run button's busy state;
  // the no-parameter path uses this flag directly.
  submitting = false,
  serverViolations,
  initial,
  entry,
}: FormWidgetPanelProps) {
  const [schema, setSchema] = useState<Schema | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const t = useT('chat')

  // Fetch the AST once per (universe, schemaURI). The substrate-side
  // lookup is in-memory (catalog Phase-2 pre-registered the type), so
  // this is a single KTSP round-trip.
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    setSchema(null)
    // Filter to the `ui-form` extension we will render. The lib's
    // form widget picks the "default" variant out of the schema's
    // `ui-form` extensions; asking for just that variant in the
    // server-side filter saves a client-side walk and keeps the
    // wire payload minimal.
    // Filter to the requested `ui-form` variant. `*` for the
    // synthesised 'default' so the substrate doesn't accidentally
    // strip type metadata the layout reads from sibling extensions.
    GetSchemaAST(universeName, schemaURI, 'ui-form', formName === 'default' ? '*' : formName)
      .then((result: main.SchemaASTResult) => {
        if (cancelled) return
        if (result.error) {
          setError(result.error)
          setLoading(false)
          return
        }
        if (!result.ast) {
          setError(t('form.noAst'))
          setLoading(false)
          return
        }
        // Wails returns json.RawMessage as an already-decoded object.
        setSchema(result.ast as unknown as Schema)
        setLoading(false)
      })
      .catch((err) => {
        if (cancelled) return
        setError(String(err))
        setLoading(false)
      })
    return () => { cancelled = true }
  }, [universeName, schemaURI, formName])

  // Build the form definitions from the AST. `buildUIForms` returns a
  // map of named ui-form extensions → ready-to-render UIForms; we
  // pick the `'default'` variant (per the same fallback the lib's
  // form widget uses).
  //
  // Actions + actionMode flow through verbatim to `buildUIForms`.
  // The lib's default (`actionMode: 'append'` + undefined actions)
  // surfaces the form's schema-declared actions; if the schema
  // declares none, it falls back to `defaultFormActions`
  // (Save / Cancel / Reset / Delete). Task launcher passes a
  // [Cancel, Run] pair with 'replace' for a one-shot dialog; the Type
  // Browser preview passes `[]` with 'replace' for a chrome-free
  // read.
  // Zero-field schemas are legal ("this task takes no parameters" —
  // e.g. demo-delegation-tree declares `fields: {}` purely to document
  // that). buildUIForms throws on them, so branch BEFORE building:
  // render a plain Cancel/Run card instead of a field form.
  // SchemaTypeRef is `Type | string` (string = shorthand alias) —
  // narrow to the object form before reading `fields`.
  const rootType = schema ? schema.types?.[schema.root] : undefined
  const hasFields =
    !!rootType &&
    typeof rootType === 'object' &&
    'fields' in rootType &&
    !!rootType.fields &&
    Object.keys(rootType.fields).length > 0

  // An entry form shows its group's title in its header (once) and its
  // declared button labels.
  const lifted = useMemo(() => (entry && schema ? entryTitle(schema, formName) : null), [entry, schema, formName])
  const refs = useMemo(() => (entry && actionRefs ? declaredActions(schema, actionRefs, formName) : actionRefs), [entry, schema, actionRefs, formName])

  const forms = useMemo(() => {
    if (!schema || !hasFields) return null
    try {
      return buildUIForms({ schema: lifted?.schema ?? schema, actionRefs: refs, actionMode })
    } catch (e) {
      setError(t('form.buildFailed', { error: String(e) }))
      return null
    }
  }, [schema, lifted, hasFields, refs, actionMode])

  const form = forms?.[formName] ?? forms?.['default']

  // The value the form opens with: its defaults, with the initial values
  // of the fields it has.
  const item = useMemo(() => {
    if (!form || !rootType || typeof rootType !== 'object' || !('fields' in rootType) || !rootType.fields) return undefined
    return formItem(form.getDefaultValue() as Record<string, unknown> | undefined, initial, Object.keys(rootType.fields))
  }, [form, rootType, initial])

  // Map server violations to RHF field paths. The substrate reports
  // paths anchored at the root type (e.g. `HelloWorldInput.name` or
  // `$.name`); the form's fields are keyed by the bare property path.
  const externalErrors = useMemo(() => {
    if (!serverViolations?.length) return undefined
    const rootPrefix = schema?.root ? schema.root + '.' : null
    const map: Record<string, string> = {}
    for (const v of serverViolations) {
      if (!v.path) continue
      let p = v.path
      if (p.startsWith('$.')) p = p.slice(2)
      if (rootPrefix && p.startsWith(rootPrefix)) p = p.slice(rootPrefix.length)
      map[p] = v.message
    }
    return Object.keys(map).length > 0 ? map : undefined
  }, [serverViolations, schema])

  const HeaderIcon = resolveTablerIcon(header?.iconName)
  // The no-parameter card's run button: the caller's submit label, else « Lancer ».
  const submitRef = refs?.find((r) => typeof r !== 'string' && r.id === 'submit')
  const submitLabel = (submitRef && typeof submitRef !== 'string' && typeof submitRef.label === 'string' && submitRef.label) || t('form.run')

  // Task identity block — rendered INSIDE UIFormView's title slot via
  // `customTitle`. Replaces the lib's default `Details : New` heading
  // so there's a single header in the form card (icon + title +
  // description) instead of two stacked panels.
  const customTitleNode = entry ? (
    <Stack gap={2} style={{ flex: 1, minWidth: 0 }} data-testid="task.entry.header">
      <Text size="xs" c="dimmed" data-testid="task.entry.trail">{entry.trail}</Text>
      <Title order={2} style={{ wordBreak: 'break-word' }}>{lifted?.title || header?.title}</Title>
    </Stack>
  ) : header ? (
    <Group gap="md" align="flex-start" wrap="nowrap" style={{ flex: 1, minWidth: 0 }}>
      <HeaderIcon size={28} color="var(--mantine-color-blue-6)" />
      <Stack gap={2} style={{ flex: 1, minWidth: 0 }}>
        <Title order={3} style={{ wordBreak: 'break-word' }}>{header.title}</Title>
        {header.description && (
          <Text size="sm" c="dimmed">{header.description}</Text>
        )}
        <TaskIdentity provider={header.provider} name={header.name} uri={header.uri} />
      </Stack>
    </Group>
  ) : undefined

  const Frame = entry ? EntryFrame : CardFrame
  const content = (
        <Stack gap="lg">
          {/* Schema fetch state */}
          {loading && (
            <Center py="xl"><Loader size="sm" /></Center>
          )}

          {error && !loading && (
            <Alert icon={<IconAlertCircle size={16} />} color="red" variant="light" title={t('form.schemaUnavailable')}>
              {error}
            </Alert>
          )}

          {/* Server-side rejection — full violation list (catch-all for
              paths that map to no rendered field). Per-field messages
              additionally surface inline via externalErrors below. */}
          {serverViolations && serverViolations.length > 0 && !loading && (
            <Alert icon={<IconAlertCircle size={16} />} color="orange" variant="light" title={t('form.inputRejected')}>
              <Stack gap={2}>
                {serverViolations.map((v, i) => (
                  <Text key={i} size="sm">
                    {v.path ? <Text component="span" fw={600}>{v.path}: </Text> : null}
                    {v.message}
                  </Text>
                ))}
              </Stack>
            </Alert>
          )}

          {/* No-parameter task: schema resolved but declares zero
              fields. A field-form is meaningless — render the header +
              a plain Cancel/Run pair (Run submits an empty value). */}
          {schema && !hasFields && !loading && !error && (
            <Frame>
              <Stack gap="md">
                {customTitleNode}
                <Text size="sm" c="dimmed">
                  {t('form.noParameters')}
                </Text>
                <Group justify="flex-end" gap="sm">
                  {onCancel && (
                    <Button variant="subtle" color="gray" onClick={onCancel} disabled={submitting}>
                      {t('form.cancel')}
                    </Button>
                  )}
                  {onSubmit && (
                    <Button
                      leftSection={<IconPlayerPlay size={14} />}
                      onClick={() => onSubmit({})}
                      loading={submitting}
                    >
                      {submitLabel}
                    </Button>
                  )}
                </Group>
              </Stack>
            </Frame>
          )}

          {/* Form — single header (customTitle = icon+title+description)
              then field groups then [Cancel, Run] from actions override. */}
          {form && !loading && !error && (
            <Frame>
              {/* item is intentionally omitted (NOT `{}`). The lib's
                  `UIForm.getDefaultValue(item)` does `item ? item :
                  defaults` — an empty object is truthy, which would
                  short-circuit the schema's per-field `default:`
                  values out of the form. Omitting `item` lets the
                  defaults flow (greeting = "Hello", loud = false). */}
              <UIFormView
                form={form}
                item={item}
                isNew={true}
                customTitle={customTitleNode}
                externalErrors={externalErrors}
                onSubmit={onSubmit ? (value: Record<string, unknown>) => onSubmit(value) : undefined}
                onCancel={onCancel}
              />
            </Frame>
          )}
        </Stack>
  )
  // An entry form is the dialog's body: no scroll area nor card of its own.
  if (entry) return <Box data-testid="task.form" data-entry="dialog">{content}</Box>
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="lg" data-testid="task.form" style={{ maxWidth: 760, marginInline: 'auto' }}>
        {content}
      </Box>
    </ScrollArea>
  )
}

function CardFrame({ children }: { children: React.ReactNode }) {
  return <Card withBorder padding="md" radius="md">{children}</Card>
}

function EntryFrame({ children }: { children: React.ReactNode }) {
  return <>{children}</>
}
