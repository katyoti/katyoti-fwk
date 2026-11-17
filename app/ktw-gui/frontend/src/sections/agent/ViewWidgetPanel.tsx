// SPDX-License-Identifier: Apache-2.0
/**
 * ViewWidgetPanel — read-only sibling of `FormWidgetPanel`. Renders any
 * data-type schema as a `@ktw/display-lib` UIView. Used by the Type
 * Browser's "Try as view" action and reusable wherever else we need
 * to surface a schema-driven view of an arbitrary value.
 *
 * Mounting:
 *  1. Fetch the AST via Wails `GetSchemaAST` (with the `ui-view`
 *     extension filter when a named variant is requested; `*` when
 *     the caller wants the synthesized default that the lib's
 *     builder always provides).
 *  2. Build the UIView via `@ktw/display-lib`'s `buildUIView`.
 *  3. Render `UIViewView` with the supplied `item` (or `{}` when the
 *     caller doesn't have one — the lib's default empty-strategy
 *     surfaces an em-dash per field).
 *
 * Mirrors FormWidgetPanel's i18n bootstrap + error-surfacing semantics —
 * one code path, two delivery modes (input form vs read-only view).
 */

import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Box,
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
import { IconAlertCircle, IconShape } from '@tabler/icons-react'

import { buildUIView, UIViewView } from '@ktw/display-lib/ui-builders/view/index.ts'
import { wireRegistries } from '@ktw/display-lib/registry-wire'
import type { Schema } from '@ktw/display-lib/schema'

import { GetSchemaAST } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'
import type { main } from '../../../wailsjs/go/models'

// The display library's i18n is the GUI's, set up once at boot in the
// person's language (i18n/index.ts); only its registries are wired here.
wireRegistries()

interface ViewWidgetPanelProps {
  universeName: string
  schemaURI:    string
  /** Named ui-view extension to render. Defaults to `'default'`
   *  (the lib's builder always synthesizes one, even when the schema
   *  declares no ui-view extension). */
  viewName?:    string
  /** Optional header card metadata. When absent the panel renders the
   *  view alone. */
  header?: {
    title: string
    description?: string
    iconName?: string
  }
  /** Value to display. Defaults to `{}` — useful for type discovery
   *  (the user sees the layout even with no concrete instance). */
  item?: Record<string, unknown>
}

function resolveTablerIcon(name?: string) {
  return tablerIcon(name, IconShape)
}

export function ViewWidgetPanel({
  universeName,
  schemaURI,
  viewName = 'default',
  header,
  item = {},
}: ViewWidgetPanelProps) {
  const [schema, setSchema] = useState<Schema | null>(null)
  const [error, setError]   = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const t = useT('chat')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    setSchema(null)
    // Pass the requested ui-view variant so the substrate filters the
    // AST to only that extension. For the synthesized 'default' (no
    // declared ui-view), we ask for `*` so we don't accidentally strip
    // the type's metadata if the schema happens to ship some other
    // extension kind that the lib's default layout also reads.
    GetSchemaAST(universeName, schemaURI, 'ui-view', viewName === 'default' ? '*' : viewName)
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
        setSchema(result.ast as unknown as Schema)
        setLoading(false)
      })
      .catch((err) => {
        if (cancelled) return
        setError(String(err))
        setLoading(false)
      })
    return () => { cancelled = true }
  }, [universeName, schemaURI, viewName])

  // buildUIViews returns a map keyed by extension name; the lib's
  // builder always synthesizes a 'default' entry, so a missing variant
  // falls back gracefully.
  const view = useMemo(() => {
    if (!schema) return null
    try {
      return buildUIView({ schema, viewRef: viewName })
    } catch (e) {
      setError(t('form.buildViewFailed', { error: String(e) }))
      return null
    }
  }, [schema, viewName])

  const HeaderIcon = resolveTablerIcon(header?.iconName)

  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="lg" style={{ maxWidth: 760, marginInline: 'auto' }}>
        <Stack gap="lg">
          {header && (
            <Card withBorder padding="md" radius="md">
              <Group gap="md" align="flex-start">
                <HeaderIcon size={28} color="var(--mantine-color-blue-6)" />
                <Stack gap={4} style={{ flex: 1 }}>
                  <Title order={3}>{header.title}</Title>
                  {header.description && (
                    <Text size="sm" c="dimmed">{header.description}</Text>
                  )}
                </Stack>
              </Group>
            </Card>
          )}

          {loading && (
            <Center py="xl"><Loader size="sm" /></Center>
          )}

          {error && !loading && (
            <Alert icon={<IconAlertCircle size={16} />} color="red" variant="light" title={t('form.schemaUnavailable')}>
              {error}
            </Alert>
          )}

          {view && !loading && !error && (
            <Card withBorder padding="md" radius="md">
              <UIViewView
                view={view}
                item={item}
                enableDetailLevelToggle={false}
              />
            </Card>
          )}
        </Stack>
      </Box>
    </ScrollArea>
  )
}
