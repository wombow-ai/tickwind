'use client';

import {Layers} from 'lucide-react';
import {useEffect, useState} from 'react';
import {getScorecard, type FactorScore, type Scorecard} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'hidden';

/**
 * ScorecardCard shows where a stock ranks — as a PERCENTILE vs Tickwind's tracked universe — on
 * four INDEPENDENT factors (Value / Growth / Quality / Momentum). Every percentile is Go-computed
 * (GET /v1/stocks/{t}/scorecard); it is a DESCRIPTIVE statistic — there is deliberately no blended
 * composite "score" and the bars carry NO good/bad colour, so it never reads as a rating or
 * recommendation (the disclaimer says so). Hides itself when the stock has no factor data (e.g. an
 * ETF) or the ranking universe isn't ready. Client-fetched (per deploy-gotcha #7 — never SSR-fetch
 * the API through the tunnel).
 */
export function ScorecardCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [data, setData] = useState<{scorecard: Scorecard; populationAsOf: string} | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getScorecard(ticker, c.signal).then(r => {
      if (c.signal.aborted) return;
      if (r && r.scorecard) {
        setData(r);
        setStatus('ready');
      } else {
        setStatus('hidden');
      }
    });
    return () => c.abort();
  }, [ticker]);

  if (status === 'hidden') return null;

  const sc = data?.scorecard;
  const factors: {key: string; label: string; score?: FactorScore}[] = sc
    ? [
        {key: 'value', label: tr('scard.value'), score: sc.value},
        {key: 'growth', label: tr('scard.growth'), score: sc.growth},
        {key: 'quality', label: tr('scard.quality'), score: sc.quality},
        {key: 'momentum', label: tr('scard.momentum'), score: sc.momentum},
      ].filter(f => f.score) // omit factors with no available sub-metric (insufficient-not-wrong)
    : [];

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-1 flex flex-wrap items-center gap-2">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <Layers size={15} className={dark ? 'text-indigo-300' : 'text-indigo-500'} />
          {tr('scard.title')}
        </h2>
        {sc && (
          <span className={cx('text-[10.5px]', t.faint)}>
            {tr('scard.pop').replace('{n}', String(sc.population))}
          </span>
        )}
      </div>
      <p className={cx('mb-3 text-[11.5px]', t.sub)}>{tr('scard.sub')}</p>

      {status === 'loading' || !sc ? (
        <div className={cx('h-32 rounded-xl', t.skel)} />
      ) : (
        <div className="space-y-2.5">
          {factors.map(f => (
            <FactorBar key={f.key} label={f.label} score={f.score!} dark={dark} t={t} tr={tr} />
          ))}
        </div>
      )}

      <p className={cx('mt-3 text-[11px] leading-snug', t.faint)}>{tr('scard.disclaimer')}</p>
    </section>
  );
}

function FactorBar({
  label,
  score,
  dark,
  t,
  tr,
}: {
  label: string;
  score: FactorScore;
  dark: boolean;
  t: Tokens;
  tr: (k: string) => string;
}) {
  const pct = Math.max(0, Math.min(100, score.percentile));
  // Neutral fill — NO green/red good-bad cue (that would read as a rating). The bar length IS the
  // percentile; the user reads the magnitude, not a verdict.
  const fill = dark ? 'bg-indigo-400/70' : 'bg-indigo-500/70';
  const track = dark ? 'bg-slate-800' : 'bg-slate-200';
  return (
    <div title={tr('scard.inputs').replace('{n}', String(score.inputs))}>
      <div className="mb-0.5 flex items-center justify-between text-[12px]">
        <span className={cx('font-medium', t.sub)}>{label}</span>
        <span className={cx('font-semibold tabular-nums', t.text)}>{Math.round(pct)}</span>
      </div>
      <div className={cx('h-2 w-full overflow-hidden rounded-full', track)}>
        <div className={cx('h-full rounded-full', fill)} style={{width: `${pct}%`}} />
      </div>
    </div>
  );
}
