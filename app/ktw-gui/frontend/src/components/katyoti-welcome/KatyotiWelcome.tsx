// SPDX-License-Identifier: Apache-2.0
/* KaTyoti animated welcome panel — drop-in React + TS component.
 *
 * Usage:
 *   import { KatyotiWelcome } from './katyoti/KatyotiWelcome';
 *   <KatyotiWelcome onSelectNode={(id) => ...} onTaskToggle={(t, on) => ...} />
 *
 * The scene keeps a fixed 1774:887 aspect ratio and letterboxes inside its
 * container, so just give the parent a size (e.g. 100vw/100vh for a splash).
 */
import React, { useEffect, useRef, useState } from 'react';
import { NODES, CORE, W, H, SceneNode, NodeInfo } from './data';
import { CanvasScene } from './engine';
import { KnowledgePanel, TasksPanel, InfoPopover } from './Panels';
import { ICONS } from './Icons';
import './katyoti.css';

// Vite-style asset resolution. (CRA/webpack: swap for individual imports.)
const spriteUrls = import.meta.glob('./assets/sprites/*.png', { eager: true, import: 'default' }) as Record<string, string>;
const spriteUrl = (key: string) => spriteUrls[`./assets/sprites/${key}.png`];

const KIND_GLOW: Record<string, string> = {
  team: 'radial-gradient(circle, rgba(224,169,63,0.28), rgba(224,169,63,0) 68%)',
  agent: 'radial-gradient(circle, rgba(255,214,130,0.32), rgba(255,214,130,0) 68%)',
  delivery: 'radial-gradient(circle, rgba(240,178,74,0.40), rgba(240,178,74,0) 66%)',
  brain: 'radial-gradient(circle, rgba(245,196,90,0.42), rgba(245,196,90,0) 66%)',
  core: 'radial-gradient(circle, rgba(243,192,73,0.5), rgba(243,192,73,0) 70%)',
};
const KIND_AMP: Record<string, string> = { team: '6px', agent: '9px', delivery: '5px', brain: '11px', core: '4px' };
const hash = (s: string) => { let h = 0; for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0; return Math.abs(h); };

type Pick = (info: NodeInfo, at: [number, number]) => void;

function Sprite({ node, onPick }: { node: SceneNode; onPick: Pick }) {
  const left = (node.box[0] / W) * 100, top = (node.box[1] / H) * 100, width = ((node.box[2] - node.box[0]) / W) * 100;
  const hh = hash(node.id);
  const fdelay = (-(hh % 50) / 10).toFixed(2) + 's';
  const Icon = ICONS[node.id];
  const click = () => onPick(node.info, [(node.box[0] + node.box[2]) / 2, node.box[1] + 6]);
  return (
    <div className="kw-sprite" style={{ left: left + '%', top: top + '%', width: width + '%', ['--rev' as any]: node.reveal + 's' }}>
      <div className="kw-floatWrap" style={{ ['--fdur' as any]: (5.5 + (hh % 40) / 10).toFixed(2) + 's', ['--amp' as any]: KIND_AMP[node.kind], animationDelay: fdelay }}>
        <div className="kw-spriteGlow" style={{ background: KIND_GLOW[node.kind], animationDelay: fdelay }} />
        {Icon ? (
          <div className="kw-spriteImg" style={{ cursor: 'pointer' }} onClick={click}><Icon /></div>
        ) : (
          <img src={spriteUrl(node.spriteKey)} alt="" draggable={false} className="kw-spriteImg" onClick={click} />
        )}
      </div>
    </div>
  );
}

function NodeLabel({ node, onPick }: { node: SceneNode; onPick: Pick }) {
  if (!node.label || !node.labelPos) return null;
  const [x, y] = node.labelPos;
  const center = node.labelAlign === 'center';
  return (
    <div className={'kw-nodeLabel' + (center ? ' center' : '') + (node.kind === 'delivery' ? ' delivery' : '')}
         style={{ left: (x / W) * 100 + '%', top: (y / H) * 100 + '%', ['--rev' as any]: (node.reveal + 0.35) + 's' }}
         onClick={() => onPick(node.info, [x + (center ? 0 : 40), y - 8])}>
      <span className="kw-nlName">{node.label}</span>
      {node.tag ? <span className="kw-nlTag">{node.tag}</span> : null}
    </div>
  );
}

function CoreAccent() {
  return (
    <div className="kw-coreAccent" style={{ left: (CORE[0] / W) * 100 + '%', top: (CORE[1] / H) * 100 + '%' }}>
      <div className="kw-orbit kw-orbit1"><span className="kw-planet" /></div>
      <div className="kw-orbit kw-orbit2"><span className="kw-planet small" /></div>
      <div className="kw-coreHalo" />
    </div>
  );
}

export interface KatyotiWelcomeProps {
  className?: string;
  onSelectNode?: (id: string, info: NodeInfo) => void;
  onTaskToggle?: (task: string, on: boolean) => void;
}

export function KatyotiWelcome({ className, onSelectNode, onTaskToggle }: KatyotiWelcomeProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [active, setActive] = useState<{ info: NodeInfo; at: [number, number] } | null>(null);
  const onPick: Pick = (info, at) => setActive({ info, at });

  useEffect(() => {
    const eng = new CanvasScene(canvasRef.current!);
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setActive(null); };
    window.addEventListener('keydown', onKey);
    return () => { eng.destroy(); window.removeEventListener('keydown', onKey); };
  }, []);

  const pick: Pick = (info, at) => { onPick(info, at); const n = NODES.find((x) => x.info === info); if (n) onSelectNode?.(n.id, info); };

  return (
    <div className={'kw-viewport' + (className ? ' ' + className : '')}>
      <div className="kw-stage">
        <canvas ref={canvasRef} className="kw-bgCanvas" />
        <div className="kw-layer">
          {NODES.map((n) => <Sprite key={n.id} node={n} onPick={pick} />)}
          <CoreAccent />
          <KnowledgePanel onPick={pick} />
          <TasksPanel onPick={pick} onToggle={onTaskToggle} />
          {NODES.map((n) => <NodeLabel key={'l_' + n.id} node={n} onPick={pick} />)}
        </div>
        <div className="kw-vignette" />
        <InfoPopover active={active} onClose={() => setActive(null)} />
      </div>
    </div>
  );
}

export default KatyotiWelcome;
