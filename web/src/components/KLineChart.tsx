'use client';

import {
  CandlestickSeries,
  ColorType,
  createChart,
  HistogramSeries,
  type ISeriesApi,
  LineSeries,
  LineStyle,
  type LogicalRange,
  type Time,
} from 'lightweight-charts';
import {useEffect, useRef, useState} from 'react';
import {aggregate, type Timeframe} from '@/lib/aggregate';
import {getCandles, type Candle, type Quote} from '@/lib/api';
import {bollinger, macd, rsi, sma, type Series} from '@/lib/indicators';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'empty' | 'error';

// The "N日线" moving averages + their overlay colours.
const MAS = [
  {period: 5, color: '#f59e0b'},
  {period: 10, color: '#3b82f6'},
  {period: 20, color: '#a855f7'},
  {period: 60, color: '#14b8a6'},
];

// Bollinger Bands (20, 2σ): a toggleable envelope on the price pane. The middle
// band is SMA20 (already drawn as MA20), so only the upper/lower bands are shown.
const BOLL_COLOR = '#6366f1';

// Chart timeframes. 1D/5D = intraday (fetched per resolution); 1Day = raw daily
// candles; W/M/Q/Y aggregate the daily series client-side (no refetch).
type TF = '1D' | '5D' | '1Day' | Timeframe;
const TFS: {id: TF; key: string}[] = [
  {id: '1D', key: 'kline.tf.i'},
  {id: '5D', key: 'kline.tf.5d'},
  {id: '1Day', key: 'kline.tf.d'},
  {id: 'W', key: 'kline.tf.w'},
  {id: 'M', key: 'kline.tf.m'},
  {id: 'Q', key: 'kline.tf.q'},
  {id: 'Y', key: 'kline.tf.y'},
];

/** Intraday Alpaca resolution for a timeframe ('' = daily/aggregated, no fetch). */
function intradayRes(tf: TF): string {
  return tf === '1D' ? '5Min' : tf === '5D' ? '15Min' : '';
}

// How a live quote folds into the displayed series, per timeframe family:
// 'intraday' extends/appends minute buckets (any session — 1D/5D show extended
// hours); 'daily' extends today's candle or appends it (regular session only,
// so pre/post prints never distort the daily close, matching Google/Futu);
// 'bucket' only extends the last W/M/Q/Y aggregate, never appends.
type StitchMode = 'intraday' | 'daily' | 'bucket';

/**
 * Folds the card's live quote into the tail of the candle series, so the chart
 * tip always agrees with the (real-time) price card above it. Returns the
 * amended-or-appended tail candle, or null when the quote is stale, for another
 * symbol's session type, or older than the data already charted.
 */
function stitchTail(
  view: readonly Candle[],
  quote: Quote,
  mode: StitchMode,
  resSec: number,
): Candle | null {
  if (view.length === 0 || !(quote.price > 0)) return null;
  const last = view[view.length - 1];
  if (mode === 'intraday') {
    const qms = Date.parse(quote.at);
    const lastms = Date.parse(last.time);
    if (!Number.isFinite(qms) || !Number.isFinite(lastms)) return null;
    const bucket = Math.floor(qms / 1000 / resSec);
    const lastBucket = Math.floor(lastms / 1000 / resSec);
    if (bucket < lastBucket) return null; // already have a newer bar
    if (bucket === lastBucket) {
      return {
        ...last,
        close: quote.price,
        high: Math.max(last.high, quote.price),
        low: Math.min(last.low, quote.price),
      };
    }
    return {
      time: new Date(bucket * resSec * 1000).toISOString(),
      open: quote.price,
      high: quote.price,
      low: quote.price,
      close: quote.price,
      volume: 0,
    };
  }
  // Daily-derived views: only a live regular-session price belongs in the
  // daily candle. (During regular hours the UTC date == the ET trading date,
  // so the date slice below is safe.)
  if (quote.session !== 'regular') return null;
  const qd = quote.at.slice(0, 10);
  const ld = last.time.slice(0, 10);
  if (qd < ld) return null;
  if (qd === ld || mode === 'bucket') {
    return {
      ...last,
      close: quote.price,
      high: Math.max(last.high, quote.price),
      low: Math.min(last.low, quote.price),
    };
  }
  // First print of a new trading day with no daily bar yet: synthesize one.
  return {time: qd, open: quote.price, high: quote.price, low: quote.price, close: quote.price, volume: 0};
}

/** Replaces the tail candle in place (same time) or appends a new one. */
function applyTail(view: Candle[], tail: Candle): void {
  if (view.length > 0 && view[view.length - 1].time === tail.time) {
    view[view.length - 1] = tail;
  } else {
    view.push(tail);
  }
}

/** {time,value}[] from an indicator Series, dropping null warmup points. */
function lineData(
  times: string[],
  series: Series,
  toT: (iso: string) => Time,
): {time: Time; value: number}[] {
  const out: {time: Time; value: number}[] = [];
  for (let i = 0; i < series.length; i++) {
    const v = series[i];
    if (v !== null) out.push({time: toT(times[i]), value: v});
  }
  return out;
}

/**
 * The K-line (candlestick) chart for the stock detail page: daily candles with
 * MA5/10/20/60 overlays, plus Volume, MACD(12,26,9) and RSI(14) panes. Indicators
 * are computed client-side from the full /candles history (lib/indicators).
 * lightweight-charts is canvas + imperative, so this is a client component that
 * builds the chart in an effect and tears it down on cleanup.
 */
export function KLineChart({ticker, quote}: {ticker: string; quote?: Quote}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [status, setStatus] = useState<Status>('loading');
  const [candles, setCandles] = useState<Candle[]>([]);
  const [showBoll, setShowBoll] = useState(false);
  const [tf, setTf] = useState<TF>('1Day');
  const [intraday, setIntraday] = useState<Record<string, Candle[]>>({});
  const intradayReq = useRef<Set<string>>(new Set());
  const containerRef = useRef<HTMLDivElement>(null);
  // Remembers the user's pan/zoom so a rebuild (dark or Bollinger toggle) doesn't
  // snap the view back to the default window; reset on ticker change.
  const rangeRef = useRef<LogicalRange | null>(null);
  // Live-stitch plumbing: the mounted candle series + the working copy of its
  // data, so a quote tick can extend the tail via series.update() without the
  // (expensive) full chart rebuild. lastQuoteRef lets a rebuild (tf/dark/BOLL
  // toggle) re-apply the most recent quote immediately instead of waiting for
  // the next tick.
  const seriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null);
  const stitchRef = useRef<{
    view: Candle[];
    mode: StitchMode;
    resSec: number;
    toT: (iso: string) => Time;
  } | null>(null);
  const lastQuoteRef = useRef<Quote | null>(null);

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    rangeRef.current = null; // new stock → default to the most-recent window
    lastQuoteRef.current = null; // never stitch another symbol's quote
    setIntraday({});
    intradayReq.current = new Set();
    getCandles(ticker, c.signal).then(
      r => {
        const cs = r.candles ?? [];
        setCandles(cs);
        setStatus(cs.length === 0 ? 'empty' : 'ready');
      },
      () => setStatus('error'),
    );
    return () => c.abort();
  }, [ticker]);

  // Switching timeframe re-buckets the series; a saved daily pan/zoom is
  // meaningless on weekly/monthly, so reset to the default window.
  useEffect(() => {
    rangeRef.current = null;
  }, [tf]);

  // Intraday timeframes (1D/5D) fetch their own bars per resolution, cached for
  // the session (refetched per ticker). Daily/aggregated views need no fetch.
  useEffect(() => {
    const res = intradayRes(tf);
    if (!res) return;
    const k = ticker + '|' + res;
    if (intradayReq.current.has(k)) return;
    intradayReq.current.add(k);
    const c = new AbortController();
    getCandles(ticker, c.signal, res).then(
      r => setIntraday(prev => ({...prev, [res]: r.candles ?? []})),
      () => intradayReq.current.delete(k), // allow retry on failure
    );
    return () => c.abort();
  }, [tf, ticker]);

  useEffect(() => {
    if (status !== 'ready' || !containerRef.current || candles.length === 0) return;

    const isIntraday = tf === '1D' || tf === '5D';
    const res = intradayRes(tf);
    const base = isIntraday
      ? (intraday[res] ?? [])
      : tf === '1Day'
        ? candles
        : aggregate(candles, tf as Timeframe);
    // lightweight-charts wants a unix timestamp for intraday, a date string for daily.
    const toT = (iso: string): Time =>
      isIntraday
        ? (Math.floor(Date.parse(iso) / 1000) as unknown as Time)
        : (iso.slice(0, 10) as unknown as Time);
    // Fold the latest live quote into the tail so the chart opens already
    // agreeing with the price card (ticks after this go through series.update).
    const mode: StitchMode = isIntraday ? 'intraday' : tf === '1Day' ? 'daily' : 'bucket';
    const resSec = tf === '1D' ? 300 : tf === '5D' ? 900 : 86400;
    const view = base.slice();
    const q0 = lastQuoteRef.current;
    if (q0 && q0.ticker.toUpperCase() === ticker.toUpperCase()) {
      const tail = stitchTail(view, q0, mode, resSec);
      if (tail) applyTail(view, tail);
    }
    const closes = view.map(c => c.close);
    const times = view.map(c => c.time);
    const grid = dark ? '#1e293b' : '#eef2f7';
    const axis = dark ? '#94a3b8' : '#64748b';
    const up = '#16a34a';
    const down = '#dc2626';

    const chart = createChart(containerRef.current, {
      autoSize: true,
      layout: {
        background: {type: ColorType.Solid, color: 'transparent'},
        textColor: axis,
        attributionLogo: true, // satisfies the lightweight-charts (Apache-2.0) attribution
        fontSize: 11,
      },
      grid: {vertLines: {color: grid}, horzLines: {color: grid}},
      rightPriceScale: {borderColor: grid},
      timeScale: {borderColor: grid, rightOffset: 4, timeVisible: isIntraday, secondsVisible: false},
    });

    // Candles (pane 0).
    const candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: up,
      downColor: down,
      borderUpColor: up,
      borderDownColor: down,
      wickUpColor: up,
      wickDownColor: down,
    });
    candleSeries.setData(
      view.map(c => ({
        time: toT(c.time),
        open: c.open,
        high: c.high,
        low: c.low,
        close: c.close,
      })),
    );
    seriesRef.current = candleSeries;
    stitchRef.current = {view, mode, resSec, toT};

    // MA overlays (pane 0).
    for (const ma of MAS) {
      const s = chart.addSeries(LineSeries, {
        color: ma.color,
        lineWidth: 1,
        priceLineVisible: false,
        lastValueVisible: false,
        crosshairMarkerVisible: false,
      });
      s.setData(lineData(times, sma(closes, ma.period), toT));
    }

    // Bollinger Bands (20, 2σ) on pane 0 — upper + lower envelope, dashed.
    // Toggled off by default to keep the price pane uncluttered.
    if (showBoll) {
      const bb = bollinger(closes, 20, 2);
      for (const band of [bb.upper, bb.lower]) {
        const s = chart.addSeries(LineSeries, {
          color: BOLL_COLOR,
          lineWidth: 1,
          lineStyle: LineStyle.Dashed,
          priceLineVisible: false,
          lastValueVisible: false,
          crosshairMarkerVisible: false,
        });
        s.setData(lineData(times, band, toT));
      }
    }

    // Volume (pane 1).
    const vol = chart.addSeries(
      HistogramSeries,
      {priceFormat: {type: 'volume'}, priceLineVisible: false, lastValueVisible: false},
      1,
    );
    vol.setData(
      view.map(c => ({
        time: toT(c.time),
        value: c.volume,
        color: c.close >= c.open ? `${up}55` : `${down}55`,
      })),
    );

    // MACD (pane 2): histogram + line + signal.
    const m = macd(closes);
    const hist = chart.addSeries(
      HistogramSeries,
      {priceLineVisible: false, lastValueVisible: false},
      2,
    );
    hist.setData(
      times
        .map((tm, i) => ({i, v: m.histogram[i]}))
        .filter(x => x.v !== null)
        .map(x => ({
          time: toT(times[x.i]),
          value: x.v as number,
          color: (x.v as number) >= 0 ? `${up}99` : `${down}99`,
        })),
    );
    const macdLine = chart.addSeries(
      LineSeries,
      {color: '#3b82f6', lineWidth: 1, priceLineVisible: false, lastValueVisible: false},
      2,
    );
    macdLine.setData(lineData(times, m.macd, toT));
    const sigLine = chart.addSeries(
      LineSeries,
      {color: '#f59e0b', lineWidth: 1, priceLineVisible: false, lastValueVisible: false},
      2,
    );
    sigLine.setData(lineData(times, m.signal, toT));

    // RSI (pane 3).
    const rsiLine = chart.addSeries(
      LineSeries,
      {color: '#a855f7', lineWidth: 1, priceLineVisible: false, lastValueVisible: false},
      3,
    );
    rsiLine.setData(lineData(times, rsi(closes), toT));

    // Pane heights: price dominant, indicator panes compact.
    const panes = chart.panes();
    panes[0]?.setHeight(280);
    panes[1]?.setHeight(70);
    panes[2]?.setHeight(90);
    panes[3]?.setHeight(80);

    // Restore the user's prior view across rebuilds (dark/Bollinger toggle);
    // otherwise default to the most recent ~130 sessions (full ~3y is loaded, so
    // panning/scrolling left reveals it with no round-trip).
    const n = view.length;
    const ts = chart.timeScale();
    const saved = rangeRef.current;
    if (isIntraday) {
      ts.fitContent(); // show the whole 1D/5D session(s)
    } else if (saved) {
      ts.setVisibleLogicalRange(saved);
    } else if (n > 130) {
      ts.setVisibleLogicalRange({from: n - 130, to: n - 1});
    } else {
      ts.fitContent();
    }
    // Capture subsequent pans/zooms so the next rebuild restores them.
    ts.subscribeVisibleLogicalRangeChange(r => {
      if (r) rangeRef.current = r;
    });

    return () => {
      seriesRef.current = null; // a tick must never update a removed chart
      stitchRef.current = null;
      chart.remove();
    };
  }, [status, candles, dark, showBoll, tf, intraday, ticker]);

  // Live tick: extend the charted tail with each quote the price card shows.
  // series.update() replaces the tail bar (same time) or appends (newer time),
  // so the chart stays in lock-step with the card without a rebuild. Indicator
  // panes (MA/VOL/MACD/RSI) refresh on the next rebuild — their period values
  // barely move within a bar, and a per-tick recompute isn't worth the jank.
  useEffect(() => {
    if (!quote || quote.ticker.toUpperCase() !== ticker.toUpperCase()) return;
    lastQuoteRef.current = quote;
    const ctx = stitchRef.current;
    const s = seriesRef.current;
    if (!ctx || !s) return;
    const tail = stitchTail(ctx.view, quote, ctx.mode, ctx.resSec);
    if (!tail) return;
    applyTail(ctx.view, tail);
    s.update({
      time: ctx.toT(tail.time),
      open: tail.open,
      high: tail.high,
      low: tail.low,
      close: tail.close,
    });
  }, [quote, ticker]);

  return (
    <section className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className={cx('text-[14px] font-bold', t.text)}>{tr('kline.title')}</h2>
          <div className={cx('inline-flex items-center rounded-lg border p-0.5 text-[11px] font-semibold', t.border)}>
            {TFS.map(f => (
              <button
                key={f.id}
                onClick={() => setTf(f.id)}
                aria-pressed={tf === f.id}
                className={cx(
                  'rounded-md px-2 py-0.5 transition',
                  tf === f.id
                    ? dark
                      ? 'bg-slate-700 text-white'
                      : 'bg-white text-slate-900 shadow-sm'
                    : t.sub,
                )}
              >
                {tr(f.key)}
              </button>
            ))}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[10.5px] font-semibold">
          {MAS.map(ma => (
            <span key={ma.period} className="inline-flex items-center gap-1" style={{color: ma.color}}>
              <span className="inline-block h-0.5 w-3 rounded" style={{background: ma.color}} />
              MA{ma.period}
            </span>
          ))}
          <span className={t.faint}>· MACD · RSI · VOL</span>
          <button
            onClick={() => setShowBoll(v => !v)}
            aria-pressed={showBoll}
            className={cx(
              'rounded-full border px-2 py-0.5 text-[10.5px] font-semibold transition',
              showBoll ? '' : cx(t.faint, t.border),
            )}
            style={showBoll ? {color: BOLL_COLOR, borderColor: BOLL_COLOR} : undefined}
          >
            BOLL
          </button>
        </div>
      </div>

      {status === 'loading' && (
        <div className={cx('h-[520px] w-full animate-pulse rounded-xl', dark ? 'bg-slate-800/50' : 'bg-slate-100')} />
      )}
      {status === 'empty' && (
        <div className={cx('flex h-[200px] items-center justify-center text-[13px]', t.faint)}>
          {tr('kline.empty')}
        </div>
      )}
      {status === 'error' && (
        <div className={cx('flex h-[200px] items-center justify-center text-[13px]', t.faint)}>
          {tr('states.errorTitle')}
        </div>
      )}
      <div ref={containerRef} className={cx('h-[520px] w-full', status === 'ready' ? 'block' : 'hidden')} />

      <p className={cx('mt-2 text-[10.5px]', t.faint)}>{tr('kline.footer')}</p>
    </section>
  );
}
