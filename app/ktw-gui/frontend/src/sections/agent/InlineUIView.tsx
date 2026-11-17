// SPDX-License-Identifier: Apache-2.0
/**
 * InlineUIView — in-process, read-only view rendered DIRECTLY from a schema
 * AST (no iframe, no fetch). Renders the user's submitted answer as a
 * schema-driven view in the chat thread.
 *
 * Read-only chat card: NO action buttons (empty actionRefs + replace) and
 * NO "View : …" heading (hideTitle) — the step title is shown by the
 * surrounding AnswerView message. No outer Card (avoid the double-box).
 */
import { useMemo } from 'react'
import { Text } from '@mantine/core'

import { buildUIView, UIViewView } from '@ktw/display-lib/ui-builders/view/index.ts'
import { wireRegistries } from '@ktw/display-lib/registry-wire'
import type { Schema } from '@ktw/display-lib/schema'
import type { ActionMode } from '@ktw/display-lib/ui-core/action/index.ts'
import { withFieldLabels } from './schema-labels'

// The display library's i18n is the GUI's, set up once at boot in the
// person's language (i18n/index.ts); only its registries are wired here.
wireRegistries()

const NO_ACTIONS: ActionMode = 'replace'

interface InlineUIViewProps {
  schema: Schema
  item: Record<string, unknown>
}

export function InlineUIView({ schema, item }: InlineUIViewProps) {
  const view = useMemo(() => {
    try {
      return buildUIView({ schema: withFieldLabels(schema), viewRef: 'default', actionRefs: [], actionMode: NO_ACTIONS })
    } catch {
      return null
    }
  }, [schema])

  if (!view) {
    return <Text size="sm" c="dimmed">{JSON.stringify(item)}</Text>
  }

  return <UIViewView view={view} item={item} hideTitle enableDetailLevelToggle={false} />
}
