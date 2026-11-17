// SPDX-License-Identifier: Apache-2.0
/**
 * GuidePlayer — plays a recipe over the GUI's own controls (gt12).
 *
 * Mounted ONCE in BaseLayout, so it plays on the universe picker as well as
 * inside a universe. For each step: resolve the target (resolveTarget.ts),
 * dim the page around it (a spotlight div whose huge box-shadow is the dim;
 * pointer-events none, so the person can use the control), show the callout,
 * and wait for what completes the step:
 *   - `click`  a capture-phase document click inside the resolved target;
 *   - `route`  the active tab kind or the main-pane route equals `step.route`
 *              (zustand subscribe on both stores);
 *   - `manual` the person presses Next.
 * A step whose control cannot be found plays as manual without a highlight,
 * and the control is reported (console + the store's `missing` list).
 * A step's `check` runs when the step is entered; `do` runs after an explicit
 * confirmation. The step's text names controls as `{control:<anchor>}`; the
 * callout says each one's label as the GUI shows it (./controls.ts), read
 * again as the screen changes.
 */

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react'
import { Badge, Button, FocusTrap, Group, Loader, Paper, Portal, Text } from '@mantine/core'
import { IconAlertTriangle, IconCheck, IconX } from '@tabler/icons-react'
import { Markdown } from '../components/Markdown'
import { useAppStore } from '../store'
import { useTabsStore } from '../store/tabsSlice'
import type { LoadedRecipe } from './recipes'
import { useGuidePlayer } from './player'
import { isTour, runCheck, runStepAction, type CheckResult, type Step } from './recipes'
import { errorText, errorTextWith, type ErrorMsg } from '../i18n/errors'
import { resolveTarget } from './resolveTarget'
import { stepAddress } from '../address'
import { FURTHER_STEP, furtherShown, furtherTarget } from './further'
import { fetchRecipe } from './recipes'
import { openGuide } from './player'
import { openAddress } from '../sections/shell/openAddress'
import { useGuideT } from './i18n'
import { tFor, useAppLanguage } from '../i18n'
import { recipeText } from './recipes'
import { CONTROL_PLACEHOLDER, liveControlLabel, sayControls } from './controls'

const Z = 1000
const PAD = 6
const GAP = 12
const MARGIN = 8
/** A target still absent this long after the step starts is reported. */
const MISSING_GRACE_MS = 1500

interface Box {
  top: number
  left: number
  width: number
  height: number
}

function sameBox(a: Box | null, b: Box | null): boolean {
  if (!a || !b) return a === b
  return a.top === b.top && a.left === b.left && a.width === b.width && a.height === b.height
}

/** The current "where am I": the displayed universe's active tab kind and the
 *  main-pane route kind. */
function currentRoutes(): string[] {
  const app = useAppStore.getState()
  const out: string[] = [app.mainPaneRoute.kind]
  const u = app.displayedUniverseName
  if (u) {
    const t = useTabsStore.getState().getForUniverse(u)
    const active = t.openTabs.find((x) => x.id === t.activeTabId)
    if (active) out.push(active.kind)
  }
  return out
}

export function GuidePlayer() {
  const status = useGuidePlayer((s) => s.status)
  const uri    = useGuidePlayer((s) => s.uri)
  const recipe = useGuidePlayer((s) => s.recipe)
  const step   = useGuidePlayer((s) => s.step)
  const error  = useGuidePlayer((s) => s.error)
  const close  = useGuidePlayer((s) => s.close)
  const t      = useGuideT()

  if (status === 'idle') return null

  if (status !== 'playing' || !recipe || !uri) {
    return (
      <Portal>
        <div style={{ position: 'fixed', inset: 0, zIndex: Z, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.35)' }}>
          <Paper shadow="xl" radius="md" p="md" withBorder style={{ width: 340 }}>
            {status === 'loading' ? (
              <Group gap="xs"><Loader size="xs" /><Text size="sm">{t('player.loading')}</Text></Group>
            ) : (
              <>
                <Text size="sm" c="red" mb="sm">{t('player.openFailed', { error })}</Text>
                <Group justify="flex-end"><Button size="xs" variant="default" onClick={close}>{t('player.close')}</Button></Group>
              </>
            )}
          </Paper>
        </div>
      </Portal>
    )
  }

  return <StepOverlay key={`${uri}#${step}`} uri={uri} recipe={recipe} index={step} />
}

type CheckState = { status: 'idle' | 'running' | 'ok' | 'fail'; detail?: string; detailMsg?: ErrorMsg | null }

function StepOverlay({ uri, recipe, index }: { uri: string; recipe: LoadedRecipe; index: number }) {
  const step: Step = recipe.steps[index]
  const total = recipe.steps.length
  const last = index === total - 1
  const tour = isTour(recipe)
  const lastCheckIndex = recipe.steps.reduce((acc, s, i) => (s.check ? i : acc), -1)
  const t = useGuideT()
  const translations = useGuidePlayer((s) => s.translations)
  const lang = useAppLanguage()

  const nextRaw        = useGuidePlayer((s) => s.next)
  // One step advances once: a click and a store change landing together
  // must not skip the following step.
  const advancedRef = useRef(false)
  const next = useCallback(() => {
    if (advancedRef.current) return
    advancedRef.current = true
    nextRaw()
  }, [nextRaw])
  const back           = useGuidePlayer((s) => s.back)
  const close          = useGuidePlayer((s) => s.close)
  const reportMissing  = useGuidePlayer((s) => s.reportMissing)
  const complete       = useGuidePlayer((s) => s.complete)
  const missingCount   = useGuidePlayer((s) => s.missing.filter((m) => m.recipe === uri).length)

  const overlayRef = useRef<HTMLDivElement>(null)
  const calloutRef = useRef<HTMLDivElement>(null)
  const targetRef  = useRef<HTMLElement | null>(null)

  const [box, setBox]               = useState<Box | null>(null)
  const [found, setFound]           = useState(false)
  const [missing, setMissing]       = useState(false)
  const [viewport, setViewport]     = useState({ w: window.innerWidth, h: window.innerHeight })
  const [callout, setCallout]       = useState({ w: 360, h: 180 })
  const [satisfied, setSatisfied]   = useState(false)
  const [check, setCheck]           = useState<CheckState>({ status: 'idle' })
  const [confirmDo, setConfirmDo]   = useState(false)
  const [doState, setDoState]       = useState<CheckState>({ status: 'idle' })

  // ── A step naming an element opens it first: the target
  // below is then found as the element appears.
  useEffect(() => {
    const universe = useAppStore.getState().displayedUniverseName
    if (step.open && universe) void openAddress(stepAddress(universe, step.open))
  }, [step.open, index])

  // ── Target resolution: now, on DOM mutations (throttled), on resize/scroll.
  useEffect(() => {
    let scrolled = false
    const update = () => {
      const el = resolveTarget(step.target, overlayRef.current)
      targetRef.current = el
      setFound(!!el)
      if (el) {
        setMissing(false)
        if (!scrolled) {
          scrolled = true
          el.scrollIntoView({ block: 'nearest', inline: 'nearest' })
        }
        const r = el.getBoundingClientRect()
        const nb = { top: r.top, left: r.left, width: r.width, height: r.height }
        setBox((prev) => (sameBox(prev, nb) ? prev : nb))
      } else {
        setBox((prev) => (prev === null ? prev : null))
      }
      setViewport((v) => (v.w === window.innerWidth && v.h === window.innerHeight ? v : { w: window.innerWidth, h: window.innerHeight }))
    }
    update()
    if (!step.target) return

    let timer: number | null = null
    const schedule = () => {
      if (timer !== null) return
      timer = window.setTimeout(() => { timer = null; update() }, 100)
    }
    const mo = new MutationObserver((records) => {
      const ov = overlayRef.current
      if (ov && records.every((r) => ov.contains(r.target))) return // our own re-render
      schedule()
    })
    mo.observe(document.body, { childList: true, subtree: true, attributes: true })
    window.addEventListener('resize', schedule)
    window.addEventListener('scroll', schedule, true)
    const miss = window.setTimeout(() => {
      if (!targetRef.current) {
        setMissing(true)
        reportMissing(index, step.target)
      }
    }, MISSING_GRACE_MS)
    return () => {
      mo.disconnect()
      window.removeEventListener('resize', schedule)
      window.removeEventListener('scroll', schedule, true)
      if (timer !== null) window.clearTimeout(timer)
      window.clearTimeout(miss)
    }
  }, [step.target, index, reportMissing])

  useEffect(() => {
    if (step.target) return
    const onResize = () => setViewport({ w: window.innerWidth, h: window.innerHeight })
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [step.target])

  // ── The step's text: each `{control:…}` said as the GUI labels it now —
  // the control on screen first, else its registry key — re-read while the
  // screen changes (a dialog opening shows the control a later sentence names).
  const rawSay = recipeText({ recipe, translations }, lang).say(step)
  const [said, setSaid] = useState('')
  useLayoutEffect(() => {
    const say = () => sayControls(rawSay, {
      live: (anchor, name) => liveControlLabel(anchor, name),
      translate: (nsKey) => {
        const i = nsKey.indexOf(':')
        const key = nsKey.slice(i + 1)
        const out = tFor(nsKey.slice(0, i), lang)(key, { universe: useAppStore.getState().displayedUniverseName ?? '' })
        return out && out !== key ? out : null
      },
    })
    setSaid(say())
    CONTROL_PLACEHOLDER.lastIndex = 0
    if (!CONTROL_PLACEHOLDER.test(rawSay)) return
    let timer: number | null = null
    const mo = new MutationObserver(() => {
      if (timer !== null) return
      timer = window.setTimeout(() => { timer = null; setSaid(say()) }, 150)
    })
    mo.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['data-guide-label', 'data-guide'] })
    return () => {
      mo.disconnect()
      if (timer !== null) window.clearTimeout(timer)
    }
  }, [rawSay, lang])

  // A step waits for a click only when its control is on screen.
  const awaitKind = step.await ?? 'manual'
  const effectiveAwait = awaitKind === 'click' && !found ? 'manual' : awaitKind

  // ── await: click — capture phase, so the control's own handler still runs.
  useEffect(() => {
    if (effectiveAwait !== 'click') return
    const onClick = (e: MouseEvent) => {
      const t = targetRef.current
      if (t && e.target instanceof Node && t.contains(e.target)) {
        window.setTimeout(() => next(), 0)
      }
    }
    document.addEventListener('click', onClick, true)
    return () => document.removeEventListener('click', onClick, true)
  }, [effectiveAwait, next])

  // ── await: route — already there on entry = satisfied (Next shown);
  // arriving there completes the step.
  useEffect(() => {
    if (awaitKind !== 'route' || !step.route) return
    const want = step.route
    if (currentRoutes().includes(want)) setSatisfied(true)
    const onChange = () => {
      if (currentRoutes().includes(want)) next()
    }
    const u1 = useAppStore.subscribe(onChange)
    const u2 = useTabsStore.subscribe(onChange)
    return () => { u1(); u2() }
  }, [awaitKind, step.route, next])

  // ── check: run on entry and on demand. The last check passing completes
  // the recipe as `checked`.
  const doCheck = useCallback(async () => {
    if (!step.check) return
    setCheck({ status: 'running' })
    const res = await runCheck(step.check, useAppStore.getState().displayedUniverseName, step.with)
    setCheck({ status: res.ok ? 'ok' : 'fail', detail: res.detail, detailMsg: res.detailMsg })
    if (res.ok && index === lastCheckIndex) void complete(uri, 'checked')
  }, [step.check, step.with, index, lastCheckIndex, uri, complete])

  useEffect(() => { void doCheck() }, [doCheck])

  const doIt = async () => {
    if (!step.do) return
    setConfirmDo(false)
    setDoState({ status: 'running' })
    try {
      const out = (await runStepAction(step.do, useAppStore.getState().displayedUniverseName, step.with)) as Partial<CheckResult> | null
      setDoState({ status: 'ok', detail: out?.detail ?? t('player.done'), detailMsg: out?.detailMsg })
      if (step.check) void doCheck()
    } catch (e) {
      setDoState({ status: 'fail', detail: errorText(e) })
    }
  }

  const nextEnabled = step.check
    ? check.status === 'ok'
    : effectiveAwait === 'manual' || (effectiveAwait === 'route' && satisfied) || (effectiveAwait === 'route' && !step.route)

  // ── keyboard: Escape leaves (or cancels the confirmation).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape' || e.defaultPrevented) return
      if (confirmDo) setConfirmDo(false)
      else close()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [confirmDo, close])

  // ── callout placement: below the target, else above, clamped; centred
  // when there is no target.
  useLayoutEffect(() => {
    const el = calloutRef.current
    if (!el) return
    const w = el.offsetWidth
    const h = el.offsetHeight
    setCallout((c) => (c.w === w && c.h === h ? c : { w, h }))
  })

  let top: number
  let left: number
  if (box) {
    const below = box.top + box.height + PAD + GAP
    const above = box.top - PAD - GAP - callout.h
    top = below + callout.h <= viewport.h - MARGIN ? below : above >= MARGIN ? above : viewport.h - callout.h - MARGIN
    left = box.left + box.width / 2 - callout.w / 2
  } else {
    top = (viewport.h - callout.h) / 2
    left = (viewport.w - callout.w) / 2
  }
  top = Math.max(MARGIN, Math.min(top, viewport.h - callout.h - MARGIN))
  left = Math.max(MARGIN, Math.min(left, viewport.w - callout.w - MARGIN))

  const onCalloutKey = (e: ReactKeyboardEvent) => {
    if (e.key !== 'Enter' || !nextEnabled || confirmDo) return
    const t = e.target as HTMLElement
    if (t.tagName === 'BUTTON' || t.tagName === 'A' || t.tagName === 'INPUT') return
    e.preventDefault()
    next()
  }

  const dim = 'rgba(0,0,0,0.45)'

  return (
    <Portal>
      <div ref={overlayRef} data-guide-overlay="true" style={{ position: 'fixed', inset: 0, zIndex: Z, pointerEvents: 'none' }}>
        {box ? (
          <div
            aria-hidden
            style={{
              position: 'fixed',
              top: box.top - PAD,
              left: box.left - PAD,
              width: box.width + PAD * 2,
              height: box.height + PAD * 2,
              borderRadius: 8,
              boxShadow: `0 0 0 9999px ${dim}`,
              outline: '2px solid var(--mantine-color-blue-5)',
              transition: 'top 120ms, left 120ms, width 120ms, height 120ms',
              pointerEvents: 'none',
            }}
          />
        ) : (
          <div aria-hidden style={{ position: 'fixed', inset: 0, background: dim, pointerEvents: 'none' }} />
        )}

        <FocusTrap active>
          <Paper
            ref={calloutRef}
            role="dialog"
            aria-modal="false"
            aria-label={t('player.stepLabel', { name: recipeText({ recipe, translations }, lang).name, n: index + 1, total })}
            shadow="xl"
            radius="md"
            p="md"
            withBorder
            onKeyDown={onCalloutKey}
            style={{ position: 'fixed', top, left, width: 360, maxWidth: `calc(100vw - ${MARGIN * 2}px)`, pointerEvents: 'auto' }}
          >
            <Group justify="space-between" mb={4} wrap="nowrap">
              <Text size="xs" c="dimmed" truncate>{recipeText({ recipe, translations }, lang).name}</Text>
              <Badge size="xs" variant="light">{t('player.counter', { n: index + 1, total })}</Badge>
            </Group>

            {step.id === FURTHER_STEP
              ? <FurtherList uri={uri} recipe={recipe} onOpen={close} />
              : <Markdown text={said} />}

            {missing && step.target && (
              <Group gap={4} mt={4} wrap="nowrap" align="flex-start">
                <IconAlertTriangle size={14} color="var(--mantine-color-orange-6)" style={{ flexShrink: 0, marginTop: 2 }} />
                <Text size="xs" c="orange">
                  {t('player.missing')}
                  {missingCount > 0 && ` ${t('player.missingCount', { count: missingCount })}`}
                </Text>
              </Group>
            )}

            {effectiveAwait === 'click' && (
              <Text size="xs" c="dimmed" mt={4}>{t('player.awaitClick')}</Text>
            )}
            {effectiveAwait === 'route' && !satisfied && step.route && (
              <Text size="xs" c="dimmed" mt={4}>{t('player.awaitRoute')}</Text>
            )}

            {step.check && check.status !== 'idle' && (
              <Group gap={4} mt={6} wrap="nowrap" align="flex-start">
                {check.status === 'running' && <Loader size={12} />}
                {check.status === 'ok' && <IconCheck size={14} color="var(--mantine-color-green-6)" style={{ flexShrink: 0, marginTop: 2 }} />}
                {check.status === 'fail' && <IconX size={14} color="var(--mantine-color-red-6)" style={{ flexShrink: 0, marginTop: 2 }} />}
                <Text size="xs" c={check.status === 'ok' ? 'green' : check.status === 'fail' ? 'red' : 'dimmed'}>
                  {check.status === 'running' ? t('player.checking') : errorTextWith(t, check.detail, check.detailMsg)}
                </Text>
              </Group>
            )}

            {step.do && doState.status !== 'idle' && (
              <Text size="xs" mt={4} c={doState.status === 'fail' ? 'red' : 'dimmed'}>
                {doState.status === 'running' ? t('player.running') : errorTextWith(t, doState.detail, doState.detailMsg)}
              </Text>
            )}

            {confirmDo && step.do ? (
              <Paper withBorder p="xs" mt="sm" radius="sm">
                <Text size="xs" mb={6}>{t('player.confirmDoBefore')} <code>{step.do}</code> {t('player.confirmDoAfter')}</Text>
                <Group gap="xs" justify="flex-end">
                  <Button size="compact-xs" variant="default" onClick={() => setConfirmDo(false)}>{t('player.cancel')}</Button>
                  <Button size="compact-xs" onClick={() => void doIt()} data-autofocus>{t('player.runIt')}</Button>
                </Group>
              </Paper>
            ) : null}

            <Group justify="space-between" mt="sm" wrap="nowrap">
              <Button size="compact-xs" variant="subtle" color="gray" onClick={close} data-guide="player.skip">
                {tour ? t('player.skipTour') : t('player.skip')}
              </Button>
              <Group gap={6} wrap="nowrap">
                <Button size="compact-xs" variant="default" onClick={back} disabled={index === 0}>{t('player.back')}</Button>
                {step.do && (
                  <Button size="compact-xs" variant="light" onClick={() => setConfirmDo(true)} disabled={doState.status === 'running'} data-guide="guide.do-it">
                    {t('player.doIt')}
                  </Button>
                )}
                {step.check && (
                  <Button size="compact-xs" variant="light" onClick={() => void doCheck()} loading={check.status === 'running'} data-guide="guide.check">
                    {t('player.check')}
                  </Button>
                )}
                {nextEnabled && (
                  <Button size="compact-xs" onClick={next} data-autofocus={!confirmDo || undefined}>
                    {last ? t('player.finish') : t('player.next')}
                  </Button>
                )}
              </Group>
            </Group>
          </Paper>
        </FocusTrap>
      </div>
    </Portal>
  )
}

/**
 * « Pour aller plus loin »: the recipe's `next`, each a button
 * - a guide by its name in the person's language, opening it; a document by
 * its address, opening it in the universe shown.
 */
function FurtherList({ uri, recipe, onOpen }: { uri: string; recipe: LoadedRecipe; onOpen: () => void }) {
  const t = useGuideT()
  const lang = useAppLanguage()
  const next = (recipe as { next?: string[] }).next ?? []
  // A guide shows once loaded: one this universe does not have (its sidecar is
  // not a member) never does (the usage guide names a solution's guides).
  const [names, setNames] = useState<Record<string, string>>({})
  useEffect(() => {
    const universe = useAppStore.getState().displayedUniverseName
    for (const e of next) {
      const target = furtherTarget(e, uri)
      if (target.kind !== 'guide') continue
      void fetchRecipe(target.uri, universe)
        .then((entry) => setNames((n) => ({ ...n, [e]: recipeText(entry, lang).name || e })))
        .catch(() => undefined)
    }
  }, [uri, lang, next.join('|')])
  const open = (e: string) => {
    const target = furtherTarget(e, uri)
    const universe = useAppStore.getState().displayedUniverseName
    onOpen()
    if (target.kind === 'address') {
      if (universe) void openAddress(stepAddress(universe, target.address))
      return
    }
    void fetchRecipe(target.uri, universe).then(openGuide).catch(() => undefined)
  }
  return (
    <div data-guide-step="further">
      <Text fw={600} size="sm" mb={4}>{t('player.further')}</Text>
      <Text size="xs" c="dimmed" mb={6}>{t('player.furtherIntro')}</Text>
      <Group gap={6}>
        {furtherShown(next, names, uri).map((e) => (
          <Button key={e} size="compact-xs" variant="light" onClick={() => open(e)}>{names[e] ?? e}</Button>
        ))}
      </Group>
    </div>
  )
}
