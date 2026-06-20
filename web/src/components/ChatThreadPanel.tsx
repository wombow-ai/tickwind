'use client';

import {Loader2, Plus, Send, ShieldCheck} from 'lucide-react';
import {useCallback, useEffect, useRef, useState} from 'react';
import {MsgRow, type Msg} from '@/components/chatRender';
import {clearChat, getChatHistory, getConvHistory, postChat, postConvChat} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

// ChatThreadPanel renders one conversation's messages + composer, for BOTH surfaces:
// a per-stock chat ({kind:'stock'}) and the unified hub ({kind:'conversation'}). The
// caller owns the auth/Pro gate + page chrome. onActivity fires after a successful send
// (so the hub can refresh the sidebar order).

export type ChatSource =
  | {kind: 'stock'; ticker: string}
  | {kind: 'conversation'; id: string; anchorTicker?: string};

const SUGGESTIONS = ['chat.suggest.valuation', 'chat.suggest.bear', 'chat.suggest.flows'];

function sourceKey(s: ChatSource): string {
  return s.kind === 'stock' ? 'stock:' + s.ticker.toUpperCase() : 'conv:' + s.id;
}

export function ChatThreadPanel({source, onActivity}: {source: ChatSource; onActivity?: () => void}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const {getToken} = useAuth();

  const [messages, setMessages] = useState<Msg[]>([]);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [err, setErr] = useState(false);
  const [meter, setMeter] = useState<{used: number; limit: number} | null>(null);
  const endRef = useRef<HTMLDivElement>(null);

  const fallbackTicker = source.kind === 'stock' ? source.ticker.toUpperCase() : (source.anchorTicker ?? '').toUpperCase();
  const key = sourceKey(source);

  // Load the persisted thread when the source changes.
  useEffect(() => {
    let active = true;
    setMessages([]);
    setMeter(null);
    const c = new AbortController();
    (async () => {
      try {
        const token = await getToken();
        const h = source.kind === 'stock'
          ? await getChatHistory(source.ticker, token, c.signal)
          : await getConvHistory(source.id, token, c.signal);
        if (active) setMessages(h.map(m => ({role: m.role, blocks: m.blocks, text: m.text})));
      } catch {
        /* empty thread is fine */
      }
    })();
    return () => {
      active = false;
      c.abort();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, getToken]);

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
        const res = source.kind === 'stock'
          ? await postChat(source.ticker, msg, token, lang)
          : await postConvChat(source.id, msg, token, lang);
        setMessages(m => [...m, {role: 'assistant', blocks: res.blocks}]);
        if (res.meter) setMeter(res.meter);
        onActivity?.();
      } catch {
        setErr(true);
      } finally {
        setSending(false);
      }
    },
    [key, lang, sending, getToken, onActivity], // eslint-disable-line react-hooks/exhaustive-deps
  );

  const resetStockThread = useCallback(async () => {
    if (source.kind !== 'stock') return;
    try {
      await clearChat(source.ticker, await getToken());
    } catch {
      /* best-effort */
    }
    setMessages([]);
    setMeter(null);
    setErr(false);
  }, [key, getToken]); // eslint-disable-line react-hooks/exhaustive-deps

  const placeholder = source.kind === 'stock' ? tr('chat.placeholder') : tr('chat.hub.placeholder');

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between gap-2">
        {meter ? (
          <span className={cx('rounded-md px-2 py-1 text-[11px] font-semibold tabular-nums', dark ? 'bg-slate-800 text-slate-300' : 'bg-slate-100 text-slate-600')}>
            {tr('chat.meter').replace('{used}', String(meter.used)).replace('{limit}', String(meter.limit))}
          </span>
        ) : (
          <span />
        )}
        {source.kind === 'stock' && messages.length > 0 && (
          <button
            type="button"
            onClick={resetStockThread}
            className={cx('inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-[11px] font-medium', t.border, t.faint, dark ? 'hover:bg-slate-800/60' : 'hover:bg-slate-50')}
          >
            <Plus size={12} /> {tr('chat.new')}
          </button>
        )}
      </div>

      <div className="mt-3 flex-1 space-y-4 overflow-y-auto">
        {messages.length === 0 ? (
          <p className={cx('text-[13px]', t.sub)}>{tr('chat.empty')}</p>
        ) : (
          messages.map((m, i) => <MsgRow key={i} m={m} fallbackTicker={fallbackTicker} dark={dark} t={t} tr={tr} />)
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
        <div className="mt-3 flex flex-wrap gap-2">
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

      <form onSubmit={e => { e.preventDefault(); send(input); }} className="mt-3 flex items-end gap-2">
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
          placeholder={placeholder}
          aria-label={placeholder}
          className={cx('min-h-[44px] flex-1 resize-none rounded-xl border px-3 py-3 text-[13px] outline-none', t.card, t.border, t.text)}
        />
        <button
          type="submit"
          disabled={sending || !input.trim()}
          aria-label={tr('chat.send')}
          className={cx('inline-flex h-[44px] shrink-0 items-center gap-1.5 rounded-xl px-4 text-[13px] font-semibold disabled:opacity-50', btnPrimary(dark))}
        >
          <Send size={14} /> {tr('chat.send')}
        </button>
      </form>
      <p className={cx('mt-2 flex items-center gap-1.5 text-[11px]', t.faint)}>
        <ShieldCheck size={12} className={dark ? 'text-emerald-400' : 'text-emerald-500'} />
        {tr('chat.disclaimer')}
      </p>
    </div>
  );
}
