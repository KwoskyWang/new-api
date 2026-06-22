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
import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  Activity,
  ArrowRight,
  BarChart3,
  CheckCircle2,
  Clipboard,
  Gauge,
  LineChart,
  LockKeyhole,
  RadioTower,
  RefreshCcw,
  ShieldCheck,
  WalletCards,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { AnimateInView } from '@/components/animate-in-view'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs'
import { useAuthStore } from '@/stores/auth-store'

const API_BASE_URL = 'https://api.ispace-ai.cloud/v1'
const DEFAULT_GLM_MODEL = 'glm-5.1'
const NEXT_GLM_MODEL = 'glm-5.2'

const HOME_MEDIA = {
  hero: {
    src: 'https://ispace-resources-1304207348.cos.ap-hongkong.myqcloud.com/cloudSphere-1.mp4',
    type: 'video/mp4',
    format: 'MP4',
    recommendedSize: '1920x1080',
  },
  routing: {
    src: 'https://ispace-resources-1304207348.cos.ap-hongkong.myqcloud.com/gpt.jpg',
    type: 'image/jpeg',
    format: 'JPG',
    recommendedSize: '1920x1080',
  },
  reliability: {
    src: 'https://ispace-resources-1304207348.cos.ap-hongkong.myqcloud.com/deepseek.jpg',
    type: 'image/jpeg',
    format: 'JPG',
    recommendedSize: '1920x1080',
  },
  fallbackPoster: {
    src: 'https://ispace-resources-1304207348.cos.ap-hongkong.myqcloud.com/screen-coding.mp4',
    type: 'video/mp4',
    format: 'MP4',
    recommendedSize: '1920x1080',
  },
} as const

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
    language: 'Python',
    code: `from openai import OpenAI

client = OpenAI(
  api_key="YOUR_API_KEY",
  base_url="${API_BASE_URL}",
)

response = client.chat.completions.create(
  model="${DEFAULT_GLM_MODEL}",
  messages=[
    {"role": "user", "content": "用一句话介绍云镜智能"}
  ],
)

print(response.choices[0].message.content)`,
  },
  curl: {
    title: 'cURL',
    language: 'Shell',
    code: `curl ${API_BASE_URL}/chat/completions \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${DEFAULT_GLM_MODEL}",
    "messages": [
      { "role": "user", "content": "Hello from 云镜智能" }
    ],
    "stream": true
  }'`,
  },
  python: {
    title: 'Python',
    language: 'Python',
    code: `import requests

response = requests.post(
  "${API_BASE_URL}/chat/completions",
  headers={
    "Authorization": "Bearer YOUR_API_KEY",
    "Content-Type": "application/json",
  },
  json={
    "model": "${NEXT_GLM_MODEL}",
    "messages": [
      {"role": "user", "content": "生成一段产品摘要"}
    ],
  },
)

print(response.json())`,
  },
  node: {
    title: 'Node.js',
    language: 'TypeScript',
    code: `import OpenAI from "openai"

const client = new OpenAI({
  apiKey: process.env.API_KEY,
  baseURL: "${API_BASE_URL}",
})

const response = await client.chat.completions.create({
  model: "${DEFAULT_GLM_MODEL}",
  messages: [
    { role: "user", content: "Create a concise API Hub intro." },
  ],
})

console.log(response.choices[0].message.content)`,
  },
}

function MediaFrame(props: {
  media: (typeof HOME_MEDIA)[keyof typeof HOME_MEDIA]
  label: string
  className?: string
}) {
  const isVideo = props.media.type.startsWith('video/')

  return (
    <div
      className={`relative overflow-hidden rounded-[2rem] border border-white/10 bg-[#05070d] shadow-[0_40px_140px_-70px_rgba(15,23,42,0.95)] ${props.className ?? ''}`}
    >
      {isVideo ? (
        <video
          className='size-full object-cover'
          src={props.media.src}
          autoPlay
          muted
          loop
          playsInline
        />
      ) : (
        <img
          className='size-full object-cover'
          src={props.media.src}
          alt={props.label}
        />
      )}
      <div className='absolute inset-0 bg-[linear-gradient(180deg,rgba(0,0,0,0.18),rgba(0,0,0,0.64))]' />
      <div className='absolute inset-x-5 bottom-5 flex items-center justify-between gap-3 text-white'>
        <span className='text-sm font-medium'>{props.label}</span>
        <span className='rounded-full border border-white/20 bg-white/10 px-3 py-1 text-[11px] backdrop-blur'>
          {props.media.format} · {props.media.recommendedSize}
        </span>
      </div>
    </div>
  )
}

function Hero(props: { isAuthenticated: boolean }) {
  const { t } = useTranslation()

  const metrics = [
    ['99.9%', t('Service availability')],
    ['<100ms', t('Extra routing latency')],
    ['5+', t('Upstream providers')],
    [t('Auto'), t('Failover and retry')],
  ]

  return (
    <section className='relative isolate min-h-[calc(100vh-4rem)] overflow-hidden px-6 pt-28 pb-16 md:pt-36'>
      <div className='absolute inset-0 -z-20 bg-[#07080a]' />
      <MediaFrame
        media={HOME_MEDIA.hero}
        label={t('Hero media replacement slot')}
        className='absolute inset-0 -z-10 rounded-none border-0 opacity-70'
      />
      <div className='absolute inset-0 -z-10 bg-[radial-gradient(circle_at_65%_20%,rgba(255,255,255,0.22),transparent_18%),linear-gradient(90deg,rgba(7,8,10,0.96),rgba(7,8,10,0.62)_46%,rgba(7,8,10,0.88))]' />

      <div className='mx-auto grid max-w-7xl gap-12 lg:grid-cols-[1fr_0.82fr] lg:items-end'>
        <AnimateInView className='max-w-4xl pt-8 text-white'>
          <Badge
            variant='outline'
            className='mb-6 border-white/20 bg-white/10 text-white backdrop-blur'
          >
            <RadioTower data-icon='inline-start' />
            {t('GLM-first multi-model API gateway')}
          </Badge>
          <h1 className='max-w-4xl text-5xl leading-[0.96] font-semibold tracking-tight text-balance md:text-7xl lg:text-8xl'>
            {t('Stable access to GLM and mainstream models through one API Hub.')}
          </h1>
          <p className='mt-7 max-w-2xl text-base leading-7 text-white/72 md:text-lg'>
            {t(
              'Yunjing Intelligence gives teams one OpenAI-compatible endpoint for GLM, OpenAI, Claude, Gemini, DeepSeek, Qwen, and more, with routing strategy, fallback, billing, and observability built in.'
            )}
          </p>
          <div className='mt-9 flex flex-wrap gap-3'>
            {props.isAuthenticated ? (
              <Button size='lg' render={<Link to='/dashboard' />}>
                {t('Go to Console')}
                <ArrowRight data-icon='inline-end' />
              </Button>
            ) : (
              <>
                <Button size='lg' render={<Link to='/sign-up' />}>
                  {t('Start now')}
                  <ArrowRight data-icon='inline-end' />
                </Button>
                <Button
                  size='lg'
                  variant='outline'
                  className='border-white/25 bg-white/10 text-white hover:bg-white/15 hover:text-white'
                  render={<Link to='/pricing' />}
                >
                  {t('Explore models')}
                </Button>
              </>
            )}
          </div>
        </AnimateInView>

        <AnimateInView
          animation='fade-up'
          delay={160}
          className='grid grid-cols-2 gap-3 text-white md:grid-cols-4 lg:grid-cols-2'
        >
          {metrics.map(([value, label]) => (
            <div
              key={`${value}-${label}`}
              className='rounded-2xl border border-white/[0.14] bg-white/[0.09] p-4 backdrop-blur-md'
            >
              <div className='text-2xl font-semibold tabular-nums md:text-3xl'>
                {value}
              </div>
              <div className='mt-1 text-xs leading-5 text-white/62'>
                {label}
              </div>
            </div>
          ))}
        </AnimateInView>
      </div>
    </section>
  )
}

function TrustBar() {
  const { t } = useTranslation()
  const items = [
    t('Configurable dynamic routing policy'),
    t('Cost-aware model selection'),
    t('High availability fallback'),
    t('Unified quota and billing'),
  ]

  return (
    <section className='border-y border-border/60 bg-background px-6 py-5'>
      <div className='mx-auto flex max-w-7xl flex-wrap items-center justify-center gap-x-8 gap-y-3 text-sm text-muted-foreground'>
        {items.map((item) => (
          <span key={item} className='inline-flex items-center gap-2'>
            <CheckCircle2 className='size-4 text-foreground/70' />
            {item}
          </span>
        ))}
      </div>
    </section>
  )
}

function RoutingStrategy() {
  const { t } = useTranslation()
  const strategies = [
    {
      icon: <ShieldCheck className='size-5' />,
      title: t('Stability first'),
      desc: t(
        'Route requests toward healthy providers and avoid weak channels before they become user-visible failures.'
      ),
      signal: '99.9%',
    },
    {
      icon: <LineChart className='size-5' />,
      title: t('Cost optimization'),
      desc: t(
        'Use policy rules and model groups to keep quality stable while choosing a better cost path when possible.'
      ),
      signal: t('Lower unit cost'),
    },
    {
      icon: <Gauge className='size-5' />,
      title: t('Low latency routing'),
      desc: t(
        'Balance route health and response time so the gateway adds less than 100ms of routing overhead.'
      ),
      signal: '<100ms',
    },
    {
      icon: <RefreshCcw className='size-5' />,
      title: t('Automatic failover'),
      desc: t(
        'When a provider times out or fails, requests can move to backup channels without changing client code.'
      ),
      signal: t('Always on'),
    },
  ]

  return (
    <section className='relative bg-[#0c0d10] px-6 py-24 text-white md:py-36'>
      <div className='mx-auto grid max-w-7xl gap-10 lg:grid-cols-[0.84fr_1.16fr]'>
        <div className='lg:sticky lg:top-24 lg:self-start'>
          <p className='text-xs font-medium tracking-[0.3em] text-white/45 uppercase'>
            {t('Dynamic routing strategy')}
          </p>
          <h2 className='mt-5 text-4xl leading-none font-semibold tracking-tight text-balance md:text-6xl'>
            {t('Route for uptime, latency, and cost before your users notice anything.')}
          </h2>
          <p className='mt-6 max-w-xl text-base leading-7 text-white/62'>
            {t(
              'The homepage should sell the infrastructure promise first: GLM is the main route, but reliability comes from policy, health checks, retries, and fallback capacity.'
            )}
          </p>
        </div>

        <div className='flex flex-col gap-6'>
          <MediaFrame
            media={HOME_MEDIA.routing}
            label={t('Routing media replacement slot')}
            className='aspect-[16/10]'
          />
          <div className='grid gap-4 md:grid-cols-2'>
            {strategies.map((strategy) => (
              <AnimateInView
                key={strategy.title}
                animation='fade-up'
                className='rounded-2xl border border-white/12 bg-white/[0.06] p-5 backdrop-blur'
              >
                <div className='mb-5 flex items-center justify-between gap-3'>
                  <div className='flex size-11 items-center justify-center rounded-xl border border-white/12 bg-white/8'>
                    {strategy.icon}
                  </div>
                  <span className='text-sm font-semibold text-white/80'>
                    {strategy.signal}
                  </span>
                </div>
                <h3 className='text-xl font-semibold'>{strategy.title}</h3>
                <p className='mt-3 text-sm leading-6 text-white/60'>
                  {strategy.desc}
                </p>
              </AnimateInView>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}

function DeveloperExperience() {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<CodeTab>('openai')
  const activeSnippet = CODE_SNIPPETS[activeTab]

  const copySnippet = async () => {
    await navigator.clipboard.writeText(activeSnippet.code)
    toast.success(t('Copied to clipboard'))
  }

  return (
    <section className='relative overflow-hidden bg-background px-6 py-24 md:py-32'>
      <div className='pointer-events-none absolute inset-x-0 top-0 h-40 bg-[linear-gradient(180deg,rgba(12,13,16,0.08),transparent)]' />
      <div className='mx-auto grid max-w-7xl gap-10 lg:grid-cols-[0.82fr_1.18fr] lg:items-center'>
        <AnimateInView>
          <p className='text-xs font-medium tracking-[0.3em] text-muted-foreground uppercase'>
            {t('Built for developers')}
          </p>
          <h2 className='mt-5 text-4xl leading-tight font-semibold tracking-tight md:text-6xl'>
            {t('Keep the SDK. Change only the endpoint.')}
          </h2>
          <p className='mt-5 max-w-xl text-base leading-7 text-muted-foreground'>
            {t(
              'OpenAI SDK, cURL, Python, and Node.js examples live in one practical console. The default examples use GLM 5.1 and GLM 5.2 with your production API base URL.'
            )}
          </p>
          <div className='mt-7 grid max-w-lg gap-3 sm:grid-cols-2'>
            {[
              t('OpenAI-compatible base URL'),
              t('One API key for model access'),
              t('Streaming and non-streaming requests'),
              t('No backend route changes required'),
            ].map((item) => (
              <div key={item} className='flex items-center gap-2 text-sm'>
                <CheckCircle2 className='size-4 text-muted-foreground' />
                <span>{item}</span>
              </div>
            ))}
          </div>
        </AnimateInView>

        <AnimateInView animation='fade-up' delay={120}>
          <Tabs
            value={activeTab}
            onValueChange={(value) => setActiveTab(value as CodeTab)}
            className='rounded-3xl border border-border bg-card p-3 shadow-[0_35px_120px_-70px_rgba(15,23,42,0.45)]'
          >
            <div className='flex flex-col gap-3 border-b border-border px-2 pb-3 md:flex-row md:items-center md:justify-between'>
              <TabsList className='h-auto flex-wrap justify-start'>
                {(Object.keys(CODE_SNIPPETS) as CodeTab[]).map((key) => (
                  <TabsTrigger key={key} value={key} className='px-3 py-1.5'>
                    {t(CODE_SNIPPETS[key].title)}
                  </TabsTrigger>
                ))}
              </TabsList>
              <Button size='sm' variant='outline' onClick={copySnippet}>
                <Clipboard data-icon='inline-start' />
                {t('Copy')}
              </Button>
            </div>

            {(Object.keys(CODE_SNIPPETS) as CodeTab[]).map((key) => {
              const snippet = CODE_SNIPPETS[key]
              return (
                <TabsContent key={key} value={key} className='m-0'>
                  <div className='flex items-center justify-between gap-3 px-2 py-4'>
                    <div>
                      <p className='text-xs text-muted-foreground'>
                        {t('Base URL')}: {API_BASE_URL}
                      </p>
                      <h3 className='mt-1 text-lg font-semibold'>
                        {t(snippet.title)}
                      </h3>
                    </div>
                    <Badge variant='secondary'>{snippet.language}</Badge>
                  </div>
                  <pre className='max-h-[520px] overflow-auto rounded-2xl bg-[#080b12] p-5 text-[12px] leading-6 text-[#dbe7ff] md:text-sm'>
                    <code>{snippet.code}</code>
                  </pre>
                </TabsContent>
              )
            })}
          </Tabs>
        </AnimateInView>
      </div>
    </section>
  )
}

function GlmAndFallback() {
  const { t } = useTranslation()

  const flow = [
    [t('Client request'), 'POST /v1/chat/completions'],
    [t('Yunjing gateway'), t('Policy, quota, logs')],
    [t('Primary route'), '支持 GLM、Deepseek、Qwen'],
    [t('Backup route'), t('Healthy provider pool')],
  ]

  return (
    <section className='bg-[#0c0d10] px-6 py-24 text-white md:py-36'>
      <div className='mx-auto grid max-w-7xl gap-10 lg:grid-cols-[1.1fr_0.9fr] lg:items-center'>
        <AnimateInView animation='fade-up'>
          <MediaFrame
            media={HOME_MEDIA.reliability}
            label={t('Reliability media replacement slot')}
            className='aspect-[16/10]'
          />
        </AnimateInView>

        <AnimateInView>
          <p className='text-xs font-medium tracking-[0.3em] text-white/45 uppercase'>
            {t('GLM first, multi-model ready')}
          </p>
          <h2 className='mt-5 text-4xl leading-tight font-semibold tracking-tight md:text-6xl'>
            {t('Make GLM the main route without betting uptime on one path.')}
          </h2>
          <p className='mt-5 text-base leading-7 text-white/62'>
            {t(
              'Yunjing Intelligence can present GLM as the primary model family while keeping other upstream providers available for fallback, special workloads, and cost balancing.'
            )}
          </p>
          <div className='mt-8 flex flex-col gap-3'>
            {flow.map(([title, desc], index) => (
              <div
                key={title}
                className='grid grid-cols-[auto_1fr] gap-4 rounded-2xl border border-white/12 bg-white/[0.06] p-4'
              >
                <div className='flex size-10 items-center justify-center rounded-xl bg-white/10 text-sm font-semibold'>
                  {index + 1}
                </div>
                <div>
                  <div className='font-semibold'>{title}</div>
                  <div className='mt-1 text-sm text-white/58'>{desc}</div>
                </div>
              </div>
            ))}
          </div>
        </AnimateInView>
      </div>
    </section>
  )
}

function Operations() {
  const { t } = useTranslation()
  const items = [
    {
      icon: <Activity className='size-5' />,
      title: t('Channel health'),
      desc: t('Monitor provider availability and keep route decisions grounded in real status.'),
    },
    {
      icon: <BarChart3 className='size-5' />,
      title: t('Usage logs'),
      desc: t('Trace requests, usage, latency, and failures from one operational surface.'),
    },
    {
      icon: <WalletCards className='size-5' />,
      title: t('Quota and billing'),
      desc: t('Connect model usage to quota, settlement, group pricing, and account balance.'),
    },
    {
      icon: <LockKeyhole className='size-5' />,
      title: t('Access control'),
      desc: t('Use groups, tokens, model permissions, and limits to govern real production traffic.'),
    },
  ]

  return (
    <section className='bg-background px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-7xl'>
        <AnimateInView className='mb-12 max-w-3xl'>
          <p className='text-xs font-medium tracking-[0.3em] text-muted-foreground uppercase'>
            {t('Operations and observability')}
          </p>
          <h2 className='mt-5 text-4xl leading-tight font-semibold tracking-tight md:text-6xl'>
            {t('The gateway is also the control plane.')}
          </h2>
          <p className='mt-5 text-base leading-7 text-muted-foreground'>
            {t(
              'The homepage should reveal the serious parts underneath: logs, billing, rate limits, health checks, and model operations.'
            )}
          </p>
        </AnimateInView>

        <div className='grid gap-4 md:grid-cols-2 lg:grid-cols-4'>
          {items.map((item, index) => (
            <AnimateInView
              key={item.title}
              delay={index * 80}
              animation='fade-up'
              className='rounded-2xl border border-border bg-card p-5'
            >
              <div className='mb-5 flex size-11 items-center justify-center rounded-xl border border-border bg-muted/50'>
                {item.icon}
              </div>
              <h3 className='text-lg font-semibold'>{item.title}</h3>
              <p className='mt-3 text-sm leading-6 text-muted-foreground'>
                {item.desc}
              </p>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}

function Faq() {
  const { t } = useTranslation()
  const faqs = [
    [
      t('Is it compatible with the OpenAI SDK?'),
      t('Yes. In most cases you only replace the base URL and API key, then keep your existing SDK call pattern.'),
    ],
    [
      t('What happens if an upstream provider fails?'),
      t('The gateway can retry and move requests to configured backup channels based on health and policy.'),
    ],
    [
      t('Can GLM be the default model family?'),
      t('Yes. The homepage and examples use GLM 5.1 and GLM 5.2 first, while keeping other providers available.'),
    ],
    [
      t('Can routing help control cost?'),
      t('Yes. Group pricing, model ratios, provider selection, and fallback rules can work together to optimize cost.'),
    ],
  ]

  return (
    <section className='bg-muted/25 px-6 py-24 md:py-32'>
      <div className='mx-auto grid max-w-7xl gap-10 lg:grid-cols-[0.8fr_1.2fr]'>
        <AnimateInView>
          <p className='text-xs font-medium tracking-[0.3em] text-muted-foreground uppercase'>
            {t('FAQ')}
          </p>
          <h2 className='mt-5 text-4xl leading-tight font-semibold tracking-tight md:text-6xl'>
            {t('A practical API Hub, not a decorative landing page.')}
          </h2>
        </AnimateInView>

        <div className='grid gap-4'>
          {faqs.map(([q, a]) => (
            <AnimateInView
              key={q}
              animation='fade-up'
              className='rounded-2xl border border-border bg-background p-6'
            >
              <h3 className='text-lg font-semibold'>{q}</h3>
              <p className='mt-3 text-sm leading-6 text-muted-foreground'>
                {a}
              </p>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}

function CTA(props: { isAuthenticated: boolean }) {
  const { t } = useTranslation()
  const fallbackMedia = HOME_MEDIA.fallbackPoster
  const isFallbackVideo = fallbackMedia.type.startsWith('video/')

  return (
    <section className='relative overflow-hidden bg-[#0c0d10] px-6 py-24 text-white md:py-32'>
      <div className='absolute inset-0 opacity-35'>
        {isFallbackVideo ? (
          <video
            className='size-full object-cover'
            src={fallbackMedia.src}
            autoPlay
            muted
            loop
            playsInline
          />
        ) : (
          <img
            className='size-full object-cover'
            src={fallbackMedia.src}
            alt={t('CTA media replacement slot')}
          />
        )}
      </div>
      <div className='absolute inset-0 bg-[linear-gradient(90deg,rgba(12,13,16,0.96),rgba(12,13,16,0.68))]' />
      <AnimateInView className='relative mx-auto max-w-5xl'>
        <p className='text-xs font-medium tracking-[0.3em] text-white/45 uppercase'>
          {t('Start with one endpoint')}
        </p>
        <h2 className='mt-5 max-w-4xl text-4xl leading-tight font-semibold tracking-tight md:text-6xl'>
          {t('Connect your application to a reliable GLM-first model gateway.')}
        </h2>
        <p className='mt-5 max-w-2xl text-base leading-7 text-white/62'>
          {t(
            'Keep your current backend contracts, preserve the admin-driven navigation, and let the homepage explain why the gateway is stable, reliable, and cost-aware.'
          )}
        </p>
        <div className='mt-9 flex flex-wrap gap-3'>
          {props.isAuthenticated ? (
            <Button size='lg' render={<Link to='/dashboard' />}>
              {t('Go to Console')}
              <ArrowRight data-icon='inline-end' />
            </Button>
          ) : (
            <Button size='lg' render={<Link to='/sign-up' />}>
              {t('Start now')}
              <ArrowRight data-icon='inline-end' />
            </Button>
          )}
          <Button
            size='lg'
            variant='outline'
            className='border-white/25 bg-white/10 text-white hover:bg-white/15 hover:text-white'
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
      </AnimateInView>
    </section>
  )
}

export function Home() {
  const { auth } = useAuthStore()
  const isAuthenticated = !!auth.user

  return (
    <PublicLayout showMainContainer={false}>
      <main className='relative overflow-x-clip'>
        <Hero isAuthenticated={isAuthenticated} />
        <TrustBar />
        <RoutingStrategy />
        <DeveloperExperience />
        <GlmAndFallback />
        <Operations />
        <Faq />
        <CTA isAuthenticated={isAuthenticated} />
        <Footer />
      </main>
    </PublicLayout>
  )
}
