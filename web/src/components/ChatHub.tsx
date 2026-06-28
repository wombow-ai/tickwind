'use client';

import {ArrowRight, Lock, Menu, Pencil, Plus, Search, Settings as SettingsIcon, Trash2, X} from 'lucide-react';
import {usePathname, useSearchParams} from 'next/navigation';
import {useCallback, useEffect, useMemo, useState} from 'react';
import {ChatThreadPanel} from '@/components/ChatThreadPanel';
import Link from '@/components/LocalLink';
import {BrandLoader} from '@/components/ui/BrandLoader';
import {LogoMark} from '@/components/ui/atoms';
import {
  type ChatUsage,
  type Conversation,
  deleteConversation,
  getChatUsage,
  listConversations,
  renameConversation,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {chatVars, CHAT_MONO} from '@/lib/chatTheme';
import {useEntitlement} from '@/lib/entitlement';
import {type Lang, useT} from '@/lib/i18n';
import {DEFAULT_LOCALE, isLocale, localizedPath} from '@/lib/locale';
import {useDark} from '@/lib/theme';
import {cx} from '@/lib/ui';

/**
 * ChatHub — the full-screen, ChatGPT/Claude-style AI chat hub (/chat), in the owner's warm
 * "Claude style" design. A sidebar of the user's conversations + the active thread (the shared
 * ChatThreadPanel). Signed-in users only (free users get a small monthly taste; Pro is
 * unlimited + shows the quota bar). A ?ticker= query opens (or creates) that stock's conversation.
 * Lives in the (fullscreen) route group so it has no TopNav/Footer — it feels like its own app.
 * The sidebar brand links home (back to the main site); the "Use my data" privacy control lives
 * in /settings (reached via the sidebar Settings gear), not in the chat chrome.
 */
export function ChatHub() {
  const dark = useDark();
  const tr = useT();
  const {user, loading: authLoading, getToken} = useAuth();
  const {isPro, loading: entLoading} = useEntitlement();
  const sp = useSearchParams();
  const pathname = usePathname();
  const locale: Lang = (() => {
    const seg = pathname.slice(1).split('/', 1)[0];
    return isLocale(seg) ? seg : DEFAULT_LOCALE;
  })();
  // The open conversation lives in the URL (?c=<id>) so leaving + returning (or a hard reload)
  // restores it — NOT just React state, which a remount wipes. ?ticker= is the per-stock
  // warm-start (?c= wins when both are present). These are read REACTIVELY (not frozen at first
  // render): useSearchParams() is empty during the SSR/pre-hydration pass inside the Suspense
  // boundary, so a once-captured value would miss the real ?c= and strand the user on a draft.
  const cParam = sp.get('c') || '';
  const tickerParam = (sp.get('ticker') || '').toUpperCase();

  // writeUrl soft-syncs the URL to the open conversation WITHOUT remounting ChatHub (same
  // /chat pathname, query-only change → Next does a client-side update, so selectedId state,
  // the thread cache, and the draft ref all survive). 'replace' for draft-adoption/deletion
  // (one continuous view — no history entry); 'push' for an explicit selection (Back reverses).
  const writeUrl = useCallback(
    (convId: string | null, mode: 'push' | 'replace') => {
      const href = localizedPath(locale, convId ? `/chat?c=${convId}` : '/chat');
      if (typeof window === 'undefined') return;
      // Use the NATIVE History API, NOT router.push/replace: a router navigation refetches the
      // route's RSC payload, which flashes/"refreshes" the page after an answer (a draft adopting
      // its new conversation) AND churns the sidebar so the optimistic conversation list is lost.
      // Next syncs useSearchParams/usePathname with pushState/replaceState, so ?c= restore +
      // back/forward still work, with NO refetch.
      if (mode === 'push') window.history.pushState(null, '', href);
      else window.history.replaceState(null, '', href);
    },
    [locale],
  );

  const [convs, setConvs] = useState<Conversation[]>([]);
  const [convsFetched, setConvsFetched] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [meter, setMeter] = useState<ChatUsage | null>(null);
  // The per-turn meter (from the stream's done event) carries only {used,limit}; merge it so the
  // window reset date + Pro subscription dates (fetched once on load) survive across messages.
  const mergeMeter = useCallback(
    (m: {used: number; limit: number}) => setMeter((prev) => (prev ? {...prev, ...m} : m)),
    [],
  );
  // selectedId === null → a NEW-chat DRAFT (the default landing). draftAnchor anchors it to a
  // stock when arriving from a stock page (?ticker=); the conversation is created on first send.
  const [draftAnchor, setDraftAnchor] = useState('');

  const refresh = useCallback(async () => {
    const list = await listConversations(await getToken());
    setConvs(list);
    return list;
  }, [getToken]);

  // Heavy fetch — conversations + usage, ONCE when auth resolves (NOT on ?c= changes; those are
  // handled by the cheap restore effect below, so a selection never re-fetches the list).
  useEffect(() => {
    if (!user) {
      setLoading(false);
      return;
    }
    let active = true;
    (async () => {
      try {
        const token = await getToken();
        const list = await listConversations(token);
        // Quota is tier-aware server-side (Pro → monthly bucket for the meter; free → weekly
        // bucket for the low-quota nudge), so fetch it for both tiers on load.
        let usage: ChatUsage | null = null;
        try {
          usage = await getChatUsage(token);
        } catch {
          /* the per-turn meter will fill it in */
        }
        if (active) {
          setConvs(list);
          if (usage) setMeter(usage);
          setConvsFetched(true);
          // NB: loading stays true here — the restore effect clears it AFTER it sets the
          // initial selection, so the hub never flashes a draft before the URL's ?c= resolves.
        }
      } catch {
        if (active) setLoading(false); // error → drop the loader, show the (empty) hub
      }
    })();
    return () => {
      active = false;
    };
  }, [user, isPro, getToken]);

  // Restore the open conversation FROM THE URL (?c=) — reactively, so a fresh mount, a hard
  // reload, and back/forward all reopen the right thread. Gated on convsFetched so the
  // stale-id cleanup only fires AFTER a real fetch confirms the id isn't owned (else a
  // pre-fetch [] would wrongly drop a valid ?c=). Sets state only — never re-fetches.
  useEffect(() => {
    if (!convsFetched) return;
    if (cParam) {
      if (convs.some(c => c.id === cParam)) {
        setSelectedId(cParam);
        setDraftAnchor('');
      } else {
        // Unknown / not-owned / deleted id → land on a draft and drop the broken ?c=.
        setSelectedId(null);
        setDraftAnchor('');
        writeUrl(null, 'replace');
      }
    } else {
      // No ?c= → NEW-chat draft (anchored to ?ticker= when arriving from a stock page).
      setSelectedId(null);
      setDraftAnchor(tickerParam);
    }
    setLoading(false); // selection resolved → safe to render the hub (no draft flash)
  }, [cParam, tickerParam, convs, convsFetched, writeUrl]);

  // New chat → a fresh GENERAL draft (no conversation created until the first message). Push
  // (clears ?c=) so Back returns to the conversation you left.
  const newChat = useCallback(() => {
    setSelectedId(null);
    setDraftAnchor('');
    setQuery('');
    setSidebarOpen(false);
    writeUrl(null, 'push');
  }, [writeUrl]);

  // Select an existing conversation from the sidebar → push (a deliberate navigation; Back reverses).
  const openConv = useCallback(
    (id: string) => {
      setSelectedId(id);
      setSidebarOpen(false);
      writeUrl(id, 'push');
    },
    [writeUrl],
  );

  // The draft created its conversation on first send → adopt it + refresh the sidebar. Replace
  // (NOT push): the draft and its new conversation are one continuous view, so Back must not
  // land on the same conversation rendered as an empty draft. Also drops a consumed ?ticker=.
  const onDraftCreated = useCallback(
    async (convId: string) => {
      await refresh();
      setSelectedId(convId);
      setDraftAnchor('');
      writeUrl(convId, 'replace');
    },
    [refresh, writeUrl],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteConversation(id, await getToken());
      const list = await refresh();
      setSelectedId(prev => {
        if (prev !== id) return prev;
        // The open conversation was deleted → fall to the next one (or a draft) and mirror
        // the URL (replace — a deletion shouldn't add a history entry).
        const next = list[0]?.id ?? null;
        writeUrl(next, 'replace');
        return next;
      });
    },
    [getToken, refresh, writeUrl],
  );


  const rename = useCallback(
    async (c: Conversation) => {
      const next = typeof window !== 'undefined' ? window.prompt(tr('chat.hub.renamePrompt'), c.title) : null;
      if (next && next.trim()) {
        await renameConversation(c.id, next.trim(), await getToken());
        await refresh();
      }
    },
    [getToken, refresh, tr],
  );

  // TODAY vs EARLIER grouping by updated_at, after the search filter.
  const {today, earlier} = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = q ? convs.filter(c => (c.title || c.anchor_ticker || '').toLowerCase().includes(q)) : convs;
    const isToday = (iso?: string) => {
      if (!iso) return false;
      const d = new Date(iso);
      const n = new Date();
      return d.getFullYear() === n.getFullYear() && d.getMonth() === n.getMonth() && d.getDate() === n.getDate();
    };
    return {today: list.filter(c => isToday(c.updated_at)), earlier: list.filter(c => !isToday(c.updated_at))};
  }, [convs, query]);

  if (authLoading || entLoading || loading) {
    return <Shell dark={dark}><Center><BrandLoader size={60} accent="var(--accent)" color="var(--text2)" label={tr('chat.thinking')} /></Center></Shell>;
  }
  // Anonymous → must sign in. Signed-in FREE users may chat on a small monthly taste (the
  // backend enforces the quota + nudges upgrade when used up), so there's no Pro gate here.
  if (!user) {
    return <Shell dark={dark}><Center><Gate icon={<Lock size={20} />} title={tr('chat.gate.login.title')} body={tr('chat.gate.login.body')} cta={tr('chat.login')} href="/login" /></Center></Shell>;
  }

  const selected = convs.find(c => c.id === selectedId) || null;
  const activeTitle = selected ? (selected.title || selected.anchor_ticker || tr('chat.hub.untitled')) : (draftAnchor || tr('chat.hub.newTitle'));
  const activeSub = selected?.anchor_ticker || '';

  return (
    <Shell dark={dark}>
      <div className="relative flex flex-1 min-h-0 overflow-hidden">
        {sidebarOpen && <div onClick={() => setSidebarOpen(false)} className="absolute inset-0 z-30 lg:hidden" style={{background: 'rgba(0,0,0,.45)'}} />}

        {/* SIDEBAR */}
        <aside
          className={cx('absolute inset-y-0 left-0 z-40 flex w-[284px] flex-col transition-transform lg:static lg:w-[272px] lg:translate-x-0', sidebarOpen ? 'translate-x-0' : '-translate-x-[110%] lg:translate-x-0')}
          style={{background: 'var(--surface)', borderRight: '1px solid var(--border)'}}
        >
          <div style={{padding: '14px 14px 10px'}}>
            <div style={{display: 'flex', alignItems: 'center', gap: 9, marginBottom: 14}}>
              {/* Brand → back to the main Tickwind site (the chat is a chrome-free full-screen app). */}
              <Link href="/" title={tr('chat.hub.home')} style={{display: 'flex', alignItems: 'center', gap: 9, textDecoration: 'none', minWidth: 0}}>
                <LogoMark size={26} accent="var(--accent)" />
                <div style={{display: 'flex', alignItems: 'baseline', gap: 7}}>
                  <span style={{fontWeight: 600, fontSize: 15, letterSpacing: '-.01em', color: 'var(--text)'}}>Tickwind</span>
                  <span style={{fontSize: 9, fontWeight: 600, letterSpacing: '.1em', padding: '2px 5px', borderRadius: 5, background: 'var(--accent-soft)', color: 'var(--accent2)', fontFamily: CHAT_MONO}}>AI</span>
                </div>
              </Link>
              <button type="button" onClick={() => setSidebarOpen(false)} className="lg:hidden" style={{marginLeft: 'auto', color: 'var(--text3)', background: 'transparent', border: 'none', cursor: 'pointer'}}><X size={16} /></button>
            </div>
            <button type="button" onClick={newChat} style={{width: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8, padding: 9, borderRadius: 10, background: 'var(--accent-fill)', border: '1px solid var(--accent-line)', color: 'var(--accent2)', fontWeight: 600, fontSize: 13, cursor: 'pointer'}}>
              <Plus size={15} /> {tr('chat.hub.new')}
            </button>
          </div>

          <div style={{padding: '0 14px 8px'}}>
            <div style={{display: 'flex', alignItems: 'center', gap: 8, padding: '7px 10px', borderRadius: 9, background: 'var(--surface2)', border: '1px solid var(--border)'}}>
              <Search size={13} style={{color: 'var(--text3)'}} />
              <input
                value={query}
                onChange={e => setQuery(e.target.value)}
                placeholder={tr('chat.hub.search')}
                style={{flex: 1, border: 'none', outline: 'none', background: 'transparent', fontSize: 12.5, color: 'var(--text)', fontFamily: 'inherit'}}
              />
            </div>
          </div>

          <div style={{flex: 1, minHeight: 0, overflowY: 'auto', padding: '4px 10px'}}>
            {convs.length === 0 ? (
              <p style={{padding: '12px 8px', fontSize: 12, color: 'var(--text3)'}}>{tr('chat.hub.empty')}</p>
            ) : (
              <>
                {today.length > 0 && <Group label={tr('chat.hub.today')} />}
                {today.map(c => <ConvRow key={c.id} c={c} active={c.id === selectedId} onSelect={() => openConv(c.id)} onRename={() => rename(c)} onDelete={() => remove(c.id)} tr={tr} />)}
                {earlier.length > 0 && <Group label={tr('chat.hub.earlier')} />}
                {earlier.map(c => <ConvRow key={c.id} c={c} active={c.id === selectedId} onSelect={() => openConv(c.id)} onRename={() => rename(c)} onDelete={() => remove(c.id)} tr={tr} />)}
              </>
            )}
          </div>

          <div style={{flex: 'none', borderTop: '1px solid var(--border)', padding: '12px 14px', display: 'flex', flexDirection: 'column', gap: 12}}>
            {/* AI-quota bar is Pro-only — free users chat on a small hidden taste. */}
            {isPro && meter && (() => {
              const pct = Math.min(100, Math.round((meter.used / Math.max(1, meter.limit)) * 100));
              const fmtDate = (iso?: string) =>
                iso ? new Date(iso).toLocaleDateString(locale === 'zh' ? 'zh-CN' : 'en-US', {month: 'short', day: 'numeric'}) : '';
              const ending = !!meter.cancel_at_period_end && !!meter.subscription_end;
              return (
                <div style={{display: 'flex', flexDirection: 'column', gap: 6}}>
                  <div title={`${meter.used} / ${meter.limit}`}>
                    <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 6}}>
                      <span style={{fontSize: 11.5, color: 'var(--text2)'}}>{tr('chat.hub.usage')}</span>
                      <span style={{fontSize: 11.5, fontFamily: CHAT_MONO, color: pct >= 100 ? 'var(--down)' : 'var(--text)', fontWeight: 500}}>{tr('chat.hub.usedPct').replace('{p}', String(pct))}</span>
                    </div>
                    <div style={{height: 5, borderRadius: 4, background: 'var(--surface2)', overflow: 'hidden'}}>
                      <div style={{height: '100%', width: `${pct}%`, borderRadius: 4, background: pct >= 100 ? 'var(--down)' : 'var(--accent)'}} />
                    </div>
                  </div>
                  {meter.reset_at && (
                    <span style={{fontSize: 10.5, color: 'var(--text3)'}}>{tr('chat.hub.resets').replace('{d}', fmtDate(meter.reset_at))}</span>
                  )}
                  {ending ? (
                    <Link href={localizedPath(locale, '/settings#subscription')} onClick={() => setSidebarOpen(false)} style={{fontSize: 10.5, lineHeight: 1.4, color: 'var(--accent2)', textDecoration: 'none', fontWeight: 600}}>
                      {tr('chat.hub.proEnding').replace('{d}', fmtDate(meter.subscription_end))}
                    </Link>
                  ) : meter.subscription_end ? (
                    <span style={{fontSize: 10.5, color: 'var(--text3)'}}>{tr('chat.hub.proRenews').replace('{d}', fmtDate(meter.subscription_end))}</span>
                  ) : null}
                </div>
              );
            })()}
            {/* Free taste: a gentle heads-up only when actually running low — not a meter
                from msg 1 (that would invite consumption + read as stingy). */}
            {!isPro && meter && meter.used / Math.max(1, meter.limit) >= 0.75 && (
              <Link href="/pro" onClick={() => setSidebarOpen(false)} style={{display: 'block', fontSize: 11.5, lineHeight: 1.45, color: 'var(--accent2)', textDecoration: 'none', padding: '8px 10px', borderRadius: 8, background: 'var(--surface2)', border: '1px solid var(--border)'}}>
                {tr('chat.hub.lowQuota')}
              </Link>
            )}
            <div style={{display: 'flex', alignItems: 'center', gap: 9}}>
              <div style={{width: 28, height: 28, borderRadius: '50%', background: 'var(--accent)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 11, fontWeight: 600, color: '#1c1404'}}>{(user.email ?? 'TW').slice(0, 2).toUpperCase()}</div>
              <div style={{flex: 1, minWidth: 0}}>
                <div style={{fontSize: 12.5, fontWeight: 500, color: 'var(--text)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis'}}>{user.email}</div>
                {isPro ? (
                  <div style={{fontSize: 10.5, color: 'var(--text3)'}}>{tr('settings.planPro')}</div>
                ) : (
                  <Link href="/pro" onClick={() => setSidebarOpen(false)} style={{fontSize: 10.5, fontWeight: 600, color: 'var(--accent2)', textDecoration: 'none'}}>{tr('settings.upgrade')} →</Link>
                )}
              </div>
              <Link href="/settings" aria-label={tr('nav.settings')} onClick={() => setSidebarOpen(false)} style={{flex: 'none', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 5, borderRadius: 7, color: 'var(--text3)'}}>
                <SettingsIcon size={16} />
              </Link>
            </div>
          </div>
        </aside>

        {/* MAIN */}
        <div className="flex flex-1 min-w-0 flex-col" style={{background: 'var(--bg)'}}>
          <div style={{flex: 'none', display: 'flex', alignItems: 'center', gap: 10, padding: '12px 18px', borderBottom: '1px solid var(--border)'}}>
            <button type="button" onClick={() => setSidebarOpen(true)} className="flex items-center justify-center lg:hidden" style={{width: 32, height: 32, borderRadius: 8, color: 'var(--text2)', background: 'transparent', border: 'none', cursor: 'pointer'}}><Menu size={16} /></button>
            <div style={{flex: 1, minWidth: 0}}>
              <div style={{fontSize: 14, fontWeight: 600, color: 'var(--text)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis'}}>{activeTitle}</div>
              {activeSub && <div style={{fontSize: 11, color: 'var(--text3)'}}>{activeSub}</div>}
            </div>
          </div>

          <div style={{flex: 1, minHeight: 0, overflow: 'hidden'}}>
            <div style={{height: '100%'}}>
              {selected ? (
                <ChatThreadPanel source={{kind: 'conversation', id: selected.id, anchorTicker: selected.anchor_ticker}} onActivity={refresh} onMeter={mergeMeter}/>
              ) : (
                // NEW-chat draft: shows the suggestion chips + composer (no auto-asked question,
                // no auto-opened thread); the conversation is created on the first send.
                <ChatThreadPanel source={{kind: 'draft', anchorTicker: draftAnchor || undefined}} onMeter={mergeMeter} onCreated={onDraftCreated} />
              )}
            </div>
          </div>
        </div>
      </div>
    </Shell>
  );
}

function Shell({dark, children}: {dark: boolean; children: React.ReactNode}) {
  return (
    <div style={{...chatVars(dark), height: '100%', display: 'flex', flexDirection: 'column', background: 'var(--bg)', color: 'var(--text)', fontFamily: 'Inter,system-ui,sans-serif', overflow: 'hidden'}}>
      {children}
    </div>
  );
}

function Center({children}: {children: React.ReactNode}) {
  return <div style={{flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24}}>{children}</div>;
}

function Group({label}: {label: string}) {
  return <div style={{fontSize: 10.5, fontWeight: 600, letterSpacing: '.08em', color: 'var(--text3)', padding: '10px 6px 5px', fontFamily: CHAT_MONO}}>{label}</div>;
}

function ConvRow({c, active, onSelect, onRename, onDelete, tr}: {c: Conversation; active: boolean; onSelect: () => void; onRename: () => void; onDelete: () => void; tr: (k: string) => string}) {
  return (
    <div className="group" style={{display: 'flex', alignItems: 'center', gap: 6, padding: '8px 10px', borderRadius: 10, cursor: 'pointer', border: '1px solid transparent', marginBottom: 2, background: active ? 'var(--surface2)' : 'transparent', borderColor: active ? 'var(--border)' : 'transparent'}}>
      <button type="button" onClick={onSelect} style={{flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 2, textAlign: 'left', border: 'none', background: 'transparent', cursor: 'pointer', padding: 0}}>
        <div style={{display: 'flex', alignItems: 'center', gap: 7, minWidth: 0}}>
          {active && <div style={{width: 3, height: 14, borderRadius: 3, background: 'var(--accent)', flex: 'none'}} />}
          <span style={{fontSize: 13, fontWeight: 500, color: 'var(--text)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis'}}>{c.title || c.anchor_ticker || tr('chat.hub.untitled')}</span>
        </div>
        {c.anchor_ticker && <span style={{fontSize: 11, color: 'var(--text3)', paddingLeft: active ? 10 : 0}}>{c.anchor_ticker}</span>}
      </button>
      <button type="button" aria-label={tr('chat.hub.rename')} onClick={onRename} className="hidden shrink-0 group-hover:block" style={{borderRadius: 6, padding: 4, color: 'var(--text3)', border: 'none', background: 'transparent', cursor: 'pointer'}}><Pencil size={12} /></button>
      <button type="button" aria-label={tr('chat.hub.delete')} onClick={onDelete} className="hidden shrink-0 group-hover:block" style={{borderRadius: 6, padding: 4, color: 'var(--text3)', border: 'none', background: 'transparent', cursor: 'pointer'}}><Trash2 size={12} /></button>
    </div>
  );
}

function Gate({icon, title, body, cta, href}: {icon: React.ReactNode; title: string; body: string; cta: string; href: string}) {
  return (
    <div style={{borderRadius: 16, border: '1px solid var(--border)', background: 'var(--surface)', padding: 28, textAlign: 'center', maxWidth: 420}}>
      <div style={{margin: '0 auto 10px', width: 44, height: 44, borderRadius: 12, background: 'var(--accent-soft)', color: 'var(--accent2)', display: 'flex', alignItems: 'center', justifyContent: 'center'}}>{icon}</div>
      <p style={{fontSize: 16, fontWeight: 700, color: 'var(--text)', margin: 0}}>{title}</p>
      <p style={{margin: '8px auto 0', maxWidth: 360, fontSize: 13, color: 'var(--text2)', lineHeight: 1.55}}>{body}</p>
      <Link href={href} style={{marginTop: 16, display: 'inline-flex', alignItems: 'center', gap: 6, padding: '9px 18px', borderRadius: 11, background: 'var(--accent)', color: '#1c1404', fontWeight: 600, fontSize: 13.5, textDecoration: 'none'}}>
        {cta} <ArrowRight size={14} />
      </Link>
    </div>
  );
}
