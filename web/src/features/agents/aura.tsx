import "./aura.css"
import * as React from "react"

import { duskPalette } from "./palette"
import { seeded } from "./seed"

/**
 * boring-avatars' "marble" composition, re-implemented so its two blurred blobs can drift.
 *
 * Same shapes, colors, and seeded placement as `<Avatar variant="marble" />`, so every agent keeps
 * the picture it already had. The difference is structure: each blob is a static, pre-blurred SVG
 * inside an HTML layer, and only the layer's `transform` is animated. The browser rasterizes the
 * blur once and the compositor moves the result, so dozens of auras stay cheap.
 *
 * While the agent works (`active`), the whole picture sways a little and a soft sheen breathes:
 * CSS animations that sit paused otherwise (see aura.css).
 */

const SIZE = 80
/** Layers overhang the frame by this much on each side, so a drifting blob never shows an edge. */
const BLEED = SIZE / 4

// boring-avatars' helpers, kept verbatim so the seeded layout matches.
function hashCode(name: string): number {
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = (hash << 5) - hash + name.charCodeAt(i)
    hash = hash & hash
  }
  return Math.abs(hash)
}
const digit = (n: number, place: number) => Math.floor((n / Math.pow(10, place)) % 10)
function unit(n: number, range: number, index?: number): number {
  const value = n % range
  return index && digit(n, index) % 2 === 0 ? -value : value
}

type Blob = { color: string; translateX: number; translateY: number; scale: number; rotate: number }

/** The marble layout for a seed: background color plus the two blobs (boring-avatars' `N`). */
export function marble(seed: string, colors: string[]) {
  const n = hashCode(seed)
  const entry = (i: number): Blob => ({
    color: colors[(n + i) % colors.length] ?? "#000",
    translateX: unit(n * (i + 1), SIZE / 10, 1),
    translateY: unit(n * (i + 1), SIZE / 10, 2),
    scale: 1.2 + unit(n * (i + 1), SIZE / 20) / 10,
    rotate: unit(n * (i + 1), 360, 1),
  })
  const [bg, a, b] = [entry(0), entry(1), entry(2)]
  // The original scales both blobs by the third entry's scale; keep that quirk.
  const transform = (p: Blob) =>
    `translate(${p.translateX} ${p.translateY}) rotate(${p.rotate} ${SIZE / 2} ${SIZE / 2}) scale(${b.scale})`
  return {
    background: bg.color,
    blobs: [
      {
        d: "M32.414 59.35L50.376 70.5H72.5v-71H33.728L26.5 13.381l19.057 27.08L32.414 59.35z",
        fill: a.color,
        transform: transform(a),
      },
      {
        d: "M22.216 24L0 46.75l14.108 38.129L78 86l-3.081-59.276-22.378 4.005 12.972 20.186-23.35 27.395L22.215 24z",
        fill: b.color,
        transform: transform(b),
      },
    ],
  }
}

/**
 * Three gentle wandering paths for a blob layer: a few degrees of turn, a little travel, a breath
 * of scale. Each segment eases in and out, so the blobs seem to float rather than move.
 */
const DRIFTS: Keyframe[][] = [
  [
    "rotate(-15deg) translate(-4%, 2.5%) scale(1.05)",
    "rotate(9deg) translate(3%, -3%) scale(0.97)",
    "rotate(21deg) translate(-1.5%, -5%) scale(1.09)",
  ],
  [
    "rotate(17deg) translate(4.5%, 1.5%) scale(1.08)",
    "rotate(-6deg) translate(-2.5%, 4.5%) scale(1)",
    "rotate(-22deg) translate(-4.5%, -2.5%) scale(1.06)",
  ],
  [
    "rotate(-19deg) translate(2.5%, -4.5%) scale(0.98)",
    "rotate(5deg) translate(-4.5%, 1%) scale(1.09)",
    "rotate(16deg) translate(4%, 4%) scale(1.03)",
  ],
].map((frames) => frames.map((transform) => ({ transform, easing: "ease-in-out" })))

/** Per-seed motion: which drift path each layer follows, how fast, and where in it it starts. */
export function auraMotion(seed: string) {
  const rand = seeded(`${seed}:aura`)
  const layer = (min: number, max: number) => {
    const duration = Math.round((min + rand.next() * (max - min)) * 1000)
    return {
      drift: rand.int(0, DRIFTS.length - 1),
      duration,
      // Start mid-cycle, so auras on screen together are never in lockstep.
      delay: -Math.round(rand.next() * duration),
      direction: rand.next() < 0.5 ? ("alternate" as const) : ("alternate-reverse" as const),
    }
  }
  return {
    blobs: [layer(17, 26), layer(21, 32)],
    // No delay: the swirl sits paused at its resting frame until the agent first gets busy.
    swirl: `${(9 + rand.next() * 4).toFixed(1)}s`,
  }
}

/** One observer for every aura: drifting is paused while an aura is scrolled out of view. */
const visibility = (() => {
  const callbacks = new WeakMap<Element, (visible: boolean) => void>()
  let observer: IntersectionObserver | undefined
  return {
    watch(el: Element, onChange: (visible: boolean) => void) {
      if (typeof IntersectionObserver === "undefined") return () => {}
      observer ??= new IntersectionObserver((entries) => {
        for (const entry of entries) callbacks.get(entry.target)?.(entry.isIntersecting)
      })
      callbacks.set(el, onChange)
      observer.observe(el)
      return () => {
        observer?.unobserve(el)
        callbacks.delete(el)
      }
    },
  }
})()

/**
 * Runs the blob drift with the Web Animations API rather than CSS animations: both run on the
 * compositor, but CSS animations dispatch an `animationiteration` event at every cycle boundary,
 * and React listens for those at the root, which wakes the main thread for a style recalc each
 * time. With dozens of auras that's a steady trickle of work; WAAPI fires no iteration events.
 * Off-screen auras pause (running animations still cost a little on every main-thread frame).
 * Reduced motion: no animations at all, the still picture.
 */
function useDrift(
  root: React.RefObject<HTMLElement | null>,
  layers: React.RefObject<(HTMLElement | null)[]>,
  motion: ReturnType<typeof auraMotion>,
) {
  React.useLayoutEffect(() => {
    const els = layers.current
    // jsdom and very old browsers: no Web Animations, just the still picture.
    if (!els.every((el) => el && typeof el.animate === "function")) return undefined
    const reduce = window.matchMedia?.("(prefers-reduced-motion: reduce)")
    let running: Animation[] = []
    let visible = true
    const sync = () => {
      running.forEach((a) => a.cancel())
      running = reduce?.matches
        ? []
        : motion.blobs.map((m, i) =>
            els[i]!.animate(DRIFTS[m.drift] ?? [], {
              duration: m.duration,
              delay: m.delay,
              direction: m.direction,
              iterations: Infinity,
            }),
          )
      if (!visible) running.forEach((a) => a.pause())
    }
    sync()
    reduce?.addEventListener("change", sync)
    const unwatch = root.current
      ? visibility.watch(root.current, (now) => {
          visible = now
          running.forEach((a) => (now ? a.play() : a.pause()))
        })
      : () => {}
    return () => {
      unwatch()
      reduce?.removeEventListener("change", sync)
      running.forEach((a) => a.cancel())
    }
  }, [root, layers, motion])
}

export function Aura({
  seed,
  active = false,
  className,
  style,
}: {
  seed: string
  /** Livelier while the agent works: a slow extra swirl and a soft breathing sheen. */
  active?: boolean
  className?: string
  style?: React.CSSProperties
}) {
  const id = React.useId()
  const { background, blobs } = React.useMemo(() => marble(seed, duskPalette(seed)), [seed])
  const motion = React.useMemo(() => auraMotion(seed), [seed])
  const filter = `aura-blur-${id}`
  const root = React.useRef<HTMLSpanElement>(null)
  const layers = React.useRef<(HTMLElement | null)[]>([])
  useDrift(root, layers, motion)

  return (
    <span
      ref={root}
      role="presentation"
      aria-hidden="true"
      data-aura=""
      data-active={active ? "" : undefined}
      className={["aura", className].filter(Boolean).join(" ")}
      style={{ "--aura-bg": background, ...style } as React.CSSProperties}
    >
      <span className="aura-swirl" style={{ animationDuration: motion.swirl }}>
        {blobs.map((blob, i) => {
          return (
            <span
              key={blob.d}
              ref={(el) => {
                layers.current[i] = el
              }}
              className="aura-layer"
              data-blend={i === 1 ? "overlay" : undefined}
            >
              <svg
                viewBox={`${-BLEED} ${-BLEED} ${SIZE + 2 * BLEED} ${SIZE + 2 * BLEED}`}
                // Inline size: menus and buttons size every descendant svg as an icon (16px)
                // with selectors that outrank class styles.
                style={{ width: "100%", height: "100%", display: "block" }}
                fill="none"
                aria-hidden="true"
              >
                {i === 0 && (
                  <defs>
                    <filter
                      id={filter}
                      filterUnits="userSpaceOnUse"
                      x={-BLEED}
                      y={-BLEED}
                      width={SIZE + 2 * BLEED}
                      height={SIZE + 2 * BLEED}
                      colorInterpolationFilters="sRGB"
                    >
                      <feGaussianBlur stdDeviation={7} />
                    </filter>
                  </defs>
                )}
                <path
                  d={blob.d}
                  fill={blob.fill}
                  transform={blob.transform}
                  filter={`url(#${filter})`}
                />
              </svg>
            </span>
          )
        })}
      </span>
      <span className="aura-sheen" />
    </span>
  )
}
