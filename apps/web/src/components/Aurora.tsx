import { useRef } from 'react'
import { gsap, reducedMotion, useGSAP } from '../lib/motion'

/** Soft color fields behind the glass, drifting slowly. */
export function Aurora() {
  const root = useRef<HTMLDivElement>(null)
  useGSAP(() => {
    if (reducedMotion()) return
    gsap.utils.toArray<HTMLElement>('[data-blob]').forEach((el, i) => {
      gsap.to(el, {
        x: () => gsap.utils.random(-120, 120), y: () => gsap.utils.random(-90, 90), scale: () => gsap.utils.random(0.85, 1.2),
        duration: 14 + i * 4, ease: 'sine.inOut', repeat: -1, yoyo: true, repeatRefresh: true,
      })
    })
  }, { scope: root })
  return (
    <div ref={root} aria-hidden className="pointer-events-none fixed inset-0 -z-10 overflow-hidden bg-bg">
      <div data-blob className="absolute -top-40 -left-32 size-[42rem] rounded-full bg-[var(--aurora-1)] blur-[110px]" />
      <div data-blob className="absolute top-1/3 -right-40 size-[38rem] rounded-full bg-[var(--aurora-2)] blur-[120px]" />
      <div data-blob className="absolute -bottom-48 left-1/4 size-[36rem] rounded-full bg-[var(--aurora-3)] blur-[120px]" />
    </div>
  )
}
