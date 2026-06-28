'use client';

import {ArrowRight, BarChart3, Check, ChevronDown, ChevronRight, Copy, Eye, FileText, Globe, type LucideIcon, Newspaper, PenLine, StickyNote, Wallet} from 'lucide-react';
import {useState} from 'react';
import {FundamentalsCard} from '@/components/FundamentalsCard';
import {IndicatorHistoryChart} from '@/components/IndicatorHistoryChart';
import {KLineChart} from '@/components/KLineChart';
import {SeasonalityCard} from '@/components/SeasonalityCard';
import {RelativeStrengthCard} from '@/components/RelativeStrengthCard';
import {EarningsReactionCard} from '@/components/EarningsReactionCard';
import {ScorecardCard} from '@/components/ScorecardCard';
import Link from '@/components/LocalLink';
import {LogoMark} from '@/components/ui/atoms';
import {Markdown} from '@/components/Markdown';
import {ChatPortfolioWidget} from '@/components/PortfolioWidgets';
import {type ChatBlock} from '@/lib/api';

// Shared chat-message rendering for both the per-stock thread and the unified hub, styled
// on the chat-hub palette (CSS vars set by the hub root — see lib/chatTheme). A widget block
// carries its own ticker (block.params.ticker, set server-side) so a cross-stock answer can
// surface AAPL's chart and MSFT's table in one thread.

const PORTFOLIO_WIDGETS = new Set(['watchlist_summary', 'holdings_pnl', 'portfolio_heatmap']);

// widgetRenderKey collapses widgets that render the SAME component (fundamentals_table +
// valuation_table → one FundamentalsCard) to a single key. dedupeBlocks then drops
// render-identical widget blocks (keeping the first); text blocks pass through. The server
// already dedupes new messages (chat.go dedupeWidgets) — this is a belt-and-suspenders
// backstop that also cleans historical / streamed messages that bypass it.
function widgetRenderKey(b: ChatBlock): string {
  const w =
    b.widget === 'fundamentals_table' || b.widget === 'valuation_table'
      ? 'fundamentals_family'
      : b.widget;
  return `${w}|${b.params?.ticker ?? ''}|${b.params?.indicator ?? ''}`;
}

function dedupeBlocks(blocks: ChatBlock[]): ChatBlock[] {
  const seen = new Set<string>();
  return blocks.filter(b => {
    if (b.kind !== 'widget') return true;
    const k = widgetRenderKey(b);
    if (seen.has(k)) return false;
    seen.add(k);
    return true;
  });
}

// `streaming` marks the live, still-being-typed assistant message. It carries its text as a
// single text BLOCK (not the `text` field) so the live and final renders share ONE render
// path (BlockView → Markdown): on `done` the block is updated in place rather than the prose
// node being unmounted and re-parsed, which is what made the message visibly "re-flash".
export type Msg = {role: 'user' | 'assistant'; blocks?: ChatBlock[]; text?: string; streaming?: boolean};

// One deterministic tool-execution step in the gray ReAct chain. Label is Go-authored
// (i18n done server-side); the frontend renders it verbatim — never a number, never model text.
export type ExecStep = {kind: string; label: string};

const KIND_ICON: Record<string, LucideIcon> = {
  facts: FileText,
  news: Newspaper,
  web: Globe,
  etf: Wallet,
  widget: BarChart3,
  watchlist: Eye,
  holdings: Wallet,
  notes: StickyNote,
  writing: PenLine,
};

/**
 * The live execution chain: a gray, Claude-style trace of the deterministic tool steps the
 * assistant ran (Reading AAPL fundamentals, Searching the web, ...), shown while the answer is
 * still being prepared. The LAST row is the current action (its icon pulses); earlier rows are
 * done (a gold check). Ephemeral — it unmounts the moment the answer starts streaming, handing
 * the live role to the streaming caret. Each label is a Go-owned string (no numbers, no model
 * prose), so this surface is anti-hallucination-safe by construction.
 */
export function ExecChain({steps, running = true, bare = false}: {steps: ExecStep[]; running?: boolean; bare?: boolean}) {
  if (steps.length === 0) return null;
  const rows = (
    <div style={{flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 5, paddingTop: bare ? 0 : 4}}>
      {steps.map((st, i) => {
          // While the tools run, the last row is the live action (pulses). Once the answer is
          // streaming (running=false), every row is done (a gold check) so nothing competes with
          // the streaming caret below.
          const current = running && i === steps.length - 1;
          const Icon = KIND_ICON[st.kind];
          return (
            <div key={i} className={'tw-exec-row tw-exec-row-in' + (current ? ' current' : '')}>
              <span className={'tw-exec-icon' + (current ? ' tw-exec-pulse' : '')}>
                {current ? Icon ? <Icon size={13} /> : <span style={{width: 5, height: 5, borderRadius: '50%', background: 'currentColor'}} /> : <Check size={13} className="tw-exec-check" />}
              </span>
              <span>{st.label}</span>
            </div>
          );
        })}
    </div>
  );
  if (bare) return rows; // inside a persisted message (avatar already shown) → just the rows
  return (
    <div style={{display: 'flex', gap: 12}}>
      <div style={{flex: 'none', width: 28, height: 28, borderRadius: 8, background: 'var(--surface2)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
        <LogoMark size={18} accent="var(--accent)" />
      </div>
      {rows}
    </div>
  );
}

// TraceBlock renders the PERSISTED execution chain on reloaded history: a quiet, collapsed
// "Steps · N" toggle that expands to the (all-done) chain. Display-only — the server never feeds
// these labels back to the model.
function TraceBlock({steps, tr}: {steps: ExecStep[]; tr: (k: string) => string}) {
  const [open, setOpen] = useState(false);
  if (!steps || steps.length === 0) return null;
  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        aria-expanded={open}
        className="tw-chat-iconbtn"
        style={{display: 'inline-flex', alignItems: 'center', gap: 5, fontSize: 11.5, fontWeight: 500, color: 'var(--text3)', border: 'none', background: 'transparent', cursor: 'pointer', padding: '2px 7px', borderRadius: 7}}
      >
        {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        {tr('chat.trace')} · {steps.length}
      </button>
      {open && (
        <div style={{marginTop: 5, marginLeft: 5}}>
          <ExecChain steps={steps} running={false} bare />
        </div>
      )}
    </div>
  );
}

const WIDGET_ANCHOR: Record<string, string> = {
  flows_summary: '#short',
  whales: '#whales',
  options: '#options',
  insider: '#insider-activity',
};

const WIDGET_LABEL: Record<string, string> = {
  kline: 'chart',
  indicators: 'indicators',
  flows_summary: 'smart-money flows',
  whales: 'institutional holders',
  options: 'options',
  insider: 'insider activity',
  valuation_table: 'valuation',
  fundamentals_table: 'fundamentals',
};

// ThinkingDots — a quiet inline pulse shown inside the assistant message before any step or
// token arrives (replaces the old separate ThinkingRow; the chain/answer fill in below it).
function ThinkingDots() {
  return (
    <span style={{display: 'inline-flex', gap: 4, alignItems: 'center', height: 16}}>
      {[0, 0.15, 0.3].map((d, i) => (
        <span key={i} className="tw-chat-dot" style={{width: 5, height: 5, borderRadius: '50%', background: 'var(--accent)', animation: `tw-chat-pulse 1.2s infinite ${d}s`}} />
      ))}
    </span>
  );
}

export function MsgRow({m, fallbackTicker, tr, liveSteps}: {m: Msg; fallbackTicker: string; tr: (k: string) => string; liveSteps?: ExecStep[]}) {
  if (m.role === 'user') {
    return (
      <div style={{display: 'flex', justifyContent: 'flex-end'}}>
        <div style={{maxWidth: '80%', padding: '11px 15px', borderRadius: '16px 16px 4px 16px', background: 'var(--bubble)', border: '1px solid var(--bubble-line)', fontSize: 14, lineHeight: 1.5, color: 'var(--text)', whiteSpace: 'pre-wrap'}}>
          {m.text}
        </div>
      </div>
    );
  }
  const blocks = dedupeBlocks(m.blocks ?? []);
  const plain = blocks.filter(b => b.kind === 'text').map(b => b.text ?? '').join('\n\n') || m.text || '';
  const hasContent = plain.trim().length > 0;
  const hasLive = !!liveSteps && liveSteps.length > 0;
  return (
    <div style={{display: 'flex', gap: 12}}>
      <div style={{flex: 'none', width: 28, height: 28, borderRadius: 8, background: 'var(--surface2)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
        <LogoMark size={18} accent="var(--accent)" />
      </div>
      <div style={{flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 14}}>
        <div style={{display: 'flex', alignItems: 'center', gap: 8}}>
          <span style={{fontSize: 12.5, fontWeight: 600, color: 'var(--text)'}}>{tr('chat.aiName')}</span>
          <span style={{fontSize: 11, color: 'var(--text3)'}}>{tr('chat.justNow')}</span>
        </div>
        {/* The execution steps render INLINE inside the message (gray, Claude-Code style), above
            the answer — NOT as a separate row pinned at the top. While streaming they're live
            (last row pulses); on done a persisted "trace" block takes over (collapsed). */}
        {hasLive && <ExecChain steps={liveSteps!} running={!!m.streaming} bare />}
        {m.streaming && !hasLive && !hasContent && <ThinkingDots />}
        {blocks.map((b, i) => <BlockView key={i} block={b} fallbackTicker={fallbackTicker} tr={tr} streaming={m.streaming} />)}
        {!m.blocks && m.text && (
          <div style={{fontSize: 14, lineHeight: 1.62, color: 'var(--text)'}}>
            <Markdown>{m.text}</Markdown>
          </div>
        )}
        {(!m.streaming || hasContent) && (
          <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, flexWrap: 'wrap', paddingTop: 8, borderTop: '1px solid var(--border)'}}>
            <span style={{fontSize: 10.5, color: 'var(--text3)', lineHeight: 1.4}}>{tr('chat.disclaimer')}</span>
            <CopyButton text={plain} tr={tr} />
          </div>
        )}
      </div>
    </div>
  );
}

function CopyButton({text, tr}: {text: string; tr: (k: string) => string}) {
  const [done, setDone] = useState(false);
  if (!text) return null;
  return (
    <button
      type="button"
      aria-label={tr('chat.copy')}
      className={done ? undefined : 'tw-chat-iconbtn'}
      onClick={() => {
        if (typeof navigator !== 'undefined' && navigator.clipboard) {
          void navigator.clipboard.writeText(text).then(() => {
            setDone(true);
            setTimeout(() => setDone(false), 1400);
          });
        }
      }}
      style={{width: 28, height: 28, borderRadius: 7, border: 'none', background: 'transparent', color: done ? 'var(--up)' : 'var(--text3)', cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center'}}
    >
      {done ? <Check size={14} /> : <Copy size={14} />}
    </button>
  );
}

function BlockView({block, fallbackTicker, tr, streaming}: {block: ChatBlock; fallbackTicker: string; tr: (k: string) => string; streaming?: boolean}) {
  if (block.kind === 'text') {
    return (
      <div style={{fontSize: 14, lineHeight: 1.62, color: 'var(--text)'}} className={streaming ? 'tw-chat-prose tw-chat-streaming' : 'tw-chat-prose'}>
        <Markdown>{block.text ?? ''}</Markdown>
      </div>
    );
  }
  if (block.kind === 'trace') {
    return <TraceBlock steps={block.steps ?? []} tr={tr} />;
  }
  const ticker = block.params?.ticker || fallbackTicker;
  return <ChatWidget widget={block.widget ?? ''} ticker={ticker} indicatorId={block.params?.indicator} range={block.params?.range} tr={tr} />;
}

// Widgets render the real Go-owned data via their own components (each already a card),
// so they're placed directly — no extra wrapper (a double card left dead space below).
function ChatWidget({widget, ticker, indicatorId, range, tr}: {widget: string; ticker: string; indicatorId?: string; range?: string; tr: (k: string) => string}) {
  if (PORTFOLIO_WIDGETS.has(widget)) {
    return <ChatPortfolioWidget type={widget} />;
  }
  if (!ticker) return null;
  // kline (price) and indicators (技术指标叠加图) both render the overlay chart inline —
  // KLineChart draws candles + the MA overlays + MACD/RSI panes, which IS the indicators
  // overlay the model says it surfaced (was falling through to a bare deep-link before).
  if (widget === 'kline' || widget === 'indicators') {
    return <KLineChart ticker={ticker} />;
  }
  // indicator_history charts ONE indicator's time series (Go-computed; numbers never enter the
  // model). The id is the catalog id the server validated (e.g. technical.rsi).
  if (widget === 'indicator_history') {
    if (!indicatorId) return null;
    return <IndicatorHistoryChart ticker={ticker} id={indicatorId} range={range} />;
  }
  // seasonality: the month-of-year return card (Go-computed; numbers never enter the model).
  if (widget === 'seasonality') {
    return <SeasonalityCard ticker={ticker} />;
  }
  // relative_strength: trailing 1M/3M/6M/1Y excess return vs SPY (Go-computed; numbers never
  // enter the model — the widget carries only the ticker).
  if (widget === 'relative_strength') {
    return <RelativeStrengthCard ticker={ticker} />;
  }
  // earnings_reaction: how the stock has historically moved around past earnings (Go-computed;
  // numbers never enter the model — the widget carries only the ticker).
  if (widget === 'earnings_reaction') {
    return <EarningsReactionCard ticker={ticker} />;
  }
  // scorecard: value/growth/quality/momentum factor PERCENTILES vs the tracked universe
  // (Go-computed; descriptive, not a rating — the widget carries only the ticker).
  if (widget === 'scorecard') {
    return <ScorecardCard ticker={ticker} />;
  }
  if (widget === 'fundamentals_table' || widget === 'valuation_table') {
    return <FundamentalsCard ticker={ticker} />;
  }
  const anchor = WIDGET_ANCHOR[widget] ?? '';
  const label = WIDGET_LABEL[widget] ?? widget;
  return (
    <Link
      href={`/stock/${encodeURIComponent(ticker)}${anchor}`}
      style={{display: 'inline-flex', alignItems: 'center', gap: 7, alignSelf: 'flex-start', padding: '8px 13px', borderRadius: 9, border: '1px solid var(--border2)', fontSize: 12.5, fontWeight: 500, color: 'var(--text)', textDecoration: 'none'}}
    >
      {tr('chat.widget.open').replace('{w}', `${ticker} ${label}`.trim())} <ArrowRight size={13} />
    </Link>
  );
}
