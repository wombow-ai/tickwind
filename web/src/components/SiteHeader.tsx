import Link from 'next/link';
import {APP_NAME, APP_TAGLINE} from '@/lib/config';

/** Top navigation bar shown on every page. */
export function SiteHeader() {
  return (
    <header className="sticky top-0 z-10 border-b border-white/10 bg-zinc-950/70 backdrop-blur">
      <div className="mx-auto flex max-w-5xl items-center gap-3 px-4 py-3 sm:px-6">
        <Link href="/" className="group flex items-center gap-2.5">
          <span className="grid h-8 w-8 place-items-center rounded-lg bg-gradient-to-br from-sky-400 to-indigo-500 text-sm font-black text-zinc-950 shadow-sm">
            T
          </span>
          <span className="flex flex-col leading-tight">
            <span className="text-sm font-semibold text-zinc-100 group-hover:text-white">
              {APP_NAME}
            </span>
            <span className="hidden text-xs text-zinc-500 sm:block">
              {APP_TAGLINE}
            </span>
          </span>
        </Link>
      </div>
    </header>
  );
}
