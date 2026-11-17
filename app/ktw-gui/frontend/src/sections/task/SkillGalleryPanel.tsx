// SPDX-License-Identifier: Apache-2.0
/**
 * SkillGalleryPanel — sibling of TaskGalleryPanel for the Skills tab.
 *
 * Card grid showing all skills available in the active universe (composite:
 * core + activated sidecars). Same search + tag-chip filter UX as the Tasks
 * tab. Skills are universe-wide today, so there's no node scoping.
 *
 * Card click is a no-op for v1 — this is a discoverability surface. A future
 * iteration may open a read-only instructions viewer.
 */
import { useEffect, useMemo, useState } from 'react'
import {
  Badge,
  Box,
  Card,
  Chip,
  Group,
  Loader,
  ScrollArea,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { EmptyState } from '@ktw/display-lib/ui-core/components/other/index.ts'
import { tablerIcon } from '@ktw/display-lib/ui-core/components/icons/index.ts'
import { IconBook2, IconSearch } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { categoryToColor } from '../../utils/categoryColor'
import { ListSkills } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { useListPrefs } from '../../hooks/useListPrefs'
import { sortItems } from './listPrefs'
import { SortSelect } from '../workspace/SearchableCardGrid'
import { useT } from '../../i18n'

function resolveTablerIcon(iconName?: string) {
  return tablerIcon(iconName, IconBook2)
}

function collectHashtags(skills: main.SkillInfo[]): string[] {
  const counts = new Map<string, number>()
  for (const s of skills) {
    for (const tag of s.hashtags ?? []) {
      counts.set(tag, (counts.get(tag) ?? 0) + 1)
    }
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([tag]) => tag)
}

function filterSkills(
  skills: main.SkillInfo[],
  query: string,
  selectedTags: string[]
): main.SkillInfo[] {
  const q = query.trim().toLowerCase()
  return skills.filter((s) => {
    if (selectedTags.length > 0) {
      const tags = s.hashtags ?? []
      for (const sel of selectedTags) {
        if (!tags.includes(sel)) return false
      }
    }
    if (!q) return true
    const haystack = [
      s.name,
      s.provider ?? '',
      s.description ?? '',
      s.category ?? '',
      ...(s.hashtags ?? []),
    ]
      .join(' ')
      .toLowerCase()
    return haystack.includes(q)
  })
}

export function SkillGalleryPanel() {
  const t = useT('screens')
  const universeName = useAppStore((s) => s.displayedUniverseName)

  const [skills, setSkills] = useState<main.SkillInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState('')
  const [selectedTags, setSelectedTags] = useState<string[]>([])

  useEffect(() => {
    let cancelled = false
    if (!universeName) {
      setSkills([])
      setLoading(false)
      return () => { cancelled = true }
    }
    setLoading(true)

    // Retry-on-empty: same race as TaskGalleryPanel. Catalog hydrates
    // shortly after Activate returns; first fetch can see 0 entries.
    const attemptFetch = (attempt: number) => {
      ListSkills(universeName)
        .then((list) => {
          if (cancelled) return
          const arr = list ?? []
          if (arr.length === 0 && attempt < 2) {
            window.setTimeout(() => {
              if (!cancelled) attemptFetch(attempt + 1)
            }, 500)
            return
          }
          setSkills(arr)
          setLoading(false)
        })
        .catch((err) => {
          console.warn('[SkillGalleryPanel] ListSkills failed:', err)
          if (!cancelled) {
            setSkills([])
            setLoading(false)
          }
        })
    }
    attemptFetch(0)

    return () => {
      cancelled = true
    }
  }, [universeName])

  const allTags = useMemo(() => collectHashtags(skills), [skills])
  const listPrefs = useListPrefs(universeName)
  const sort = listPrefs.sortOf('skills')
  const visible = useMemo(
    () => sortItems(filterSkills(skills, query, selectedTags).map((s) => ({ ...s, source: s.provider })), sort),
    [skills, query, selectedTags, sort]
  )

  if (loading) {
    return (
      <Group justify="center" py="xl">
        <Loader size="sm" />
      </Group>
    )
  }

  return (
    <Stack gap="sm" p="md" style={{ height: '100%', minHeight: 0 }}>
      <Stack gap="xs" style={{ flexShrink: 0 }}>
        <Group justify="flex-end" align="center">
          <Text size="xs" c="dimmed">
            {t('skills.count', { visible: visible.length, total: skills.length })}
          </Text>
        </Group>

        <Group gap="xs" wrap="nowrap" align="center">
          <TextInput
            size="sm"
            placeholder={t('skills.search')}
            leftSection={<IconSearch size={14} />}
            value={query}
            onChange={(e) => setQuery(e.currentTarget.value)}
            style={{ flex: 1, minWidth: 0 }}
          />
          <SortSelect value={sort} onChange={(k) => listPrefs.setSort('skills', k)} />
        </Group>

        {allTags.length > 0 && (
          <Chip.Group multiple value={selectedTags} onChange={setSelectedTags}>
            <Group gap={4}>
              {allTags.map((tag) => (
                <Chip key={tag} value={tag} size="xs" variant="outline">
                  {tag}
                </Chip>
              ))}
            </Group>
          </Chip.Group>
        )}
      </Stack>

      <ScrollArea style={{ flex: 1, minHeight: 0 }}>
        {visible.length === 0 ? (
          skills.length > 0
            ? <EmptyState message={t('skills.noMatch')} />
            : <EmptyState icon="book-2" iconSize={48} message={t('skills.none')} />
        ) : (
          <SimpleGrid cols={{ base: 1, sm: 2, lg: 3, xl: 4 }} spacing="sm">
            {visible.map((skill) => (
              <SkillCard key={`${skill.provider}:${skill.name}`} skill={skill} />
            ))}
          </SimpleGrid>
        )}
      </ScrollArea>
    </Stack>
  )
}

interface SkillCardProps {
  skill: main.SkillInfo
}

function SkillCard({ skill }: SkillCardProps) {
  const Icon = resolveTablerIcon(skill.iconName)
  const categoryColor = categoryToColor(skill.category)

  return (
    <Card
      withBorder
      shadow="sm"
      padding="sm"
      radius="md"
      style={{ height: '100%' }}
    >
      <Stack gap="xs" style={{ height: '100%' }}>
        <Group gap="xs" align="flex-start" wrap="nowrap">
          <Box style={{ flexShrink: 0, paddingTop: 2 }}>
            <Icon size={20} />
          </Box>
          <Stack gap={2} style={{ flex: 1, minWidth: 0 }}>
            <Text size="sm" fw={600} truncate>
              {skill.name}
            </Text>
            {skill.provider && (
              <Text size="xs" c="dimmed" truncate>
                {skill.provider}
              </Text>
            )}
          </Stack>
        </Group>

        {skill.description && (
          <Text size="xs" c="dimmed" lineClamp={3} style={{ flex: 1 }}>
            {skill.description}
          </Text>
        )}

        <Group gap={4} wrap="wrap">
          {skill.category && (
            <Badge size="xs" variant="filled" color={categoryColor}>
              {skill.category}
            </Badge>
          )}
          {(skill.hashtags ?? []).slice(0, 4).map((tag) => (
            <Badge key={tag} size="xs" variant="dot" color="gray">
              {tag}
            </Badge>
          ))}
          {(skill.hashtags ?? []).length > 4 && (
            <Text size="xs" c="dimmed">
              +{(skill.hashtags ?? []).length - 4}
            </Text>
          )}
        </Group>
      </Stack>
    </Card>
  )
}


