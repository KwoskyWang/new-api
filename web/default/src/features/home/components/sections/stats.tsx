/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useRef, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'

interface CounterProps {
  end: number
  suffix?: string
  duration?: number
}

function Counter({ end, suffix = '', duration = 1600 }: CounterProps) {
  const ref = useRef<HTMLSpanElement>(null)
  const startedRef = useRef(false)

  const animate = useCallback(() => {
    const el = ref.current
    if (!el) return
    const start = performance.now()
    const step = (now: number) => {
      const progress = Math.min((now - start) / duration, 1)
      const eased = 1 - Math.pow(1 - progress, 3)
      el.textContent = `${Math.round(eased * end)}${suffix}`
      if (progress < 1) requestAnimationFrame(step)
    }
    requestAnimationFrame(step)
  }, [duration, end, suffix])

  useEffect(() => {
    const el = ref.current
    if (!el) return

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting && !startedRef.current) {
          startedRef.current = true
          animate()
          observer.unobserve(el)
        }
      },
      { threshold: 0.4 }
    )

    observer.observe(el)
    return () => observer.disconnect()
  }, [animate])

  return <span ref={ref}>0{suffix}</span>
}

export function Stats() {
  const { t } = useTranslation()

  const stats = [
    { end: 50, suffix: '+', label: t('upstream services integrated') },
    { end: 100, suffix: '+', label: t('compatible API routes') },
    { end: 24, suffix: '/7', label: t('monitoring and fallback coverage') },
    { end: 1, suffix: '', label: t('GLM-first entry point') },
  ]

  return (
    <section className='relative z-10 px-6 py-8'>
      <div className='mx-auto max-w-6xl'>
        <div className='border-border/50 bg-background/70 grid gap-4 rounded-2xl border px-5 py-5 md:grid-cols-4 md:px-6'>
          {stats.map((s) => (
            <div key={s.label} className='min-w-0'>
              <div className='text-2xl font-semibold tabular-nums md:text-3xl'>
                <Counter end={s.end} suffix={s.suffix} />
              </div>
              <div className='text-muted-foreground mt-1 text-xs leading-5'>
                {s.label}
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
