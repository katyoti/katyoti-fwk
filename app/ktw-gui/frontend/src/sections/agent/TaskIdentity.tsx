// SPDX-License-Identifier: Apache-2.0
/**
 * TaskIdentity — the Provider / Name / URI lines shown under a task's
 * title + description in the run dialog. Each line renders only when its
 * value is present; the value is a copyable monospace Code.
 */
import { Code, Group, Stack, Text } from '@mantine/core'
import { useT } from '../../i18n'

interface TaskIdentityProps {
  provider?: string
  name?: string
  uri?: string
}

export function TaskIdentity({ provider, name, uri }: TaskIdentityProps) {
  const t = useT('chat')
  if (!provider && !name && !uri) return null
  return (
    <Stack gap={2} mt={4}>
      {provider && <IdentityLine label={t('identity.provider')} value={provider} />}
      {name && <IdentityLine label={t('identity.name')} value={name} />}
      {uri && <IdentityLine label={t('identity.uri')} value={uri} />}
    </Stack>
  )
}

function IdentityLine({ label, value }: { label: string; value: string }) {
  return (
    <Group gap={6} wrap="nowrap">
      <Text size="xs" c="dimmed" w={64} style={{ flexShrink: 0 }}>{label}</Text>
      <Code style={{ fontSize: 11 }}>{value}</Code>
    </Group>
  )
}
