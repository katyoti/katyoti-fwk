// SPDX-License-Identifier: Apache-2.0
// LoadingView — the reusable "KaTyoti is working" loading animation.
//
// A React port of the HTML loading screen (app/ktw-display render-loading.ts):
// the KaTyoti robot mascot inside a rotating golden energy halo over a
// dark-amber stage, with a caption + a rotating lighthearted activity line.
//
// Unlike the HTML version (a full-page iframe document), this component fills
// its PARENT (position:absolute inset:0), so it can be dropped into any
// container — e.g. a drawio viewer's empty state while the diagram streams in.
// Use it ANYWHERE a widget/artifact is still loading.

import { useEffect, useState } from 'react'
import { MASCOT_DATA_URI } from './mascot-data.ts'

export interface LoadingViewProps {
  /** Human noun for the caption ("KaTyoti is building your <noun>"). */
  noun?: string
  /** Overrides the whole headline (takes precedence over `noun`). */
  title?: string
  /** Overrides the explainer subtitle. */
  subtitle?: string
  /** Hide the rotating activity line (default: shown). */
  hideActivity?: boolean
  /** Drop the dark-amber stage background so the animation floats over the
   *  host surface (works on a light OR dark screen). The mascot, golden halo
   *  + glow stay; only the opaque backdrop is removed. */
  transparent?: boolean
}

const ACTIVITIES = [
  'Waking up the robots',
  'Untangling the neural spaghetti',
  'Polishing the gold trim',
  'Teaching pixels to behave',
  'Negotiating with the stream',
  'Summoning components from the void',
  'Aligning the layout chakras',
  'Convincing electrons to cooperate',
  'Reticulating splines',
  'Warming up the hamster wheel',
  'Herding async cats',
  'Bribing the render pipeline',
  'Consulting the rubber duck',
  'Tuning the golden frequencies',
  'Folding the data origami',
]

// Scoped CSS (prefixed `ktwl-`) — this renders embedded in a host page, so we
// must not leak generic class names like `.stage`/`.ring`.
const CSS = `
.ktwl-root { position: absolute; inset: 0; overflow: hidden;
  font-family: 'Montserrat', system-ui, -apple-system, 'Segoe UI', sans-serif;
  color: #f3e9d6;
  background: radial-gradient(120% 90% at 50% 38%, #2a1d08 0%, #181206 42%, #0c0905 100%); }
/* transparent variant: float over the host surface, keep the halo + glow.
   A faint backdrop glow behind the mascot keeps the (light) robot legible on
   a white canvas without an opaque box. */
.ktwl-root.ktwl-transparent { background: transparent; }
.ktwl-root.ktwl-transparent .ktwl-stage::before {
  content: ''; position: absolute; width: 380px; height: 380px; border-radius: 50%;
  top: 50%; left: 50%; transform: translate(-50%, -58%);
  background: radial-gradient(closest-side, rgba(40,29,8,0.55), rgba(24,18,6,0.25) 60%, transparent 75%);
  pointer-events: none; }
.ktwl-root.ktwl-transparent .ktwl-title { text-shadow: 0 1px 6px rgba(0,0,0,0.45); }
.ktwl-root.ktwl-transparent .ktwl-sub,
.ktwl-root.ktwl-transparent .ktwl-act { text-shadow: 0 1px 5px rgba(0,0,0,0.4); }
.ktwl-stage { position: absolute; inset: 0; display: flex; flex-direction: column;
  align-items: center; justify-content: center; gap: 22px; padding: 20px; text-align: center; }
.ktwl-orb { position: relative; width: 220px; height: 220px; }
.ktwl-ring { position: absolute; inset: 0; border-radius: 50%;
  -webkit-mask: radial-gradient(closest-side, transparent 70%, #000 72%, #000 93%, transparent 96%);
          mask: radial-gradient(closest-side, transparent 70%, #000 72%, #000 93%, transparent 96%);
  filter: blur(0.6px); mix-blend-mode: screen; }
.ktwl-ring.a { background: conic-gradient(from 0deg, transparent 0deg, #f5b13d 55deg, #ffe7a8 90deg, transparent 150deg, transparent 360deg);
  animation: ktwl-spin 4.6s linear infinite; opacity: .8; }
.ktwl-ring.b { background: conic-gradient(from 180deg, transparent 0deg, #c8841f 45deg, #ffd680 80deg, transparent 140deg, transparent 360deg);
  animation: ktwl-spinB 6.4s linear infinite; opacity: .55; transform: scale(.9); }
@keyframes ktwl-spin  { to { transform: rotate(360deg); } }
@keyframes ktwl-spinB { from { transform: scale(.9) rotate(0); } to { transform: scale(.9) rotate(-360deg); } }
.ktwl-glow { position: absolute; inset: 24%; border-radius: 50%;
  background: radial-gradient(closest-side, rgba(245,177,61,0.4), rgba(245,177,61,0.05) 70%, transparent 75%);
  animation: ktwl-breathe 2.8s ease-in-out infinite; }
@keyframes ktwl-breathe { 0%,100% { opacity: .5; transform: scale(.92); } 50% { opacity: 1; transform: scale(1.06); } }
.ktwl-bot { position: absolute; inset: 6%; background: center/contain no-repeat;
  filter: drop-shadow(0 10px 22px rgba(0,0,0,0.5)) drop-shadow(0 0 18px rgba(245,177,61,0.3));
  animation: ktwl-bob 3.4s ease-in-out infinite; }
@keyframes ktwl-bob { 0%,100% { transform: translateY(4px) scale(0.99); } 50% { transform: translateY(-7px) scale(1.01); } }
.ktwl-copy { max-width: 460px; display: flex; flex-direction: column; align-items: center; gap: 8px; }
.ktwl-title { font-size: 18px; font-weight: 600; color: #ffe7a8; }
.ktwl-title b { color: #f5b13d; font-weight: 700; }
.ktwl-sub { font-size: 13px; line-height: 1.5; color: #c9b894; }
.ktwl-bar { position: relative; width: 220px; height: 4px; border-radius: 4px;
  background: rgba(245,177,61,0.14); overflow: hidden; }
.ktwl-bar::after { content: ''; position: absolute; top: 0; left: 0; height: 100%; width: 40%;
  border-radius: 4px; background: linear-gradient(90deg, transparent, #f5b13d, #ffe7a8, transparent);
  animation: ktwl-sweep 1.5s ease-in-out infinite; }
@keyframes ktwl-sweep { 0% { left: -45%; } 100% { left: 105%; } }
.ktwl-act { font-size: 12.5px; color: #b9a988; min-height: 16px; }
@media (prefers-reduced-motion: reduce) {
  .ktwl-ring.a, .ktwl-ring.b { animation: none; }
  .ktwl-glow, .ktwl-bot { animation-duration: 6s; }
  .ktwl-bar::after { animation-duration: 3s; }
}
`

export function LoadingView({ noun = 'content', title, subtitle, hideActivity, transparent }: LoadingViewProps) {
  const [act, setAct] = useState(ACTIVITIES[0])

  useEffect(() => {
    if (hideActivity) return
    let last = 0
    const id = window.setInterval(() => {
      let i = Math.floor(Math.random() * ACTIVITIES.length)
      if (i === last) i = (i + 1) % ACTIVITIES.length
      last = i
      setAct(ACTIVITIES[i])
    }, 2600)
    return () => window.clearInterval(id)
  }, [hideActivity])

  const headline = title ?? `KaTyoti is preparing your <b>${noun}</b>`
  const sub =
    subtitle ?? 'The assistant is thinking and generating the result — it appears here as soon as it is ready.'

  return (
    <div
      className={transparent ? 'ktwl-root ktwl-transparent' : 'ktwl-root'}
      role="status"
      aria-live="polite"
      aria-label={`Loading ${noun}`}
    >
      <style>{CSS}</style>
      <div className="ktwl-stage">
        <div className="ktwl-orb">
          <div className="ktwl-glow" />
          <div className="ktwl-ring a" />
          <div className="ktwl-ring b" />
          <div className="ktwl-bot" style={{ backgroundImage: `url('${MASCOT_DATA_URI}')` }} />
        </div>
        <div className="ktwl-copy">
          <div className="ktwl-title" dangerouslySetInnerHTML={{ __html: headline }} />
          <div className="ktwl-sub">{sub}</div>
          <div className="ktwl-bar" aria-hidden="true" />
          {!hideActivity && <div className="ktwl-act">{act}…</div>}
        </div>
      </div>
    </div>
  )
}

export default LoadingView
