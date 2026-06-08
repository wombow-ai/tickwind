'use client';

import {
  CandlestickSeries,
  ColorType,
  createChart,
  HistogramSeries,
  LineSeries,
  LineStyle,
  type LogicalRange,
  type Time,
} from 'lightweight-charts';
import {useEffect, useRef, useState} from 'react';
import {aggregate, type Timeframe} from '@/lib/aggregate';
import {getCandles, type Candle} from '@/lib/api';
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
export function KLineChart({ticker}: {ticker: string}) {
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

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    rangeRef.current = null; // new stock → default to the most-recent window
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
    const view = isIntraday
      ? (intraday[res] ?? [])
      : tf === '1Day'
        ? candles
        : aggregate(candles, tf as Timeframe);
    // lightweight-charts wants a unix timestamp for intraday, a date string for daily.
    const toT = (iso: string): Time =>
      isIntraday
        ? (Math.floor(Date.parse(iso) / 1000) as unknown as Time)
        : (iso.slice(0, 10) as unknown as Time);
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

    return () => chart.remove();
  }, [status, candles, dark, showBoll, tf, intraday]);

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
