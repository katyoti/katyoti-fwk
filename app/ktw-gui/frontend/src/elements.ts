// SPDX-License-Identifier: Apache-2.0
/**
 * The GUI's elements — every element it draws itself has ONE id,
 * `<place>:<name>` (the study gui.element-addresses §2): `<place>`
 * says where it is drawn, `<name>` names its declaration (a built-in's name,
 * a contribution's file stem). Every segment is kebab; an id never changes
 * with the label or the language.
 *
 * This is the one list the GUI reads to draw, hide and guide: the display
 * levels, a universe's `gui.hide` and its view (its tags' elements)
 * name elements by these ids, and each drawn element carries
 * its id as `data-element`. An
 * element contributed by a pack (a global panel, a task's quick action, an
 * object's tab, a type « + New » offers, a project type offered where a
 * project is made: `project:<type>`) has an id of the same form built from
 * its declaration (`elementId`).
 */

/** Where an element is drawn. */
export const PLACES = ['rail', 'home', 'quick', 'tab', 'section', 'badge', 'screen', 'new', 'project', 'action', 'field'] as const
export type Place = (typeof PLACES)[number]

/** An element the GUI draws itself. */
export interface GuiElement {
  id: string
  /** False when the Essentiel display level hides it (a technical surface, a developer tool). */
  essential?: false
}

/** The built-in elements, by place. */
export const ELEMENTS: readonly GuiElement[] = [
  // The rail, top to bottom.
  { id: 'rail:sidebar' },
  { id: 'rail:add-universe' },
  { id: 'rail:chat' },
  { id: 'rail:inbox' },
  // Its count: « À faire »'s, behind its display switch.
  { id: 'rail:todo-count' },
  // The panels screen's way in, behind its display switch.
  { id: 'rail:panels' },
  { id: 'rail:marketplace', essential: false },
  { id: 'rail:debug', essential: false },
  { id: 'rail:feedback' },
  { id: 'rail:dev-console', essential: false },
  { id: 'rail:camera' },
  { id: 'rail:record' },
  { id: 'rail:settings' },
  { id: 'rail:profile' },
  // Home's strip: its own entries (a global panel is `home:<its stem>`).
  { id: 'home:actions' },
  { id: 'home:runs', essential: false },
  { id: 'home:errors', essential: false },
  { id: 'home:chat' },
  // Home's built-in quick actions (a task's is `quick:task/<provider>/<task>`).
  { id: 'quick:term' },
  { id: 'quick:drawio' },
  { id: 'quick:debug', essential: false },
  { id: 'quick:start-chat' },
  // An object's badges and the tree's.
  { id: 'badge:aspect', essential: false },
  { id: 'badge:capability', essential: false },
  { id: 'badge:tree' },
  // An object's technical tabs (every declared panel is `tab:<its stem>`).
  { id: 'tab:links', essential: false },
  { id: 'tab:agent-context', essential: false },
  { id: 'tab:lifecycle', essential: false },
  // An object's « Actions »: at every level, never hidden.
  { id: 'tab:actions' },
  // « Prêt à démarrer »: the screen, its tabs, the team's keys
  // import, the missing keys' form and the installation's projects folder.
  { id: 'screen:ready' },
  // « À faire », the one place: the rail's Inbox opens it,
  // its count the rail's badge - behind its display switch.
  { id: 'screen:todo' },
  // The panels screen: the open panels and the one selected,
  // behind its display switch.
  { id: 'screen:panels' },
  // « Vérifier l'installation »: the way into the screen that is
  // always there - the universe's « … » menus; never hidden (universeGui).
  { id: 'action:universe/check-install' },
  // « Redémarrer l'univers », on a running universe, behind
  // its display switch.
  { id: 'action:universe/restart' },
  { id: 'tab:ready/profile' },
  { id: 'tab:ready/programs' },
  { id: 'tab:ready/keys' },
  { id: 'action:ready/import-keys' },
  // A keys file, after the team password when the solution has key sets.
  { id: 'action:ready/import-keys-file' },
  { id: 'action:ready/profile' },
  { id: 'section:ready/profile-form' },
  { id: 'action:ready/profile-save' },
  // The profile's fields, typed in place by a proof.
  { id: 'field:ready/profile-handle' },
  { id: 'field:ready/profile-first-name' },
  { id: 'field:ready/profile-last-name' },
  { id: 'field:ready/profile-email' },
  { id: 'section:ready/keys-form' },
  { id: 'section:ready/projects-folder' },
  { id: 'action:ready/projects-folder-keep' },
  { id: 'action:ready/projects-folder-pick' },
  { id: 'section:ready/earlier-projects' },
  { id: 'action:ready/import-projects' },
  { id: 'section:ready/earlier-installations' },
  { id: 'field:ready/installation-keep' },
  { id: 'action:ready/installation-delete' },
  { id: 'field:ready/installation-ack' },
  { id: 'action:ready/installation-delete-confirm' },
  // A universe's projects: a new project from its name.
  { id: 'action:universe/new-project' },
  { id: 'section:universe/new-project' },
  { id: 'action:universe/create-project' },
  // A project whose data is older than KaTyoti: its mark, « Convertir », the
  // dialog and its confirmation.
  { id: 'badge:project/conversion' },
  { id: 'action:project/convert' },
  { id: 'section:project/conversion-dialog' },
  { id: 'action:project/conversion-confirm' },
]

/** « Vérifier l'installation »: no level, `gui.hide` or tag view leaves it out. */
export const CHECK_INSTALL_ELEMENT = 'action:universe/check-install'

/** « Redémarrer l'univers ». */
export const RESTART_UNIVERSE_ELEMENT = 'action:universe/restart'

/** An object's « Actions » tab: no level, `gui.hide` or tag view leaves it out. */
export const ACTIONS_TAB_ELEMENT = 'tab:actions'

const KEBAB = /^[a-z0-9]+(-[a-z0-9]+)*$/

/** An element's id from its place and its name: `rail:chat`, `home:mes-dossiers`. */
export function elementId(place: Place, name: string): string {
  return `${place}:${name}`
}

/** An element's place, from its id; undefined for no element id. */
export function placeOf(id: string): Place | undefined {
  const place = id.slice(0, id.indexOf(':'))
  return (PLACES as readonly string[]).includes(place) ? (place as Place) : undefined
}

/** Whether an id is an element id: a known place and a kebab name, its path
 *  segments (`/`) kebab each. */
export function isElementId(id: string): boolean {
  const i = id.indexOf(':')
  if (i < 0 || !placeOf(id)) return false
  return id.slice(i + 1).split('/').every((seg) => KEBAB.test(seg))
}

/** The quick action of a task: `quick:task/<provider>/<task>` from its URI
 *  (`ktw-acme:tasks/new-study`). */
export function taskQuickId(uri: string): string {
  const [provider, path] = uri.split(':', 2)
  return elementId('quick', `task/${provider}/${(path ?? '').replace(/^tasks?\//, '')}`)
}

/** A Home strip entry's element id from its strip id: `home:runs`; a global
 *  panel is answered as `home:<stem>` already. */
export function homeElementId(entryId: string): string {
  return entryId.startsWith('home:') ? entryId : elementId('home', entryId.replace(/^panel:/, ''))
}

/**
 * An object tab's id from a reference to it: a full id
 * (`tab:links`, `tab:children/record`, `section:fields`) as is; a panel's stem
 * (`links`, what a page's navigate, an Inbox item or an aspect names) or a
 * former strip id (`panel:links`, kept in a saved tab) as `tab:<stem>`.
 */
export function objectTabId(ref: string): string {
  if (/^(tab|section):/.test(ref)) return ref
  return elementId('tab', ref.replace(/^panel:/, ''))
}

/** An object section's element id: `section:<stem>` from its
 *  tab id (`section:fields` as is, `panel:<stem>` or `tab:<stem>` re-placed). */
export function objectSectionId(ref: string): string {
  if (ref.startsWith('section:')) return ref
  return elementId('section', ref.replace(/^(panel|tab):/, ''))
}

/** An object action's element id: `action:<provider>/<path>` from
 *  the method it runs (`ktw-agents:plans/finish` is
 *  `action:ktw-agents/plans/finish`), as a task's quick action is named. */
export function objectActionId(uri: string): string {
  const [provider, path] = uri.split(':', 2)
  return elementId('action', `${provider}/${path ?? ''}`)
}

/** What the Essentiel display level hides: the elements marked so. */
export const ESSENTIAL_HIDDEN: ReadonlySet<string> = new Set(ELEMENTS.filter((e) => e.essential === false).map((e) => e.id))

/** A task's URI from its quick action's id (`quick:task/<provider>/<task>`
 *  is `<provider>:tasks/<task>`); '' for no task's quick action. */
export function taskURIOf(quickId: string): string {
  const task = /^quick:task\/([^/]+)\/(.+)$/.exec(quickId)
  return task ? `${task[1]}:tasks/${task[2]}` : ''
}
