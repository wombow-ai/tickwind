'use client';

import {Activity, Gauge, SlidersHorizontal} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {
  getMyPrefs,
  getStockIndicators,
  indicatorSlug,
  putMyPrefs,
  type StockIndicator,
  type StockIndicatorsResponse,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, fmtCompactUSD, fmtPrice, tok} from '@/lib/ui';
import {IndicatorPicker} from './IndicatorPicker';
import {
  clearSelection,
  idsFromPrefs,
  loadSelection,
  prefsFromIds,
  resolveSelection,
  saveSelection,
} from '@/lib/indicatorSelection';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'hidden';

/** Display order of the domain groups (technical → fundamental → sentiment). */
const DOMAIN_ORDER: Record<string, number> = {technical: 0, fundamental: 1, sentiment: 2};

/**
 * Formats an indicator's headline value by its unit: `%` appends a percent sign,
 * `x` appends an "x" multiple suffix (e.g. `1.8x`), `price` uses the USD price
 * formatter, `usd` renders a large dollar amount compact (`$4.5T` — FCF / enterprise
 * value), and `ratio`/empty render a plain trimmed number (small counts/per-share).
 * Values are shown exactly as computed — never invented or rounded into a different
 * figure.
 */
function fmtValue(value: number, unit?: string): string {
  switch (unit) {
    case '%':
      return `${trim(value)}%`;
    case 'x':
      return `${trim(value)}x`;
    case 'price':
      return fmtPrice('$', value);
    case 'usd':
      return fmtCompactUSD(value);
    default:
      return trim(value);
  }
}

/** Trims a number to at most two decimals without trailing zeros. */
function trim(value: number): string {
  const abs = Math.abs(value);
  const decimals = abs >= 100 ? 1 : 2;
  return Number(value.toFixed(decimals)).toLocaleString('en-US', {
    maximumFractionDigits: decimals,
  });
}

/**
 * Per-stock indicators panel: groups the `ok` indicators by domain (Technical /
 * Fundamental / Sentiment), shows each value with its unit and a short
 * interpretation hint, and surfaces `insufficient` ones as "—". `unsupported`
 * indicators are hidden. A small market-context strip (VIX + Fear & Greed) leads
 * when available. The whole panel renders nothing when the endpoint 404s or
 * returns no displayable (ok / insufficient) indicators — it complements, and
 * never replaces, the headline FundamentalsCard.
 */
export function IndicatorsPanel({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {user, getToken} = useAuth();
  const signedIn = !!user;
  const [data, setData] = useState<StockIndicatorsResponse | null>(null);
  const [status, setStatus] = useState<Status>('loading');
  // Selected ids in display order. A pure VIEW filter + ordering over the
  // already-computed payload (default = the payload's P0 set, derived so it
  // equals today's panel and grows with the catalog). Global preference: when
  // signed-in the SERVER prefs win over anonymous localStorage; otherwise
  // localStorage; else the P0 default.
  const [selected, setSelected] = useState<string[]>([]);
  const [resolved, setResolved] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    setResolved(false);
    getStockIndicators(ticker, c.signal).then(
      async r => {
        // 404 (null) or nothing displayable → hide the whole panel.
        const displayable =
          r?.indicators.some(i => i.status === 'ok' || i.status === 'insufficient') ?? false;
        if (!r || !displayable) {
          setStatus('hidden');
          return;
        }
        // Resolve the selection against THIS payload. Signed-in: server prefs
        // win; fall back to anon localStorage when the server has none, and
        // migrate that local selection up once. Anonymous: localStorage only.
        // Defensive: any prefs-call failure degrades silently to localStorage.
        let saved = loadSelection();
        if (signedIn) {
          try {
            const token = await getToken();
            const serverIds = idsFromPrefs(await getMyPrefs(token, c.signal));
            if (serverIds) {
              saved = serverIds; // server wins
            } else if (saved) {
              // One-time migrate-up: the server has no selection yet but the
              // user has a local one — push it so it follows them across devices.
              void putMyPrefs(token, prefsFromIds(saved)).catch(() => {});
            }
          } catch {
            // Network / auth hiccup → keep the localStorage fallback.
          }
        }
        if (c.signal.aborted) return;
        setSelected(resolveSelection(r.indicators, saved));
        setResolved(true);
        setData(r);
        setStatus('ready');
      },
      () => setStatus('hidden'),
    );
    return () => c.abort();
  }, [ticker, signedIn, getToken]);

  // Persist the selection, debounced ~500ms. Anonymous localStorage is always
  // written (so a signed-out session keeps it); when signed-in, also push to
  // the server (which wins on the next load). Skipped until the selection is
  // resolved so the initial resolve never overwrites a saved blob with the
  // default. Server write is best-effort — a failure never breaks the UI.
  const persistTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (!resolved) return;
    if (persistTimer.current) clearTimeout(persistTimer.current);
    persistTimer.current = setTimeout(() => {
      saveSelection(selected);
      if (signedIn) {
        void (async () => {
          try {
            await putMyPrefs(await getToken(), prefsFromIds(selected));
          } catch {
            // Degrade to localStorage-only; the selection still applies locally.
          }
        })();
      }
    }, 500);
    return () => {
      if (persistTimer.current) clearTimeout(persistTimer.current);
    };
  }, [selected, resolved, signedIn, getToken]);

  // "Reset to default" — clear the saved blob and fall back to the payload's
  // P0 default. The persist effect re-saves the default, which is harmless.
  const reset = useCallback(() => {
    clearSelection();
    if (data) setSelected(resolveSelection(data.indicators, null));
  }, [data]);

  // Render only the SELECTED subset, in saved order, grouped by domain in
  // canonical order. `unsupported` stay hidden; a selected id a stock can't
  // compute shows the existing insufficient ("—") row.
  const groups = useMemo(() => {
    if (!data) return [];
    const byId = new Map(data.indicators.map(i => [i.id, i] as const));
    const map = new Map<string, StockIndicator[]>();
    for (const id of selected) {
      const ind = byId.get(id);
      if (!ind || ind.status === 'unsupported') continue;
      const list = map.get(ind.domain) ?? [];
      list.push(ind); // preserve the user's chosen order within each domain
      map.set(ind.domain, list);
    }
    return [...map.entries()].sort(
      (a, b) => (DOMAIN_ORDER[a[0]] ?? 9) - (DOMAIN_ORDER[b[0]] ?? 9),
    );
  }, [data, selected]);

  if (status === 'hidden') return null;

  if (status === 'loading' || !data) {
    return (
      <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        <div className={cx('mb-3 h-4 w-24 rounded', t.skel)} />
        <div className="space-y-2">
          {Array.from({length: 4}).map((_, i) => (
            <div key={i} className={cx('h-9 rounded-lg', t.skel)} />
          ))}
        </div>
      </section>
    );
  }

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
          <Activity size={15} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          {tr('ind2.title')}
        </h2>
        {data.as_of && (
          <span className={cx('text-[10.5px]', t.faint)}>
            {tr('ind2.asOf').replace('{d}', data.as_of)}
          </span>
        )}
        <button
          onClick={() => setPickerOpen(true)}
          aria-expanded={pickerOpen}
          aria-haspopup="dialog"
          className={cx(
            'ml-auto inline-flex items-center gap-1 rounded-lg px-2 py-1 text-[10.5px] font-medium',
            t.surf2,
            t.sub,
            'hover:opacity-80',
          )}
        >
          <SlidersHorizontal size={12} />
          {tr('ind2.picker.customize')}
        </button>
        <span className={cx('text-[10.5px]', t.faint)}>
          <Link href="/indicators" className="hover:underline">
            {tr('ind2.learnMore')}
          </Link>
        </span>
      </div>

      {data.market_context && (
        <MarketContextStrip ctx={data.market_context} dark={dark} t={t} tr={tr} />
      )}

      <div className="space-y-4">
        {groups.map(([domain, items]) => (
          <DomainGroup key={domain} domain={domain} items={items} dark={dark} t={t} tr={tr} />
        ))}
      </div>

      {pickerOpen && (
        <IndicatorPicker
          indicators={data.indicators}
          selected={selected}
          onChange={setSelected}
          onReset={reset}
          onClose={() => setPickerOpen(false)}
        />
      )}
    </section>
  );
}

/** The VIX + Fear & Greed market-backdrop strip (each chip hidden when absent). */
function MarketContextStrip({
  ctx,
  dark,
  t,
  tr,
}: {
  ctx: NonNullable<StockIndicatorsResponse['market_context']>;
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
}) {
  return (
    <div className="mb-3 flex flex-wrap gap-2">
      {ctx.vix != null && (
        <span
          className={cx(
            'inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-[12px]',
            t.surf2,
          )}
        >
          <Activity size={13} className={dark ? 'text-amber-300' : 'text-amber-500'} />
          <span className={t.faint}>{tr('ind2.vix')}</span>
          <span className={cx('font-bold tabular-nums', t.text)}>{trim(ctx.vix)}</span>
        </span>
      )}
      {ctx.fear_greed && (
        <span
          className={cx(
            'inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-[12px]',
            t.surf2,
          )}
        >
          <Gauge size={13} className={dark ? 'text-teal-300' : 'text-teal-600'} />
          <span className={t.faint}>{tr('ind2.fearGreed')}</span>
          <span className={cx('font-bold tabular-nums', t.text)}>{ctx.fear_greed.score}</span>
          {ctx.fear_greed.label && (
            <span className={cx('font-medium', t.sub)}>{ctx.fear_greed.label}</span>
          )}
        </span>
      )}
    </div>
  );
}

/** One domain section (header + its indicator rows). */
function DomainGroup({
  domain,
  items,
  dark,
  t,
  tr,
}: {
  domain: string;
  items: StockIndicator[];
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
}) {
  const domainName = items[0]?.domain_name ?? domain;
  const heading = tr(`ind2.domain.${domain}`);
  return (
    <div>
      <h3 className={cx('mb-2 text-[11px] font-semibold uppercase tracking-wide', t.faint)}>
        {heading === `ind2.domain.${domain}` ? domainName : heading}
      </h3>
      <div className={cx('divide-y rounded-xl border', t.hair, t.border)}>
        {items.map(ind => (
          <IndicatorRow key={ind.id} ind={ind} dark={dark} t={t} />
        ))}
      </div>
    </div>
  );
}

/** One indicator row: name (deep-linked) + value/extra + interpretation hint. */
function IndicatorRow({ind, dark, t}: {ind: StockIndicator; dark: boolean; t: Tokens}) {
  const tr = useT();
  const {lang} = useLang();
  // English-default name; the Chinese name leads only in the Chinese UI.
  const name = lang === 'zh' && ind.name_zh ? ind.name_zh : ind.name_en;
  const ok = ind.status === 'ok';

  return (
    <div className="flex items-start justify-between gap-3 px-3 py-2.5">
      <div className="min-w-0 flex-1">
        <Link
          href={`/indicators/${indicatorSlug(ind.id)}`}
          className={cx('text-[13px] font-semibold hover:underline', t.text)}
        >
          {name}
          {ind.abbr && <span className={cx('ml-1.5 text-[11px] font-medium', t.sub)}>{ind.abbr}</span>}
        </Link>
        {ok && ind.interpretation && (
          <p className={cx('mt-0.5 truncate text-[11.5px]', t.faint)}>{ind.interpretation}</p>
        )}
      </div>
      <div className="shrink-0 text-right">
        {ok && ind.value != null ? (
          <>
            <div className={cx('text-[14px] font-bold tabular-nums', t.text)}>
              {fmtValue(ind.value, ind.unit)}
            </div>
            {ind.extra && Object.keys(ind.extra).length > 0 && (
              <div className={cx('mt-0.5 flex flex-wrap justify-end gap-x-2 text-[10.5px] tabular-nums', t.faint)}>
                {Object.entries(ind.extra).map(([key, v]) => (
                  <span key={key}>
                    {key} {fmtValue(v, ind.unit)}
                  </span>
                ))}
              </div>
            )}
          </>
        ) : (
          <div className={cx('text-[14px] font-bold tabular-nums', dark ? 'text-slate-600' : 'text-slate-300')}>
            {tr('ind2.empty')}
          </div>
        )}
      </div>
    </div>
  );
}
