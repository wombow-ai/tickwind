import Link from 'next/link';
import {BreezeDemo} from './BreezeDemo';

/** Landing design variant "Breeze": friendly, warm, approachable consumer-fintech. */
export default function BreezeLanding() {
  return (
    <main className="min-h-screen bg-gradient-to-b from-emerald-50 via-white to-white text-zinc-900">
      <header className="mx-auto flex max-w-6xl items-center justify-between px-6 py-5">
        <Wordmark />
        <nav className="flex items-center gap-2 text-sm">
          <Link
            href="#"
            className="rounded-full px-4 py-2 font-semibold text-zinc-600 transition hover:text-zinc-900"
          >
            Log in
          </Link>
          <Link
            href="#"
            className="rounded-full bg-emerald-500 px-5 py-2.5 font-bold text-white shadow-lg shadow-emerald-500/30 transition hover:bg-emerald-600"
          >
            Start free
          </Link>
        </nav>
      </header>

      <section className="mx-auto max-w-6xl px-6 pb-8 pt-14 text-center">
        <span className="inline-flex items-center gap-2 rounded-full bg-emerald-100 px-4 py-1.5 text-sm font-bold text-emerald-700">
          🌬️ Your stocks, all in one breezy place
        </span>
        <h1 className="mx-auto mt-6 max-w-3xl text-balance text-5xl font-extrabold tracking-tight sm:text-6xl">
          Read every tick. See where the{' '}
          <span className="text-emerald-500">market&apos;s blowing.</span>
        </h1>
        <p className="mx-auto mt-5 max-w-xl text-lg text-zinc-600">
          Price, news, filings and the chatter for any stock — on one friendly
          screen. Save the links you love, track what you watch.
        </p>
        <div className="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row">
          <Link
            href="#"
            className="w-full rounded-full bg-emerald-500 px-7 py-3.5 text-center font-bold text-white shadow-xl shadow-emerald-500/30 transition hover:bg-emerald-600 sm:w-auto"
          >
            Start free — it&apos;s free
          </Link>
          <Link
            href="#demo"
            className="w-full rounded-full border-2 border-emerald-200 bg-white px-7 py-3.5 text-center font-bold text-emerald-700 transition hover:border-emerald-300 sm:w-auto"
          >
            See a live example
          </Link>
        </div>
      </section>

      <section id="demo" className="mx-auto max-w-md px-6 py-8">
        <BreezeDemo ticker="AAPL" />
      </section>

      <section className="mx-auto max-w-6xl px-6 py-14">
        <div className="grid gap-5 sm:grid-cols-3">
          <Feature
            emoji="⚡"
            title="Always-on prices"
            body="Pre-market, regular, after-hours, even overnight. Never goes dark."
          />
          <Feature
            emoji="📰"
            title="News + filings"
            body="Everything a company says — and what's said about it — in one feed."
          />
          <Feature
            emoji="📎"
            title="Chatter + clips"
            body="Social buzz, plus the X / Xiaohongshu / TikTok links you save."
          />
        </div>
      </section>

      <footer className="mx-auto max-w-6xl px-6 py-10 text-sm text-zinc-500">
        <div className="flex items-center justify-between border-t border-emerald-100 pt-6">
          <Wordmark />
          <p>© 2026 Tickwind · Not investment advice.</p>
        </div>
      </footer>
    </main>
  );
}

function Wordmark() {
  return (
    <div className="flex items-center gap-2">
      <span
        aria-hidden
        className="grid h-8 w-8 place-items-center rounded-xl bg-emerald-500 text-lg text-white"
      >
        ↟
      </span>
      <span className="text-lg font-extrabold tracking-tight">Tickwind</span>
    </div>
  );
}

function Feature({
  emoji,
  title,
  body,
}: {
  emoji: string;
  title: string;
  body: string;
}) {
  return (
    <div className="rounded-3xl bg-white p-6 shadow-lg shadow-emerald-600/5 ring-1 ring-emerald-50 transition hover:-translate-y-1 hover:shadow-xl">
      <div className="text-3xl">{emoji}</div>
      <h3 className="mt-3 text-lg font-bold text-zinc-900">{title}</h3>
      <p className="mt-1.5 text-sm text-zinc-600">{body}</p>
    </div>
  );
}
