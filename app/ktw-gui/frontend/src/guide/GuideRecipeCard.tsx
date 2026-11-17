// SPDX-License-Identifier: Apache-2.0
/**
 * GuideRecipeCard — what a ```guide-recipe fenced block in a chat message
 * renders (modules/guide, decision OD-013). The agent offers a recipe by
 * writing its URI in such a block; the card shows the recipe once it is read
 * from the catalog — so an invented URI shows nothing to start — and its
 * button plays it in this GUI.
 */
import { useEffect, useState } from 'react'
import { Badge, Button, Group, Paper, Stack, Text } from '@mantine/core'
import { IconMap2, IconPlayerPlay } from '@tabler/icons-react'
import { useAppStore } from '../store'
import { startRecipe } from './player'
import { fetchRecipe, isTour, recipeText, type RecipeEntry } from './recipes'
import { useGuideT } from './i18n'
import { useAppLanguage } from '../i18n'

/** The recipe URI of a block body: a `uri:` line, else the first non-empty line. */
export function recipeURIOf(block: string): string {
  const lines = block.split('\n').map((l) => l.trim()).filter(Boolean)
  const tagged = lines.find((l) => l.toLowerCase().startsWith('uri:'))
  return (tagged ? tagged.slice(4) : lines[0] ?? '').trim()
}

export function GuideRecipeCard({ block }: { block: string }) {
  const uri = recipeURIOf(block)
  const displayed = useAppStore((s) => s.displayedUniverseName)
  const [entry, setEntry] = useState<RecipeEntry | null>(null)
  const [error, setError] = useState<string | null>(null)
  const t = useGuideT()
  const lang = useAppLanguage()

  useEffect(() => {
    let live = true
    setEntry(null)
    setError(null)
    if (!uri.includes(':/guide-recipe/')) {
      setError('not a guide recipe')
      return
    }
    fetchRecipe(uri, displayed)
      .then((e) => { if (live) setEntry(e) })
      .catch((e) => { if (live) setError(e instanceof Error ? e.message : String(e)) })
    return () => { live = false }
  }, [uri, displayed])

  if (error) {
    return (
      <Text size="xs" c="dimmed" my={6}>
        {t('card.unavailableBefore')} <code>{uri}</code> {t('card.unavailableAfter')}
      </Text>
    )
  }
  if (!entry) {
    return <Text size="xs" c="dimmed" my={6}>{t('card.lookingUp')}</Text>
  }
  const r = entry.recipe
  const tx = recipeText(entry, lang)
  return (
    <Paper withBorder radius="md" p="sm" my={8} data-guide="chat.recipe-card">
      <Stack gap={6}>
        <Group gap="xs" wrap="nowrap">
          <IconMap2 size={16} />
          <Text fw={600} size="sm">{tx.name}</Text>
          <Badge size="xs" variant="light">{isTour(r) ? t('card.tour') : t('card.recipe')}</Badge>
          <Badge size="xs" variant="outline">{t('card.steps', { count: r.steps.length })}</Badge>
        </Group>
        {tx.description && <Text size="xs" c="dimmed">{tx.description}</Text>}
        <Group>
          <Button size="xs" leftSection={<IconPlayerPlay size={14} />} onClick={() => void startRecipe(uri)}>
            {t('card.startThis')}
          </Button>
        </Group>
      </Stack>
    </Paper>
  )
}
