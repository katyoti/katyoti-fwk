// SPDX-License-Identifier: Apache-2.0
import { Box, Group, Stack, Text } from '@mantine/core'
import { DEBUG_TESTS } from './debugTests'

interface DebugTreePaneProps {
  activeTest: string | null
  onSelect:   (test: string) => void
}

export function DebugTreePane({ activeTest, onSelect }: DebugTreePaneProps) {

  return (
    <Box
      style={{
        height: '100%',
        background: 'var(--mantine-color-body)',
        borderRight: '1px solid var(--mantine-color-default-border)',
        overflow: 'auto',
      }}
    >
      <Group gap="xs" px="sm" py="xs" style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
        <Text size="sm" fw={600}>Debug tests</Text>
      </Group>

      <Stack gap={0} p="xs">
        {DEBUG_TESTS.map((group) => (
          <Box key={group.id} mb="xs">
            <Text size="xs" c="dimmed" fw={500} px="xs" py={4} tt="uppercase" style={{ letterSpacing: 0.4 }}>
              {group.label}
            </Text>
            {group.tests.map((t) => {
              const active = t.id === activeTest
              return (
                <Box
                  key={t.id}
                  onClick={() => onSelect(t.id)}
                  style={{
                    cursor: 'pointer',
                    padding: '6px 10px',
                    borderRadius: 4,
                    background: active ? 'var(--mantine-color-blue-light)' : 'transparent',
                    color: active ? 'var(--mantine-color-blue-light-color)' : 'inherit',
                    transition: 'background 100ms ease',
                  }}
                  onMouseEnter={(e) => {
                    if (!active) e.currentTarget.style.background = 'var(--mantine-color-default-hover)'
                  }}
                  onMouseLeave={(e) => {
                    if (!active) e.currentTarget.style.background = 'transparent'
                  }}
                >
                  <Text size="sm" fw={active ? 500 : 400}>{t.label}</Text>
                  {t.description && (
                    <Text size="xs" c="dimmed" truncate>{t.description}</Text>
                  )}
                </Box>
              )
            })}
          </Box>
        ))}
      </Stack>
    </Box>
  )
}
