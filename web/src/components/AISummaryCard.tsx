'use client';

import {ArrowRight, Sparkles} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/**
 * The AI box on the stock detail page — a FUNNEL to the paid AI Deep Research
 * report (it does NOT call the LLM on render).
 *
 * The per-stock auto-digest was removed (owner, 2026-06): it fired an LLM call on
 * every stock-page open, which is too costly per view. The market-wide AI summary
 * now lives only on the home page (the daily morning briefing). This box instead
 * pitches the on-demand AI Deep Research (valuation / fundamentals / technicals /
 * flows / sentiment + a two-sided bull/bear read, AI-written + source-attributed)
 * and links into it — driving users to the flagship paid feature, no per-view cost.
 */
export function AISummaryCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  return (
    <section
      className={cx(
        'mb-6 rounded-2xl border p-4',
        t.card,
        t.border,
        dark ? 'bg-violet-500/[0.05]' : 'bg-violet-50/50',
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <Sparkles size={15} className={dark ? 'text-violet-300' : 'text-violet-500'} />
          {tr('deep.title')}
        </h2>
        <span
          className={cx(
            'rounded-md px-1.5 py-0.5 text-[10px] font-bold',
            dark ? 'bg-violet-500/15 text-violet-300' : 'bg-violet-50 text-violet-600',
          )}
        >
          {tr('ai.badge')}
        </span>
        <div className="ml-auto">
          <DeepEntry ticker={ticker} dark={dark} tr={tr} />
        </div>
      </div>
      <p className={cx('mt-2 text-[12.5px]', t.sub)}>
        {tr('deep.subtitle').replace('{t}', ticker)}
      </p>
    </section>
  );
}

/**
 * The entry button to the dedicated AI Deep Research report. Subtle, Aurora-styled,
 * bilingual; a locale-aware link to `/stock/{ticker}/research`. The deep report is
 * gated (login + monthly quota) — that UX lives on the target route, so this is
 * just the navigation affordance. Exported + reused on the Research tab too.
 */
export function DeepEntry({
  ticker,
  dark,
  tr,
  className,
}: {
  ticker: string;
  dark: boolean;
  tr: (key: string) => string;
  className?: string;
}) {
  return (
    <Link
      href={`/stock/${encodeURIComponent(ticker)}/research`}
      className={cx(
        'inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-[11px] font-semibold transition',
        dark
          ? 'border-violet-500/30 text-violet-300 hover:border-violet-400/50 hover:bg-violet-500/10'
          : 'border-violet-200 text-violet-600 hover:border-violet-300 hover:bg-violet-50',
        className,
      )}
    >
      {tr('deep.entry')}
      <ArrowRight size={12} />
    </Link>
  );
}
