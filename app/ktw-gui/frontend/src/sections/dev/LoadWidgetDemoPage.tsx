// SPDX-License-Identifier: Apache-2.0
// LoadWidgetDemoPage — the standalone loading-screen widget (`kind="load"`).
//
// Mounts the bun `/widget/load` route, which always renders the shared
// "assistant is working" loading screen. Doubles as a smoke test of the
// <IframeWidget> primitive (URL resolution, mount, the `ready` handshake,
// resize) since the loader posts the same lifecycle messages.

import { useCallback, useEffect, useRef, useState } from 'react'
import { ActionIcon, Code, ScrollArea, Stack, Text, TextInput, Tooltip } from '@mantine/core'
import { IconTrash } from '@tabler/icons-react'
import {
  IframeWidget,
  type IframeWidgetStatus,
} from '../../components/iframe-widget'
import type { IframeMessage } from '@display/postmessage'
import { DevTestChrome } from './DevTestChrome'

const DEV_PORT_KEY = 'display.devPort'
const DEV_PORT_DEFAULT = '9990'

interface LoggedEvent {
  ts: number
  msg: IframeMessage
}

export function LoadWidgetDemoPage() {
  const [iframeKey, setIframeKey] = useState(0)
  const [status, setStatus] = useState<IframeWidgetStatus>('loading')
  const [statusDetail, setStatusDetail] = useState<string | undefined>()
  const [events, setEvents] = useState<LoggedEvent[]>([])
  const [devPort, setDevPort] = useState<string>(
    () => window.localStorage.getItem(DEV_PORT_KEY) ?? DEV_PORT_DEFAULT,
  )
  const eventViewportRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const v = eventViewportRef.current
    if (!v) return
    v.scrollTo({ top: v.scrollHeight, behavior: 'smooth' })
  }, [events.length])

  useEffect(() => {
    if (devPort && /^\d+$/.test(devPort)) {
      window.localStorage.setItem(DEV_PORT_KEY, devPort)
    }
  }, [devPort])

  const onIframeMessage = useCallback((msg: IframeMessage) => {
    setEvents((prev) => [...prev.slice(-19), { ts: Date.now(), msg }])
  }, [])
  const onStatusChange = useCallback((s: IframeWidgetStatus, detail?: string) => {
    setStatus(s)
    setStatusDetail(detail)
  }, [])
  const onReload = () => {
    setEvents([])
    setIframeKey((k) => k + 1)
  }

  const controls = (
    <TextInput
      size="xs"
      value={devPort}
      onChange={(e) => setDevPort(e.currentTarget.value)}
      w={90}
      placeholder={DEV_PORT_DEFAULT}
      styles={{ input: { fontSize: 11 } }}
    />
  )

  return (
    <DevTestChrome
      title="load-widget"
      status={status}
      statusDetail={statusDetail}
      controls={controls}
      eventCount={events.length}
      eventLabel="messages"
      onReload={onReload}
      content={
        <IframeWidget
          key={iframeKey}
          kind="load"
          onStatusChange={onStatusChange}
          onIframeMessage={onIframeMessage}
          fillHeight
          extraParams={{ display: 'ui' }}
        />
      }
      schema={
        <Text size="xs" c="dimmed" p="md">
          The <Code>load</Code> widget renders the shared loading screen on its
          own — no schema, no payload.
        </Text>
      }
      events={
        <Stack gap="xs">
          <Tooltip label="Clear events">
            <ActionIcon variant="subtle" size="sm" onClick={() => setEvents([])} style={{ alignSelf: 'flex-end' }}>
              <IconTrash size={14} />
            </ActionIcon>
          </Tooltip>
          <ScrollArea
            h={300}
            viewportRef={eventViewportRef}
            style={{ background: 'var(--mantine-color-dark-9)', borderRadius: 4 }}
          >
            <Stack gap={2} p="xs">
              {events.length === 0 ? (
                <Text size="xs" c="dimmed">
                  no events yet — the iframe should emit <Code>ready</Code> and <Code>resize</Code> on load
                </Text>
              ) : (
                events.map((e, i) => (
                  <Code key={`${e.ts}-${i}`} block style={{ fontSize: 11 }}>
                    {new Date(e.ts).toLocaleTimeString()} — {e.msg.type} — {JSON.stringify(e.msg)}
                  </Code>
                ))
              )}
            </Stack>
          </ScrollArea>
        </Stack>
      }
    />
  )
}
