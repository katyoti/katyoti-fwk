// SPDX-License-Identifier: Apache-2.0
/**
 * SessionOutputPanel — schema-driven structured view of a task's
 * output (ms-schema-driven-ui US-S2).
 *
 * Pipeline:
 *  1. Fetch the LLM journal for the session (Wails `GetSessionJournal`).
 *  2. Walk turns → tool calls; pick the latest `complete` tool call.
 *     Its `input` object IS the structured output the agent declared
 *     when it called the completion tool. For hello-world that's
 *     `{ greeting_file: "...", rendered_greeting: "..." }`.
 *  3. Fetch the task's output-schema AST via `GetSchemaAST` (the same
 *     round-trip the input form uses, but pointing at the
 *     `<provider>:/data-type/task/<name>/output` URI).
 *  4. Build a UIView from the schema + render via `@ktw/display-lib`'s
 *     `UIViewView`. Falls back to raw JSON when either the schema URI
 *     isn't published or the task didn't ship an output-schema.
 *
 * Mounted as the "Output" tab inside [SessionViewer](./SessionViewer.tsx)
 * — peer to "Chat" and "Journal". Lazy-loads on tab activation so the
 * journal scan + schema fetch don't run for sessions the user doesn't
 * open the Output tab for.
 */

import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Box,
  Card,
  Center,
  Code,
  Loader,
  ScrollArea,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import { IconAlertCircle } from '@tabler/icons-react'

import {
  GetSessionJournal,
  GetSchemaAST,
  ListSessions,
  ListTasksDetail,
} from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { useT } from '../../i18n'

import { buildUIView, UIViewView } from '@ktw/display-lib/ui-builders/view/index.ts'
import { wireRegistries } from '@ktw/display-lib/registry-wire'
import type { Schema } from '@ktw/display-lib/schema'

// The display library's i18n is the GUI's, set up once at boot in the
// person's language (i18n/index.ts); only its registries are wired here.
wireRegistries()

interface Props {
  universeName: string
  /** Explicit session id (e.g. "2026-04-14T13-29-33_hello-world"). Used
   *  by the Session viewer's tab where the user has clicked a specific
   *  historical session. When omitted, the panel resolves the LATEST
   *  session matching `task` — that's how ChatPanel uses it (the
   *  top-level agent never emits a `session.created` we can hook, so
   *  we look up by name once the run has terminated). */
  sessionId?:   string
  /** Task name. Required when `sessionId` is omitted (used to look up
   *  the matching session); otherwise used to find the task's
   *  outputSchemaURI in ListTasksDetail. */
  task?:        string
}

export function SessionOutputPanel({ universeName, sessionId, task }: Props) {
  const t = useT('shell')
  const [loading, setLoading]   = useState(true)
  const [error, setError]       = useState<string | null>(null)
  const [output, setOutput]     = useState<Record<string, unknown> | null>(null)
  const [schema, setSchema]     = useState<Schema | null>(null)
  const [taskTitle, setTaskTitle] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    const run = async () => {
      setLoading(true)
      setError(null)
      setOutput(null)
      setSchema(null)
      setTaskTitle(null)
      try {
        // 0. Resolve the session id. Caller-supplied wins; otherwise
        //    list the universe's sessions and pick the latest matching
        //    the task name (sessions are sorted newest-first by
        //    `ListSessions`). Used by the ChatPanel Output tab where
        //    the top-level run's session id isn't surfaced to the UI.
        let resolvedSessionId = sessionId
        if (!resolvedSessionId && task) {
          const listResult = await ListSessions(universeName)
          if (cancelled) return
          if (listResult.error) {
            throw new Error(`Resolve session: ${listResult.error}`)
          }
          const match = (listResult.sessions ?? []).find(
            (s) => s.task === task || s.task === bareTaskName(task),
          )
          if (!match) {
            // No session matched — the directory may not exist yet (the
            // recorder lazy-creates on first write). Friendly empty
            // state, not an error.
            setLoading(false)
            return
          }
          resolvedSessionId = match.id
        }
        if (!resolvedSessionId) {
          // Neither sessionId nor task → nothing to look up.
          setLoading(false)
          return
        }

        // 1. Journal — extract the latest `complete` tool call's input.
        const journalResult = await GetSessionJournal(universeName, resolvedSessionId)
        if (cancelled) return
        if (journalResult.error) {
          // A MISSING journal is not an error surface. It happens for: a chat
          // session (its result is the conversation, not a discrete output),
          // a run that died before its first journal write, or one still
          // starting (the recorder lazy-creates sessions/<id>/ on first write).
          // Degrade to the friendly empty state instead of a red "Output
          // unavailable". Only genuinely unexpected errors bubble up.
          if (/not found|no such|does not exist|no journal/i.test(journalResult.error)) {
            setLoading(false)
            return
          }
          throw new Error(journalResult.error)
        }
        const completeInput = extractCompleteInput(journalResult.conversation)
        if (!completeInput) {
          // Session ended without a `complete` call (still running, errored,
          // or older format). Surface a friendly empty state — not an error.
          setLoading(false)
          return
        }
        if (cancelled) return
        setOutput(completeInput)

        // 2. Find the task's outputSchemaURI by name. If the session id
        //    doesn't tell us the task (e.g. resumed sessions), fall back
        //    to the journal's `conv.task`.
        const taskName = task || journalResult.conversation?.task || ''
        if (!taskName) {
          // No task identity → raw JSON fallback. Not an error.
          setLoading(false)
          return
        }
        // The caller may hand us the task URI (`ktw-demo:hello-world` —
        // RunTaskPanel prefers `task.uri` for exact-match dispatch) while
        // TaskInfo.name is the bare name. Match on either, plus the
        // provider-stripped form.
        const tasksResult = await ListTasksDetail(universeName)
        if (cancelled) return
        const bare = bareTaskName(taskName)
        const taskInfo = (tasksResult ?? []).find(
          (t) => t.name === taskName || t.uri === taskName || t.name === bare,
        )
        // Prefer the task's declared title; humanize the bare name otherwise
        // ("hello-world" → "Hello World"). Drives the "Task : <name> — Result"
        // header below.
        setTaskTitle(taskInfo?.title || humanizeTaskName(taskInfo?.name || bare))
        if (!taskInfo?.outputSchemaURI) {
          // Task has no output-schema declared → raw JSON fallback.
          setLoading(false)
          return
        }

        // 3. Schema AST.
        const schemaResult = await GetSchemaAST(
          universeName,
          taskInfo.outputSchemaURI,
          'ui-view',
          'default',
        )
        if (cancelled) return
        if (schemaResult.error) {
          // Schema fetch failed — log + show raw JSON. The output is still
          // visible to the user, just unstructured.
          // eslint-disable-next-line no-console
          console.warn('[SessionOutputPanel] GetSchemaAST:', schemaResult.error)
          setLoading(false)
          return
        }
        if (schemaResult.ast) {
          setSchema(schemaResult.ast as unknown as Schema)
        }
        setLoading(false)
      } catch (e) {
        if (cancelled) return
        setError(String(e))
        setLoading(false)
      }
    }
    void run()
    return () => { cancelled = true }
  }, [universeName, sessionId, task])

  // Build the view from the schema. `buildUIViews` returns a map of
  // ui-view extensions; we pick the default variant (synthesized when
  // the schema doesn't declare any).
  const view = useMemo(() => {
    if (!schema) return null
    try {
      // buildUIView (single) wraps buildUIViews for the default form
      // ref. Identical pattern to FormWidgetPanel's buildUIForms('default').
      //
      // `actionMode: 'replace'` with no actionRefs yields an empty action
      // set, suppressing the default close/edit/delete button row. Those
      // CRUD affordances are synthesized for record views; task output is
      // read-only reporting — there's nothing to edit or delete.
      return buildUIView({ schema, actionMode: 'replace' })
    } catch (e) {
      // eslint-disable-next-line no-console
      console.warn('[SessionOutputPanel] buildUIView failed:', e)
      return null
    }
  }, [schema])

  if (loading) {
    return <Center py="xl"><Loader size="sm" /></Center>
  }
  if (error) {
    return (
      <Box p="md">
        <Alert icon={<IconAlertCircle size={16} />} color="red" variant="light" title={t('sessionOutput.unavailable')}>
          {error}
        </Alert>
      </Box>
    )
  }
  if (!output) {
    return (
      <Center py="xl">
        <Stack align="center" gap={4}>
          <Text size="sm" c="dimmed" fw={500}>{t('sessionOutput.empty')}</Text>
          <Text size="xs" c="dimmed">{t('sessionOutput.notCompleted')}</Text>
        </Stack>
      </Center>
    )
  }

  // Header shown above the output card. Reads "Task : <name> — Result"
  // (e.g. "Task : Hello World — Result"). Replaces the generic "View"
  // heading the UIViewView would otherwise render — task output is a run
  // result, not an editable record.
  const headerTitle = taskTitle ? t('sessionOutput.taskResult', { task: taskTitle }) : t('sessionOutput.result')

  return (
    <ScrollArea style={{ height: '100%' }} scrollbarSize={6}>
      <Box p="md">
        <Stack gap="md">
          {view ? (
            <Card withBorder padding="md" radius="md">
              <Title order={1} mb="md">{headerTitle}</Title>
              {/* Schema-driven structured view — labels, formatting,
                  per-field renderers per the output-schema. The view's own
                  "View" title is suppressed (hideTitle) in favor of the
                  result-framed header above. */}
              <UIViewView
                view={view}
                item={output}
                enableDetailLevelToggle={false}
                hideTitle
              />
            </Card>
          ) : (
            <Card withBorder padding="md" radius="md">
              <Stack gap="xs">
                <Title order={1} mb="xs">{headerTitle}</Title>
                <Text size="xs" c="dimmed" fw={600} tt="uppercase">{t('sessionOutput.raw')}</Text>
                <Text size="xs" c="dimmed">
                  {t('sessionOutput.noSchema')}
                </Text>
                <Code block style={{ whiteSpace: 'pre-wrap', fontSize: 12 }}>
                  {JSON.stringify(output, null, 2)}
                </Code>
              </Stack>
            </Card>
          )}
        </Stack>
      </Box>
    </ScrollArea>
  )
}

/** Strip the provider + slot prefix off a task identifier:
 *  `ktw-demo:hello-world` → `hello-world`,
 *  `ktw-demo:tasks/hello-world` → `hello-world`, bare names pass
 *  through. */
function bareTaskName(n: string): string {
  const afterColon = n.includes(':') ? n.slice(n.indexOf(':') + 1) : n
  const lastSlash = afterColon.lastIndexOf('/')
  return lastSlash >= 0 ? afterColon.slice(lastSlash + 1) : afterColon
}

/** Humanize a bare task name for display: `hello-world` → `Hello World`.
 *  Used only as a fallback when the task declares no `title:`. */
function humanizeTaskName(name: string): string {
  return name
    .split(/[-_]/)
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ')
}

/** Walk the parsed journal for the LATEST `complete` tool call. Its
 *  `input` is the structured payload the agent passed to the completion
 *  tool — that IS the task's output. Returns the decoded object or
 *  null when no complete call was found. */
function extractCompleteInput(
  conv: main.SessionJournalConversation | undefined,
): Record<string, unknown> | null {
  if (!conv?.turns) return null
  // Scan turns from latest backward so a session with multiple completion
  // attempts surfaces the most recent payload.
  for (let i = conv.turns.length - 1; i >= 0; i--) {
    const calls = conv.turns[i].toolCalls ?? []
    for (let j = calls.length - 1; j >= 0; j--) {
      const call = calls[j]
      if (call.name === 'complete' && call.input) {
        // call.input arrives as an already-decoded object (Wails parses
        // the json.RawMessage on its side); guard against a stray string.
        if (typeof call.input === 'string') {
          try {
            return JSON.parse(call.input) as Record<string, unknown>
          } catch {
            return null
          }
        }
        return call.input as unknown as Record<string, unknown>
      }
    }
  }
  return null
}
