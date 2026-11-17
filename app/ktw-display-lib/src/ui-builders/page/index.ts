// SPDX-License-Identifier: Apache-2.0
// ui-builders/page/ — DSL composition framework. Per
// the display module's decision on where the UI lives (modules/display/decisions/), this is the default extension surface
// for LLM-driven UI. Sidecars contribute schemas + DSL templates only;
// the rendering pipeline lives entirely here.

export type {
  // Common
  UIPageNode,
  UIPageNodeKind,
  UIPageNodeBase,
  UIPanelSpec,

  // Layout (8)
  UILayoutNode,
  UIVBoxNode,
  UIHBoxNode,
  UIGridNode,
  UIBoxNode,
  UICardNode,
  UISectionNode,
  UITabsNode,
  UIAccordionNode,
  UISplitNode,

  // Atomic (9)
  UIAtomicNode,
  UIHeadingNode,
  UITextNode,
  UIDividerNode,
  UIBadgeNode,
  UILinkNode,
  UIImageNode,
  UIKPINode,
  UICodeBlockNode,
  UIMarkdownNode,

  // Schema-bound (4)
  UISchemaBoundNode,
  UIFormNode,
  UIViewNode,
  UITableNode,
  UIWizardNode,
  UITimelineLanesNode,
} from './ui-page.types.ts'

export {
  LAYOUT_KINDS,
  ATOMIC_KINDS,
  SCHEMA_BOUND_KINDS,
  ALL_KINDS,
  isLayoutNode,
  isAtomicNode,
  isSchemaBoundNode,
  hasSchemaBound,
  childrenOf,
  assertNeverNode,
} from './ui-page.types.ts'

export {
  UIPageNodeJSONSchema,
  UI_KIND_CATALOG,
  type UIKindCatalogEntry,
} from './ui-page.json-schema.ts'

export { UIPageView, type UIPageViewProps } from './ui-page.view.tsx'

// Interactive nodes — they act through the host (ui-page.interactive.ts).
export type {
  UIInteractiveNode,
  UIPageAction,
  UIPageActionEvent,
  UIButtonNode,
  UIListNode,
  UITreeNode,
  UILanesNode,
  UILane,
  UIGaugeNode,
  UIGaugeSegment,
  GaugePart,
  UIGanttNode,
  UIGanttTask,
  GanttCalendar,
  UIGraphNode,
  UIGraphBox,
  PlacedBox,
  PlacedFrame,
  UIGraphEdge,
  UISelectNode,
  UIBoardNode,
  UIBoardRow,
  UIGridBoardNode,
  UIGridRow,
  UIGridColumn,
  GridMove,
  UIMenuNode,
  UIMenuItem,
  UIObjectCardNode,
  UIObjectCardRow,
  UIObjectCardBadge,
  UIChoiceOption,
  UIChoiceSource,
} from './ui-page.interactive.ts'
export type { UINavigateTarget, UIActionVerb, UIActionResult, UIPanelNode } from './ui-page.interactive.ts'

// A display panel as a screen follows it.
export type { PanelEvent, PanelState, PanelView } from './panel.ts'
export { isPanelId, panelAskForm, panelView, reducePanel, panelTemplateFetch, templatePage, withPanelTemplate } from './panel.ts'
export {
  resolveActionArgs, resolveAction, actionVerb, ACTION_VERBS, parseActionResult, resultPanel, navigateTarget,
  nestRows, ganttSpan, ganttCalendar, layerGraph, pathOf, gaugeBar,
  choiceOptions, choiceEvent, menuItemAction, boardRows, boardSelected, boardToggleEvent, gridCell, gridCounts, gridMove, gridKeyTarget,
} from './ui-page.interactive.ts'
export { INTERACTIVE_KINDS, isInteractiveNode } from './ui-page.types.ts'

// Host renderer overrides, and the page context an override acts through.
export { registerPageNodeRenderer, pageNodeOverride } from './node-overrides.ts'
export { dataAttrs } from './nodes/data-attrs.ts'
// A drawio node's thumbnail, for a host that draws the full view itself.
export { DrawioThumbnail, DrawioSketchSVG } from './nodes/drawio-thumbnail.tsx'
export { usePageContext, useItem, useRunAction, useSelection, routePageAction, type PageSelection } from './page-context.tsx'

// The one answer and its ops by node id.
export type { Spec, PageAnswer, PageOp, PageOpKind, SectionLoad, SectionProps, PageState } from './answer.ts'
export { isNamedSpec, isSection, isPushedSection, pageState, applyPageOps } from './answer.ts'
// A section loaded through the host on a progressive page.
export { sectionMode, sectionWhen, sectionLoadsNow, sectionContent, placeholderShape, sectionLoad, selectionKeys, selectionReady, resolveSelectionArgs } from './nodes/section-load.tsx'
