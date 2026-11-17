// SPDX-License-Identifier: Apache-2.0
/**
 * TreeSettingsMenu — the projects tree's display settings: an
 * icon in the project panel's header opening a menu of the tree's settings,
 * kept in the GUI settings so they come back at the next start. « Show
 * types » is the first; another tree setting is one more item.
 */
import { ActionIcon, Checkbox, Menu, Tooltip } from '@mantine/core'
import { IconAdjustmentsHorizontal } from '@tabler/icons-react'
import { useT } from '../../i18n'
import { useTreePrefs } from '../../hooks/useTreePrefs'
import { TREE_SETTINGS, TREE_SETTING_SHOW_TYPES } from './treeSettings'

export function TreeSettingsMenu() {
  const t = useT('workspace')
  const { showTypes, setShowTypes } = useTreePrefs()
  return (
    <Menu position="bottom-end" shadow="md" width={200} closeOnItemClick={false}>
      <Menu.Target>
        <Tooltip label={t('tree.settings')} withArrow>
          <ActionIcon variant="subtle" color="gray" size="xs" aria-label={t('tree.settings')} data-guide={TREE_SETTINGS}>
            <IconAdjustmentsHorizontal size={14} />
          </ActionIcon>
        </Tooltip>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>{t('tree.settings')}</Menu.Label>
        <Menu.Item
          data-guide={TREE_SETTING_SHOW_TYPES}
          aria-checked={showTypes}
          role="menuitemcheckbox"
          leftSection={<Checkbox size="xs" checked={showTypes} readOnly tabIndex={-1} styles={{ input: { cursor: 'pointer' } }} />}
          onClick={() => setShowTypes(!showTypes)}
        >
          {t('tree.showTypes')}
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  )
}
