/**
 * "Horizon" — the public marketing landing page for Tickwind.
 *
 * Design direction: ultra-clean editorial minimal. A near-monochrome white/zinc
 * surface with a single confident indigo accent used sparingly, big calm
 * typography, generous whitespace, and flat hairline borders (no shadows).
 *
 * This route owns its entire visual frame — its own top nav, hero, live demo,
 * value props, and footer — and renders as a self-contained light surface so
 * the editorial palette holds regardless of the surrounding app chrome.
 */

import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {LiveDemo} from './LiveDemo';
import {PulseIcon, FeedIcon, ChatterIcon, ArrowUpRightIcon} from './icons';

export const metadata: Metadata = {
  title: 'Tickwind — Read every tick. See where the market’s blowing.',
  description:
    'A calm, personal stock-watching screen: all-session live prices ' +
    '(including overnight), filings, news, and social chatter on one page.',
};

export default function HorizonPage() {
  return (
    <div className="-mx-4 -my-8 min-h-screen bg-white text-zinc-900 sm:-mx-6 [color-scheme:light]">
      <TopNav />
      <main>
        <Hero />
        <ValueProps />
      </main>
      <SiteFooter />
    </div>
  );
}

/** Shared horizontal padding + max width for every band on the page. */
const SHELL = 'mx-auto w-full max-w-5xl px-6 sm:px-8';

/** Top navigation: wordmark, log in, and the primary "Start free" action. */
function TopNav() {
  return (
    <header className="sticky top-0 z-20 border-b border-zinc-100 bg-white/85 backdrop-blur">
      <div className={`${SHELL} flex h-16 items-center justify-between`}>
        <Link
          href="/designs/horizon"
          className="flex items-center gap-2 text-base font-semibold tracking-tight text-zinc-900"
        >
          <span aria-hidden className="text-indigo-700">
            ◆
          </span>
          Tickwind
        </Link>
        <nav className="flex items-center gap-2 sm:gap-4">
          <Link
            href="#"
            className="rounded-lg px-3 py-2 text-sm font-medium text-zinc-600 transition-colors hover:text-zinc-900"
          >
            Log in
          </Link>
          <Link
            href="#"
            className="rounded-lg bg-zinc-900 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-zinc-700"
          >
            Start free
          </Link>
        </nav>
      </div>
    </header>
  );
}

/** Hero band: eyebrow, headline, subhead, CTAs, and the live demo card. */
function Hero() {
  return (
    <section className={`${SHELL} pt-16 pb-20 sm:pt-24 sm:pb-28`}>
      <div className="grid items-center gap-12 lg:grid-cols-[1.05fr_1fr] lg:gap-16">
        <div>
          <p className="mb-6 inline-flex items-center gap-2 rounded-full border border-zinc-200 px-3 py-1 text-xs font-medium text-zinc-500">
            <span aria-hidden className="h-1.5 w-1.5 rounded-full bg-indigo-600" />
            tick + wind — price move &amp; direction
          </p>
          <h1 className="text-balance text-5xl font-semibold leading-[1.05] tracking-tight text-zinc-900 sm:text-6xl">
            Read every tick.
            <br />
            See where the
            <br className="hidden sm:block" /> market&rsquo;s{' '}
            <span className="text-indigo-700">blowing.</span>
          </h1>
          <p className="mt-6 max-w-md text-lg leading-relaxed text-zinc-600">
            Tickwind brings a stock&rsquo;s all-session live price, filings,
            news, and social chatter onto one calm screen — pre-market through
            overnight.
          </p>
          <div className="mt-9 flex flex-col gap-3 sm:flex-row sm:items-center">
            <Link
              href="#"
              className="inline-flex items-center justify-center rounded-lg bg-indigo-700 px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-indigo-800"
            >
              Start free
            </Link>
            <Link
              href="#live-example"
              className="inline-flex items-center justify-center gap-1.5 rounded-lg border border-zinc-300 px-6 py-3 text-sm font-semibold text-zinc-800 transition-colors hover:border-zinc-400 hover:bg-zinc-50"
            >
              See a live example
              <ArrowUpRightIcon className="h-4 w-4 text-zinc-400" />
            </Link>
          </div>
          <p className="mt-5 text-sm text-zinc-400">
            No account needed to look around. Personal use, always calm.
          </p>
        </div>

        <div className="lg:pl-2">
          <LiveDemo />
        </div>
      </div>
    </section>
  );
}

/** A single value proposition cell. */
interface ValueProp {
  icon: typeof PulseIcon;
  title: string;
  body: string;
}

const VALUE_PROPS: readonly ValueProp[] = [
  {
    icon: PulseIcon,
    title: 'All-session live price',
    body: 'Pre-market, regular, after-hours, and overnight — one continuous price, clearly labeled by session so you always know which tape you’re reading.',
  },
  {
    icon: FeedIcon,
    title: 'Filings + news in one feed',
    body: 'SEC filings and company headlines land together, newest first. The material disclosure and the story about it, side by side — no tab-hopping.',
  },
  {
    icon: ChatterIcon,
    title: 'Social chatter + saved links',
    body: 'See what people are posting, then clip your own links into the same per-stock feed. Your research and the crowd’s, kept in one place.',
  },
];

/** Three value props on a precise hairline grid. */
function ValueProps() {
  return (
    <section className="border-y border-zinc-100 bg-zinc-50/60">
      <div className={`${SHELL} py-16 sm:py-20`}>
        <h2 className="max-w-2xl text-balance text-3xl font-semibold tracking-tight text-zinc-900 sm:text-4xl">
          One screen for the whole picture.
        </h2>
        <p className="mt-4 max-w-xl text-base text-zinc-600">
          Everything you check on a ticker, gathered and made calm — so you can
          read the move instead of hunting for it.
        </p>
        <div className="mt-12 grid gap-px overflow-hidden rounded-2xl border border-zinc-200 bg-zinc-200 sm:grid-cols-3">
          {VALUE_PROPS.map(prop => (
            <ValuePropCell key={prop.title} prop={prop} />
          ))}
        </div>
      </div>
    </section>
  );
}

/** Renders one value prop as a flat, bordered cell. */
function ValuePropCell({prop}: {prop: ValueProp}) {
  const {icon: Icon, title, body} = prop;
  return (
    <article className="bg-white p-7 sm:p-8">
      <span className="inline-flex h-10 w-10 items-center justify-center rounded-lg border border-zinc-200 text-indigo-700">
        <Icon className="h-5 w-5" />
      </span>
      <h3 className="mt-5 text-lg font-semibold tracking-tight text-zinc-900">
        {title}
      </h3>
      <p className="mt-2 text-sm leading-relaxed text-zinc-600">{body}</p>
    </article>
  );
}

/** Closing footer: wordmark, a soft restating of the pitch, and fine print. */
function SiteFooter() {
  return (
    <footer className={`${SHELL} py-14`}>
      <div className="flex flex-col gap-8 border-t border-zinc-100 pt-10 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <Link
            href="/designs/horizon"
            className="flex items-center gap-2 text-base font-semibold tracking-tight text-zinc-900"
          >
            <span aria-hidden className="text-indigo-700">
              ◆
            </span>
            Tickwind
          </Link>
          <p className="mt-3 max-w-xs text-sm text-zinc-500">
            Read every tick. See where the market&rsquo;s blowing.
          </p>
        </div>
        <div className="flex items-center gap-5 text-sm text-zinc-500">
          <Link href="#" className="transition-colors hover:text-zinc-900">
            Log in
          </Link>
          <Link href="#" className="transition-colors hover:text-zinc-900">
            Start free
          </Link>
          <Link
            href="#live-example"
            className="transition-colors hover:text-zinc-900"
          >
            Live example
          </Link>
        </div>
      </div>
      <p className="mt-8 text-xs text-zinc-400">
        © {new Date().getFullYear()} Tickwind · For personal use · Market data
        from public sources. Not investment advice.
      </p>
    </footer>
  );
}
