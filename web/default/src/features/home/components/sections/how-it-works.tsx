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
import { BarChart3, Settings, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AnimateInView } from '@/components/animate-in-view'

export function HowItWorks() {
  const { t } = useTranslation()

  const steps = [
    {
      num: '1',
      title: t('Set the surface'),
      desc: t(
        'Keep the top navigation from your admin configuration and let the homepage do the branding work underneath it.'
      ),
      icon: <Settings className='size-6' strokeWidth={1.5} />,
    },
    {
      num: '2',
      title: t('Show the routing story'),
      desc: t(
        'Present GLM first, then explain how other providers and fallback logic fit into the same gateway.'
      ),
      icon: <Zap className='size-6' strokeWidth={1.5} />,
    },
    {
      num: '3',
      title: t('Reveal the operating system'),
      desc: t(
        'Bring billing, metrics, logs, and quota control into view as the page scrolls.'
      ),
      icon: <BarChart3 className='size-6' strokeWidth={1.5} />,
    },
  ]

  return (
    <section className='relative z-10 px-6 py-20 md:py-28'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-10 max-w-2xl'>
          <p className='text-muted-foreground text-xs font-medium uppercase tracking-[0.28em]'>
            {t('Scroll structure')}
          </p>
          <h2 className='mt-3 text-3xl leading-tight font-semibold tracking-tight md:text-5xl'>
            {t('The page should feel like a controlled descent, not a list.')}
          </h2>
        </AnimateInView>

        <div className='grid gap-4 md:grid-cols-3'>
          {steps.map((step, i) => (
            <AnimateInView
              key={step.num}
              delay={i * 120}
              animation='fade-up'
            >
              <div className='glass-2 border-border/50 h-full rounded-2xl border p-6'>
                <div className='mb-5 flex items-center justify-between'>
                  <div className='flex size-12 items-center justify-center rounded-2xl border border-border/50 bg-muted/30'>
                    {step.icon}
                  </div>
                  <span className='text-muted-foreground text-3xl font-semibold tabular-nums'>
                    {step.num}
                  </span>
                </div>
                <h3 className='text-xl font-semibold'>{step.title}</h3>
                <p className='text-muted-foreground mt-3 text-sm leading-6'>
                  {step.desc}
                </p>
              </div>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}
