'use client';

import {ArrowRight} from 'lucide-react';
import {FundamentalsCard} from '@/components/FundamentalsCard';
import {KLineChart} from '@/components/KLineChart';
import Link from '@/components/LocalLink';
import {Markdown} from '@/components/Markdown';
import {type ChatBlock} from '@/lib/api';
import {cx, type Tokens} from '@/lib/ui';

// Shared chat-message rendering for both the per-stock thread and the unified hub. A
// widget block carries its own ticker (block.params.ticker, set server-side) so a
// cross-stock answer can surface AAPL's chart and MSFT's table in one thread.

export type Msg = {role: 'user' | 'assistant'; blocks?: ChatBlock[]; text?: string};

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

export function MsgRow({m, fallbackTicker, dark, t, tr}: {m: Msg; fallbackTicker: string; dark: boolean; t: Tokens; tr: (k: string) => string}) {
  if (m.role === 'user') {
    return (
      <div className="flex justify-end">
        <div className={cx('max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-br-sm px-3.5 py-2 text-[13px]', dark ? 'bg-violet-500/20 text-violet-50' : 'bg-violet-100 text-violet-900')}>
          {m.text}
        </div>
      </div>
    );
  }
  return (
    <div className="flex justify-start">
      <div className={cx('max-w-[92%] rounded-2xl rounded-bl-sm border px-3.5 py-2.5', t.card, t.border)}>
        {(m.blocks ?? []).map((b, i) => <BlockView key={i} block={b} fallbackTicker={fallbackTicker} dark={dark} t={t} tr={tr} />)}
        {!m.blocks && m.text && (
          <div className={cx('text-[13px] leading-relaxed', t.text)}>
            <Markdown>{m.text}</Markdown>
          </div>
        )}
      </div>
    </div>
  );
}

function BlockView({block, fallbackTicker, dark, t, tr}: {block: ChatBlock; fallbackTicker: string; dark: boolean; t: Tokens; tr: (k: string) => string}) {
  if (block.kind === 'text') {
    return (
      <div className={cx('text-[13px] leading-relaxed', t.text)}>
        <Markdown>{block.text ?? ''}</Markdown>
      </div>
    );
  }
  const ticker = block.params?.ticker || fallbackTicker;
  return <ChatWidget widget={block.widget ?? ''} ticker={ticker} dark={dark} t={t} tr={tr} />;
}

function ChatWidget({widget, ticker, dark, t, tr}: {widget: string; ticker: string; dark: boolean; t: Tokens; tr: (k: string) => string}) {
  if (!ticker) return null;
  if (widget === 'kline') {
    return (
      <div className="my-2">
        <KLineChart ticker={ticker} />
      </div>
    );
  }
  if (widget === 'fundamentals_table' || widget === 'valuation_table') {
    return (
      <div className="my-2">
        <FundamentalsCard ticker={ticker} />
      </div>
    );
  }
  const anchor = WIDGET_ANCHOR[widget] ?? '';
  const label = WIDGET_LABEL[widget] ?? widget;
  return (
    <Link
      href={`/stock/${encodeURIComponent(ticker)}${anchor}`}
      className={cx('my-2 inline-flex items-center gap-1.5 rounded-lg border px-3 py-2 text-[12.5px] font-medium', t.border, t.sub, dark ? 'hover:bg-slate-800/50' : 'hover:bg-slate-50')}
    >
      {tr('chat.widget.open').replace('{w}', `${ticker} ${label}`.trim())} <ArrowRight size={13} />
    </Link>
  );
}
