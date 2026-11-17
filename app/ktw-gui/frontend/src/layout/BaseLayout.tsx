// SPDX-License-Identifier: Apache-2.0
import { Split } from '@gfazioli/mantine-split-pane'
import { useAppStore } from '../store'
import { useUniverses } from '../hooks/useUniverses'
import { IconRibbon }    from '../sections/shell/IconRibbon'
import { ProjectPanel }  from '../sections/shell/ProjectPanel'
import { MainPaneTabs }   from '../sections/shell/MainPaneTabs'
import { UniversePicker } from '../pages/UniversePicker'
import { UniverseEditPane } from '../pages/UniverseEditPane'
import { UniverseDeletePane } from '../pages/UniverseDeletePane'
import { CameraOverlay } from '../sections/media/CameraOverlay'
import { DevPanel } from '../sections/dev/DevPanel'
import { TerminalView } from '../sections/terminal/TerminalView'
import { SessionViewer } from '../sections/shell/SessionViewer'
import { GuideAtStart, GuideDrawer, GuidePlayer, GuideReader } from '../guide'
import { FirstGuide } from '../guide/FirstGuide'

/**
 * Root layout:
 *   Fixed IconRibbon (44px) | Main pane (router-driven content)
 *
 * The main pane content is driven by store.mainPaneRoute:
 *   - welcome           → UniversePicker (welcome / empty-state grid)
 *   - universe          → existing ProjectPanel + MainArea (chat/agent)
 *   - edit/<name>       → UniverseEditPane
 *   - delete/<name>     → UniverseDeletePane
 *
 * The optional left ProjectPanel only renders inside the `universe` route —
 * other routes use the full main-pane width for focused tasks (welcome
 * onboarding, edit form, delete confirmation).
 */
export function BaseLayout() {
  useUniverses()

  const projectPaneOpen = useAppStore((s) => s.projectPaneOpen)
  const route           = useAppStore((s) => s.mainPaneRoute)

  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      <IconRibbon />

      {/* Floating camera PiP — renders only when the camera is on.
          Position-fixed at root so it sits above any route. */}
      <CameraOverlay />

      {/* In-app developer console (console + IPC + errors), toggled from the
          ribbon's terminal icon. Position-fixed at root, above all routes. */}
      <DevPanel />

      {/* The guide: the player plays a recipe on any route (picker included);
          the drawer is the guide where there are no tabs. */}
      <GuidePlayer />
      <GuideReader />
      <GuideAtStart />
      <FirstGuide />
      <GuideDrawer />

      <div style={{ flex: 1, minWidth: 0, height: '100%' }}>
        {route.kind === 'welcome' && <UniversePicker />}

        {route.kind === 'edit' && <UniverseEditPane name={route.name} />}

        {route.kind === 'delete' && <UniverseDeletePane name={route.name} />}

        {route.kind === 'universe' && (
          projectPaneOpen ? (
            <Split style={{ height: '100%' }}>
              <Split.Pane initialWidth={260} minWidth={200} maxWidth={400}>
                <ProjectPanel />
              </Split.Pane>
              <Split.Resizer />
              <Split.Pane grow>
                <MainPaneTabs />
              </Split.Pane>
            </Split>
          ) : (
            <MainPaneTabs />
          )
        )}

        {route.kind === 'terminal' && (
          <Split style={{ height: '100%' }}>
            <Split.Pane initialWidth={260} minWidth={200} maxWidth={400}>
              <ProjectPanel />
            </Split.Pane>
            <Split.Resizer />
            <Split.Pane grow>
              <TerminalView id={route.id} />
            </Split.Pane>
          </Split>
        )}

        {route.kind === 'session' && (
          projectPaneOpen ? (
            <Split style={{ height: '100%' }}>
              <Split.Pane initialWidth={260} minWidth={200} maxWidth={400}>
                <ProjectPanel />
              </Split.Pane>
              <Split.Resizer />
              <Split.Pane grow>
                <SessionViewer universeName={route.universeName} sessionId={route.sessionId} />
              </Split.Pane>
            </Split>
          ) : (
            <SessionViewer universeName={route.universeName} sessionId={route.sessionId} />
          )
        )}

      </div>
    </div>
  )
}

