// SPDX-License-Identifier: Apache-2.0
/**
 * SessionTeamPanel — the Team tab for a (cold) session in the SessionViewer.
 *
 * The live ChatPanel renders <TeamPanel/> fed by live team.* events; for a
 * cold/opened session there are no live events, so we hydrate the team store
 * from GetSessionTaskTree (the journal projection — now built from the
 * framework task.started/task.output events, so it includes agent_as_human
 * children) and point the view at this session's scope.
 */

import { TeamPanel } from '../team/TeamPanel'

interface Props {
  universeName: string
  sessionId:    string
}

// SessionTeamPanel — the Team tab for a finished session. It simply drives the
// shared TeamPanel in COLD mode: TeamPanel.coldSessionId is the sole hydration
// authority (fetch journal task-tree → hydrate → point the view at it), so
// switching sessions never needs a manual refresh and the live runningSessionId
// reconstruction path stays disabled here.
export function SessionTeamPanel({ sessionId }: Props) {
  return <TeamPanel coldSessionId={sessionId} />
}
