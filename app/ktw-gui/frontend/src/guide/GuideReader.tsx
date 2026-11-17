// SPDX-License-Identifier: Apache-2.0
/**
 * GuideReader — a Show me's or an Explain me's window:
 * the guide's markdown in the person's language, a Show me's video a click
 * away. Mounted once beside the player, so a guide opens from the browser or
 * by itself at start (a shown-once Welcome) on any route. A long text (three
 * headings or more - a concept document) opens full screen, beside its table
 * of contents.
 */

import { useEffect, useState } from 'react'
import { Button, Group, Modal } from '@mantine/core'
import { IconFileText, IconVideo } from '@tabler/icons-react'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { Markdown } from '../components/Markdown'
import { useAppLanguage } from '../i18n'
import { useGuideT } from './i18n'
import { useGuidePlayer } from './player'
import { useAppStore } from '../store'
import { ADMIN_UNIVERSE } from '../ktsp'
import { fetchSourced, recipeText } from './recipes'
import { openAddress } from '../sections/shell/openAddress'
import { stepAddress } from '../address'
import { headingsOf, TOC_MIN_HEADINGS } from '../components/document/toc'

/** The height of the reader's sticky header (Mantine's modal header). */
const READER_HEADER_PX = 56

export function GuideReader() {
  const reading = useGuidePlayer((s) => s.reading)
  const closeReading = useGuidePlayer((s) => s.closeReading)
  const lang = useAppLanguage()
  const t = useGuideT()
  // A guide whose text is data (What's new) opened from the guide list: its
  // source answers the latest version's text.
  const universe = useAppStore((s) => s.displayedUniverseName) || ADMIN_UNIVERSE
  const [sourced, setSourced] = useState<{ uri: string; markdown: string; document?: string } | null>(null)
  const source = reading?.recipe.source?.trim()
  const needs = !!source && !reading?.recipe.body?.trim()
  useEffect(() => {
    if (!reading || !needs || !source) return
    let cancelled = false
    void fetchSourced(universe)(source, '').then((got) => {
      if (!cancelled) setSourced({ uri: reading.uri, markdown: got?.markdown ?? '', document: got?.document })
    }).catch(() => { if (!cancelled) setSourced({ uri: reading.uri, markdown: '' }) })
    return () => { cancelled = true }
  }, [reading, needs, source, universe])
  if (!reading) return null
  const tx = recipeText(reading, lang)
  if (needs) tx.body = sourced?.uri === reading.uri ? sourced.markdown || t('reader.nothingNew') : ''
  // The whole document the text comes from (an installation's letter, the
  // full change log below it): opened by its address.
  const document = reading.document || (sourced?.uri === reading.uri ? sourced.document : undefined)
  const openDocument = () => {
    if (!document) return
    closeReading()
    void openAddress(stepAddress(universe, `object/${document}`))
  }
  const video = reading.recipe.video
  const long = headingsOf(tx.body ?? '').length >= TOC_MIN_HEADINGS
  return (
    <Modal opened onClose={closeReading} title={tx.name} size="lg" fullScreen={long}>
      {/* The full-screen modal's sticky header is 53 px: the table sticks below it. */}
      {tx.body && <Markdown text={tx.body} toc={long} tocTop={READER_HEADER_PX} />}
      {document && (
        <Group justify="flex-start" mt="md">
          <Button size="xs" variant="light" leftSection={<IconFileText size={14} />} onClick={openDocument}
            data-guide="reader.full-log">
            {t('reader.fullLog')}
          </Button>
        </Group>
      )}
      {video && (
        <Group justify="flex-end" mt="md">
          <Button size="xs" leftSection={<IconVideo size={14} />} onClick={() => BrowserOpenURL(video)}>
            {t('browser.watch')}
          </Button>
        </Group>
      )}
    </Modal>
  )
}
