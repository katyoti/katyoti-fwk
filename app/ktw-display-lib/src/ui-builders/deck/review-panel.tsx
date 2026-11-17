// SPDX-License-Identifier: Apache-2.0
// The review drawer of a deck (modules/slidedeck/, OD-017): the threads on the
// slide shown, all the threads by slide, and what changed since the last
// review marked done. The rules are review.ts's; the host keeps the file.
import { useState } from 'react'
import type { Slide } from './parse.ts'
import {
  addThread, changesSince, markReviewed, reply, setResolved, slideTitle, threadSlide,
  type DeckReview, type ReviewThread,
} from './review.ts'

export interface ReviewPanelProps {
  review: DeckReview
  slides: Slide[]
  /** The slide shown, by its index in reading order. */
  current: number
  /** Who is writing: the person's name. */
  author: string
  onChange: (review: DeckReview) => void
  /** Show slide `index`. */
  onGo: (index: number) => void
  onClose: () => void
}

type Tab = 'slide' | 'all' | 'changes'

const now = () => new Date().toISOString()
const newId = () => `t-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`
const when = (iso: string) => {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'short' })
}
const label = (slides: Slide[], i: number) => {
  const t = slides[i] ? slideTitle(slides[i]!) : ''
  return `${i + 1}${t ? ` · ${t}` : ''}`
}

/** The number of open threads, for the toolbar's button. */
export function openThreads(review: DeckReview): number {
  return review.threads.filter((t) => !t.resolved).length
}

export function ReviewPanel({ review, slides, current, author, onChange, onGo, onClose }: ReviewPanelProps) {
  const [tab, setTab] = useState<Tab>('slide')
  const [draft, setDraft] = useState('')
  // Set when the user has just marked a review: the Changes tab says so.
  const [justMarked, setJustMarked] = useState(false)
  const here = review.threads.filter((t) => threadSlide(t, slides) === current)
  const changes = changesSince(review, slides)
  const last = review.reviews?.[review.reviews.length - 1]

  const comment = () => {
    const text = draft.trim()
    if (!text) return
    onChange(addThread(review, slides, current, author, text, now(), newId()))
    setDraft('')
  }

  return (
    <aside className="deck-review" aria-label="Review">
      <header className="deck-review-head">
        <strong>Review</strong>
        <button type="button" className="deck-review-close" onClick={onClose} title="Close the review">×</button>
      </header>
      <nav className="deck-review-tabs" role="tablist">
        <button type="button" role="tab" aria-selected={tab === 'slide'} onClick={() => { setTab('slide'); setJustMarked(false) }}>This slide ({here.length})</button>
        <button type="button" role="tab" aria-selected={tab === 'all'} onClick={() => { setTab('all'); setJustMarked(false) }}>All ({review.threads.length})</button>
        <button type="button" role="tab" aria-selected={tab === 'changes'} onClick={() => setTab('changes')}>
          Changes{changes && changes.length ? ` (${changes.length})` : ''}
        </button>
      </nav>
      <div className="deck-review-body">
        {tab === 'slide' && (
          <>
            <div className="deck-review-where">Slide {label(slides, current)}</div>
            {here.length === 0 && <p className="deck-review-empty">No comment on this slide.</p>}
            {here.map((t) => (
              <Thread key={t.id} t={t}
                onReply={(text) => onChange(reply(review, slides, t.id, author, text, now()))}
                onResolve={(r) => onChange(setResolved(review, t.id, r))} />
            ))}
            <div className="deck-review-new">
              <textarea value={draft} onChange={(e) => setDraft(e.target.value)} rows={3}
                placeholder="Comment on this slide…"
                onKeyDown={(e) => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) comment() }} />
              <button type="button" onClick={comment} disabled={!draft.trim()}>Comment</button>
            </div>
          </>
        )}
        {tab === 'all' && (
          <>
            {review.threads.length === 0 && <p className="deck-review-empty">No comment yet.</p>}
            {review.threads.map((t) => {
              const at = threadSlide(t, slides)
              return (
                <button key={t.id} type="button" className="deck-review-row" data-resolved={t.resolved ? 'true' : undefined}
                  disabled={at < 0} onClick={() => { onGo(at); setTab('slide') }}>
                  <span className="deck-review-slide">{at >= 0 ? label(slides, at) : `Slide removed${t.slide.title ? ` · ${t.slide.title}` : ''}`}</span>
                  <span className="deck-review-text">{t.comments[0]?.text}</span>
                  <span className="deck-review-meta">{t.comments[0]?.author} · {t.comments.length} {t.comments.length > 1 ? 'messages' : 'message'}{t.resolved ? ' · resolved' : ''}</span>
                </button>
              )
            })}
          </>
        )}
        {tab === 'changes' && (
          <>
            {!changes && <p className="deck-review-empty">No review marked yet. Mark one done: the next look shows what changed since.</p>}
            {changes && last && (
              <div className="deck-review-where">Since the review of {when(last.at)}{last.by ? ` (${last.by})` : ''}</div>
            )}
            {justMarked && <p className="deck-review-done">✓ Marked as reviewed just now — the changes count from here.</p>}
            {changes && changes.length === 0 && !justMarked && <p className="deck-review-empty">Nothing changed.</p>}
            {changes?.map((c, i) => (
              <button key={i} type="button" className="deck-review-row" data-change={c.kind}
                disabled={c.kind === 'removed'} onClick={() => onGo(c.index)}>
                <span className="deck-review-kind">{c.kind}</span>
                <span className="deck-review-text">{c.kind === 'removed' ? `was slide ${c.index + 1}` : `slide ${c.index + 1}`}{c.title ? ` · ${c.title}` : ''}</span>
              </button>
            ))}
          </>
        )}
      </div>
      <footer className="deck-review-foot">
        <span className="deck-review-meta">
          {last ? `Last review: ${when(last.at)}${last.by ? ` · ${last.by}` : ''}` : 'Never reviewed'}
        </span>
        <button type="button"
          onClick={() => { onChange(markReviewed(review, slides, author, now())); setJustMarked(true); setTab('changes') }}
          title="Remember the slides as they are now: the Changes tab counts from here">
          ✓ Mark reviewed
        </button>
      </footer>
    </aside>
  )
}

function Thread({ t, onReply, onResolve }: { t: ReviewThread; onReply: (text: string) => void; onResolve: (resolved: boolean) => void }) {
  const [text, setText] = useState('')
  const send = () => {
    if (!text.trim()) return
    onReply(text.trim())
    setText('')
  }
  return (
    <div className="deck-review-thread" data-resolved={t.resolved ? 'true' : undefined}>
      {t.comments.map((c, i) => (
        <div key={i} className="deck-review-comment">
          <div className="deck-review-meta">{c.author} · {when(c.at)}</div>
          <div className="deck-review-text">{c.text}</div>
        </div>
      ))}
      {!t.resolved && (
        <div className="deck-review-reply">
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder="Reply…"
            onKeyDown={(e) => { if (e.key === 'Enter') send() }} />
        </div>
      )}
      <button type="button" className="deck-review-resolve" onClick={() => onResolve(!t.resolved)}>
        {t.resolved ? 'Reopen' : 'Resolve'}
      </button>
    </div>
  )
}
