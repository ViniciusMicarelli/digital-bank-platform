import { useGSAP } from '@gsap/react'
import gsap from 'gsap'
import { useRef, type RefObject } from 'react'

gsap.registerPlugin(useGSAP)

export const reducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches

/** The reveal every page gets: short, opacity only (never blocks taps). */
export const reveal = { y: 10, opacity: 0, duration: 0.35, ease: 'power2.out', stagger: 0.03, clearProps: 'transform,opacity' }

/**
 * Elements marked data-reveal inside the returned ref rise and fade in, one
 * after the other, when the scope mounts (or when deps change). Inside the
 * app layout, which already reveals each page, it does nothing (two tweens on
 * the same elements fight each other).
 */
export function useReveal<T extends HTMLElement>(deps: unknown[] = []): RefObject<T | null> {
  const scope = useRef<T>(null)
  useGSAP(() => {
    if (reducedMotion() || scope.current?.closest('[data-layout-main]')) return
    gsap.from('[data-reveal]', reveal)
  }, { scope, dependencies: deps })
  return scope
}

/** Animates a number between values (display only; the exact value comes from the API). */
export function useCountUp(target: number, format: (n: number) => string): RefObject<HTMLSpanElement | null> {
  const el = useRef<HTMLSpanElement>(null)
  const last = useRef(0)
  useGSAP(() => {
    if (!el.current) return
    if (reducedMotion() || !Number.isFinite(target)) {
      el.current.textContent = format(target)
      last.current = target
      return
    }
    const box = { v: last.current }
    gsap.to(box, {
      v: target, duration: 1.1, ease: 'power3.out',
      onUpdate: () => { if (el.current) el.current.textContent = format(box.v) },
      onComplete: () => { last.current = target; if (el.current) el.current.textContent = format(target) },
    })
  }, { dependencies: [target] })
  return el
}

export { gsap, useGSAP }
