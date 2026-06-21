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

const UP = '#16a34a';
const DOWN = '#dc2626';

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
export function IndicatorHistoryChart({ticker, id, period}: {ticker: string; id: string; period?: number}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const containerRef = useRef<HTMLDivElement>(null);
  const [data, setData] = useState<IndicatorHistory | null>(null);
  const [status, setStatus] = useState<Status>('loading');

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

    if (id === 'technical.macd') {
      if (data.lines?.histogram) {
        const h = chart.addSeries(HistogramSeries, {priceLineVisible: false, lastValueVisible: false});
        h.setData(
          data.lines.histogram.map(p => ({
            time: p.date as unknown as Time,
            value: p.value,
            color: p.value >= 0 ? UP : DOWN,
          })),
        );
      }
      const line = chart.addSeries(LineSeries, {color: '#3b82f6', lineWidth: 1, priceLineVisible: false});
      line.setData(toSeries(data.points));
      if (data.lines?.signal) {
        const s = chart.addSeries(LineSeries, {
          color: '#f59e0b',
          lineWidth: 1,
          priceLineVisible: false,
          lastValueVisible: false,
          crosshairMarkerVisible: false,
        });
        s.setData(toSeries(data.lines.signal));
      }
    } else {
      const primary = chart.addSeries(LineSeries, {color: accent, lineWidth: 2, priceLineVisible: false});
      primary.setData(toSeries(data.points));
      // Bollinger bands (dashed envelope around the middle line).
      for (const k of ['upper', 'lower'] as const) {
        const band = data.lines?.[k];
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
  }, [data, status, dark, id]);

  if (status === 'empty') {
    return <p className={cx('px-3 py-3 text-[11.5px]', t.faint)}>{tr('ind2.chart.empty')}</p>;
  }

  return (
    <div className="relative" style={{height: 168}}>
      <div ref={containerRef} className="h-full w-full" />
      {status === 'loading' && (
        <div className={cx('absolute inset-0 m-2 animate-pulse rounded-lg', t.skel)} />
      )}
    </div>
  );
}
