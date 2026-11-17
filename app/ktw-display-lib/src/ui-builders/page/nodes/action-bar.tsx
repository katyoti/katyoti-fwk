// SPDX-License-Identifier: Apache-2.0
// Renderer of the page DSL's `action-bar` node: a bar kept at
// the bottom of the page while it scrolls, its one-line summary computed live
// from the page's working data - what a picker ticked, the sources, the
// sections, an estimate - and its buttons, the last the main one. Each button
// runs with the summary's values as its element: `${item.<part>}` in its args
// and label, `when` and `disabledWhen` (`missing`: a required part at 0).

import { dataAttrs } from './data-attrs.ts'
import { Fragment, type ReactNode } from 'react'
import { Button, Group, Paper, Text, Tooltip } from '@mantine/core'

import { usePageContext, useRunAction } from '../page-context.tsx'
import { barSummary, type UIActionBarNode } from '../ui-page.interactive.ts'
import { actionDisabled, actionLabel, actionShown } from './interactive.tsx'

const NO_HOST = 'Runs in a panel'

export function ActionBarRenderer({ node }: { node: UIActionBarNode }): ReactNode {
  const { draft } = usePageContext()
  const runOf = useRunAction()
  const { values, shown } = barSummary(node, draft)
  if (node.showWhen && !values[node.showWhen]) return null
  const actions = (node.actions ?? []).filter((a) => actionShown(a, values))
  return (
    <Paper {...dataAttrs(node)} p="sm" radius="md" bg="dark.7" c="white" data-element={node.id ? `action-bar:${node.id}` : undefined}
      style={node.sticky === false ? undefined : { position: 'sticky', bottom: 12, zIndex: 5 }}>
      <Group justify="space-between" wrap="wrap" gap="sm">
        <Text size="sm" data-guide="action-bar.summary">
          {shown.map((p, i) => (
            <Fragment key={p.id}>
              {i > 0 && ' · '}
              <span data-guide="action-bar.part" data-guide-name={p.id}>
                {p.before}{p.strong ? <b>{p.value}</b> : p.value}{p.after}
              </span>
            </Fragment>
          ))}
        </Text>
        <Group gap="xs" wrap="nowrap">
          {actions.map((a, i) => {
            const run = runOf(a)
            const why = actionDisabled(a, values)
            const main = i === actions.length - 1
            const btn = (
              <Button key={a.id ?? i} size={main ? 'md' : 'sm'} variant={main ? 'white' : 'subtle'} color={main ? 'dark' : 'gray.3'}
                disabled={!run || why !== undefined} onClick={() => run?.(values)}
                data-guide="page.action" data-guide-name={a.uri ?? a.id} data-guide-label={a.label}>
                {actionLabel(a.label, values) || a.id || 'Run'}
              </Button>
            )
            const tip = why ?? (run ? undefined : NO_HOST)
            return tip ? <Tooltip key={a.id ?? i} label={tip} withArrow multiline w={260}><span>{btn}</span></Tooltip> : btn
          })}
        </Group>
      </Group>
    </Paper>
  )
}
