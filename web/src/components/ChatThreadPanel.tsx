'use client';

import {Plus, Send} from 'lucide-react';
import {useCallback, useEffect, useRef, useState} from 'react';
import {MsgRow, type Msg} from '@/components/chatRender';
import {clearChat, getChatHistory, getConvHistory, postChat, postConvChat} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {chatVars, CHAT_MONO} from '@/lib/chatTheme';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';

// ChatThreadPanel renders one conversation's messages + composer, for BOTH surfaces: a
// per-stock chat ({kind:'stock'}) and the unified hub ({kind:'conversation'}). The caller
// owns the auth/Pro gate + page chrome. onActivity fires after a successful send (so the hub
// can refresh the sidebar order); onMeter reports the monthly meter so the hub header can show
// it. Styled on the chat-hub palette (CSS vars set on this root so it also themes the embed).

export type ChatSource =
  | {kind: 'stock'; ticker: string}
  | {kind: 'conversation'; id: string; anchorTicker?: string};

const SUGGESTIONS = ['chat.suggest.valuation', 'chat.suggest.bear', 'chat.suggest.flows'];

function sourceKey(s: ChatSource): string {
  return s.kind === 'stock' ? 'stock:' + s.ticker.toUpperCase() : 'conv:' + s.id;
}

export function ChatThreadPanel({source, onActivity, onMeter}: {source: ChatSource; onActivity?: () => void; onMeter?: (m: {used: number; limit: number}) => void}) {
  const dark = useDark();
  const tr = useT();
  const {lang} = useLang();
  const {getToken} = useAuth();

  const [messages, setMessages] = useState<Msg[]>([]);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [err, setErr] = useState(false);
  const [meter, setMeter] = useState<{used: number; limit: number} | null>(null);
  const listRef = useRef<HTMLDivElement>(null);

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

  // Auto-scroll the messages CONTAINER to the latest — NOT the page.
  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
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
        if (res.meter) {
          setMeter(res.meter);
          onMeter?.(res.meter);
        }
        onActivity?.();
      } catch {
        setErr(true);
      } finally {
        setSending(false);
      }
    },
    [key, lang, sending, getToken, onActivity, onMeter], // eslint-disable-line react-hooks/exhaustive-deps
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
  const sendActive = input.trim().length > 0 && !sending;

  return (
    <div style={{...chatVars(dark), display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0, color: 'var(--text)'}}>
      {source.kind === 'stock' && (
        <div style={{flex: 'none', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, marginBottom: 10}}>
          {meter ? (
            <span style={{fontSize: 11, fontWeight: 600, fontFamily: CHAT_MONO, color: 'var(--text2)', background: 'var(--surface2)', border: '1px solid var(--border)', padding: '3px 8px', borderRadius: 6}}>
              {tr('chat.meter').replace('{used}', String(meter.used)).replace('{limit}', String(meter.limit))}
            </span>
          ) : <span />}
          {messages.length > 0 && (
            <button type="button" onClick={resetStockThread} style={{display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 11, fontWeight: 500, color: 'var(--text3)', border: '1px solid var(--border)', background: 'transparent', borderRadius: 999, padding: '4px 10px', cursor: 'pointer'}}>
              <Plus size={12} /> {tr('chat.new')}
            </button>
          )}
        </div>
      )}

      <div ref={listRef} style={{flex: 1, minHeight: 0, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 22, padding: '4px 2px'}}>
        {messages.length === 0 && !sending ? (
          <p style={{fontSize: 13, color: 'var(--text2)'}}>{tr('chat.empty')}</p>
        ) : (
          messages.map((m, i) => <MsgRow key={i} m={m} fallbackTicker={fallbackTicker} dark={dark} tr={tr} />)
        )}
        {sending && <ThinkingRow tr={tr} />}
        {err && <p style={{fontSize: 12.5, color: 'var(--down)'}}>{tr('chat.error')}</p>}
      </div>

      {messages.length === 0 && !sending && (
        <div style={{marginTop: 12, display: 'flex', flexWrap: 'wrap', gap: 8}}>
          {SUGGESTIONS.map(k => (
            <button key={k} type="button" onClick={() => send(tr(k))} style={{borderRadius: 999, border: '1px solid var(--border)', background: 'var(--surface)', padding: '7px 13px', fontSize: 12, fontWeight: 500, color: 'var(--text2)', cursor: 'pointer'}}>
              {tr(k)}
            </button>
          ))}
        </div>
      )}

      <form onSubmit={e => { e.preventDefault(); send(input); }} style={{marginTop: 12}}>
        <div style={{display: 'flex', alignItems: 'flex-end', gap: 10, padding: '8px 8px 8px 14px', borderRadius: 15, background: 'var(--surface)', border: '1px solid var(--border2)'}}>
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
            style={{flex: 1, resize: 'none', border: 'none', outline: 'none', background: 'transparent', color: 'var(--text)', fontSize: 14, lineHeight: 1.5, maxHeight: 140, minHeight: 24, padding: '5px 0', fontFamily: 'inherit'}}
          />
          <button
            type="submit"
            disabled={!sendActive}
            aria-label={tr('chat.send')}
            style={{flex: 'none', width: 34, height: 34, borderRadius: 10, display: 'flex', alignItems: 'center', justifyContent: 'center', border: 'none', cursor: sendActive ? 'pointer' : 'default', background: sendActive ? 'var(--accent)' : 'var(--surface2)', color: sendActive ? '#1c1404' : 'var(--text3)'}}
          >
            <Send size={15} />
          </button>
        </div>
        <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, marginTop: 7, flexWrap: 'wrap'}}>
          <span style={{fontSize: 10.5, color: 'var(--text3)'}}>{tr('chat.disclaimer')}</span>
          <span style={{fontSize: 10.5, color: 'var(--text3)', fontFamily: CHAT_MONO}}>{tr('chat.sendHint')}</span>
        </div>
      </form>
    </div>
  );
}

function ThinkingRow({tr}: {tr: (k: string) => string}) {
  return (
    <div style={{display: 'flex', gap: 12}}>
      <div style={{flex: 'none', width: 28, height: 28, borderRadius: 8, background: 'var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontWeight: 700, fontSize: 12, color: '#1c1404'}}>T</div>
      <div style={{display: 'flex', alignItems: 'center', gap: 9}}>
        <span style={{fontSize: 12.5, fontWeight: 600, color: 'var(--text)'}}>{tr('chat.aiName')}</span>
        <span style={{display: 'flex', gap: 3, alignItems: 'center'}}>
          {[0, 0.15, 0.3].map((d, i) => (
            <span key={i} style={{width: 5, height: 5, borderRadius: '50%', background: 'var(--accent)', animation: `tw-chat-pulse 1.2s infinite ${d}s`}} />
          ))}
        </span>
        <span style={{fontSize: 11.5, color: 'var(--text3)'}}>{tr('chat.thinking')}</span>
      </div>
    </div>
  );
}
