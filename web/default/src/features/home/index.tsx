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
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { CTA } from './components/sections/cta'
import { Features } from './components/sections/features'
import { Hero } from './components/sections/hero'
import { HowItWorks } from './components/sections/how-it-works'
import { Stats } from './components/sections/stats'

type CodeTab = 'openai' | 'curl' | 'python' | 'node'

const CODE_SNIPPETS: Record<
  CodeTab,
  {
    title: string
    language: string
    code: string
  }
> = {
  openai: {
    title: 'OpenAI SDK',
    language: 'bash',
    code: `from openai import OpenAI

client = OpenAI(
  api_key='YOUR_API_KEY',
  base_url='https://your-domain/v1',
)

resp = client.chat.completions.create(
  model='glm-4.5',
  messages=[{'role': 'user', 'content': '你好，云镜智能'}],
)
print(resp.choices[0].message.content)`,
  },
  curl: {
    title: 'cURL',
    language: 'bash',
    code: `curl https://your-domain/v1/chat/completions \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "glm-4.5",
    "messages": [
      { "role": "user", "content": "Hello from 云镜智能" }
    ],
    "stream": true
  }'`,
  },
  python: {
    title: 'Python',
    language: 'bash',
    code: `import requests

resp = requests.post(
  'https://your-domain/v1/responses',
  headers={'Authorization': 'Bearer YOUR_API_KEY'},
  json={
    'model': 'glm-4.5',
    'input': 'Write a concise summary.',
  },
)
print(resp.json())`,
  },
  node: {
    title: 'Node.js',
    language: 'bash',
    code: `import OpenAI from 'openai'

const client = new OpenAI({
  apiKey: process.env.API_KEY,
  baseURL: 'https://your-domain/v1',
})

const res = await client.responses.create({
  model: 'glm-4.5',
  input: 'Create a one-paragraph product note.',
})

console.log(res.output_text)`,
  },
}

function CodePanel() {
  const { t } = useTranslation()
  const tabs = (Object.keys(CODE_SNIPPETS) as CodeTab[]).map((key) => ({
    key,
    ...CODE_SNIPPETS[key],
  }))

  return (
    <section className='relative z-10 px-6 pb-24 md:pb-32'>
      <div className='mx-auto max-w-6xl'>
        <div className='grid gap-6 lg:grid-cols-[1.12fr_0.88fr]'>
          <div className='glass-3 border-border/50 overflow-hidden rounded-2xl border'>
            <div className='border-border/40 flex flex-wrap items-center gap-2 border-b px-4 py-3'>
              {tabs.map((tab, index) => (
                <button
                  key={tab.key}
                  className={`rounded-full px-3 py-1.5 text-xs font-medium transition-colors ${
                    index === 0
                      ? 'bg-foreground text-background'
                      : 'text-muted-foreground hover:text-foreground'
                  }`}
                  type='button'
                >
                  {t(tab.title)}
                </button>
              ))}
              <span className='text-muted-foreground ml-auto text-[11px]'>
                {t('Multiple SDKs, one surface')}
              </span>
            </div>
            <div className='bg-background/60 grid gap-px md:grid-cols-2'>
              <div className='border-border/30 bg-background p-5'>
                <p className='text-muted-foreground mb-3 text-[11px] uppercase tracking-[0.2em]'>
                  {t('How developers connect')}
                </p>
                <h3 className='text-lg font-semibold'>
                  {t('Keep your current integration style')}
                </h3>
                <p className='text-muted-foreground mt-2 text-sm leading-6'>
                  {t(
                    'Use OpenAI-compatible APIs, cURL, Python, Node.js, Anthropic-style access, and streaming flows without changing your backend.'
                  )}
                </p>
              </div>
              <div className='border-border/30 bg-background p-5'>
                <p className='text-muted-foreground mb-3 text-[11px] uppercase tracking-[0.2em]'>
                  {t('What stays on screen')}
                </p>
                <div className='grid grid-cols-2 gap-2'>
                  {[
                    'OpenAI SDK',
                    'cURL',
                    'Responses API',
                    'Streaming',
                    'Embeddings',
                    'Rerank',
                  ].map((item) => (
                    <div
                      key={item}
                      className='border-border/40 bg-muted/30 rounded-lg border px-3 py-2 text-sm'
                    >
                      {t(item)}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>

          <div className='glass-3 border-border/50 overflow-hidden rounded-2xl border'>
            <div className='border-border/40 flex items-center justify-between border-b px-4 py-3'>
              <div>
                <p className='text-muted-foreground text-[11px] uppercase tracking-[0.2em]'>
                  {t('Routing demo')}
                </p>
                <h3 className='text-sm font-semibold'>
                  {t('GLM first, fail over automatically')}
                </h3>
              </div>
              <span className='rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2.5 py-1 text-[11px] text-emerald-500'>
                {t('Live')}
              </span>
            </div>
            <div className='space-y-4 p-5'>
              <div className='rounded-xl border border-white/10 bg-[#0b1020] p-4 text-[#dbe7ff] shadow-[0_20px_80px_-35px_rgba(79,70,229,0.65)]'>
                <div className='mb-3 flex items-center justify-between text-[11px] text-[#8aa0d6]'>
                  <span>{t('Request enters Cloud Gateway')}</span>
                  <span>{t('5 ms')}</span>
                </div>
                <div className='space-y-2 font-mono text-[11px] leading-6'>
                  <div>{'POST /v1/chat/completions'}</div>
                  <div>{'model: glm-4.5'}</div>
                  <div>{'policy: latency + cost + availability'}</div>
                  <div>{'selected provider: GLM'}</div>
                </div>
              </div>
              <div className='grid gap-3 md:grid-cols-3'>
                {[
                  ['Fallback', t('Automatic retry and fallback')],
                  ['Billing', t('Usage, quota, and settlement')],
                  ['Observe', t('Logs, metrics, and status')],
                ].map(([title, desc]) => (
                  <div
                    key={title}
                    className='border-border/40 bg-background rounded-xl border p-4'
                  >
                    <p className='mb-1 text-sm font-semibold'>{title}</p>
                    <p className='text-muted-foreground text-xs leading-5'>
                      {desc}
                    </p>
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

function DeveloperStrip() {
  const { t } = useTranslation()

  return (
    <section className='relative z-10 px-6 py-20 md:py-28'>
      <div className='mx-auto max-w-6xl'>
        <div className='grid gap-6 lg:grid-cols-[0.95fr_1.05fr]'>
          <div className='space-y-4'>
            <p className='text-muted-foreground text-xs font-medium uppercase tracking-[0.28em]'>
              {t('Developer experience')}
            </p>
            <h2 className='text-3xl leading-tight font-semibold tracking-tight md:text-5xl'>
              {t('The developer surface stays familiar.')}
            </h2>
            <p className='text-muted-foreground max-w-xl text-base leading-7'>
              {t(
                'Cloud Gateway keeps the OpenAI-compatible path, plus the special-case formats you already rely on. That means your homepage can explain the product without forcing a backend change.'
              )}
            </p>
            <div className='flex flex-wrap gap-2'>
              {[
                'OpenAI Responses',
                'Claude Messages',
                'Gemini',
                'Realtime',
                'Embeddings',
                'Images',
                'Audio',
              ].map((item) => (
                <span
                  key={item}
                  className='border-border/40 bg-muted/30 rounded-full border px-3 py-1.5 text-xs'
                >
                  {t(item)}
                </span>
              ))}
            </div>
          </div>

          <div className='grid gap-3 sm:grid-cols-2'>
            {Object.values(CODE_SNIPPETS).map((snippet, index) => (
              <div
                key={snippet.title}
                className={`glass-2 border-border/50 rounded-2xl border p-4 ${
                  index === 0 ? 'sm:col-span-2' : ''
                }`}
              >
                <div className='mb-3 flex items-center justify-between gap-3'>
                  <div>
                    <p className='text-muted-foreground text-[11px] uppercase tracking-[0.22em]'>
                      {t('Quick start')}
                    </p>
                    <h3 className='text-sm font-semibold'>{t(snippet.title)}</h3>
                  </div>
                  <span className='text-muted-foreground text-[11px]'>
                    {snippet.language}
                  </span>
                </div>
                <pre className='overflow-x-auto rounded-xl bg-[#0b1020] p-4 text-[11px] leading-6 text-[#dbe7ff]'>
                  <code>{snippet.code}</code>
                </pre>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}

function ScrollNarrative() {
  const { t } = useTranslation()

  const blocks = [
    {
      title: t('Unified gateway'),
      desc: t(
        'One surface for your users, one control plane for your team, one place to manage routes and quotas.'
      ),
    },
    {
      title: t('Model-first routing'),
      desc: t(
        'GLM leads the homepage story, while other providers remain available as a flexible pool behind it.'
      ),
    },
    {
      title: t('Operational depth'),
      desc: t(
        'The homepage should hint at the strength underneath: billing, logs, monitoring, rate limits, and channel health.'
      ),
    },
  ]

  return (
    <section className='relative z-10 px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-6xl'>
        <div className='grid gap-6 lg:grid-cols-[0.9fr_1.1fr]'>
          <div className='sticky top-24 self-start'>
            <p className='text-muted-foreground mb-3 text-xs font-medium uppercase tracking-[0.28em]'>
              {t('Scroll story')}
            </p>
            <h2 className='text-3xl leading-tight font-semibold tracking-tight md:text-5xl'>
              {t('A homepage that moves like a film, not a brochure.')}
            </h2>
            <p className='text-muted-foreground mt-4 max-w-md text-base leading-7'>
              {t(
                'As you scroll, the story should tighten from brand, to product, to developer workflow, to operational confidence.'
              )}
            </p>
          </div>

          <div className='space-y-4'>
            {blocks.map((block, index) => (
              <div
                key={block.title}
                className='glass-1 border-border/50 min-h-[180px] rounded-2xl border p-6 md:p-8'
                style={{
                  transform: `translateY(${index * 6}px)`,
                }}
              >
                <p className='text-muted-foreground mb-2 text-[11px] uppercase tracking-[0.22em]'>
                  0{index + 1}
                </p>
                <h3 className='text-xl font-semibold md:text-2xl'>
                  {block.title}
                </h3>
                <p className='text-muted-foreground mt-3 max-w-2xl text-sm leading-6 md:text-base'>
                  {block.desc}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}

export function Home() {
  const { t } = useTranslation()
  const { auth } = useAuthStore()
  const isAuthenticated = !!auth.user

  return (
    <PublicLayout showMainContainer={false}>
      <main className='relative overflow-x-clip'>
        <div
          aria-hidden
          className='pointer-events-none absolute inset-0 -z-10'
          style={{
            background: [
              'radial-gradient(circle at 18% 12%, rgba(59,130,246,0.18), transparent 28%)',
              'radial-gradient(circle at 85% 18%, rgba(16,185,129,0.14), transparent 22%)',
              'linear-gradient(180deg, rgba(2,6,23,0.02), transparent 18%, transparent 72%, rgba(2,6,23,0.04))',
            ].join(', '),
          }}
        />

        <Hero isAuthenticated={isAuthenticated} />

        <section className='relative z-10 px-6 pb-6'>
          <div className='mx-auto max-w-6xl'>
            <div className='border-border/50 bg-background/70 grid gap-4 rounded-2xl border px-5 py-5 md:grid-cols-4 md:px-6'>
              {[
                [t('Home'), t('Brand signal, first impression')],
                [t('Console'), t('Usage, control, and operations')],
                [t('Model Square'), t('GLM plus the rest of the pool')],
                [t('Docs'), t('SDKs, cURL, and fast start')],
              ].map(([title, desc]) => (
                <div key={title as string} className='min-w-0'>
                  <p className='text-sm font-semibold'>{title}</p>
                  <p className='text-muted-foreground mt-1 text-xs leading-5'>
                    {desc}
                  </p>
                </div>
              ))}
            </div>
          </div>
        </section>

        <Stats />
        <ScrollNarrative />
        <Features />
        <HowItWorks />
        <CodePanel />
        <DeveloperStrip />
        <CTA isAuthenticated={isAuthenticated} />
        <Footer />
      </main>
    </PublicLayout>
  )
}
