// SPDX-License-Identifier: Apache-2.0
/* KaTyoti — crisp vector versions of the gold line-art nodes (core + delivery icons). */
import React from 'react';

const GOLD = '#e9bb52';
const GOLD_HI = '#ffe6ac';

const wrapStyle: React.CSSProperties = {
  width: '100%', height: '100%', overflow: 'visible',
  filter: 'drop-shadow(0 0 5px rgba(243,192,73,.5))',
};

export function CoreIcon() {
  const cxp = 135, cyp = 118, rx = 126, ry = 101;
  const angles = [-90, -45, -18, 18, 45, 90, 135, 162, 198, 225];
  return (
    <svg viewBox="0 0 270 236" preserveAspectRatio="xMidYMid meet" style={wrapStyle}>
      <defs>
        <linearGradient id="kt_T" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#ffeebb" /><stop offset=".5" stopColor="#e7b84e" /><stop offset="1" stopColor="#a9772a" />
        </linearGradient>
      </defs>
      <ellipse cx={cxp} cy={cyp} rx={rx} ry={ry} fill="none" stroke={GOLD} strokeWidth="2" opacity="0.92" />
      <ellipse cx={cxp} cy={cyp} rx="95" ry="74" fill="none" stroke={GOLD} strokeWidth="1.5" opacity="0.85" />
      {angles.map((a, i) => {
        const r = (a * Math.PI) / 180;
        return <circle key={i} cx={cxp + rx * Math.cos(r)} cy={cyp + ry * Math.sin(r)} r="3.1" fill={GOLD_HI} />;
      })}
      <rect x="98" y="80" width="74" height="13" rx="2.5" fill="url(#kt_T)" />
      <rect x="127" y="80" width="16" height="84" rx="2.5" fill="url(#kt_T)" />
      <rect x="120" y="169" width="30" height="5" rx="2.5" fill="url(#kt_T)" opacity="0.95" />
    </svg>
  );
}

function Circle({ children }: { children: React.ReactNode }) {
  return (
    <svg viewBox="0 0 100 100" preserveAspectRatio="xMidYMid meet" style={wrapStyle}>
      <g fill="none" stroke={GOLD} strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="50" cy="50" r="40" strokeWidth="2.2" />
        {children}
      </g>
    </svg>
  );
}

export const ReportsIcon = () => (
  <Circle>
    <rect x="36" y="52" width="7.5" height="12" rx="1.5" fill={GOLD} stroke="none" />
    <rect x="46.5" y="44" width="7.5" height="20" rx="1.5" fill={GOLD} stroke="none" />
    <rect x="57" y="36" width="7.5" height="28" rx="1.5" fill={GOLD} stroke="none" />
  </Circle>
);
export const DecisionsIcon = () => (
  <Circle>
    <circle cx="47" cy="53" r="14" />
    <circle cx="47" cy="53" r="6.5" />
    <circle cx="47" cy="53" r="1.6" fill={GOLD} stroke="none" />
    <line x1="51" y1="49" x2="67" y2="33" />
    <path d="M60 32.5 H67.5 V40" />
  </Circle>
);
export const WorkflowsIcon = () => (
  <Circle>
    <rect x="42" y="30" width="16" height="11" rx="2" />
    <rect x="28" y="58" width="15" height="11" rx="2" />
    <rect x="42.5" y="58" width="15" height="11" rx="2" />
    <rect x="57" y="58" width="15" height="11" rx="2" />
    <path d="M50 41 V49 M35.5 49 H64.5 M35.5 49 V58 M50 49 V58 M64.5 49 V58" />
  </Circle>
);
export const ResultsIcon = () => (
  <Circle>
    <polyline points="33,62 44,51 52,57 67,39" />
    <path d="M59 39 H67.5 V47.5" />
  </Circle>
);

export const ICONS: Record<string, React.FC> = {
  reports: ReportsIcon, decisions: DecisionsIcon, workflows: WorkflowsIcon, results: ResultsIcon,
};
