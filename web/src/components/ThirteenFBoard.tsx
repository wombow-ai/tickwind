'use client';

import {Briefcase} from 'lucide-react';
import Link from 'next/link';
import {useEffect, useState} from 'react';
import {getThirteenF, type FundHoldings, type WhalePosition} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, fmtCompactUSD, tok} from '@/lib/ui';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'error';

/** Quarter-end date "2026-03-31" → "2026 Q1". */
function asOfQuarter(period: string): string {
  const m = /^(\d{4})-(\d{2})/.exec(period);
  if (!m) return period;
  return `${m[1]} Q${Math.ceil(+m[2] / 3)}`;
}

/**
 * The 13F "whale holdings" board: a curated set of famous fund managers'
 * latest quarterly SEC 13F holdings, with quarter-over-quarter changes. 13F is
 * public-domain and filed ~45 days after quarter-end, so every card is labeled
 * as-of its quarter — historical, not live, and not a recommendation.
 */
export function ThirteenFBoard() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [funds, setFunds] = useState<FundHoldings[]>([]);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getThirteenF(c.signal).then(
      r => {
        setFunds(r.funds ?? []);
        setStatus('ready');
      },
      () => setStatus('error'),
    );
    return () => c.abort();
  }, []);

  return (
    <div className="w-full">
      <header className="mb-4">
        <h1 className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}>
          <Briefcase size={20} className={dark ? 'text-violet-300' : 'text-violet-600'} />
          {tr('13f.title')}
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('13f.subtitle')}</p>
      </header>

      <div className={cx('mb-4 rounded-xl border p-3 text-[12px]', t.border, dark ? 'bg-slate-900' : 'bg-slate-50', t.sub)}>
        {tr('13f.disclaimer')}
      </div>

      {status === 'loading' && <FeedSkeleton />}
      {status === 'error' && <ErrorState onRetry={() => location.reload()} />}
      {status === 'ready' && funds.length === 0 && (
        <EmptyState label={tr('13f.empty')} sub={tr('13f.emptySub')} icon={Briefcase} />
      )}
      {status === 'ready' && funds.length > 0 && (
        <div className="tw-fade space-y-4">
          {funds.map(f => (
            <FundCard key={f.slug} f={f} t={t} dark={dark} tr={tr} />
          ))}
        </div>
      )}

      <p className={cx('mt-4 text-center text-[11px]', t.faint)}>{tr('13f.footer')}</p>
    </div>
  );
}

function FundCard({f, t, dark, tr}: {f: FundHoldings; t: Tokens; dark: boolean; tr: (k: string) => string}) {
  return (
    <section className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-3 flex flex-wrap items-baseline gap-x-2 gap-y-1">
        <Link href={`/fund/${encodeURIComponent(f.slug)}`} className={cx('text-[15px] font-bold hover:underline', t.accentText)}>
          {f.manager}
        </Link>
        <Link href={`/fund/${encodeURIComponent(f.slug)}`} className={cx('text-[12.5px] hover:underline', t.sub)}>
          {f.name}
        </Link>
        <span className="ml-auto flex items-center gap-2">
          <span className={cx('rounded-full px-2 py-0.5 text-[10.5px] font-semibold', dark ? 'bg-slate-800 text-slate-300' : 'bg-slate-100 text-slate-600')}>
            {tr('13f.asOf')} {asOfQuarter(f.period)}
          </span>
          <span className={cx('text-[11px]', t.faint)}>· {fmtCompactUSD(f.value)}</span>
        </span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-[12.5px] tabular-nums">
          <thead>
            <tr className={cx('text-left', t.faint)}>
              <th className="py-1.5 pr-2 font-medium">{tr('13f.colStock')}</th>
              <th className="px-2 py-1.5 text-right font-medium">{tr('13f.colValue')}</th>
              <th className="px-2 py-1.5 text-right font-medium">{tr('13f.colPct')}</th>
              <th className="py-1.5 pl-2 text-right font-medium">{tr('13f.colChange')}</th>
            </tr>
          </thead>
          <tbody>
            {f.positions.map((p, i) => (
              <PositionRow key={`${p.ticker || p.issuer}-${i}`} p={p} t={t} dark={dark} tr={tr} />
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function PositionRow({p, t, dark, tr}: {p: WhalePosition; t: Tokens; dark: boolean; tr: (k: string) => string}) {
  const palette: Record<string, string> = {
    new: dark ? 'bg-sky-500/15 text-sky-300' : 'bg-sky-50 text-sky-600',
    add: dark ? 'bg-emerald-500/15 text-emerald-300' : 'bg-emerald-50 text-emerald-600',
    trim: dark ? 'bg-rose-500/15 text-rose-300' : 'bg-rose-50 text-rose-600',
    hold: dark ? 'bg-slate-800 text-slate-400' : 'bg-slate-100 text-slate-500',
  };
  return (
    <tr className={cx('border-t', t.hair)}>
      <td className="py-2 pr-2">
        {p.ticker ? (
          <Link href={`/stock/${encodeURIComponent(p.ticker)}`} className={cx('font-bold', t.accentText)}>
            {p.ticker}
          </Link>
        ) : (
          <span className={cx('font-semibold', t.sub)}>{p.issuer}</span>
        )}
        {p.ticker && <span className={cx('ml-1.5 hidden text-[11px] sm:inline', t.faint)}>{p.issuer}</span>}
      </td>
      <td className={cx('px-2 py-2 text-right font-semibold', t.text)}>{fmtCompactUSD(p.value)}</td>
      <td className={cx('px-2 py-2 text-right', t.sub)}>{p.pct.toFixed(1)}%</td>
      <td className="py-2 pl-2 text-right">
        <span className={cx('rounded px-1.5 py-0.5 text-[10.5px] font-bold', palette[p.change] ?? palette.hold)}>
          {tr(`13f.change.${p.change}`)}
          {(p.change === 'add' || p.change === 'trim') && p.chg_pct !== 0 ? ` ${p.chg_pct > 0 ? '+' : ''}${p.chg_pct.toFixed(0)}%` : ''}
        </span>
      </td>
    </tr>
  );
}
