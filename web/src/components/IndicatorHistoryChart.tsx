'use client';

import {
  ColorType,
  createChart,
  HistogramSeries,
  LineSeries,
  LineStyle,
  type Time,
} from 'lightweight-charts';
import {useEffect, useRef, useState} from 'react';
import {getIndicatorHistory, type IndicatorHistory, type IndicatorHistoryPoint} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'empty';
type Range = '3M' | '1Y' | 'Max';

const UP = '#16a34a';
const DOWN = '#dc2626';
const RANGES: Range[] = ['3M', '1Y', 'Max'];

/** Maps an external range hint (incl. the chat's 3M/1Y/5Y) to a chart range; defaults to 1Y. */
function normalizeRange(r?: string): Range {
  if (r === '3M') return '3M';
  if (r === 'Max' || r === '5Y' || r === 'ALL') return 'Max';
  return '1Y';
}

/** Slices an ascending dated series to the trailing window (Max = the full series). */
function windowByRange(pts: IndicatorHistoryPoint[], range: Range): IndicatorHistoryPoint[] {
  if (range === 'Max' || pts.length === 0) return pts;
  const days = range === '3M' ? 92 : 366;
  const last = new Date(pts[pts.length - 1].date + 'T00:00:00Z');
  last.setUTCDate(last.getUTCDate() - days);
  const cutoff = last.toISOString().slice(0, 10);
  let i = 0;
  while (i < pts.length && pts[i].date < cutoff) i++;
  return pts.slice(i);
}

/** {time,value}[] for lightweight-charts from a dated indicator series. */
function toSeries(pts: IndicatorHistoryPoint[]): {time: Time; value: number}[] {
  return pts.map(p => ({time: p.date as unknown as Time, value: p.value}));
}

/**
 * IndicatorHistoryChart draws ONE technical indicator's time series (the date-aligned line a
 * chart shows) under its row in the indicators panel — the time-series counterpart to the
 * single-point value. Every number is Go-computed (GET /v1/stocks/{t}/indicator-history), so
 * the chart's latest point equals the panel value (one source of truth, anti-hallucination
 * safe). MACD renders as histogram + line + signal; Bollinger as middle + upper/lower bands;
 * the rest as a single accent line. lightweight-charts is imperative/canvas, so the chart is
 * built in an effect and torn down on cleanup (mirrors KLineChart).
 */
export function IndicatorHistoryChart({ticker, id, period, range: rangeProp}: {ticker: string; id: string; period?: number; range?: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const containerRef = useRef<HTMLDivElement>(null);
  const [data, setData] = useState<IndicatorHistory | null>(null);
  const [status, setStatus] = useState<Status>('loading');
  // Visible window — defaults to the caller's hint (the chat passes 1Y/3M/5Y), else 1Y, so a
  // 5-year series isn't crammed into the small pane. Re-windows in place (no refetch).
  const [range, setRange] = useState<Range>(() => normalizeRange(rangeProp));

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getIndicatorHistory(ticker, id, period, c.signal).then(h => {
      if (c.signal.aborted) return;
      if (h && h.points.length > 0) {
        setData(h);
        setStatus('ready');
      } else {
        setStatus('empty');
      }
    });
    return () => c.abort();
  }, [ticker, id, period]);

  useEffect(() => {
    if (!containerRef.current || status !== 'ready' || !data) return;
    const grid = dark ? '#1e293b' : '#eef2f7';
    const axis = dark ? '#94a3b8' : '#64748b';
    const accent = dark ? '#2dd4bf' : '#0d9488'; // teal, matches the indicators panel

    const chart = createChart(containerRef.current, {
      autoSize: true,
      layout: {
        background: {type: ColorType.Solid, color: 'transparent'},
        textColor: axis,
        attributionLogo: true, // lightweight-charts (Apache-2.0) attribution
        fontSize: 10,
      },
      grid: {vertLines: {color: grid}, horzLines: {color: grid}},
      rightPriceScale: {borderColor: grid},
      timeScale: {borderColor: grid, rightOffset: 3, timeVisible: false, secondsVisible: false},
      handleScale: false, // a compact read-only sparkline-style pane (no zoom fights with the page)
      handleScroll: false,
    });

    // Window every series to the selected range (same cutoff → all lines stay aligned).
    const pts = windowByRange(data.points, range);
    const win = (k: string) => (data.lines?.[k] ? windowByRange(data.lines[k], range) : undefined);

    if (id === 'technical.macd') {
      const hist = win('histogram');
      if (hist) {
        const h = chart.addSeries(HistogramSeries, {priceLineVisible: false, lastValueVisible: false});
        h.setData(hist.map(p => ({time: p.date as unknown as Time, value: p.value, color: p.value >= 0 ? UP : DOWN})));
      }
      const line = chart.addSeries(LineSeries, {color: '#3b82f6', lineWidth: 1, priceLineVisible: false});
      line.setData(toSeries(pts));
      const signal = win('signal');
      if (signal) {
        const s = chart.addSeries(LineSeries, {
          color: '#f59e0b',
          lineWidth: 1,
          priceLineVisible: false,
          lastValueVisible: false,
          crosshairMarkerVisible: false,
        });
        s.setData(toSeries(signal));
      }
    } else {
      const primary = chart.addSeries(LineSeries, {color: accent, lineWidth: 2, priceLineVisible: false});
      primary.setData(toSeries(pts));
      // Bollinger bands (dashed envelope around the middle line).
      for (const k of ['upper', 'lower'] as const) {
        const band = win(k);
        if (band) {
          const s = chart.addSeries(LineSeries, {
            color: '#6366f1',
            lineWidth: 1,
            lineStyle: LineStyle.Dashed,
            priceLineVisible: false,
            lastValueVisible: false,
            crosshairMarkerVisible: false,
          });
          s.setData(toSeries(band));
        }
      }
    }
    chart.timeScale().fitContent();
    return () => chart.remove();
  }, [data, status, dark, id, range]);

  if (status === 'empty') {
    return <p className={cx('px-3 py-3 text-[11.5px]', t.faint)}>{tr('ind2.chart.empty')}</p>;
  }

  return (
    <div>
      <div className="mb-1.5 flex justify-end gap-1">
        {RANGES.map(r => (
          <button
            key={r}
            type="button"
            onClick={() => setRange(r)}
            aria-pressed={range === r}
            className={cx(
              'rounded-md px-2 py-0.5 text-[10.5px] font-semibold transition',
              range === r
                ? dark
                  ? 'bg-teal-500/15 text-teal-300'
                  : 'bg-teal-50 text-teal-600'
                : cx(t.faint, 'hover:opacity-80'),
            )}
          >
            {r}
          </button>
        ))}
      </div>
      <div className="relative" style={{height: 168}}>
        <div ref={containerRef} className="h-full w-full" />
        {status === 'loading' && (
          <div className={cx('absolute inset-0 m-2 animate-pulse rounded-lg', t.skel)} />
        )}
      </div>
    </div>
  );
}
