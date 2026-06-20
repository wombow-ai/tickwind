'use client';

import {ArrowLeft, ArrowRight, Loader2, Lock, Send, ShieldCheck, Sparkles} from 'lucide-react';
import {useCallback, useEffect, useRef, useState} from 'react';
import {FundamentalsCard} from '@/components/FundamentalsCard';
import {KLineChart} from '@/components/KLineChart';
import Link from '@/components/LocalLink';
import {Markdown} from '@/components/Markdown';
import {type ChatBlock, getChatHistory, postChat} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useEntitlement} from '@/lib/entitlement';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok, type Tokens} from '@/lib/ui';

/**
 * ChatThread — Product B: the personalized, ticker-scoped AI chat (Pro-gated). A Pro
 * member asks their own question; the assistant answers in prose blocks and may surface
 * real Tickwind widgets inline. Anti-hallucination is enforced server-side (every number
 * is sourced; no advice); this view just renders the blocks + the gates.
 */

type Msg = {role: 'user' | 'assistant'; blocks?: ChatBlock[]; text?: string};

const SUGGESTIONS = ['chat.suggest.valuation', 'chat.suggest.bear', 'chat.suggest.flows'];

export function ChatThread({ticker}: {ticker: string}) {
  const norm = ticker.toUpperCase();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const {user, loading: authLoading, getToken} = useAuth();
  const {isPro, loading: entLoading} = useEntitlement();

  const [messages, setMessages] = useState<Msg[]>([]);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [err, setErr] = useState(false);
  const [meter, setMeter] = useState<{used: number; limit: number} | null>(null);
  const endRef = useRef<HTMLDivElement>(null);

  // Load the persisted thread once authed + Pro.
  useEffect(() => {
    if (!user || !isPro) return;
    const c = new AbortController();
    (async () => {
      try {
        const token = await getToken();
        const h = await getChatHistory(norm, token, c.signal);
        setMessages(h.map(m => ({role: m.role, blocks: m.blocks, text: m.text})));
      } catch {
        /* empty / not-found thread is fine */
      }
    })();
    return () => c.abort();
  }, [user, isPro, norm, getToken]);

  useEffect(() => {
    endRef.current?.scrollIntoView({behavior: 'smooth'});
  }, [messages, sending]);

  const send = useCallback(
    async (raw: string) => {
      const msg = raw.trim();
      if (!msg || sending) return;
      setErr(false);
      setSending(true);
      setInput('');
      setMessages(m => [...m, {role: 'user', text: msg}]);
      try {
        const token = await getToken();
        const res = await postChat(norm, msg, token, lang);
        setMessages(m => [...m, {role: 'assistant', blocks: res.blocks}]);
        if (res.meter) setMeter(res.meter);
      } catch {
        setErr(true);
      } finally {
        setSending(false);
      }
    },
    [norm, lang, sending, getToken],
  );

  if (authLoading || entLoading) {
    return (
      <Wrap dark={dark} t={t} tr={tr} norm={norm} meter={null}>
        <div className={cx('flex items-center gap-2 rounded-2xl border p-6 text-[13px]', t.card, t.border, t.sub)}>
          <Loader2 size={15} className="animate-spin" /> {tr('chat.thinking')}
        </div>
      </Wrap>
    );
  }
  if (!user) {
    return (
      <Wrap dark={dark} t={t} tr={tr} norm={norm} meter={null}>
        <Gate dark={dark} t={t} icon={<Lock size={20} />} title={tr('chat.gate.login.title')} body={tr('chat.gate.login.body')} cta={tr('chat.login')} href="/login" />
      </Wrap>
    );
  }
  if (!isPro) {
    return (
      <Wrap dark={dark} t={t} tr={tr} norm={norm} meter={null}>
        <Gate dark={dark} t={t} icon={<Sparkles size={20} />} title={tr('chat.gate.pro.title')} body={tr('chat.gate.pro.body').replace('{t}', norm)} cta={tr('chat.gate.cta')} href="/pro" />
      </Wrap>
    );
  }

  return (
    <Wrap dark={dark} t={t} tr={tr} norm={norm} meter={meter}>
      <div className="space-y-4">
        {messages.length === 0 ? (
          <p className={cx('text-[13px]', t.sub)}>{tr('chat.empty')}</p>
        ) : (
          messages.map((m, i) => <MsgRow key={i} m={m} ticker={norm} dark={dark} t={t} tr={tr} />)
        )}
        {sending && (
          <div className={cx('flex items-center gap-2 text-[12.5px]', t.faint)}>
            <Loader2 size={14} className="animate-spin" /> {tr('chat.thinking')}
          </div>
        )}
        {err && <p className="text-[12.5px] text-rose-500">{tr('chat.error')}</p>}
        <div ref={endRef} />
      </div>

      {messages.length === 0 && (
        <div className="mt-4 flex flex-wrap gap-2">
          {SUGGESTIONS.map(k => (
            <button
              key={k}
              type="button"
              onClick={() => send(tr(k))}
              className={cx('rounded-full border px-3 py-1.5 text-[12px] font-medium transition', t.border, t.sub, dark ? 'hover:bg-slate-800/60' : 'hover:bg-slate-50')}
            >
              {tr(k)}
            </button>
          ))}
        </div>
      )}

      <form onSubmit={e => { e.preventDefault(); send(input); }} className="mt-4 flex items-end gap-2">
        <textarea
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={e => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              send(input);
            }
          }}
          rows={1}
          placeholder={tr('chat.placeholder')}
          className={cx('min-h-[44px] flex-1 resize-none rounded-xl border px-3 py-3 text-[13px] outline-none', t.card, t.border, t.text)}
        />
        <button
          type="submit"
          disabled={sending || !input.trim()}
          className={cx('inline-flex h-[44px] shrink-0 items-center gap-1.5 rounded-xl px-4 text-[13px] font-semibold disabled:opacity-50', btnPrimary(dark))}
        >
          <Send size={14} /> {tr('chat.send')}
        </button>
      </form>
      <p className={cx('mt-2 flex items-center gap-1.5 text-[11px]', t.faint)}>
        <ShieldCheck size={12} className={dark ? 'text-emerald-400' : 'text-emerald-500'} />
        {tr('chat.disclaimer')}
      </p>
    </Wrap>
  );
}

function Wrap({
  dark,
  t,
  tr,
  norm,
  meter,
  children,
}: {
  dark: boolean;
  t: Tokens;
  tr: (k: string) => string;
  norm: string;
  meter: {used: number; limit: number} | null;
  children: React.ReactNode;
}) {
  return (
    <div className="mx-auto max-w-3xl tw-fade">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <Link href={`/stock/${encodeURIComponent(norm)}`} className={cx('inline-flex items-center gap-1 text-[12px]', t.faint)}>
            <ArrowLeft size={13} /> {tr('chat.back').replace('{t}', norm)}
          </Link>
          <h1 className={cx('mt-1 flex items-center gap-1.5 text-[18px] font-bold', t.text)}>
            <Sparkles size={17} className={dark ? 'text-violet-300' : 'text-violet-500'} />
            {tr('chat.title').replace('{t}', norm)}
          </h1>
        </div>
        {meter && (
          <span className={cx('mt-5 shrink-0 rounded-md px-2 py-1 text-[11px] font-semibold tabular-nums', dark ? 'bg-slate-800 text-slate-300' : 'bg-slate-100 text-slate-600')}>
            {tr('chat.meter').replace('{used}', String(meter.used)).replace('{limit}', String(meter.limit))}
          </span>
        )}
      </div>
      <p className={cx('mb-4 text-[12.5px]', t.sub)}>{tr('chat.subtitle').replace('{t}', norm)}</p>
      {children}
    </div>
  );
}

function Gate({
  dark,
  t,
  icon,
  title,
  body,
  cta,
  href,
}: {
  dark: boolean;
  t: Tokens;
  icon: React.ReactNode;
  title: string;
  body: string;
  cta: string;
  href: string;
}) {
  return (
    <div className={cx('rounded-2xl border p-6 text-center', t.card, t.border, dark ? 'bg-violet-500/[0.05]' : 'bg-violet-50/50')}>
      <div className={cx('mx-auto mb-2 flex h-10 w-10 items-center justify-center rounded-full', dark ? 'bg-violet-500/15 text-violet-300' : 'bg-violet-100 text-violet-600')}>{icon}</div>
      <p className={cx('text-[15px] font-bold', t.text)}>{title}</p>
      <p className={cx('mx-auto mt-1.5 max-w-md text-[12.5px]', t.sub)}>{body}</p>
      <Link href={href} className={cx('mt-3 inline-flex items-center gap-1 rounded-full px-4 py-1.5 text-[12.5px] font-semibold', btnPrimary(dark))}>
        {cta} <ArrowRight size={13} />
      </Link>
    </div>
  );
}

function MsgRow({m, ticker, dark, t, tr}: {m: Msg; ticker: string; dark: boolean; t: Tokens; tr: (k: string) => string}) {
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
        {(m.blocks ?? []).map((b, i) => <BlockView key={i} block={b} ticker={ticker} dark={dark} t={t} tr={tr} />)}
        {!m.blocks && m.text && (
          <div className={cx('text-[13px] leading-relaxed', t.text)}>
            <Markdown>{m.text}</Markdown>
          </div>
        )}
      </div>
    </div>
  );
}

function BlockView({block, ticker, dark, t, tr}: {block: ChatBlock; ticker: string; dark: boolean; t: Tokens; tr: (k: string) => string}) {
  if (block.kind === 'text') {
    return (
      <div className={cx('text-[13px] leading-relaxed', t.text)}>
        <Markdown>{block.text ?? ''}</Markdown>
      </div>
    );
  }
  return <ChatWidget widget={block.widget ?? ''} ticker={ticker} dark={dark} t={t} tr={tr} />;
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

function ChatWidget({widget, ticker, dark, t, tr}: {widget: string; ticker: string; dark: boolean; t: Tokens; tr: (k: string) => string}) {
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
      {tr('chat.widget.open').replace('{w}', label)} <ArrowRight size={13} />
    </Link>
  );
}
