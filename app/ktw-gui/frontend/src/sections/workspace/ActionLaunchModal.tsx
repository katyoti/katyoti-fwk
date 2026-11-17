// SPDX-License-Identifier: Apache-2.0
import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Group, Loader, Modal, Stack, Text } from '@mantine/core'

import { buildUIForms, UIFormView } from '@ktw/display-lib/ui-builders/form/index.ts'
import { initI18n } from '@ktw/display-lib/ui-core/i18n/index.ts'
import { wireRegistries } from '@ktw/display-lib/registry-wire'
import type { Schema } from '@ktw/display-lib/schema'
import { WorkspaceRunAction } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { dispatcherFor } from '../../ktsp'
import { GuiHostServices } from '../../components/compositions/hostServices'
import { useT } from '../../i18n'
import { errorText } from '../../i18n/errors'

// The form layout resolves its labels through i18n and its field renderers
// through the registries: both must be wired before the first form renders.
void initI18n({ language: 'en' })
wireRegistries()

/** What ActionService.Describe answers that the modal reads. */
interface Described {
  label?: string
  description?: string
  input_schema_ast?: AST
}

/** The canonical DSL AST of an action's input (its root type's fields). */
interface AST {
  name?: string
  root?: string
  types?: Record<string, { type?: string; fields?: Record<string, { optional?: boolean; type?: string }> }>
}

/** freeInput is the action's input without what the launch fills itself —
 *  `node_id` and the values the binding pre-binds — or null when nothing is
 *  left to ask. */
export function freeInput(ast: AST | undefined, bound: Record<string, unknown>): AST | null {
  const root = ast?.root ? ast.types?.[ast.root] : undefined
  if (!ast || !root?.fields) return null
  const fields = Object.fromEntries(
    Object.entries(root.fields).filter(([name]) => name !== 'node_id' && !(name in bound)),
  )
  if (Object.keys(fields).length === 0) return null
  return { ...ast, types: { ...ast.types, [ast.root!]: { ...root, fields } } }
}

/** missingRequired names the free input's required top-level fields that
 *  have no value yet. */
function missingRequired(ast: AST | null, values: Record<string, unknown>): string[] {
  const fields = ast?.root ? ast.types?.[ast.root]?.fields ?? {} : {}
  return Object.entries(fields)
    .filter(([name, f]) => !f.optional && (values[name] === undefined || values[name] === '' || values[name] === null))
    .map(([name]) => name)
}

/**
 * Launch dialog for an object's action: the confirmation gate (`confirm: true`
 * on the binding) and the form of its input — the display library's form fed
 * by the action's input AST (ActionService.Describe `input_schema_ast`), so a
 * nested object or a list of rows is filled like any field. `node_id` and the
 * values the binding pre-binds are filled automatically and not shown.
 */
export function ActionLaunchModal({
  action, universeName, nodeId, onClose, onResult,
}: {
  action: main.WorkspaceQuickAction
  universeName: string
  nodeId: string
  onClose: () => void
  onResult: (r: main.WorkspaceRunActionResult) => void
}) {
  const [describing, setDescribing] = useState(true)
  const [describeError, setDescribeError] = useState('')
  const [input, setInput] = useState<AST | null>(null)
  const [values, setValues] = useState<Record<string, unknown>>({})
  const [running, setRunning] = useState(false)
  const [runError, setRunError] = useState('')
  const t = useT('workspace')

  const bound = useMemo(
    () => (action.boundInput ?? {}) as Record<string, unknown>,
    [action.boundInput],
  )

  useEffect(() => {
    let cancelled = false
    setDescribing(true)
    dispatcherFor(universeName)('ktw-agents:action/describe', { uri: action.uri })
      .then((res) => {
        if (cancelled) return
        setDescribing(false)
        setInput(freeInput((res as Described | undefined)?.input_schema_ast, bound))
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setDescribing(false)
        setDescribeError(String(err instanceof Error ? err.message : err))
      })
    return () => { cancelled = true }
  }, [universeName, action.uri, bound])

  const [formError, setFormError] = useState('')
  const form = useMemo(() => {
    if (!input) return null
    try {
      return buildUIForms({ schema: input as unknown as Schema, actionRefs: [], actionMode: 'replace' })['default'] ?? null
    } catch (e) {
      setFormError(t('launch.buildFailed', { error: String(e) }))
      return null
    }
  }, [input, t])

  const missing = missingRequired(input, values)

  const launch = async () => {
    setRunning(true)
    setRunError('')
    const payload: Record<string, unknown> = { node_id: nodeId, ...bound }
    for (const [k, v] of Object.entries(values)) {
      if (v !== undefined && v !== '') payload[k] = v
    }
    try {
      const result = await WorkspaceRunAction(universeName, action.uri, JSON.stringify(payload))
      setRunning(false)
      if (result.error) {
        setRunError(errorText(result))
        return
      }
      onResult(result)
      onClose()
    } catch (err) {
      setRunning(false)
      setRunError(String(err))
    }
  }

  return (
    <Modal opened onClose={onClose} title={t('launch.title', { name: action.label || action.uri })} centered size={input ? 'lg' : 'md'}>
      <Stack gap="sm">
        {describing && <Group justify="center"><Loader size="sm" /></Group>}
        {describeError && (
          <Alert color="red" variant="light" title={t('launch.cannotInspect')}>
            {describeError}
          </Alert>
        )}
        {!describing && !describeError && (
          <>
            {action.confirm && (
              <Text size="sm">
                {input ? t('launch.confirmWithParams') : t('launch.confirm')}
              </Text>
            )}
            {formError && <Alert color="orange" variant="light">{formError}</Alert>}
            {form && (
              <GuiHostServices universeName={universeName}>
                <UIFormView form={form} isNew hideTitle
                  onChange={(v) => setValues((v ?? {}) as Record<string, unknown>)} />
              </GuiHostServices>
            )}
            {runError && (
              <Alert color="red" variant="light" title={t('launch.failed')}>
                {runError}
              </Alert>
            )}
            <Group justify="flex-end" mt="xs">
              <Button variant="default" onClick={onClose} disabled={running}>{t('common.cancel')}</Button>
              <Button onClick={launch} loading={running} disabled={missing.length > 0} data-guide="action.run"
                title={missing.length > 0 ? t('launch.required', { names: missing.join(', ') }) : undefined}>
                {t('launch.run')}
              </Button>
            </Group>
          </>
        )}
      </Stack>
    </Modal>
  )
}

/**
 * needsLaunchDialog reports whether the action requires the modal at
 * all: a confirm gate, or at least one free (non-pre-filled) input field,
 * nested ones included. Plain actions launch directly with `{node_id, ...bound}`.
 */
export function needsLaunchDialog(
  action: main.WorkspaceQuickAction,
  inputSchema: string,
): boolean {
  if (action.confirm) return true
  return schemaToFields(inputSchema, (action.boundInput ?? {}) as Record<string, unknown>).length > 0
}

/** schemaToFields lists the free fields of a JSON Schema input (its root
 *  object, or the definition its `$ref` names), skipping node_id (auto-filled)
 *  and any pre-bound keys. */
export function schemaToFields(
  inputSchema: string,
  bound: Record<string, unknown>,
): string[] {
  if (!inputSchema) return []
  type Obj = { $ref?: string; properties?: Record<string, unknown>; $defs?: Record<string, Obj>; definitions?: Record<string, Obj> }
  let parsed: Obj
  try {
    parsed = JSON.parse(inputSchema) as Obj
  } catch {
    return []
  }
  let root: Obj | undefined = parsed
  if (parsed.$ref) {
    const name = parsed.$ref.replace(/^#\/(\$defs|definitions)\//, '')
    root = parsed.$defs?.[name] ?? parsed.definitions?.[name]
  }
  if (!root?.properties) return []
  return Object.keys(root.properties).filter((name) => name !== 'node_id' && !(name in bound))
}
