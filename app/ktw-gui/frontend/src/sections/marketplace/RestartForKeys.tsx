// SPDX-License-Identifier: Apache-2.0
/**
 * RestartForKeys — shown once keys were written into a universe: its programs
 * read their .env when they start, so a running universe sees the new keys
 * only after a restart (until then a provider gets the literal `${{env:KEY}}`).
 * Restarting is offered, never done behind the user's back.
 */

import { useState } from 'react'
import { Alert, Button, Group, Text } from '@mantine/core'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'

export function RestartForKeys({ universe, onDone }: { universe: string; onDone?: () => void }) {
  const t = useT('marketplace')
  const running = useAppStore((s) => s.universes.find((u) => u.name === universe)?.busRunning)
  const [busy, setBusy] = useState(false)
  if (!running) return null

  const restart = async () => {
    setBusy(true)
    try {
      const st = useAppStore.getState()
      await st.deactivateUniverse(universe)
      await st.activateUniverse(universe)
      onDone?.()
    } finally {
      setBusy(false)
    }
  }

  return (
    <Alert color="yellow" variant="light" mt="xs" mb="sm" data-guide="keys.restart">
      <Group justify="space-between" wrap="nowrap" gap="xs">
        <Text size="xs">{t('keysRestart.hint', { universe })}</Text>
        <Button size="xs" variant="light" color="yellow" loading={busy} onClick={() => void restart()}>
          {t('keysRestart.button')}
        </Button>
      </Group>
    </Alert>
  )
}
