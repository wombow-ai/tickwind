'use client';

import {Plus, Send} from 'lucide-react';
import {useCallback, useEffect, useRef, useState} from 'react';
import {MsgRow, type Msg} from '@/components/chatRender';
import {type ChatResponse, clearChat, createConversation, getChatHistory, getConvHistory, postChat, postConvChatStream} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {chatVars, CHAT_MONO} from '@/lib/chatTheme';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';

// ChatThreadPanel renders one conversation's messages + composer, for BOTH surfaces: a
// per-stock chat ({kind:'stock'}) and the unified hub ({kind:'conversation'}). The caller
// owns the auth/Pro gate + page chrome. onActivity fires after a successful send; onMeter
// reports the monthly meter so the hub header can show it.

export type ChatSource =
  | {kind: 'stock'; ticker: string}
  | {kind: 'conversation'; id: string; anchorTicker?: string}
  // A NEW-chat draft: no conversation exists yet (so the hub lands on suggestions, never an
  // auto-asked question or an old thread). The conversation is created lazily on first send,
  // anchored to anchorTicker when arriving from a stock page (?ticker=).
  | {kind: 'draft'; anchorTicker?: string};

const SUGGESTIONS = ['chat.suggest.valuation', 'chat.suggest.bear', 'chat.suggest.flows'];

// In-memory thread cache (keyed by source). Switching back to an already-loaded conversation
// is INSTANT — no backend round-trip — since the user is the only writer of their own threads.
const threadCache = new Map<string, Msg[]>();

function sourceKey(s: ChatSource): string {
  if (s.kind === 'stock') return 'stock:' + s.ticker.toUpperCase();
  if (s.kind === 'draft') return 'draft:' + (s.anchorTicker ?? '');
  return 'conv:' + s.id;
}

export function ChatThreadPanel({source, onActivity, onMeter, onCreated}: {source: ChatSource; onActivity?: () => void; onMeter?: (m: {used: number; limit: number}) => void; onCreated?: (convId: string) => void}) {
  const dark = useDark();
  const tr = useT();
  const {lang} = useLang();
  const {getToken} = useAuth();

  const [messages, setMessages] = useState<Msg[]>([]);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [streamStarted, setStreamStarted] = useState(false);
  const [err, setErr] = useState(false);
  const [meter, setMeter] = useState<{used: number; limit: number} | null>(null);
  const listRef = useRef<HTMLDivElement>(null);
  // For a draft source: the conversation id created on first send (then reused for the turn).
  const draftConvId = useRef<string | null>(null);

  const fallbackTicker = source.kind === 'stock' ? source.ticker.toUpperCase() : (source.anchorTicker ?? '').toUpperCase();
  const key = sourceKey(source);

  // Load the persisted thread when the source changes — from cache instantly if present,
  // otherwise fetch once and cache it.
  useEffect(() => {
    setMeter(null);
    // A draft has no conversation yet — start empty (suggestions), create on first send.
    if (source.kind === 'draft') {
      draftConvId.current = null;
      setMessages([]);
      return;
    }
    const cached = threadCache.get(key);
    if (cached) {
      setMessages(cached);
      return;
    }
    let active = true;
    setMessages([]);
    const c = new AbortController();
    (async () => {
      try {
        const token = await getToken();
        const h = source.kind === 'stock'
          ? await getChatHistory(source.ticker, token, c.signal)
          : await getConvHistory(source.id, token, c.signal);
        const loaded = h.map(m => ({role: m.role, blocks: m.blocks, text: m.text}));
        threadCache.set(key, loaded);
        if (active) setMessages(loaded);
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
      setStreamStarted(false);
      setInput('');
      setMessages(m => {
        const next = [...m, {role: 'user' as const, text: msg}];
        threadCache.set(key, next);
        return next;
      });
      try {
        const token = await getToken();
        let res: ChatResponse;
        let createdConvId: string | null = null;
        if (source.kind === 'stock') {
          res = await postChat(source.ticker, msg, token, lang);
        } else {
          // conversation OR draft → stream to a conversation. A draft creates the
          // conversation lazily on this first send (anchored to its ticker when present),
          // so the hub never auto-asks a question or auto-opens an old thread.
          let convId: string;
          if (source.kind === 'draft') {
            if (!draftConvId.current) {
              const conv = await createConversation(source.anchorTicker ? {anchorTicker: source.anchorTicker} : {title: tr('chat.hub.newTitle')}, token);
              draftConvId.current = conv.id;
              createdConvId = conv.id;
            }
            convId = draftConvId.current;
          } else {
            convId = source.id;
          }
          // Stream: append tokens to a live assistant message; `done` reconciles with the
          // authoritative advice-filtered blocks (so the anti-hallucination filter wins).
          res = await postConvChatStream(convId, msg, token, lang, tok => {
            setStreamStarted(true);
            setMessages(m => {
              const next = [...m];
              const last = next[next.length - 1];
              if (last && last.role === 'assistant' && last.blocks === undefined) {
                next[next.length - 1] = {...last, text: (last.text ?? '') + tok};
              } else {
                next.push({role: 'assistant', text: tok});
              }
              return next;
            });
          });
        }
        setMessages(m => {
          const next = [...m];
          const last = next[next.length - 1];
          const finalMsg: Msg = {role: 'assistant', blocks: res.blocks};
          if (last && last.role === 'assistant' && last.blocks === undefined) {
            next[next.length - 1] = finalMsg; // replace the streamed placeholder
          } else {
            next.push(finalMsg);
          }
          threadCache.set(key, next);
          // Seed the new conversation's cache so the hub loads it instantly on the switch.
          if (createdConvId) threadCache.set('conv:' + createdConvId, next);
          return next;
        });
        if (res.meter) {
          setMeter(res.meter);
          onMeter?.(res.meter);
        }
        onActivity?.();
        if (createdConvId) onCreated?.(createdConvId); // hub: select the new conversation + refresh
      } catch {
        setErr(true);
        setMessages(m => {
          const next = [...m];
          const last = next[next.length - 1];
          if (last && last.role === 'assistant' && last.blocks === undefined) next.pop();
          return next;
        });
      } finally {
        setSending(false);
        setStreamStarted(false);
      }
    },
    [key, lang, sending, getToken, onActivity, onMeter, onCreated], // eslint-disable-line react-hooks/exhaustive-deps
  );

  const resetStockThread = useCallback(async () => {
    if (source.kind !== 'stock') return;
    try {
      await clearChat(source.ticker, await getToken());
    } catch {
      /* best-effort */
    }
    threadCache.delete(key);
    setMessages([]);
    setMeter(null);
    setErr(false);
  }, [key, getToken]); // eslint-disable-line react-hooks/exhaustive-deps

  const placeholder = source.kind === 'stock' ? tr('chat.placeholder') : tr('chat.hub.placeholder');
  const sendActive = input.trim().length > 0 && !sending;
  const empty = messages.length === 0 && !sending;

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

      {/* Full-width scroll surface — the WHOLE area scrolls; the thread is centered inside. */}
      <div ref={listRef} style={{flex: 1, minHeight: 0, overflowY: 'auto'}}>
        <div style={{maxWidth: 760, margin: '0 auto', padding: '18px 20px', display: 'flex', flexDirection: 'column', gap: 22}}>
          {empty ? (
            <p style={{fontSize: 13, color: 'var(--text2)'}}>{tr('chat.empty')}</p>
          ) : (
            messages.map((m, i) => <MsgRow key={i} m={m} fallbackTicker={fallbackTicker} tr={tr} />)
          )}
          {sending && !streamStarted && <ThinkingRow tr={tr} />}
          {err && <p style={{fontSize: 12.5, color: 'var(--down)'}}>{tr('chat.error')}</p>}
        </div>
      </div>

      {/* Composer footer — full width, content centered, pinned below the scroll. */}
      <div style={{flex: 'none', borderTop: '1px solid var(--border)', padding: '12px 20px 14px'}}>
        <div style={{maxWidth: 760, margin: '0 auto'}}>
          {empty && (
            <div style={{marginBottom: 10, display: 'flex', flexWrap: 'wrap', gap: 8}}>
              {SUGGESTIONS.map(k => (
                <button key={k} type="button" onClick={() => send(tr(k))} style={{borderRadius: 999, border: '1px solid var(--border)', background: 'var(--surface)', padding: '7px 13px', fontSize: 12, fontWeight: 500, color: 'var(--text2)', cursor: 'pointer'}}>
                  {tr(k)}
                </button>
              ))}
            </div>
          )}
          <form onSubmit={e => { e.preventDefault(); send(input); }}>
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
          </form>
          <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, marginTop: 7, flexWrap: 'wrap'}}>
            <span style={{fontSize: 10.5, color: 'var(--text3)'}}>{tr('chat.disclaimer')}</span>
            <span style={{fontSize: 10.5, color: 'var(--text3)', fontFamily: CHAT_MONO}}>{tr('chat.sendHint')}</span>
          </div>
        </div>
      </div>
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
