// SPDX-License-Identifier: Apache-2.0
/**
 * FirstRunChoice — the guide's welcome offer on the universe picker
 * (modules/guide/concepts.md, "First run"). Shown until the person has done
 * or declined the welcome tour — whether or not they already have universes,
 * so someone who never took the tour is offered it too. "Show me around"
 * plays it; "I know my way" is remembered as a completion (`skipped`), so the
 * offer does not come back. The wording adapts: no user universe yet (the
 * admin universe is always listed) is a first run.
 */
import { useEffect } from 'react'
import { Button, Group, Paper, Stack, Text } from '@mantine/core'
import { IconMap2 } from '@tabler/icons-react'
import { startRecipe, useGuidePlayer } from './player'
import { completedHas } from './recipes'
import { useGuideT } from './i18n'

/** The welcome tour, carried by the hub: in every universe's catalog. */
export const WELCOME_RECIPE_URI = 'ktw-hub:/guide-recipe/welcome'

export function FirstRunChoice({ userUniverses }: { userUniverses: number }) {
  const completed = useGuidePlayer((s) => s.completed)
  const loaded = useGuidePlayer((s) => s.completionsLoaded)
  const refresh = useGuidePlayer((s) => s.refreshCompletions)
  const complete = useGuidePlayer((s) => s.complete)
  const t = useGuideT()

  useEffect(() => {
    void refresh()
  }, [refresh])

  if (!loaded || completedHas(completed, 'welcome')) return null
  const firstRun = userUniverses === 0

  return (
    <Paper withBorder p="md" radius="lg" style={{ backgroundColor: 'rgba(0,0,0,0.55)' }} data-guide="first-run">
      <Stack gap="xs" align="center">
        <Group gap="xs">
          <IconMap2 size={18} />
          <Text fw={600}>{firstRun ? t('firstRun.titleFirst') : t('firstRun.titleAgain')}</Text>
        </Group>
        <Text size="sm" c="dimmed" ta="center" maw={420}>
          {firstRun ? t('firstRun.textFirst') : t('firstRun.textAgain')}
        </Text>
        <Group gap="sm">
          <Button color="violet" onClick={() => void startRecipe(WELCOME_RECIPE_URI)}>
            {t('firstRun.showMeAround')}
          </Button>
          <Button variant="subtle" color="gray" onClick={() => void complete(WELCOME_RECIPE_URI, 'skipped')}>
            {t('firstRun.knowMyWay')}
          </Button>
        </Group>
      </Stack>
    </Paper>
  )
}
