'use client';

import {ArrowRight, Check, Copy} from 'lucide-react';
import {useState} from 'react';
import {FundamentalsCard} from '@/components/FundamentalsCard';
import {IndicatorHistoryChart} from '@/components/IndicatorHistoryChart';
import {KLineChart} from '@/components/KLineChart';
import Link from '@/components/LocalLink';
import {Markdown} from '@/components/Markdown';
import {ChatPortfolioWidget} from '@/components/PortfolioWidgets';
import {type ChatBlock} from '@/lib/api';

// Shared chat-message rendering for both the per-stock thread and the unified hub, styled
// on the chat-hub palette (CSS vars set by the hub root — see lib/chatTheme). A widget block
// carries its own ticker (block.params.ticker, set server-side) so a cross-stock answer can
// surface AAPL's chart and MSFT's table in one thread.

const PORTFOLIO_WIDGETS = new Set(['watchlist_summary', 'holdings_pnl', 'portfolio_heatmap']);

// `streaming` marks the live, still-being-typed assistant message. It carries its text as a
// single text BLOCK (not the `text` field) so the live and final renders share ONE render
// path (BlockView → Markdown): on `done` the block is updated in place rather than the prose
// node being unmounted and re-parsed, which is what made the message visibly "re-flash".
export type Msg = {role: 'user' | 'assistant'; blocks?: ChatBlock[]; text?: string; streaming?: boolean};

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

export function MsgRow({m, fallbackTicker, tr}: {m: Msg; fallbackTicker: string; tr: (k: string) => string}) {
  if (m.role === 'user') {
    return (
      <div style={{display: 'flex', justifyContent: 'flex-end'}}>
        <div style={{maxWidth: '80%', padding: '11px 15px', borderRadius: '16px 16px 4px 16px', background: 'var(--bubble)', border: '1px solid var(--bubble-line)', fontSize: 14, lineHeight: 1.5, color: 'var(--text)', whiteSpace: 'pre-wrap'}}>
          {m.text}
        </div>
      </div>
    );
  }
  const plain = (m.blocks ?? []).filter(b => b.kind === 'text').map(b => b.text ?? '').join('\n\n') || m.text || '';
  return (
    <div style={{display: 'flex', gap: 12}}>
      <div style={{flex: 'none', width: 28, height: 28, borderRadius: 8, background: 'var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontWeight: 700, fontSize: 12, color: '#1c1404'}}>T</div>
      <div style={{flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 14}}>
        <div style={{display: 'flex', alignItems: 'center', gap: 8}}>
          <span style={{fontSize: 12.5, fontWeight: 600, color: 'var(--text)'}}>{tr('chat.aiName')}</span>
          <span style={{fontSize: 11, color: 'var(--text3)'}}>{tr('chat.justNow')}</span>
        </div>
        {(m.blocks ?? []).map((b, i) => <BlockView key={i} block={b} fallbackTicker={fallbackTicker} tr={tr} />)}
        {!m.blocks && m.text && (
          <div style={{fontSize: 14, lineHeight: 1.62, color: 'var(--text)'}}>
            <Markdown>{m.text}</Markdown>
          </div>
        )}
        <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, flexWrap: 'wrap', paddingTop: 8, borderTop: '1px solid var(--border)'}}>
          <span style={{fontSize: 10.5, color: 'var(--text3)', lineHeight: 1.4}}>{tr('chat.disclaimer')}</span>
          <CopyButton text={plain} tr={tr} />
        </div>
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

function BlockView({block, fallbackTicker, tr}: {block: ChatBlock; fallbackTicker: string; tr: (k: string) => string}) {
  if (block.kind === 'text') {
    return (
      <div style={{fontSize: 14, lineHeight: 1.62, color: 'var(--text)'}} className="tw-chat-prose">
        <Markdown>{block.text ?? ''}</Markdown>
      </div>
    );
  }
  const ticker = block.params?.ticker || fallbackTicker;
  return <ChatWidget widget={block.widget ?? ''} ticker={ticker} indicatorId={block.params?.indicator} tr={tr} />;
}

// Widgets render the real Go-owned data via their own components (each already a card),
// so they're placed directly — no extra wrapper (a double card left dead space below).
function ChatWidget({widget, ticker, indicatorId, tr}: {widget: string; ticker: string; indicatorId?: string; tr: (k: string) => string}) {
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
    return <IndicatorHistoryChart ticker={ticker} id={indicatorId} />;
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
