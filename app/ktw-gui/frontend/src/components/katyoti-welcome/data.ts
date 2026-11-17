// SPDX-License-Identifier: Apache-2.0
/* KaTyoti welcome scene — geometry + copy. All coords in banner space (1774x887).
   Sprites are PURE (no labels/connectors baked in); labels are rendered live. */

export interface NodeInfo { title: string; sub: string; body: string; }
export interface SceneNode {
  id: string;
  spriteKey: string;                 // assets/sprites/<spriteKey>.png
  box: [number, number, number, number];
  reveal: number;
  kind: 'core' | 'brain' | 'team' | 'agent' | 'delivery';
  label?: string;
  tag?: string;
  labelPos?: [number, number];
  labelAlign?: 'left' | 'center';
  info: NodeInfo;
}
export interface Connector {
  id: string;
  from: [number, number];
  to: [number, number];
  group: 'team' | 'agent' | 'delivery' | 'task';
  dotted?: boolean;
  curve?: number;
}

export const W = 1774;
export const H = 887;
export const CORE: [number, number] = [855, 387];

export const NODES: SceneNode[] = [
  { id: 'core', spriteKey: 'coreLogo', box: [740, 272, 970, 502], reveal: 0.45, kind: 'core',
    info: { title: 'KaTyoti', sub: 'Unified intelligence core', body: 'Every team, agent and task orbits a single source of truth. KaTyoti routes knowledge and work between people and AI in real time.' } },
  { id: 'brain', spriteKey: 'brain', box: [798, 75, 952, 222], reveal: 1.40, kind: 'brain',
    info: { title: 'Neural Core', sub: 'Reasoning engine', body: 'Context from documents, databases and policies is fused into one reasoning layer that powers every agent.' } },

  { id: 'legalTeam', spriteKey: 'legalTeam', box: [322, 70, 552, 290], reveal: 1.30, kind: 'team',
    label: 'Legal Team', tag: 'Contracts, Compliance', labelPos: [344, 286], labelAlign: 'left',
    info: { title: 'Legal Team', sub: 'Contracts & Compliance', body: 'Draft, review and track agreements with an AI partner that knows your policies and flags risk before it ships.' } },
  { id: 'researchTeam', spriteKey: 'researchTeam', box: [170, 326, 408, 506], reveal: 1.45, kind: 'team',
    label: 'Research Team', tag: 'Analysis, Insights', labelPos: [214, 508], labelAlign: 'left',
    info: { title: 'Research Team', sub: 'Analysis & Insights', body: 'Turn scattered sources into structured insight. Ask in plain language, get cited answers in seconds.' } },
  { id: 'dataTeam', spriteKey: 'dataTeam', box: [336, 480, 572, 696], reveal: 1.60, kind: 'team',
    label: 'Data Team', tag: 'Data Management', labelPos: [360, 700], labelAlign: 'left',
    info: { title: 'Data Team', sub: 'Data Management', body: 'Pipelines, catalogs and quality checks orchestrated from one place — with agents watching for drift.' } },
  { id: 'productTeam', spriteKey: 'productTeam', box: [564, 518, 808, 750], reveal: 1.75, kind: 'team',
    label: 'Product Team', tag: 'Roadmap, Priorities', labelPos: [586, 760], labelAlign: 'left',
    info: { title: 'Product Team', sub: 'Roadmap & Priorities', body: 'Synthesize feedback, score opportunities and keep the roadmap honest with live signals from the whole org.' } },
  { id: 'hrTeam', spriteKey: 'hrTeam', box: [928, 606, 1074, 792], reveal: 1.90, kind: 'team',
    label: 'HR Team', tag: 'People, Culture', labelPos: [1004, 806], labelAlign: 'left',
    info: { title: 'HR Team', sub: 'People & Culture', body: 'Onboarding, policy answers and culture signals — handled with empathy and zero busywork.' } },
  { id: 'opsTeam', spriteKey: 'opsTeam', box: [996, 452, 1206, 696], reveal: 2.00, kind: 'team',
    label: 'Operations Team', tag: 'Processes, Execution', labelPos: [1098, 700], labelAlign: 'left',
    info: { title: 'Operations Team', sub: 'Processes & Execution', body: 'Standardize the playbook and let agents run the repeatable parts while your team handles the exceptions.' } },

  { id: 'legalAgent', spriteKey: 'legalAgent', box: [1124, 60, 1280, 196], reveal: 1.95, kind: 'agent',
    label: 'Legal AI Agent', tag: 'Contract Review', labelPos: [1150, 200], labelAlign: 'left',
    info: { title: 'Legal AI Agent', sub: 'Contract Review', body: 'Reads every clause against your standards, surfaces deviations and proposes redlines for human sign-off.' } },
  { id: 'researchAgent', spriteKey: 'researchAgent', box: [1222, 240, 1366, 360], reveal: 2.10, kind: 'agent',
    label: 'Research AI Agent', tag: 'Information Retrieval', labelPos: [1232, 362], labelAlign: 'left',
    info: { title: 'Research AI Agent', sub: 'Information Retrieval', body: 'Finds, ranks and cites the right source across all connected knowledge — never a dead-end search again.' } },
  { id: 'opsAgent', spriteKey: 'opsAgent', box: [1224, 398, 1372, 530], reveal: 2.25, kind: 'agent',
    label: 'Ops AI Agent', tag: 'Process Automation', labelPos: [1228, 540], labelAlign: 'left',
    info: { title: 'Ops AI Agent', sub: 'Process Automation', body: 'Executes multi-step workflows end to end, escalating to a human only when judgment is required.' } },

  { id: 'reports', spriteKey: 'reports', box: [1475, 139, 1563, 225], reveal: 2.45, kind: 'delivery',
    label: 'Reports', labelPos: [1519, 232], labelAlign: 'center',
    info: { title: 'Reports', sub: 'Output', body: 'Board-ready summaries generated from live work — always current, always sourced.' } },
  { id: 'decisions', spriteKey: 'decisions', box: [1475, 281, 1563, 367], reveal: 2.55, kind: 'delivery',
    label: 'Decisions', labelPos: [1519, 374], labelAlign: 'center',
    info: { title: 'Decisions', sub: 'Output', body: 'Options weighed, trade-offs explained, recommendation on the table — you make the call.' } },
  { id: 'workflows', spriteKey: 'workflows', box: [1475, 431, 1563, 517], reveal: 2.65, kind: 'delivery',
    label: 'Workflows', labelPos: [1519, 524], labelAlign: 'center',
    info: { title: 'Workflows', sub: 'Output', body: 'Repeatable processes captured once and run forever, with humans in the loop where it matters.' } },
  { id: 'results', spriteKey: 'results', box: [1475, 591, 1563, 677], reveal: 2.75, kind: 'delivery',
    label: 'Results', labelPos: [1519, 682], labelAlign: 'center',
    info: { title: 'Results', sub: 'Output', body: 'Measurable outcomes — hours saved, risk reduced, velocity up — tracked from the same core.' } },
];

const C = (id: string, from: [number, number], to: [number, number], group: Connector['group'],
  opts: Partial<Connector> = {}): Connector => ({ id, from, to, group, dotted: false, curve: 0, ...opts });

export const CONNECTORS: Connector[] = [
  C('c_legal', [505, 252], CORE, 'team'),
  C('c_research', [405, 442], CORE, 'team'),
  C('c_data', [475, 650], CORE, 'team'),
  C('c_product', [690, 695], CORE, 'team'),
  C('c_hr', [1025, 710], CORE, 'team'),
  C('c_ops', [1140, 612], CORE, 'team'),
  C('c_aLegal', CORE, [1205, 178], 'agent'),
  C('c_aResearch', CORE, [1295, 345], 'agent'),
  C('c_aOps', CORE, [1293, 508], 'agent'),
  C('c_dReports', [1272, 150], [1484, 185], 'delivery', { dotted: true, curve: -30 }),
  C('c_dDecisions', [1360, 320], [1484, 327], 'delivery', { dotted: true, curve: -16 }),
  C('c_dWorkflows', [1360, 500], [1484, 477], 'delivery', { dotted: true, curve: 22 }),
  C('c_dResults', [1345, 520], [1484, 635], 'delivery', { dotted: true, curve: 55 }),
  C('c_tasks', [862, 600], [856, 472], 'task', { dotted: true }),
];

export const BRAIN_STREAM = { from: [858, 280] as [number, number], to: [873, 234] as [number, number] };

export const KNOWLEDGE = {
  title: 'KNOWLEDGE',
  boxPx: [612, 92, 754, 266] as [number, number, number, number],
  items: [
    { label: 'Documents', icon: 'doc' },
    { label: 'Databases', icon: 'db' },
    { label: 'Policies', icon: 'shield' },
    { label: 'Guidelines', icon: 'book' },
    { label: 'Internal wikis', icon: 'globe' },
  ],
  info: { title: 'Knowledge', sub: 'What the agents read', body: 'Documents, databases, policies, guidelines and wikis — connected once, kept in sync, grounding every answer.' },
};

export const TASKS = {
  title: 'TASKS',
  boxPx: [808, 640, 920, 848] as [number, number, number, number],
  items: ['Analyze', 'Draft', 'Summarize', 'Automate', 'Monitor'],
  info: { title: 'Tasks', sub: 'What the agents do', body: 'Pick what the agents handle — analyze, draft, summarize, automate, monitor. Toggle a task to put it in motion.' },
};
