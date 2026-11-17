// SPDX-License-Identifier: Apache-2.0
/* KaTyoti live panels: KNOWLEDGE list, TASKS checklist, icons, info popover. */
import React, { useState } from 'react';
import { KNOWLEDGE, TASKS, NodeInfo } from './data';
import { useT } from '../../i18n';

type IconName = 'doc' | 'db' | 'shield' | 'book' | 'globe';

export function KIcon({ name }: { name: IconName }) {
  const c = {
    viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor',
    strokeWidth: 1.6, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const,
  };
  switch (name) {
    case 'doc': return (<svg {...c}><path d="M14 3v4a1 1 0 0 0 1 1h4" /><path d="M17 21H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7l5 5v11a2 2 0 0 1-2 2z" /><path d="M9 9h1M9 13h6M9 17h6" /></svg>);
    case 'db': return (<svg {...c}><ellipse cx="12" cy="5" rx="7" ry="3" /><path d="M5 5v6c0 1.7 3.1 3 7 3s7-1.3 7-3V5" /><path d="M5 11v6c0 1.7 3.1 3 7 3s7-1.3 7-3v-6" /></svg>);
    case 'shield': return (<svg {...c}><path d="M12 3l7 3v5c0 4.5-3 7.5-7 9-4-1.5-7-4.5-7-9V6l7-3z" /><path d="M9.5 12l1.8 1.8L15 10" /></svg>);
    case 'book': return (<svg {...c}><path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H12v16H6.5A2.5 2.5 0 0 0 4 21.5z" /><path d="M20 5.5A2.5 2.5 0 0 0 17.5 3H12v16h5.5A2.5 2.5 0 0 1 20 21.5z" /></svg>);
    case 'globe': return (<svg {...c}><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3c2.5 2.6 2.5 15.4 0 18M12 3c-2.5 2.6-2.5 15.4 0 18" /></svg>);
  }
}

const W = 1774, H = 887;
function pct(box: [number, number, number, number]) {
  return {
    left: (box[0] / W) * 100 + '%', top: (box[1] / H) * 100 + '%',
    width: ((box[2] - box[0]) / W) * 100 + '%', height: ((box[3] - box[1]) / H) * 100 + '%',
  };
}

type Pick = (info: NodeInfo, at: [number, number]) => void;

export function KnowledgePanel({ onPick }: { onPick: Pick }) {
  const k = KNOWLEDGE;
  return (
    <div className="kw-kPanel kw-panel" style={pct(k.boxPx)}
         onClick={() => onPick(k.info, [(k.boxPx[0] + k.boxPx[2]) / 2, k.boxPx[1] + 16])}>
      <div className="kw-panelTitle">{k.title}</div>
      <ul className="kw-kList">
        {k.items.map((it, i) => (
          <li key={it.label} className="kw-kItem" style={{ ['--d' as any]: (2.05 + i * 0.11) + 's' }}>
            <span className="kw-kIco"><KIcon name={it.icon as IconName} /></span>
            <span>{it.label}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function Check({ on, delay }: { on: boolean; delay: string }) {
  return (
    <span className={'kw-tCheck' + (on ? ' on' : '')}>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
        <path className="kw-tTick" style={{ animationDelay: delay }} d="M5 12.5l4.2 4.5L19 7" />
      </svg>
    </span>
  );
}

export function TasksPanel({ onPick, onToggle }: { onPick: Pick; onToggle?: (task: string, on: boolean) => void }) {
  const tk = TASKS;
  const [checks, setChecks] = useState(tk.items.map(() => true));
  const toggle = (i: number, e: React.MouseEvent) => {
    e.stopPropagation();
    setChecks((c) => { const n = c.map((v, j) => (j === i ? !v : v)); onToggle?.(tk.items[i], n[i]); return n; });
  };
  return (
    <div className="kw-tPanel kw-panel" style={pct(tk.boxPx)}
         onClick={() => onPick(tk.info, [(tk.boxPx[0] + tk.boxPx[2]) / 2, tk.boxPx[1] + 14])}>
      <div className="kw-panelTitle">{tk.title}</div>
      <ul className="kw-tList">
        {tk.items.map((label, i) => (
          <li key={label} className={'kw-tItem' + (checks[i] ? '' : ' off')}
              style={{ ['--d' as any]: (2.15 + i * 0.12) + 's' }} onClick={(e) => toggle(i, e)}>
            <Check on={checks[i]} delay={(2.35 + i * 0.12) + 's'} />
            <span>{label}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function InfoPopover({ active, onClose }: { active: { info: NodeInfo; at: [number, number] } | null; onClose: () => void }) {
  const t = useT('components');
  if (!active) return null;
  const { info, at } = active;
  const leftPct = Math.max(4, Math.min(72, (at[0] / W) * 100));
  const topPct = Math.max(3, Math.min(70, (at[1] / H) * 100));
  return (
    <div className="kw-infoLayer" onClick={onClose}>
      <div className="kw-infoCard" style={{ left: leftPct + '%', top: topPct + '%' }} onClick={(e) => e.stopPropagation()}>
        <div className="kw-infoBar" />
        <div className="kw-infoBody">
          <div className="kw-infoSub">{info.sub}</div>
          <div className="kw-infoTitle">{info.title}</div>
          <p className="kw-infoText">{info.body}</p>
        </div>
        <button className="kw-infoX" onClick={onClose} aria-label={t('welcome.close')}>×</button>
      </div>
    </div>
  );
}
