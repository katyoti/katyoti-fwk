// SPDX-License-Identifier: Apache-2.0
/**
 * ConversionMark — the project panel's header says when the served project's
 * data waits for this KaTyoti's conversion, with « Convertir »;
 * data newer than this KaTyoti says to update KaTyoti. Nothing
 * shows at this KaTyoti's version. Read from CWM's ProjectsKnown, again each
 * time the universe's views are read again and each time the workspace's data
 * changes (a conversion committed by another process). The dialog is drawn apart from
 * the mark, on the project it was opened on: the conversion's re-read finds
 * the project up to date and drops the mark, the dialog stays with its end
 * until the person closes it.
 */
import { useEffect, useState, type ReactNode } from 'react'
import { Badge, Button, Tooltip } from '@mantine/core'
import { IconArrowsExchange } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { dispatcherFor } from '../../ktsp'
import { useT } from '../../i18n'
import { ConversionDialog } from './ConversionDialog'
import { CONVERSION_BADGE, CONVERT_ACTION, PROJECTS_KNOWN, conversionHeader, servedProject, type KnownProject } from './conversion'

export function ConversionMark({ universe }: { universe: string }) {
  const t = useT('shell')
  const revision = useAppStore((s) => s.universeRevision)
  const dataRevision = useAppStore((s) => s.workspaceTreeRevision)
  const [project, setProject] = useState<KnownProject | undefined>(undefined)
  const [converting, setConverting] = useState<KnownProject | null>(null)

  useEffect(() => {
    let live = true
    dispatcherFor(universe)(PROJECTS_KNOWN, {})
      .then((out) => { if (live) setProject(servedProject(out)) })
      .catch(() => { if (live) setProject(undefined) })
    return () => { live = false }
  }, [universe, revision, dataRevision])

  const { mark, dialog } = conversionHeader(project, converting)
  let badge: ReactNode = null
  if (mark.kind === 'newer') {
    badge = (
      <Tooltip label={t('conversion.newerHint', { from: mark.from })} withArrow multiline w={260}>
        <Badge size="xs" color="red" variant="light" data-element={CONVERSION_BADGE} data-state="newer">{t('conversion.newer')}</Badge>
      </Tooltip>
    )
  } else if (mark.kind === 'waiting' && project) {
    badge = (
      <>
        <Tooltip label={t('conversion.waitingHint', { from: mark.from })} withArrow multiline w={260}>
          <Badge size="xs" color="orange" variant="light" data-element={CONVERSION_BADGE} data-state="waiting">{t('conversion.waiting')}</Badge>
        </Tooltip>
        <Button size="compact-xs" variant="light" color="orange" leftSection={<IconArrowsExchange size={12} />} onClick={() => setConverting(project)} data-element={CONVERT_ACTION}>
          {t('conversion.convert')}
        </Button>
      </>
    )
  }
  // One shape whatever the mark: the dialog keeps its place (and its end)
  // when the re-read drops the mark.
  return (
    <>
      {badge}
      {dialog && <ConversionDialog universe={universe} project={dialog} opened onClose={() => setConverting(null)} />}
    </>
  )
}
