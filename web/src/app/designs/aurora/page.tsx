import Link from 'next/link';
import {AuroraDemo} from './AuroraDemo';

/** Landing design variant "Aurora": light, airy, premium — soft gradients. */
export default function AuroraLanding() {
  return (
    <main className="relative min-h-screen overflow-hidden bg-white text-slate-900">
      <div aria-hidden className="pointer-events-none absolute inset-0 -z-10">
        <div className="absolute -top-40 left-1/2 h-[40rem] w-[60rem] -translate-x-1/2 rounded-full bg-gradient-to-tr from-sky-200 via-teal-100 to-indigo-200 opacity-60 blur-3xl" />
        <div className="absolute right-[-10rem] top-1/3 h-[30rem] w-[30rem] rounded-full bg-gradient-to-tr from-indigo-200 to-sky-100 opacity-40 blur-3xl" />
      </div>

      <header className="mx-auto flex max-w-6xl items-center justify-between px-6 py-5">
        <Wordmark />
        <nav className="flex items-center gap-3 text-sm">
          <Link href="#" className="font-medium text-slate-600 transition hover:text-slate-900">
            Log in
          </Link>
          <Link
            href="#"
            className="rounded-full bg-slate-900 px-4 py-2 font-medium text-white shadow-sm transition hover:bg-slate-700"
          >
            Start free
          </Link>
        </nav>
      </header>

      <section className="mx-auto max-w-6xl px-6 pb-10 pt-16 text-center">
        <span className="inline-flex items-center gap-2 rounded-full border border-slate-200 bg-white/70 px-3 py-1 text-xs font-medium text-slate-600 backdrop-blur">
          <span className="h-1.5 w-1.5 rounded-full bg-teal-500" />
          Live across every session — even overnight
        </span>
        <h1 className="mx-auto mt-6 max-w-3xl text-balance text-5xl font-bold tracking-tight sm:text-6xl">
          Read every tick.
          <br />
          <span className="bg-gradient-to-r from-sky-600 via-teal-500 to-indigo-600 bg-clip-text text-transparent">
            See where the market&apos;s blowing.
          </span>
        </h1>
        <p className="mx-auto mt-5 max-w-xl text-lg text-slate-600">
          One calm screen for a stock&apos;s all-session price, filings, news, and
          the chatter around it — plus the links you save.
        </p>
        <div className="mt-8 flex items-center justify-center gap-3">
          <Link
            href="#"
            className="rounded-full bg-gradient-to-r from-sky-600 to-indigo-600 px-6 py-3 font-medium text-white shadow-lg shadow-indigo-600/20 transition hover:opacity-90"
          >
            Start free
          </Link>
          <Link
            href="#demo"
            className="rounded-full border border-slate-200 bg-white/70 px-6 py-3 font-medium text-slate-700 backdrop-blur transition hover:bg-white"
          >
            See a live example
          </Link>
        </div>
      </section>

      <section id="demo" className="mx-auto max-w-3xl px-6 py-10">
        <AuroraDemo ticker="AAPL" />
      </section>

      <section className="mx-auto max-w-6xl px-6 py-16">
        <div className="grid gap-6 sm:grid-cols-3">
          <Feature
            title="All-session live price"
            body="Pre-market, regular, after-hours and overnight — the price never goes dark."
          />
          <Feature
            title="Filings + news in one feed"
            body="SEC filings and company news, unified and timestamped."
          />
          <Feature
            title="Chatter + your saved links"
            body="Social sentiment from StockTwits, plus the X / Xiaohongshu / TikTok links you clip."
          />
        </div>
      </section>

      <footer className="mx-auto max-w-6xl px-6 py-10 text-sm text-slate-500">
        <div className="flex items-center justify-between border-t border-slate-200 pt-6">
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
        className="grid h-7 w-7 place-items-center rounded-lg bg-gradient-to-tr from-sky-500 to-indigo-600 text-white"
      >
        ↟
      </span>
      <span className="text-lg font-semibold tracking-tight">Tickwind</span>
    </div>
  );
}

function Feature({title, body}: {title: string; body: string}) {
  return (
    <div className="rounded-2xl border border-slate-200 bg-white/70 p-6 shadow-sm backdrop-blur transition hover:-translate-y-0.5 hover:shadow-md">
      <h3 className="font-semibold text-slate-900">{title}</h3>
      <p className="mt-2 text-sm text-slate-600">{body}</p>
    </div>
  );
}
