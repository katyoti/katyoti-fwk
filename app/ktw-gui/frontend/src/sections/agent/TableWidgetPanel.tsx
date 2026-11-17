// SPDX-License-Identifier: Apache-2.0
/**
 * TableWidgetPanel — read-only sibling of `FormWidgetPanel` and
 * `ViewWidgetPanel`. Renders any data-type schema as a
 * `@ktw/display-lib` UITable. Used by the Type Browser's "Try as
 * table" action and reusable wherever else we want to surface a
 * schema-driven tabular view.
 *
 * Mounting:
 *  1. Fetch the AST via Wails `GetSchemaAST` (filtered to the
 *     requested `ui-table` variant; `*` for the synthesised default).
 *  2. Build the UITable via `@ktw/display-lib`'s `buildUITable`.
 *  3. Render `UITableView` with the supplied `data` (defaults to `[]`
 *     for type-discovery contexts — the user still sees the column
 *     headers + empty-state message).
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

import { buildUITable, UITableView } from '@ktw/display-lib/ui-builders/table/index.ts'
import { wireRegistries } from '@ktw/display-lib/registry-wire'
import type { Schema } from '@ktw/display-lib/schema'

import { GetSchemaAST } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'
import type { main } from '../../../wailsjs/go/models'

// The display library's i18n is the GUI's, set up once at boot in the
// person's language (i18n/index.ts); only its registries are wired here.
wireRegistries()

interface TableWidgetPanelProps {
  universeName: string
  schemaURI:    string
  /** Named ui-table extension to render. Defaults to `'default'`
   *  (the lib's builder always synthesises one even when no
   *  ui-table extension is declared). */
  tableName?:   string
  header?: {
    title: string
    description?: string
    iconName?: string
  }
  /** Rows to display. Defaults to `[]` — the table still renders its
   *  column headers + a "no rows" empty state. */
  data?: Record<string, unknown>[]
}

function resolveTablerIcon(name?: string) {
  return tablerIcon(name, IconShape)
}

export function TableWidgetPanel({
  universeName,
  schemaURI,
  tableName = 'default',
  header,
  data = [],
}: TableWidgetPanelProps) {
  const [schema, setSchema] = useState<Schema | null>(null)
  const [error, setError]   = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const t = useT('chat')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    setSchema(null)
    GetSchemaAST(universeName, schemaURI, 'ui-table', tableName === 'default' ? '*' : tableName)
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
  }, [universeName, schemaURI, tableName])

  const table = useMemo(() => {
    if (!schema) return null
    try {
      return buildUITable({ schema, tableRef: tableName })
    } catch (e) {
      setError(t('form.buildTableFailed', { error: String(e) }))
      return null
    }
  }, [schema, tableName])

  const HeaderIcon = resolveTablerIcon(header?.iconName)

  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="lg" style={{ maxWidth: 980, marginInline: 'auto' }}>
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

          {table && !loading && !error && (
            <Card withBorder padding="md" radius="md">
              <UITableView
                table={table}
                data={data}
              />
            </Card>
          )}
        </Stack>
      </Box>
    </ScrollArea>
  )
}
