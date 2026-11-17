// SPDX-License-Identifier: Apache-2.0
/**
 * Guide anchors — how a guide player finds the controls a recipe points at.
 *
 * A step finds
 * its control by its anchor ONLY, never by role and visible label: labels are
 * GUI text, and the GUI is translated. Every control a recipe points at
 * carries `data-guide`. This registry is the one list of them: a Go test
 * (internal/test/guide/anchors_test.go) refuses a recipe that targets an id
 * missing here, and an id here that its file does not place.
 *
 * Convention — two attributes, never more:
 *   - `data-guide`      names the KIND of control (`marketplace.row`).
 *   - `data-guide-name` names the INSTANCE in a dynamic list (`ktw-example`).
 * So a recipe targets `{guide: "marketplace.row", name: "ktw-example"}`, i.e.
 * `[data-guide="marketplace.row"][data-guide-name="ktw-example"]`. A static
 * control carries `data-guide` alone.
 *
 * `GUIDE_ANCHORS` lists every `data-guide` id the GUI places, the file that
 * places it, and which of the first recipes use it (1 create/open universe,
 * 2 attach project, 3 tree/tabs, 4 actions, 5 chat here, 6 inbox,
 * 7 marketplace, 8 config-set, 9 run task, 10 record). `objtab.<id>` is one
 * family: every VerticalTabBody strip entry carries `objtab.` + its entry id
 * (an object's tab is `tab:<panel stem>`, so its Actions tab is
 * `objtab.tab:actions`; the Global tab's strip entries are bare ids).
 * `picker.create` is placed on both create controls of the picker: the
 * create card, and the button shown when there is no universe yet.
 *
 * A recipe's TEXT names a control by its anchor too: `{control:<anchor>}`, or
 * `{control:<anchor>|<name>}` for an instance. The player (./controls.ts)
 * says the label the GUI shows for it, in bold, in the GUI's language: the
 * `data-guide-label` of the control when it is on screen and carries one (a
 * contribution's text, or a toggle's current word), else the anchor's
 * `label` key below, else the instance name. The same Go tests refuse a
 * placeholder naming an anchor missing here, and a `label` key missing from
 * the en or fr locales (internal/test/guide/labels_test.go).
 */

import type { KeyboardEvent } from 'react'

export interface GuideAnchor {
  /** File (under frontend/src) that places the attribute. */
  file: string
  /** First-recipe numbers (see header) that point at it. */
  recipeSteps: string[]
  /** True when instances are told apart by `data-guide-name`. */
  dynamic: boolean
  /** The i18n key of the text the control shows, `<namespace>:<key>`: the
   *  key the placing component itself renders (one source). `{name}` in it is
   *  the instance (`marketplace:tabs.{name}`). What a recipe's
   *  `{control:<anchor>}` says when the control is not on screen. */
  label?: string
  /** The control shows a contribution's text (a panel's name, an action's
   *  label, a task's title), translated by the hub's bundle, not by a GUI key:
   *  the placing component puts that text in `data-guide-label`. */
  contributed?: boolean
}

export const GUIDE_ANCHORS: Record<string, GuideAnchor> = {
  'picker.universe':     { file: 'pages/UniversePicker.tsx',                  recipeSteps: ['1'],      dynamic: true },
  'picker.universe.warning': { file: 'pages/UniversePicker.tsx',             recipeSteps: [],         dynamic: true },
  'picker.create':       { file: 'pages/UniversePicker.tsx',                  recipeSteps: ['1'],      dynamic: false, label: 'universes:create.title' },
  'create-universe.submit': { file: 'sections/univers/UniverseCreationModal.tsx', recipeSteps: ['1'],  dynamic: false, label: 'universes:create.submit' },
  'ribbon.universe':     { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: ['1', '2'], dynamic: true },
  'tree.node':           { file: 'sections/workspace/WorkspaceObjectsTree.tsx', recipeSteps: ['3'],    dynamic: true },
  'tree.group':          { file: 'sections/workspace/WorkspaceObjectsTree.tsx', recipeSteps: [],       dynamic: true },
  'tree.type-badge':     { file: 'sections/workspace/WorkspaceObjectsTree.tsx', recipeSteps: [],       dynamic: false },
  'tree.project-refused': { file: 'sections/workspace/WorkspaceObjectsTree.tsx', recipeSteps: [],      dynamic: false, label: 'workspace:tree.projectRefused' },
  'tree.project-refused-reason': { file: 'sections/workspace/WorkspaceObjectsTree.tsx', recipeSteps: [], dynamic: false },
  'tree.project-refused-open-universe': { file: 'sections/workspace/WorkspaceObjectsTree.tsx', recipeSteps: [], dynamic: false, label: 'workspace:tree.openUniversePage' },
  'object.header-panel': { file: 'sections/workspace/ObjectTab.tsx',          recipeSteps: [],         dynamic: true },
  'object.type-icon':    { file: 'sections/workspace/ObjectTab.tsx',          recipeSteps: [],         dynamic: false },
  'object.type-eyebrow': { file: 'sections/workspace/ObjectTab.tsx',          recipeSteps: [],         dynamic: false },
  'object.chips':        { file: 'sections/workspace/ObjectTab.tsx',          recipeSteps: [],         dynamic: false },
  'object.capabilities': { file: 'sections/workspace/ObjectTab.tsx',          recipeSteps: [],         dynamic: false },
  'project.add-project': { file: 'sections/shell/ProjectPanel.tsx',          recipeSteps: [],         dynamic: false },
  'project.new-project': { file: 'sections/workspace/NewAnywhere.tsx',        recipeSteps: [],         dynamic: false },
  'project.link-existing': { file: 'sections/workspace/NewAnywhere.tsx',      recipeSteps: [],         dynamic: false },
  'project.menu':        { file: 'sections/shell/ProjectPanel.tsx',           recipeSteps: [],         dynamic: false },
  'object.new-type':     { file: 'sections/workspace/NewMenu.tsx',            recipeSteps: [],         dynamic: true },
  'object.new-menu':     { file: 'sections/workspace/NewMenu.tsx',            recipeSteps: [],         dynamic: false },
  'object.new-empty':   { file: 'sections/workspace/NewMenu.tsx',            recipeSteps: [],         dynamic: false },
  'object.new':          { file: 'sections/workspace/NewMenu.tsx',            recipeSteps: [],         dynamic: false, label: 'workspace:objectTab.newButton' },
  'objtab.tab:actions':       { file: 'sections/workspace/VerticalTabBody.tsx', recipeSteps: ['3', '4'], dynamic: false, contributed: true },
  'objtab.tab:agent-context': { file: 'sections/workspace/VerticalTabBody.tsx', recipeSteps: ['3', '5'], dynamic: false, contributed: true },
  'objtab.tab:lifecycle':     { file: 'sections/workspace/VerticalTabBody.tsx', recipeSteps: [],         dynamic: false, contributed: true },
  'objtab.tab:signoff-board': { file: 'sections/workspace/VerticalTabBody.tsx', recipeSteps: [],         dynamic: false, contributed: true },
  'objtab.runs':         { file: 'sections/workspace/VerticalTabBody.tsx',    recipeSteps: ['9'],      dynamic: false, label: 'home:strip.runs' },
  'lifecycle.move':      { file: 'sections/workspace/LifecycleView.tsx',      recipeSteps: [],         dynamic: true,  contributed: true },
  'page.action':         { file: '../../../ktw-display-lib/src/ui-builders/page/nodes/interactive.tsx', recipeSteps: [], dynamic: true, contributed: true },
  'card':                { file: 'sections/workspace/SearchableCardGrid.tsx', recipeSteps: ['4', '9'], dynamic: true },
  'quick-action':        { file: 'sections/workspace/SearchableCardGrid.tsx', recipeSteps: ['4'],      dynamic: true, contributed: true }, // a task's quick action: its title is the task's
  'action.run':          { file: 'sections/workspace/ActionLaunchModal.tsx',  recipeSteps: ['4'],      dynamic: false, label: 'workspace:launch.run' },
  'task.card':           { file: 'sections/task/TaskGalleryPanel.tsx',        recipeSteps: ['9'],      dynamic: true,  contributed: true },
  'task.run':            { file: 'sections/agent/RunTaskPanel.tsx',           recipeSteps: ['9'],      dynamic: false, label: 'chat:form.run' },
  'todo.screen':         { file: 'sections/shell/RailScreen.tsx',             recipeSteps: ['6'],      dynamic: false },
  'marketplace.row':     { file: 'sections/marketplace/SidecarsPanel.tsx',    recipeSteps: ['7'],      dynamic: true },
  'marketplace.install': { file: 'sections/marketplace/SidecarsPanel.tsx',    recipeSteps: ['7'],      dynamic: false, label: 'marketplace:sidecars.install' },
  'marketplace.wire':    { file: 'sections/marketplace/SidecarsPanel.tsx',    recipeSteps: ['7'],      dynamic: false, label: 'marketplace:sidecars.wire' },
  'marketplace.tab':     { file: 'sections/marketplace/MarketplaceRoot.tsx',  recipeSteps: ['8'],      dynamic: true,  label: 'marketplace:tabs.{name}' },
  'marketplace.subscriptions': { file: 'sections/marketplace/MarketplaceRoot.tsx', recipeSteps: [],    dynamic: false },
  'marketplace.doctor':  { file: 'sections/marketplace/SidecarsPanel.tsx',    recipeSteps: [],         dynamic: false },
  'solution.row':        { file: 'sections/marketplace/SolutionsPanel.tsx',   recipeSteps: [],         dynamic: true },
  'vault.row':           { file: 'sections/marketplace/VaultsSection.tsx',    recipeSteps: [],         dynamic: true },
  'seed.row':            { file: 'sections/marketplace/SeedsPanel.tsx',       recipeSteps: [],         dynamic: true },
  'seed.demo':           { file: 'sections/marketplace/SeedsPanel.tsx',          recipeSteps: [],         dynamic: false },
  'infra.snapshot':      { file: 'sections/marketplace/InfraPanel.tsx',       recipeSteps: [],         dynamic: true },
  'ready.badge':         { file: 'sections/ready/ReadyBadge.tsx',             recipeSteps: [],         dynamic: false },
  'ready.screen':        { file: 'sections/ready/ReadyScreen.tsx',            recipeSteps: [],         dynamic: false },
  'ready.sidecar':       { file: 'sections/ready/ReadyScreen.tsx',            recipeSteps: [],         dynamic: true },
  'ready.program':       { file: 'sections/ready/ReadyScreen.tsx',            recipeSteps: [],         dynamic: true },
  'ready.install-all':   { file: 'sections/ready/ReadyScreen.tsx',            recipeSteps: [],         dynamic: false, label: 'ready:programs.installAll' },
  'ready.password':      { file: 'sections/ready/ReadyScreen.tsx',            recipeSteps: [],         dynamic: false },
  'ready.keys-form':     { file: 'sections/ready/ReadyScreen.tsx',            recipeSteps: [],         dynamic: false },
  'configset.row':     { file: 'sections/configsets/ConfigSetsPanel.tsx',   recipeSteps: ['8'],      dynamic: true },
  'configset.apply':     { file: 'sections/configsets/ConfigSetsPanel.tsx',   recipeSteps: ['8'],      dynamic: false, label: 'marketplace:configsets.apply' },
  'ribbon.chat':         { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: ['5'],      dynamic: false, label: 'shell:ribbon.chat' },
  'ribbon.inbox':        { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: ['6'],      dynamic: false, label: 'shell:ribbon.inbox' },
  'ribbon.marketplace':  { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: ['7'],      dynamic: false, label: 'shell:ribbon.marketplace' },
  'ribbon.record':       { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: ['10'],     dynamic: false, label: 'shell:ribbon.startRecording' },
  'ribbon.add-universe': { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: ['1'],      dynamic: false, label: 'shell:ribbon.addUniverse' },
  'add-universe.create': { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: ['1'],      dynamic: false, label: 'shell:universe.createNew' },
  'universe-menu.edit':  { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: ['2'],      dynamic: false, label: 'shell:universe.edit' },
  'universe.hub-keeps-running': { file: 'sections/shell/IconRibbon.tsx',     recipeSteps: [],         dynamic: false, label: 'shell:universe.hubKeepsRunning' },
  'profile.help':        { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: [],         dynamic: false, label: 'shell:profile.help' },
  'profile.concepts':    { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: [],         dynamic: false, label: 'shell:profile.concepts' },
  'tabs.global':         { file: 'sections/shell/MainPaneTabs.tsx',           recipeSteps: ['3', '9'], dynamic: false, label: 'shell:tabs.global' },
  'project.tab':         { file: 'sections/shell/ProjectPanel.tsx',           recipeSteps: ['3', '9'], dynamic: true,  label: 'shell:project.{name}' },
  'universe-edit.projects': { file: 'pages/UniverseEditPane.tsx',            recipeSteps: ['2'],      dynamic: false, label: 'universes:edit.tabs.projects' },
  'universe-edit.solution': { file: 'pages/UniverseEditPane.tsx',            recipeSteps: [],         dynamic: false, label: 'universes:edit.solution.section' },
  'universe-edit.solution-input': { file: 'pages/UniverseEditPane.tsx',      recipeSteps: [],         dynamic: false, label: 'universes:edit.solution.pick' },
  'universe-edit.give-solution': { file: 'pages/UniverseEditPane.tsx',       recipeSteps: [],         dynamic: false, label: 'universes:edit.solution.give' },
  'projects.attach':     { file: 'pages/UniverseProjectsPanel.tsx',           recipeSteps: ['2'],      dynamic: false, label: 'universes:projects.attach' },
  'object.chat-here':    { file: 'sections/workspace/AgentContextView.tsx',   recipeSteps: ['5'],      dynamic: false, label: 'workspace:agentContext.chatHere' },
  'guide.do-it':         { file: 'guide/GuidePlayer.tsx',                     recipeSteps: [],         dynamic: false, label: 'guide:player.doIt' },
  'guide.check':         { file: 'guide/GuidePlayer.tsx',                     recipeSteps: [],         dynamic: false, label: 'guide:player.check' },
  'document.toc':        { file: 'components/Markdown.tsx',                   recipeSteps: [],         dynamic: false, label: 'components:documentView.toc' },
  // A rendered markdown's own parts, and a deck's export menu.
  'markdown.admonition': { file: 'components/Markdown.tsx',                      recipeSteps: [],         dynamic: false },
  'markdown.admonition.title': { file: 'components/Markdown.tsx',                      recipeSteps: [],         dynamic: false },
  'markdown.toc':        { file: 'components/Markdown.tsx',                      recipeSteps: [],         dynamic: false },
  'markdown.status':     { file: 'components/Markdown.tsx',                      recipeSteps: [],         dynamic: false },
  'deck.export':         { file: 'components/compositions/UIDeck.tsx',           recipeSteps: [],         dynamic: false },
  'deck.export-pdf':     { file: 'components/compositions/UIDeck.tsx',           recipeSteps: [],         dynamic: false },
  'deck.export-pptx':    { file: 'components/compositions/UIDeck.tsx',           recipeSteps: [],         dynamic: false },
  'deck.export-pptx-pictures': { file: 'components/compositions/UIDeck.tsx',           recipeSteps: [],         dynamic: false },
  'deck.export-html':    { file: 'components/compositions/UIDeck.tsx',           recipeSteps: [],         dynamic: false },
  'first-run':           { file: 'guide/FirstRunChoice.tsx',                  recipeSteps: [],         dynamic: false },
  'chat.recipe-card':    { file: 'guide/GuideRecipeCard.tsx',                 recipeSteps: [],         dynamic: false },
  // Placed and now registered too: a recipe can point at them.
  'cards.show-all':      { file: 'sections/workspace/SearchableCardGrid.tsx', recipeSteps: [],         dynamic: false },
  'keys.restart':        { file: 'sections/marketplace/RestartForKeys.tsx',   recipeSteps: [],         dynamic: false },
  'panel.empty':         { file: 'sections/workspace/EmptyInvite.tsx',        recipeSteps: [],         dynamic: false },
  'panel.empty.action':  { file: 'sections/workspace/EmptyInvite.tsx',        recipeSteps: [],         dynamic: false, contributed: true },
  'profile-menu.level':  { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: [],         dynamic: true },
  'quick-action.remove': { file: 'sections/workspace/SearchableCardGrid.tsx', recipeSteps: [],         dynamic: false },
  'card.star':           { file: 'sections/workspace/SearchableCardGrid.tsx', recipeSteps: [],         dynamic: false },
  'reader.full-log':     { file: 'guide/GuideReader.tsx',                     recipeSteps: [],         dynamic: false },
  'player.skip':         { file: 'guide/GuidePlayer.tsx',                     recipeSteps: [],         dynamic: false },
  'ribbon.feedback':     { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: [],         dynamic: false },
  'solution.needs':      { file: 'sections/marketplace/SolutionsPanel.tsx',   recipeSteps: [],         dynamic: false },
  'solution.seed':       { file: 'sections/marketplace/SolutionsPanel.tsx',      recipeSteps: [],         dynamic: false },
  'solution.seed-demo':  { file: 'sections/marketplace/SolutionsPanel.tsx',      recipeSteps: [],         dynamic: false },
  // What the usage guide points at.
  'ribbon.camera':       { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: [],         dynamic: false },
  'profile.open':        { file: 'sections/shell/IconRibbon.tsx',             recipeSteps: [],         dynamic: false },
  'profile-menu.profile':    { file: 'sections/shell/IconRibbon.tsx',         recipeSteps: [],         dynamic: false, label: 'shell:profile.profile' },
  'profile-menu.appearance': { file: 'sections/shell/IconRibbon.tsx',         recipeSteps: [],         dynamic: false, label: 'shell:profile.appearance' },
  'profile-menu.feedback':   { file: 'sections/shell/IconRibbon.tsx',         recipeSteps: [],         dynamic: false },
  // An object pane, a page's result and a program's install, as a recipe can show them.
  'object.header':       { file: 'sections/workspace/ObjectTab.tsx',          recipeSteps: [],         dynamic: false },
  'objtab.badge':        { file: 'sections/workspace/VerticalTabBody.tsx',    recipeSteps: [],         dynamic: false },
  'page.drawer':         { file: 'sections/workspace/PanelDrawer.tsx',        recipeSteps: [],         dynamic: false },
  'page.undo':           { file: 'components/compositions/PageHost.tsx',      recipeSteps: [],         dynamic: false, label: 'workspace:pageUndo' },
  'page.reader':         { file: 'sections/workspace/FileReaderDrawer.tsx',   recipeSteps: [],         dynamic: true },
  'object.edit-core':    { file: 'sections/workspace/ObjectTab.tsx',          recipeSteps: [],         dynamic: false },
  'object.breadcrumb':   { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: false },
  'object.crumb':        { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: true },
  'object.crumb-here':   { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: false },
  'object.state':        { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: false },
  'object.id-line':      { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: false },
  'object.retire':       { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: false },
  'object.more':         { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: false },
  'object.hard-delete':  { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: false },
  'object.retire-dialog': { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: false },
  'object.retire-confirm': { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: false },
  'object.children-cards': { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: true },
  'object.children-new': { file: 'sections/workspace/ObjectTab.tsx',             recipeSteps: [],         dynamic: true },
  'object.save-core':    { file: 'sections/workspace/ObjectTab.tsx',          recipeSteps: [],         dynamic: false },
  'links.add':           { file: 'sections/workspace/ObjectCorePanels.tsx',   recipeSteps: [],         dynamic: false },
  'links.target-search': { file: 'sections/workspace/ObjectCorePanels.tsx',   recipeSteps: [],         dynamic: false },
  'links.found':         { file: 'sections/workspace/ObjectCorePanels.tsx',   recipeSteps: [],         dynamic: true },
  'ready.program-admin':      { file: 'sections/ready/ReadyScreen.tsx',       recipeSteps: [],         dynamic: false, label: 'ready:programs.admin' },
  'ready.program-installing': { file: 'sections/ready/ReadyScreen.tsx',       recipeSteps: [],         dynamic: false },
  'ready.program-not-found':  { file: 'sections/ready/ReadyScreen.tsx',       recipeSteps: [],         dynamic: false },
  'ready.program-step':       { file: 'sections/ready/ReadyScreen.tsx',       recipeSteps: [],         dynamic: false },
  'ready.program-report':     { file: 'sections/ready/ReadyScreen.tsx',       recipeSteps: [],         dynamic: false, label: 'ready:programs.reportOpen' },
  'ready.program-report-text': { file: 'sections/ready/ReadyScreen.tsx',      recipeSteps: [],         dynamic: false },
  'ready.program-report-copy': { file: 'sections/ready/ReadyScreen.tsx',      recipeSteps: [],         dynamic: false, label: 'ready:programs.copy' },
  'ready.not-installed':      { file: 'sections/ready/ReadyScreen.tsx',       recipeSteps: [],         dynamic: false },
  'ready.reinstall':          { file: 'sections/ready/ReadyScreen.tsx',       recipeSteps: [],         dynamic: false, label: 'ready:programs.reinstall' },
  'ready.keys-file':          { file: 'sections/ready/ReadyScreen.tsx',       recipeSteps: [],         dynamic: false },
  'marketplace.pin-behind':   { file: 'sections/marketplace/SidecarsPanel.tsx', recipeSteps: [],       dynamic: false },
  'marketplace.pin-update':   { file: 'sections/marketplace/SidecarsPanel.tsx', recipeSteps: [],       dynamic: false },
  'task.starting':            { file: 'sections/agent/RunTaskPanel.tsx',      recipeSteps: [],         dynamic: false },
  // The dev installation shown as one.
  'infra.dev-build':     { file: 'sections/marketplace/InfraPanel.tsx',         recipeSteps: [],         dynamic: false },
  'infra.dev-hint':      { file: 'sections/marketplace/InfraPanel.tsx',         recipeSteps: [],         dynamic: false, label: 'marketplace:infra.devHint' },
  'infra.replace-dev':   { file: 'sections/marketplace/InfraPanel.tsx',         recipeSteps: [],         dynamic: false },
  'marketplace.dev-build': { file: 'sections/marketplace/SidecarsPanel.tsx',      recipeSteps: [],         dynamic: false },
  'marketplace.store-kept': { file: 'sections/marketplace/SidecarsPanel.tsx',      recipeSteps: [],         dynamic: false },
  'marketplace.replace-dev': { file: 'sections/marketplace/SidecarsPanel.tsx',      recipeSteps: [],         dynamic: false },
  // A display panel called inline, its states and controls.
  'marketplace.call-panel': { file: 'sections/marketplace/SidecarsPanel.tsx',      recipeSteps: [],         dynamic: false },
  'ready.install-panel': { file: 'sections/ready/ReadyScreen.tsx',              recipeSteps: [],         dynamic: false },
  'solution.install-panel': { file: 'sections/univers/SolutionInstallForm.tsx',    recipeSteps: [],         dynamic: false },
  'run.panels':          { file: 'sections/scheduler/RunDrawer.tsx',            recipeSteps: [],         dynamic: false },
  'ribbon.panels':       { file: 'sections/shell/IconRibbon.tsx',               recipeSteps: [],         dynamic: false, label: 'shell:ribbon.panels' },
  'panels.list':         { file: 'sections/panels/PanelsScreen.tsx',            recipeSteps: [],         dynamic: false },
  'panel.meta':          { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false },
  'panel.lost':          { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false },
  'panel.progress':      { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false },
  'panel.page-error':    { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false },
  'panel.ask':           { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false },
  'panel.ask-form':      { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false },
  'panel.ask-form-error': { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false },
  'panel.answer':        { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false, label: 'components:displayPanel.ask.send' },
  'panel.chat':          { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false },
  'panel.answered':      { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false },
  'panel.complete':      { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false },
  'panel.cancel':        { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false, label: 'components:displayPanel.cancel' },
  'panel.open-tab':      { file: 'sections/panels/DisplayPanel.tsx',            recipeSteps: [],         dynamic: false, label: 'components:displayPanel.openTab' },
  // The object pane's one header.
  'object.header-actions': { file: 'sections/workspace/ObjectTab.tsx',            recipeSteps: [],         dynamic: false },
  // The conversion dialog of a project not yet converted.
  'conversion.reading':    { file: 'sections/project/ConversionDialog.tsx',      recipeSteps: [],         dynamic: false },
  'conversion.up-to-date': { file: 'sections/project/ConversionDialog.tsx',     recipeSteps: [],         dynamic: false },
  'conversion.versions':   { file: 'sections/project/ConversionDialog.tsx',      recipeSteps: [],         dynamic: false },
  'conversion.steps':      { file: 'sections/project/ConversionDialog.tsx',      recipeSteps: [],         dynamic: false },
  'conversion.step':       { file: 'sections/project/ConversionDialog.tsx',      recipeSteps: [],         dynamic: false },
  'conversion.tree':       { file: 'sections/project/ConversionDialog.tsx',      recipeSteps: [],         dynamic: false },
  'conversion.no-way-back': { file: 'sections/project/ConversionDialog.tsx',     recipeSteps: [],         dynamic: false },
  'conversion.end':        { file: 'sections/project/ConversionDialog.tsx',      recipeSteps: [],         dynamic: false },
  'conversion.error':      { file: 'sections/project/ConversionDialog.tsx',      recipeSteps: [],         dynamic: false },
}

/** onKeyDown for a non-button element given role="button"/"tab": Enter or
 *  Space runs `fn`, as a click would. Keys pressed on a nested control (a
 *  close or info button inside a card or tab) are left to that control. */
/** The mark the guide player's overlay carries (its callout, its buttons). */
export const GUIDE_OVERLAY_ATTR = 'data-guide-overlay'

/** Whether a press landed inside the guide player's overlay - its callout,
 *  « Suivant » - and not on the screen it points at. */
export function pressedInGuide(target: EventTarget | null): boolean {
  return typeof Element !== 'undefined' && target instanceof Element && !!target.closest(`[${GUIDE_OVERLAY_ATTR}]`)
}

/** A menu the guide points inside stays open through a press on the guide:
 * « Suivant » is a click outside the menu, and closing it
 *  would take away the control the next step points at. Any other close
 *  (an item chosen, Escape, a press elsewhere) closes it. */
export function menuOpenAfter(next: boolean, lastPressInGuide: boolean): boolean | null {
  if (!next && lastPressInGuide) return null // ignored: the menu stays as it is
  return next
}

export function activateOnKey(fn: () => void) {
  return (e: KeyboardEvent<HTMLElement>) => {
    if (e.target !== e.currentTarget) return
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      fn()
    }
  }
}
