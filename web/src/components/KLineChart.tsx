'use client';

import {
  CandlestickSeries,
  ColorType,
  createChart,
  HistogramSeries,
  LineSeries,
  type Time,
} from 'lightweight-charts';
import {useEffect, useRef, useState} from 'react';
import {getCandles, type Candle} from '@/lib/api';
import {macd, rsi, sma, type Series} from '@/lib/indicators';
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

/** Daily bar date → lightweight-charts business-day time. */
function toTime(iso: string): Time {
  return iso.slice(0, 10) as unknown as Time;
}

/** {time,value}[] from an indicator Series, dropping null warmup points. */
function lineData(times: string[], series: Series): {time: Time; value: number}[] {
  const out: {time: Time; value: number}[] = [];
  for (let i = 0; i < series.length; i++) {
    const v = series[i];
    if (v !== null) out.push({time: toTime(times[i]), value: v});
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
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
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

  useEffect(() => {
    if (status !== 'ready' || !containerRef.current || candles.length === 0) return;

    const closes = candles.map(c => c.close);
    const times = candles.map(c => c.time);
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
      timeScale: {borderColor: grid, rightOffset: 4},
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
      candles.map(c => ({
        time: toTime(c.time),
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
      s.setData(lineData(times, sma(closes, ma.period)));
    }

    // Volume (pane 1).
    const vol = chart.addSeries(
      HistogramSeries,
      {priceFormat: {type: 'volume'}, priceLineVisible: false, lastValueVisible: false},
      1,
    );
    vol.setData(
      candles.map(c => ({
        time: toTime(c.time),
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
          time: toTime(times[x.i]),
          value: x.v as number,
          color: (x.v as number) >= 0 ? `${up}99` : `${down}99`,
        })),
    );
    const macdLine = chart.addSeries(
      LineSeries,
      {color: '#3b82f6', lineWidth: 1, priceLineVisible: false, lastValueVisible: false},
      2,
    );
    macdLine.setData(lineData(times, m.macd));
    const sigLine = chart.addSeries(
      LineSeries,
      {color: '#f59e0b', lineWidth: 1, priceLineVisible: false, lastValueVisible: false},
      2,
    );
    sigLine.setData(lineData(times, m.signal));

    // RSI (pane 3).
    const rsiLine = chart.addSeries(
      LineSeries,
      {color: '#a855f7', lineWidth: 1, priceLineVisible: false, lastValueVisible: false},
      3,
    );
    rsiLine.setData(lineData(times, rsi(closes)));

    // Pane heights: price dominant, indicator panes compact.
    const panes = chart.panes();
    panes[0]?.setHeight(280);
    panes[1]?.setHeight(70);
    panes[2]?.setHeight(90);
    panes[3]?.setHeight(80);

    // Default to the most recent ~130 sessions; the full history (~3y) is loaded,
    // so panning/scrolling left reveals it with no round-trip.
    const n = candles.length;
    if (n > 130) {
      chart.timeScale().setVisibleLogicalRange({from: n - 130, to: n - 1});
    } else {
      chart.timeScale().fitContent();
    }

    return () => chart.remove();
  }, [status, candles, dark]);

  return (
    <section className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <h2 className={cx('text-[14px] font-bold', t.text)}>{tr('kline.title')}</h2>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[10.5px] font-semibold">
          {MAS.map(ma => (
            <span key={ma.period} className="inline-flex items-center gap-1" style={{color: ma.color}}>
              <span className="inline-block h-0.5 w-3 rounded" style={{background: ma.color}} />
              MA{ma.period}
            </span>
          ))}
          <span className={t.faint}>· MACD · RSI · VOL</span>
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
