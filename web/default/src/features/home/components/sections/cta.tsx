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
import { AnimateInView } from '@/components/animate-in-view'

interface CTAProps {
  isAuthenticated?: boolean
}

export function CTA(props: CTAProps) {
  const { t } = useTranslation()

  if (props.isAuthenticated) return null

  return (
    <section className='relative z-10 px-6 py-20 md:py-28'>
      <div className='mx-auto max-w-4xl'>
        <AnimateInView animation='scale-in' className='text-center'>
          <div className='glass-3 border-border/50 rounded-3xl border px-6 py-10 md:px-10 md:py-14'>
            <p className='text-muted-foreground text-xs font-medium uppercase tracking-[0.28em]'>
              {t('Start here')}
            </p>
            <h2 className='mt-4 text-3xl leading-tight font-semibold tracking-tight md:text-5xl'>
              {t('Bring your homepage up to the level of your product.')}
            </h2>
            <p className='text-muted-foreground mx-auto mt-4 max-w-2xl text-sm leading-6 md:text-base'>
              {t(
                'The backend stays untouched. The navigation stays governed by your admin settings. The homepage gets a richer story.'
              )}
            </p>
            <div className='mt-8 flex flex-wrap justify-center gap-3'>
              <Button size='lg' render={<Link to='/sign-up' />}>
                {t('Start now')}
                <ArrowRight className='ml-1.5' />
              </Button>
              <Button
                size='lg'
                variant='outline'
                render={
                  <a
                    href='https://docs.newapi.pro/en/docs'
                    target='_blank'
                    rel='noopener noreferrer'
                  />
                }
              >
                {t('Read the docs')}
              </Button>
            </div>
          </div>
        </AnimateInView>
      </div>
    </section>
  )
}
