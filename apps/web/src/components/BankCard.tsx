import { Nfc } from 'lucide-react'
import { useId, useRef } from 'react'
import { cx } from '../lib/format'
import { gsap, reducedMotion, useGSAP } from '../lib/motion'

export type CardSkin = 'debit' | 'credit' | 'blocked' | 'temporary'

const skins: Record<CardSkin, string> = {
  debit: 'bg-[linear-gradient(135deg,#0e7a5f_0%,#0a4d3c_55%,#07362a_100%)]',
  credit: 'bg-[linear-gradient(135deg,#4a37a8_0%,#2d2075_55%,#1c1450_100%)]',
  blocked: 'bg-[linear-gradient(135deg,#475467,#1d2939)] grayscale',
  temporary: 'bg-[linear-gradient(135deg,#b45309_0%,#92400e_55%,#78350f_100%)]',
}

function Chip() {
  const id = useId()
  return (
    <svg viewBox="0 0 40 30" aria-hidden className="h-auto w-[12cqw]">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#f6e7b0" /><stop offset=".5" stopColor="#d4b062" /><stop offset="1" stopColor="#f3dc96" />
        </linearGradient>
      </defs>
      <rect x="0.5" y="0.5" width="39" height="29" rx="5" fill={`url(#${id})`} stroke="#a88a43" strokeOpacity=".5" />
      <path d="M0 10h12m16 0h12M0 20h12m16 0h12M12 0v30m16-30v30M12 10h16v10H12z" stroke="#8a6d2c" strokeOpacity=".55" fill="none" />
    </svg>
  )
}

function Network() {
  return (
    <span aria-hidden className="flex items-center">
      <span className="size-[7.5cqw] rounded-full bg-white/85" />
      <span className="-ml-[3cqw] size-[7.5cqw] rounded-full bg-white/45 ring-1 ring-white/30" />
    </span>
  )
}

export interface BankCardProps {
  holder: string
  last4: string
  expiry: string
  skin?: CardSkin
  label?: string
  /** Full number and CVV: shows the back. */
  revealed?: { pan: string; cvv: string } | null
  interactive?: boolean
  className?: string
}

/** A payment card with a light sheen that follows the pointer and a flip to its back. */
export function BankCard({ holder, last4, expiry, skin = 'debit', label = 'Débito', revealed, interactive = true, className }: BankCardProps) {
  const root = useRef<HTMLDivElement>(null)
  const inner = useRef<HTMLDivElement>(null)
  const shine = useRef<HTMLDivElement>(null)

  useGSAP(() => {
    gsap.to(inner.current, { rotateY: revealed ? 180 : 0, duration: reducedMotion() ? 0 : 0.8, ease: 'power3.inOut' })
  }, { dependencies: [!!revealed], scope: root })

  useGSAP((_, contextSafe) => {
    if (!interactive || reducedMotion() || !root.current || !contextSafe) return
    const el = root.current
    const rx = gsap.quickTo(el, 'rotateX', { duration: 0.6, ease: 'power3' })
    const ry = gsap.quickTo(el, 'rotateY', { duration: 0.6, ease: 'power3' })
    const move = contextSafe((e: PointerEvent) => {
      const r = el.getBoundingClientRect()
      const px = (e.clientX - r.left) / r.width
      const py = (e.clientY - r.top) / r.height
      rx((0.5 - py) * 10)
      ry((px - 0.5) * 12)
      gsap.to(shine.current, { '--x': `${px * 100}%`, '--y': `${py * 100}%`, opacity: 1, duration: 0.3 })
    })
    const leave = contextSafe(() => {
      rx(0)
      ry(0)
      gsap.to(shine.current, { opacity: 0, duration: 0.5 })
    })
    el.addEventListener('pointermove', move)
    el.addEventListener('pointerleave', leave)
    return () => {
      el.removeEventListener('pointermove', move)
      el.removeEventListener('pointerleave', leave)
    }
  }, { scope: root, dependencies: [interactive] })

  // Sizes in cqw (percent of the card's width) keep proportions at any size.
  const face = cx('absolute inset-0 flex flex-col justify-between overflow-hidden rounded-[6cqw] p-[6.5cqw] text-white shadow-lift ring-1 ring-white/10 [backface-visibility:hidden]', skins[skin])
  const small = 'text-[2.9cqw] tracking-wider text-white/60 uppercase'
  const pan = 'tabular font-mono text-[5.9cqw] leading-none tracking-[0.14em] text-white/95'
  return (
    <div className={cx('@container [perspective:1200px]', className)}>
      <div ref={root} className="relative aspect-[1.586] w-full [transform-style:preserve-3d]" aria-label={`Cartão ${label.toLowerCase()} final ${last4}`} role="img">
        <div ref={inner} className="absolute inset-0 [transform-style:preserve-3d]">
          {/* Each face is also hidden outright when it is not the one shown:
              backface-visibility alone fails when an ancestor fades (opacity
              flattens 3D), and the empty back would cover the front. */}
          <div className={cx(face, revealed && 'invisible transition-[visibility] delay-300')}>
            <div ref={shine} aria-hidden className="pointer-events-none absolute inset-0 opacity-0 [background:radial-gradient(circle_at_var(--x,50%)_var(--y,50%),rgb(255_255_255/0.28),transparent_45%)]" />
            <div className="flex items-start justify-between">
              <span className="text-[4.6cqw] leading-none font-bold tracking-tight">Digital Bank</span>
              <span className="rounded-full bg-white/15 px-[2.6cqw] py-[0.8cqw] text-[2.9cqw] leading-none font-semibold tracking-wide uppercase">{label}</span>
            </div>
            <div className="space-y-[4cqw]">
              <div className="flex items-center gap-[3cqw]"><Chip /><Nfc aria-hidden className="size-[6.5cqw] rotate-90 opacity-80" /></div>
              <p className={pan}>•••• •••• •••• {last4}</p>
            </div>
            <div className="flex items-end justify-between gap-[3cqw]">
              <div className="min-w-0">
                <p className={small}>Titular</p>
                <p className="truncate text-[4cqw] font-semibold tracking-wide">{holder}</p>
              </div>
              <div className="flex items-end gap-[4cqw]">
                <div><p className={small}>Validade</p><p className="tabular text-[4cqw] font-semibold">{expiry}</p></div>
                <Network />
              </div>
            </div>
          </div>
          <div aria-hidden={!revealed} className={cx(face, '[transform:rotateY(180deg)] justify-start gap-[5cqw]', !revealed && 'invisible')}>
            {revealed && <>
              <div className="-mx-[6.5cqw] mt-[2cqw] h-[11cqw] bg-black/70" />
              <p className={pan}>{revealed.pan.replace(/(\d{4})(?=\d)/g, '$1 ')}</p>
              <div className="flex items-center gap-[7cqw]">
                <div><p className={small}>Validade</p><p className="tabular text-[4cqw] font-semibold">{expiry}</p></div>
                <div><p className={small}>CVV</p><p className="tabular rounded bg-white px-[2cqw] font-mono text-[4cqw] font-bold text-ink">{revealed.cvv}</p></div>
              </div>
              <p className="mt-auto text-[2.8cqw] leading-snug text-white/55">Não compartilhe estes dados. Para compras online, prefira o cartão virtual.</p>
            </>}
          </div>
        </div>
      </div>
    </div>
  )
}
