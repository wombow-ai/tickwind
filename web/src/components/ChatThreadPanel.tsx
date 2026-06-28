'use client';

import {Activity, BarChart3, Eye, type LucideIcon, Plus, Scale, Send, TrendingDown, TrendingUp, Wallet} from 'lucide-react';
import {Fragment, useCallback, useEffect, useRef, useState} from 'react';
import {ExecChain, type ExecStep, MsgRow, type Msg} from '@/components/chatRender';
import {BrandLoader} from '@/components/ui/BrandLoader';
import {LogoMark} from '@/components/ui/atoms';
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

// Starter prompts for the empty/new-chat welcome. Two contextual sets: STOCK_SUGGESTIONS
// when the chat is anchored to a ticker (per-stock thread or a ?ticker= draft), HUB_SUGGESTIONS
// for the general hub draft. Each is one clickable card that sends the prompt (like the old chips).
const STOCK_SUGGESTIONS: {key: string; icon: LucideIcon}[] = [
  {key: 'chat.suggest.valuation', icon: Scale},
  {key: 'chat.suggest.bear', icon: TrendingDown},
  {key: 'chat.suggest.fundamentals', icon: BarChart3},
  {key: 'chat.suggest.flows', icon: Activity},
];
const HUB_SUGGESTIONS: {key: string; icon: LucideIcon}[] = [
  {key: 'chat.suggest.h.compare', icon: Scale},
  {key: 'chat.suggest.h.moving', icon: Activity},
  {key: 'chat.suggest.h.pnl', icon: Wallet},
  {key: 'chat.suggest.h.bull', icon: TrendingUp},
  {key: 'chat.suggest.h.flows', icon: BarChart3},
  {key: 'chat.suggest.h.watchlist', icon: Eye},
];

// greetingKey returns a time-of-day greeting i18n key (local hour; client-only, so no SSR
// mismatch — the panel mounts after the hub's auth gate resolves).
function greetingKey(): string {
  const h = new Date().getHours();
  if (h < 12) return 'chat.welcome.morning';
  if (h < 18) return 'chat.welcome.afternoon';
  return 'chat.welcome.evening';
}

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
  // True while fetching an EXISTING (uncached) conversation's history, so the panel shows a
  // skeleton instead of the new-chat welcome — otherwise switching to a history thread flashes
  // the "New Chat" screen until the messages arrive.
  const [threadLoading, setThreadLoading] = useState(false);
  const [streamStarted, setStreamStarted] = useState(false);
  const [steps, setSteps] = useState<ExecStep[]>([]); // live ReAct execution chain (ephemeral)
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
      setThreadLoading(false);
      setMessages([]);
      return;
    }
    const cached = threadCache.get(key);
    if (cached) {
      setThreadLoading(false);
      setMessages(cached);
      return;
    }
    let active = true;
    // Show the skeleton (not the welcome screen) until the history resolves.
    setThreadLoading(true);
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
      } finally {
        if (active) setThreadLoading(false);
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
  }, [messages, sending, steps]);

  const send = useCallback(
    async (raw: string) => {
      const msg = raw.trim();
      if (!msg || sending) return;
      setErr(false);
      setSending(true);
      setStreamStarted(false);
      setSteps([]);
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
          const onTok = (tok: string) => {
            setStreamStarted(true);
            setMessages(m => {
              const next = [...m];
              const last = next[next.length - 1];
              // Accumulate the live tokens into a single text BLOCK (not the `text` field) so the
              // streaming render and the final render use the SAME path (BlockView → Markdown) —
              // `done` then updates that block in place instead of remounting the prose (no flash).
              if (last && last.role === 'assistant' && last.streaming) {
                const prev = last.blocks?.[0]?.text ?? '';
                next[next.length - 1] = {role: 'assistant', streaming: true, blocks: [{kind: 'text', text: prev + tok}]};
              } else {
                next.push({role: 'assistant', streaming: true, blocks: [{kind: 'text', text: tok}]});
              }
              return next;
            });
          };
          const popStreaming = () =>
            setMessages(m => {
              const next = [...m];
              const last = next[next.length - 1];
              if (last && last.role === 'assistant' && last.streaming) next.pop();
              return next;
            });
          // Each tool action streams in as a gray execution-chain step (Go-owned label).
          const onStep = (st: ExecStep) => setSteps(s => [...s, st]);
          try {
            res = await postConvChatStream(convId, msg, token, lang, onTok, undefined, onStep);
          } catch {
            // A dropped stream (e.g. a Cloudflare tunnel idle-cut) cancels the server turn before
            // it persists, so ONE retry on the same conversation is safe — and now that the server
            // heartbeats to keep the connection alive, it almost always succeeds. Clear the partial
            // streamed placeholder + the partial chain first so the retry starts clean.
            popStreaming();
            setStreamStarted(false);
            setSteps([]);
            res = await postConvChatStream(convId, msg, token, lang, onTok, undefined, onStep);
          }
        }
        setMessages(m => {
          const next = [...m];
          const last = next[next.length - 1];
          const finalMsg: Msg = {role: 'assistant', blocks: res.blocks};
          if (last && last.role === 'assistant' && last.streaming) {
            next[next.length - 1] = finalMsg; // reconcile the streamed placeholder with the final blocks
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
          if (last && last.role === 'assistant' && last.streaming) next.pop();
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
  // The welcome screen owns the space only when the thread is genuinely empty — NOT while a
  // history fetch is still in flight (that window shows the skeleton instead).
  const empty = messages.length === 0 && !sending && !threadLoading;
  const anchored = fallbackTicker !== '';

  // The composer (textarea + send + disclaimer row). Rendered in the centered welcome when
  // the thread is empty, or pinned in the footer once a conversation is underway.
  const composer = (
    <>
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
    </>
  );

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

      {/* Scroll surface. When the thread is EMPTY, a centered Claude-style welcome (greeting +
          suggestion cards + composer) owns the space; once a message exists it becomes the
          normal top-anchored thread and the composer drops to the pinned footer. */}
      <div ref={listRef} style={{flex: 1, minHeight: 0, overflowY: 'auto', display: empty ? 'flex' : 'block'}}>
        {empty ? (
          <WelcomeScreen anchored={anchored} ticker={fallbackTicker} tr={tr} onPick={send} composer={composer} />
        ) : threadLoading && messages.length === 0 ? (
          // Loading a history thread → the brand loader (gold, to match the chat theme),
          // centered in the thread area. Coherent with the route + chat-init loaders.
          <div style={{display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', minHeight: 240}}>
            <BrandLoader size={46} accent="var(--accent)" color="var(--text2)" label={tr('chat.thinking')} />
          </div>
        ) : (
          <div style={{maxWidth: 760, margin: '0 auto', padding: '18px 20px', display: 'flex', flexDirection: 'column', gap: 22}}>
            {messages.map((m, i) => {
              // Once the answer is streaming, keep the (now all-done) execution chain visible
              // ABOVE the streaming message — so a preamble token can't kill it and the user sees
              // what was looked up. running=false → no pulse, it defers to the streaming caret.
              const chainAbove = m.streaming && i === messages.length - 1 && steps.length > 0;
              return (
                <Fragment key={i}>
                  {chainAbove && <ExecChain steps={steps} running={false} />}
                  <MsgRow m={m} fallbackTicker={fallbackTicker} tr={tr} />
                </Fragment>
              );
            })}
            {/* Before the answer starts streaming: the live chain (or the thinking dots). */}
            {sending && !streamStarted && (steps.length > 0 ? <ExecChain steps={steps} /> : <ThinkingRow tr={tr} />)}
            {err && <p style={{fontSize: 12.5, color: 'var(--down)'}}>{tr('chat.error')}</p>}
          </div>
        )}
      </div>

      {/* Composer footer — pinned below the scroll, only while a conversation is underway
          (when empty, the composer lives inside the centered welcome instead). */}
      {!empty && (
        <div style={{flex: 'none', borderTop: '1px solid var(--border)', padding: '12px 20px 14px'}}>
          <div style={{maxWidth: 760, margin: '0 auto'}}>{composer}</div>
        </div>
      )}
    </div>
  );
}

// WelcomeScreen is the centered new-chat state: a time-aware greeting, a short subline, a
// grid of one-click starter prompts (stock-specific when anchored, hub-wide otherwise), and
// the composer. Modeled on Claude/ChatGPT/Gemini — the greeting owns the empty space and the
// cards give grounded, on-brand starting points instead of a barren canvas.
function WelcomeScreen({anchored, ticker, tr, onPick, composer}: {anchored: boolean; ticker: string; tr: (k: string) => string; onPick: (q: string) => void; composer: React.ReactNode}) {
  const suggestions = anchored ? STOCK_SUGGESTIONS : HUB_SUGGESTIONS;
  const sub = (anchored ? tr('chat.welcome.subStock') : tr('chat.welcome.sub')).replace('{t}', ticker);
  return (
    <div style={{margin: 'auto', width: '100%', maxWidth: 680, padding: '32px 24px', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 0}}>
      <div style={{marginBottom: 14}}><LogoMark size={48} accent="var(--accent)" /></div>
      <h2 style={{fontSize: 27, fontWeight: 600, letterSpacing: '-.01em', color: 'var(--text)', textAlign: 'center', margin: 0}}>{tr(greetingKey())}</h2>
      <p style={{fontSize: 14, color: 'var(--text2)', textAlign: 'center', margin: '8px 0 0', maxWidth: 460, lineHeight: 1.5}}>{sub}</p>
      <div className="grid grid-cols-1 sm:grid-cols-2" style={{gap: 10, width: '100%', marginTop: 26}}>
        {suggestions.map(({key, icon: Icon}) => {
          const text = tr(key).replace('{t}', ticker);
          return (
            <button
              key={key}
              type="button"
              onClick={() => onPick(text)}
              className="tw-chat-suggest"
              style={{display: 'flex', alignItems: 'center', gap: 10, textAlign: 'left', padding: '12px 14px', borderRadius: 12, background: 'var(--surface)', border: '1px solid var(--border)', cursor: 'pointer'}}
            >
              <Icon size={15} style={{flex: 'none', color: 'var(--accent2)'}} />
              <span style={{fontSize: 13, fontWeight: 500, color: 'var(--text)', lineHeight: 1.35}}>{text}</span>
            </button>
          );
        })}
      </div>
      <div style={{width: '100%', marginTop: 22}}>{composer}</div>
    </div>
  );
}

function ThinkingRow({tr}: {tr: (k: string) => string}) {
  return (
    <div style={{display: 'flex', gap: 12}}>
      <div style={{flex: 'none', width: 28, height: 28, borderRadius: 8, background: 'var(--surface2)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
        <LogoMark size={18} accent="var(--accent)" />
      </div>
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
