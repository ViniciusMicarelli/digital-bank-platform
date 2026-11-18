import { useEffect, useRef, useState } from 'react'
import { useReducedMotion } from 'react-native-reanimated'

/** A number that eases from its previous value to target (display only). */
export function useCountUp(target: number, duration = 900): number {
  const reduced = useReducedMotion()
  const [value, setValue] = useState(target)
  const from = useRef(0)
  useEffect(() => {
    if (reduced || !Number.isFinite(target)) {
      from.current = target
      return
    }
    const start = performance.now()
    const a = from.current
    let raf = 0
    const tick = (now: number) => {
      const t = Math.min(1, (now - start) / duration)
      const eased = 1 - Math.pow(1 - t, 3)
      setValue(a + (target - a) * eased)
      if (t < 1) raf = requestAnimationFrame(tick)
      else from.current = target
    }
    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [target, duration, reduced])
  return reduced || !Number.isFinite(target) ? target : value
}
