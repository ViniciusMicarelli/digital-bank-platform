import { useGSAP } from '@gsap/react'
import gsap from 'gsap'
import { useRef, type RefObject } from 'react'

gsap.registerPlugin(useGSAP)

export const reducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches

/**
 * Elements marked data-reveal inside the returned ref rise and fade in, one
 * after the other, when the scope mounts (or when deps change).
 */
export function useReveal<T extends HTMLElement>(deps: unknown[] = []): RefObject<T | null> {
  const scope = useRef<T>(null)
  useGSAP(() => {
    if (reducedMotion()) return
    gsap.from('[data-reveal]', { y: 18, autoAlpha: 0, duration: 0.6, ease: 'power3.out', stagger: 0.06, clearProps: 'all' })
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
