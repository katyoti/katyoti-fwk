// SPDX-License-Identifier: Apache-2.0
/**
 * MarketplaceRoot — the ONE Marketplace entry: everything that can be installed,
 * in five tabs, under one header (the subscriptions — marketplaces and their
 * trust — shared by every tab, and a reload):
 *
 *   - Infra       the control plane (katyoti, ktw-hub, ktw-gui): its versions,
 *                 update / roll back / repair, and the snapshots with the infra
 *                 version each was made with.
 *   - Sidecars    the capabilities: install, configure, wire into the universe
 *                 on screen, versions, doctor, updates.
 *   - Config-sets configuration and keys, applied to the universe on screen.
 *   - Seeds       a universe's content applied to the universe on screen.
 *   - Solutions   fixed versions of a working installation (infra, sidecars,
 *                 config-sets, seed, vaults, documents), installed whole.
 *
 * The section lives in the tab's payload (MarketplaceTabPayload), so a launcher
 * can open the Marketplace on the section it means (the « API Keys » menu item
 * opens Config-sets). Global actions (subscriptions, install, infra) need no
 * universe; a universe action names the universe it acts on.
 */

import { useMemo, useState } from 'react'
import { ActionIcon, Box, Button, Group, Tabs, Title, Tooltip } from '@mantine/core'
import { IconRefresh, IconSettings } from '@tabler/icons-react'
import { marketplaceClient } from '../../ktsp'
import type { MarketplaceSection } from '../../store/tabsSlice'
import { useT } from '../../i18n'
import { InfraPanel } from './InfraPanel'
import { SidecarsPanel } from './SidecarsPanel'
import { SeedsPanel } from './SeedsPanel'
import { SolutionsPanel } from './SolutionsPanel'
import { SubscriptionsDrawer } from './SubscriptionsDrawer'
import { ConfigSetsPanel } from '../configsets/ConfigSetsPanel'

export const MARKETPLACE_SECTIONS: MarketplaceSection[] = ['infra', 'sidecars', 'configsets', 'seeds', 'solutions']

interface Props {
  section: MarketplaceSection
  onSection: (s: MarketplaceSection) => void
}

export function MarketplaceRoot({ section, onSection }: Props) {
  const t = useT('marketplace')
  const mkt = useMemo(() => marketplaceClient(), [])
  const [drawer, setDrawer] = useState(false)
  const [count, setCount] = useState(0)
  // Bumped when the subscriptions change or on Reload: every tab reads again.
  const [reloadKey, setReloadKey] = useState(0)
  const reload = () => setReloadKey((k) => k + 1)

  return (
    <Box p="md" style={{ height: '100%', overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
      <Group justify="space-between" mb="xs">
        <Title order={4}>{t('title')}</Title>
        <Group gap="xs">
          <Button variant="light" size="xs" leftSection={<IconSettings size={14} />} onClick={() => setDrawer(true)} data-guide="marketplace.subscriptions">
            {t('subscriptions.button', { count })}
          </Button>
          <Tooltip label={t('reload')}>
            <ActionIcon variant="subtle" onClick={reload} aria-label={t('reload')}><IconRefresh size={16} /></ActionIcon>
          </Tooltip>
        </Group>
      </Group>

      <Tabs value={section} onChange={(v) => v && onSection(v as MarketplaceSection)} keepMounted={false}
        style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
        <Tabs.List mb="sm">
          {MARKETPLACE_SECTIONS.map((s) => (
            <Tabs.Tab key={s} value={s} data-guide="marketplace.tab" data-guide-name={s}>{t(`tabs.${s}`)}</Tabs.Tab>
          ))}
        </Tabs.List>
        <Tabs.Panel value="infra" style={{ flex: 1, minHeight: 0, overflowY: 'auto' }}>
          <InfraPanel mkt={mkt} reloadKey={reloadKey} onSection={onSection} />
        </Tabs.Panel>
        <Tabs.Panel value="sidecars" style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
          <SidecarsPanel mkt={mkt} reloadKey={reloadKey} />
        </Tabs.Panel>
        <Tabs.Panel value="configsets" style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
          <ConfigSetsPanel key={reloadKey} />
        </Tabs.Panel>
        <Tabs.Panel value="seeds" style={{ flex: 1, minHeight: 0 }}>
          <SeedsPanel mkt={mkt} reloadKey={reloadKey} />
        </Tabs.Panel>
        <Tabs.Panel value="solutions" style={{ flex: 1, minHeight: 0 }}>
          <SolutionsPanel mkt={mkt} reloadKey={reloadKey} />
        </Tabs.Panel>
      </Tabs>

      <SubscriptionsDrawer mkt={mkt} opened={drawer} onClose={() => setDrawer(false)} onChanged={reload} onCount={setCount} />
    </Box>
  )
}
