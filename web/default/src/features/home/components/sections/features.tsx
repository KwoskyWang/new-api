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
import {
  Code,
  Gauge,
  Globe,
  Shield,
  Sparkles,
  Users,
  Wallet,
  Workflow,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AnimateInView } from '@/components/animate-in-view'

export function Features() {
  const { t } = useTranslation()

  const cards = [
    {
      icon: <Workflow className='size-4' />,
      title: t('Unified routing'),
      desc: t(
        'Requests can move through a single OpenAI-compatible entry while the system picks the best upstream path.'
      ),
    },
    {
      icon: <Shield className='size-4' />,
      title: t('Permission control'),
      desc: t(
        'Groups, tokens, model access, and admin settings remain visible and manageable.'
      ),
    },
    {
      icon: <Globe className='size-4' />,
      title: t('Multi-provider support'),
      desc: t(
        'GLM is the main character, but every other supported provider stays in the cast.'
      ),
    },
    {
      icon: <Code className='size-4' />,
      title: t('Developer friendly'),
      desc: t(
        'SDK snippets, cURL, and familiar request shapes are kept front and center.'
      ),
    },
    {
      icon: <Gauge className='size-4' />,
      title: t('Operational depth'),
      desc: t(
        'Rate limits, metrics, retries, and fallback behavior are described as product value.'
      ),
    },
    {
      icon: <Wallet className='size-4' />,
      title: t('Billing clarity'),
      desc: t(
        'Usage, quota, settlement, and dashboards are part of the homepage story.'
      ),
    },
    {
      icon: <Users className='size-4' />,
      title: t('Team workflows'),
      desc: t(
        'The homepage speaks to operators, developers, and admin users at the same time.'
      ),
    },
    {
      icon: <Sparkles className='size-4' />,
      title: t('High polish'),
      desc: t(
        'A tighter layout, better rhythm, and stronger motion language make the brand feel current.'
      ),
    },
  ]

  return (
    <section className='relative z-10 px-6 py-20 md:py-28'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-10 max-w-2xl'>
          <p className='text-muted-foreground text-xs font-medium uppercase tracking-[0.28em]'>
            {t('Core benefits')}
          </p>
          <h2 className='mt-3 text-3xl leading-tight font-semibold tracking-tight md:text-5xl'>
            {t('Built for the API-Hub story, styled like a modern studio site.')}
          </h2>
        </AnimateInView>

        <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
          {cards.map((card, index) => (
            <AnimateInView
              key={card.title}
              delay={index * 60}
              animation='fade-up'
            >
              <div className='glass-1 border-border/50 h-full rounded-2xl border p-5'>
                <div className='mb-4 flex size-10 items-center justify-center rounded-xl border border-border/50 bg-muted/30'>
                  {card.icon}
                </div>
                <h3 className='text-base font-semibold'>{card.title}</h3>
                <p className='text-muted-foreground mt-2 text-sm leading-6'>
                  {card.desc}
                </p>
              </div>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}
