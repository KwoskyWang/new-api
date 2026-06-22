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
import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

interface HeroProps {
  isAuthenticated?: boolean
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()

  return (
    <section className='relative isolate overflow-hidden px-6 pt-28 pb-16 md:pt-36 md:pb-24'>
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0 -z-10 opacity-90'
        style={{
          backgroundImage:
            'linear-gradient(to right, rgba(148,163,184,0.08) 1px, transparent 1px), linear-gradient(to bottom, rgba(148,163,184,0.08) 1px, transparent 1px)',
          backgroundSize: '4rem 4rem',
          maskImage:
            'radial-gradient(ellipse 55% 40% at 50% 28%, black 18%, transparent 70%)',
        }}
      />
      <div
        aria-hidden
        className='pointer-events-none absolute left-1/2 top-24 -z-10 h-[32rem] w-[32rem] -translate-x-1/2 rounded-full bg-blue-500/10 blur-3xl'
      />

      <div className='mx-auto grid max-w-6xl gap-10 lg:grid-cols-[1.05fr_0.95fr] lg:items-center'>
        <div className='max-w-2xl'>
          <div className='mb-5 inline-flex items-center gap-2 rounded-full border border-border/60 bg-background/70 px-4 py-1.5 text-xs backdrop-blur'>
            <span className='size-2 rounded-full bg-emerald-500' />
            <span>{t('Cloud Gateway for GLM and multi-model access')}</span>
          </div>
          <h1 className='text-4xl leading-[1.05] font-semibold tracking-tight text-balance md:text-6xl'>
            {t('云镜智能')}
            <span className='text-muted-foreground block text-foreground/85'>
              {t('One API hub for GLM first, and everything around it.')}
            </span>
          </h1>
          <p className='text-muted-foreground mt-6 max-w-xl text-base leading-7 md:text-lg'>
            {t(
              'A modern API-Hub for teams that want a clean homepage, familiar developer entry points, and a serious operational backbone behind the scenes.'
            )}
          </p>

          <div className='mt-8 flex flex-wrap gap-3'>
            {props.isAuthenticated ? (
              <Button size='lg' render={<Link to='/dashboard' />}>
                {t('Go to Console')}
                <ArrowRight className='ml-1.5' />
              </Button>
            ) : (
              <>
                <Button size='lg' render={<Link to='/sign-up' />}>
                  {t('Start now')}
                  <ArrowRight className='ml-1.5' />
                </Button>
                <Button
                  size='lg'
                  variant='outline'
                  render={<Link to='/pricing' />}
                >
                  {t('Explore models')}
                </Button>
              </>
            )}
          </div>

          <div className='mt-8 flex flex-wrap gap-2 text-xs'>
            {[
              'GLM',
              'OpenAI SDK',
              'cURL',
              'Streaming',
              'Embeddings',
              'Rerank',
            ].map((item) => (
              <span
                key={item}
                className='border-border/50 bg-background/80 rounded-full border px-3 py-1.5'
              >
                {t(item)}
              </span>
            ))}
          </div>
        </div>

        <div className='relative'>
          <div className='glass-2 border-border/50 overflow-hidden rounded-3xl border p-4 shadow-[0_24px_90px_-45px_rgba(15,23,42,0.55)]'>
            <div className='relative aspect-[4/5] overflow-hidden rounded-2xl bg-[#050816]'>
              <div className='absolute inset-0 bg-[radial-gradient(circle_at_30%_25%,rgba(59,130,246,0.36),transparent_24%),radial-gradient(circle_at_72%_20%,rgba(16,185,129,0.22),transparent_20%),linear-gradient(180deg,rgba(15,23,42,0.1),rgba(15,23,42,0.75))]' />
              <img
                src='/logo.png'
                alt={t('云镜智能')}
                className='absolute left-1/2 top-1/2 w-[42%] -translate-x-1/2 -translate-y-1/2 object-contain opacity-85'
              />
              <div className='absolute inset-x-4 top-4 flex items-center justify-between text-[11px] text-white/70'>
                <span>{t('Routing, billing, logs')}</span>
                <span>{t('GLM priority')}</span>
              </div>
              <div className='absolute inset-x-4 bottom-4 grid grid-cols-3 gap-2 text-[11px] text-white/85'>
                {[
                  ['Latency', '42 ms'],
                  ['Fallback', 'On'],
                  ['Usage', 'Tracked'],
                ].map(([k, v]) => (
                  <div
                    key={k}
                    className='rounded-xl border border-white/10 bg-white/8 px-3 py-2 backdrop-blur-sm'
                  >
                    <div className='text-white/55'>{k}</div>
                    <div className='mt-1 font-medium'>{v}</div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
