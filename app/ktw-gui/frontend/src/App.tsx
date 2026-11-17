// SPDX-License-Identifier: Apache-2.0
import { useHotkeys } from '@mantine/hooks'
import { useAgentEvents } from './hooks/useAgentEvents'
import { useApplySettings } from './hooks/useApplySettings'
import { useChatLauncher } from './hooks/useChatLauncher'
import { useProfileSync } from './hooks/useProfileSync'
import { useSettingsPersistence } from './hooks/useSettingsPersistence'
import { useWorkspaceEvents } from './hooks/useWorkspaceEvents'
import { useContribTextsSync } from './i18n/contrib'
import { BaseLayout } from './layout/BaseLayout'
import { AppSettingsModal } from './sections/settings/AppSettingsModal'
import { FeedbackModal } from './sections/feedback/FeedbackModal'
import { ProfileModal } from './sections/profile/ProfileModal'
import { UniverseCreationModal } from './sections/univers/UniverseCreationModal'
import { ToastStack } from './sections/shell/ToastStack'
import { TaskEntryDialog } from './sections/agent/TaskEntryDialog'
// A page's drawer action can open an object's panel.
import './sections/workspace/PanelDrawer'
// … or read a file of an object in place.
import './sections/workspace/FileReaderDrawer'

export function App() {
  useAgentEvents()
  useWorkspaceEvents()
  useSettingsPersistence()
  useApplySettings()
  useProfileSync()
  useContribTextsSync()

  // Global Ctrl+I → opens the workspace chat (same target as the
  // chat icon in [IconRibbon.tsx]). Both surfaces use useChatLauncher
  // so the disabled-when-no-universe gating lives in one place.
  const { launchChat } = useChatLauncher()
  useHotkeys([['mod+i', () => { void launchChat() }]])

  return (
    <>
      <BaseLayout />
      <UniverseCreationModal />
      {/* Before the settings modal: "Set the folder" opens settings on top of it. */}
      <FeedbackModal />
      <AppSettingsModal />
      <ProfileModal />
      {/* « + Nouveau »'s entry form, over the object. */}
      <TaskEntryDialog />
      <ToastStack />
    </>
  )
}
