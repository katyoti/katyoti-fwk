// SPDX-License-Identifier: Apache-2.0
/**
 * ContentTemplatePanel — a content template (a « trame », a `content-template`
 * object) shown as the storyline it is: its parts in reading order, each with
 * what it must establish, where its facts come from, how to write it and its
 * deck and document hints. The section groups it includes are expanded in
 * place and framed with the group's name — they are changed in their group.
 *
 * The template and its expansion come from ContentTemplateObject (docsapi
 * Parts); the view writes nothing.
 */

import { useEffect, useState, type ReactNode } from 'react'
import { Alert, Badge, Box, Card, Center, Group, Loader, Paper, Stack, Text, ThemeIcon, Title, Tooltip } from '@mantine/core'
import {
  IconAlertTriangle, IconBox, IconFile, IconFiles, IconForms, IconLink, IconListDetails, IconPresentation,
  IconFileText, IconSitemap, IconStack2, IconTag,
} from '@tabler/icons-react'
import { ContentTemplateObject } from '../../../wailsjs/go/main/App'
import type { api, main } from '../../../wailsjs/go/models'
import { useT } from '../../i18n'

const SOURCE_ICONS: Record<string, ReactNode> = {
  field: <IconTag size={12} />,
  children: <IconSitemap size={12} />,
  'named-file': <IconFile size={12} />,
  files: <IconFiles size={12} />,
  links: <IconLink size={12} />,
  object: <IconBox size={12} />,
  form: <IconForms size={12} />,
}

/** Consecutive parts that came through the same groups, shown together. */
interface Run { groups: string[]; parts: { part: api.TemplatePart; n: number }[] }

function runsOf(parts: api.TemplatePart[]): Run[] {
  const out: Run[] = []
  parts.forEach((part, i) => {
    const groups = part.groups ?? []
    const last = out[out.length - 1]
    if (last && last.groups.join('/') === groups.join('/')) last.parts.push({ part, n: i + 1 })
    else out.push({ groups, parts: [{ part, n: i + 1 }] })
  })
  return out
}

export function ContentTemplatePanel({ universeName, objectId }: { universeName: string; objectId: string }) {
  const t = useT('workspace')
  const [data, setData] = useState<main.ContentTemplateObjectResult | null>(null)

  useEffect(() => {
    let cancelled = false
    setData(null)
    ContentTemplateObject(universeName, objectId)
      .then((r) => { if (!cancelled) setData(r) })
      .catch((e) => { if (!cancelled) setData({ error: e instanceof Error ? e.message : String(e) } as main.ContentTemplateObjectResult) })
    return () => { cancelled = true }
  }, [universeName, objectId])

  if (!data) return <Center p="xl"><Loader size="sm" /></Center>
  if (data.error) return <Alert m="md" color="red" variant="light" icon={<IconAlertTriangle size={16} />}>{data.error}</Alert>

  const tpl = data.template
  const parts = data.parts ?? []
  return (
    <Stack gap="md" p="md">
      <Group justify="space-between" align="flex-start" wrap="nowrap">
        <Group gap="sm" wrap="nowrap" align="flex-start">
          <ThemeIcon size="lg" radius="xl" variant="light" color="indigo"><IconListDetails size={18} /></ThemeIcon>
          <div>
            <Title order={4}>{tpl.label || tpl.name}</Title>
            {tpl.description ? <Text size="sm" c="dimmed">{tpl.description}</Text> : null}
          </div>
        </Group>
        <Group gap={6} wrap="wrap" justify="flex-end">
          {tpl.partial ? (
            <Tooltip label={t('trame.partialHint')} withArrow><Badge color="grape" variant="light">{t('trame.partial')}</Badge></Tooltip>
          ) : null}
          {tpl.purpose ? <Badge variant="outline" color="gray">{t('trame.purpose')} · {tpl.purpose}</Badge> : null}
          <Badge variant="outline" color="gray">{t('trame.target')} · {tpl.target || t('trame.anyTarget')}</Badge>
          <Badge variant="light" color="indigo">{t('trame.parts', { count: parts.length })}</Badge>
        </Group>
      </Group>

      {data.expandError ? (
        <Alert color="orange" variant="light" icon={<IconAlertTriangle size={16} />}>{t('trame.expandError', { error: data.expandError })}</Alert>
      ) : null}
      {parts.length === 0 ? <Text size="sm" c="dimmed">{t('trame.empty')}</Text> : null}

      {runsOf(parts).map((run, i) => run.groups.length === 0 ? (
        <Stack key={i} gap="sm">{run.parts.map(({ part, n }) => <PartCard key={part.section.id} part={part} n={n} />)}</Stack>
      ) : (
        <Paper key={i} withBorder radius="md" p="sm" style={{ borderStyle: 'dashed', borderColor: 'var(--mantine-color-grape-4)' }}>
          <Tooltip label={t('trame.includedHint', { group: run.groups[run.groups.length - 1] })} withArrow position="top-start">
            <Group gap={6} mb="xs">
              <IconStack2 size={14} color="var(--mantine-color-grape-6)" />
              <Text size="xs" fw={600} c="grape">{t('trame.included', { group: run.groups.join(' › ') })}</Text>
            </Group>
          </Tooltip>
          <Stack gap="sm">{run.parts.map(({ part, n }) => <PartCard key={part.section.id} part={part} n={n} />)}</Stack>
        </Paper>
      ))}
    </Stack>
  )
}

function PartCard({ part, n }: { part: api.TemplatePart; n: number }) {
  const t = useT('workspace')
  const s = part.section
  if (s.include) {
    // Not expanded (the reason is above): the include as written.
    return (
      <Card withBorder radius="md" padding="sm">
        <Group gap="xs"><IconStack2 size={14} /><Text size="sm">{t('trame.include', { group: s.include })}</Text></Group>
      </Card>
    )
  }
  const weight = s.weight ?? 0
  return (
    <Card withBorder radius="md" padding="md">
      <Group justify="space-between" wrap="nowrap" align="flex-start" mb={6}>
        <Group gap="sm" wrap="nowrap" align="flex-start">
          <ThemeIcon size="md" radius="xl" variant="filled" color="indigo"><Text size="xs" fw={700}>{n}</Text></ThemeIcon>
          <div>
            <Text fw={600}>{s.title || s.id}</Text>
            <Text size="xs" c="dimmed" ff="monospace">{s.id}</Text>
          </div>
        </Group>
        <Group gap={6} wrap="nowrap">
          {s.required ? (
            <Tooltip label={t('trame.requiredHint')} withArrow multiline w={260}><Badge color="red" variant="light">{t('trame.required')}</Badge></Tooltip>
          ) : null}
          {weight > 0 ? (
            <Tooltip label={t('trame.weight', { n: weight })} withArrow>
              <Group gap={2} wrap="nowrap">
                {[1, 2, 3, 4, 5].map((k) => (
                  <Box key={k} w={8} h={8} style={{ borderRadius: 4, background: k <= weight ? 'var(--mantine-color-indigo-6)' : 'var(--mantine-color-default-border)' }} />
                ))}
              </Group>
            </Tooltip>
          ) : null}
        </Group>
      </Group>

      {s.intent ? (
        <Box mb="xs">
          <Text size="xs" tt="uppercase" c="dimmed" fw={600}>{t('trame.intent')}</Text>
          <Text size="sm">{s.intent}</Text>
        </Box>
      ) : null}

      <Box mb="xs">
        <Text size="xs" tt="uppercase" c="dimmed" fw={600}>{t('trame.sources')}</Text>
        {(s.sources ?? []).length === 0 ? <Text size="xs" c="dimmed">{t('trame.noSources')}</Text> : (
          <Stack gap={4} mt={2}>
            {(s.sources ?? []).map((src, i) => (
              <Group key={i} gap={6} wrap="nowrap" align="flex-start">
                <Badge size="sm" variant="light" color="teal" leftSection={SOURCE_ICONS[src.from]} style={{ flexShrink: 0 }}>
                  {t(`trame.source.${src.from}`, { defaultValue: src.from })}{src.value ? ` · ${src.value}` : ''}
                </Badge>
                {src.note ? <Text size="xs">{src.note}</Text> : null}
              </Group>
            ))}
          </Stack>
        )}
      </Box>

      {s.guidance ? (
        <Box mb="xs" p="xs" style={{ borderLeft: '3px solid var(--mantine-color-indigo-3)', background: 'var(--mantine-color-default-hover)', borderRadius: 4 }}>
          <Text size="xs" tt="uppercase" c="dimmed" fw={600}>{t('trame.guidance')}</Text>
          <Text size="sm" style={{ whiteSpace: 'pre-wrap' }}>{s.guidance}</Text>
        </Box>
      ) : null}

      {s.repeat || s.deck?.layout || s.deck?.group || s.doc?.level ? (
        <Group gap={6}>
          {s.repeat ? <Badge size="sm" variant="outline" leftSection={<IconSitemap size={12} />}>{t('trame.repeat', { type: s.repeat })}</Badge> : null}
          {s.deck?.layout ? <Badge size="sm" variant="outline" color="orange" leftSection={<IconPresentation size={12} />}>{t('trame.deckLayout', { layout: s.deck.layout })}</Badge> : null}
          {s.deck?.group ? <Badge size="sm" variant="outline" color="orange" leftSection={<IconStack2 size={12} />}>{t('trame.deckGroup', { group: s.deck.group })}</Badge> : null}
          {s.doc?.level ? <Badge size="sm" variant="outline" color="blue" leftSection={<IconFileText size={12} />}>{t('trame.docLevel', { level: s.doc.level })}</Badge> : null}
        </Group>
      ) : null}
    </Card>
  )
}
