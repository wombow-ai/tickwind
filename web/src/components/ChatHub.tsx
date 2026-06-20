'use client';

import {ArrowRight, Loader2, Lock, Menu, Pencil, Plus, ShieldCheck, Sparkles, Trash2, X} from 'lucide-react';
import {useSearchParams} from 'next/navigation';
import {useCallback, useEffect, useState} from 'react';
import {ChatThreadPanel} from '@/components/ChatThreadPanel';
import Link from '@/components/LocalLink';
import {
  type Conversation,
  createConversation,
  deleteConversation,
  getMyPrefs,
  listConversations,
  putMyPrefs,
  renameConversation,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useEntitlement} from '@/lib/entitlement';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok, type Tokens} from '@/lib/ui';

/**
 * ChatHub — Product C: the unified, ChatGPT/Claude-style chat hub (/chat). A sidebar of
 * the user's conversations + the active thread (the shared ChatThreadPanel). Pro-gated.
 * A ?ticker= query opens (or creates) that stock's conversation as a warm start.
 */
export function ChatHub() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {user, loading: authLoading, getToken} = useAuth();
  const {isPro, loading: entLoading} = useEntitlement();
  const initialTicker = (useSearchParams().get('ticker') || '').toUpperCase();

  const [convs, setConvs] = useState<Conversation[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  // `chat_personal_data` pref (default true): when off, the chat endpoint serves no
  // watchlist/holdings/notes tools for the turn. Mirrors the server-side default.
  const [personalData, setPersonalData] = useState(true);

  const refresh = useCallback(async () => {
    const list = await listConversations(await getToken());
    setConvs(list);
    return list;
  }, [getToken]);

  useEffect(() => {
    if (!user || !isPro) {
      setLoading(false);
      return;
    }
    let active = true;
    (async () => {
      try {
        const token = await getToken();
        let list = await listConversations(token);
        let sel: string | null = list[0]?.id ?? null;
        if (initialTicker) {
          const conv = await createConversation({anchorTicker: initialTicker}, token);
          sel = conv.id;
          list = await listConversations(token);
        }
        let pd = true;
        try {
          const prefs = await getMyPrefs(token);
          if (typeof prefs.chat_personal_data === 'boolean') pd = prefs.chat_personal_data;
        } catch {
          /* default on */
        }
        if (active) {
          setConvs(list);
          setSelectedId(sel);
          setPersonalData(pd);
        }
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [user, isPro, initialTicker, getToken]);

  const newChat = useCallback(async () => {
    const conv = await createConversation({title: tr('chat.hub.newTitle')}, await getToken());
    await refresh();
    setSelectedId(conv.id);
    setSidebarOpen(false);
  }, [getToken, refresh, tr]);

  const remove = useCallback(
    async (id: string) => {
      await deleteConversation(id, await getToken());
      const list = await refresh();
      setSelectedId(prev => (prev === id ? list[0]?.id ?? null : prev));
    },
    [getToken, refresh],
  );

  const togglePersonalData = useCallback(async () => {
    const next = !personalData;
    setPersonalData(next); // optimistic
    try {
      await putMyPrefs(await getToken(), {chat_personal_data: next});
    } catch {
      setPersonalData(!next); // revert on failure
    }
  }, [personalData, getToken]);

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

  if (authLoading || entLoading || loading) {
    return (
      <Center>
        <div className={cx('flex items-center gap-2 text-[13px]', t.sub)}>
          <Loader2 size={15} className="animate-spin" /> {tr('chat.thinking')}
        </div>
      </Center>
    );
  }
  if (!user) {
    return (
      <Center>
        <Gate dark={dark} t={t} icon={<Lock size={20} />} title={tr('chat.gate.login.title')} body={tr('chat.gate.login.body')} cta={tr('chat.login')} href="/login" />
      </Center>
    );
  }
  if (!isPro) {
    return (
      <Center>
        <Gate dark={dark} t={t} icon={<Sparkles size={20} />} title={tr('chat.gate.pro.title')} body={tr('chat.gate.pro.body').replace('{t}', tr('chat.hub.yourPortfolio'))} cta={tr('chat.gate.cta')} href="/pro" />
      </Center>
    );
  }

  const selected = convs.find(c => c.id === selectedId) || null;

  const Sidebar = (
    <div className={cx('flex h-full w-full flex-col rounded-2xl border', t.card, t.border)}>
      <div className="flex items-center justify-between gap-2 p-3">
        <span className={cx('text-[13px] font-bold', t.text)}>{tr('chat.hub.title')}</span>
        <button type="button" onClick={newChat} className={cx('inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-[11px] font-semibold', btnPrimary(dark))}>
          <Plus size={12} /> {tr('chat.hub.new')}
        </button>
      </div>
      <div className="flex-1 space-y-1 overflow-y-auto px-2 pb-2">
        {convs.length === 0 ? (
          <p className={cx('px-2 py-3 text-[12px]', t.faint)}>{tr('chat.hub.empty')}</p>
        ) : (
          convs.map(c => (
            <div
              key={c.id}
              className={cx(
                'group flex items-center gap-1 rounded-lg px-2 py-2 text-[12.5px]',
                c.id === selectedId ? (dark ? 'bg-violet-500/15 text-violet-100' : 'bg-violet-50 text-violet-900') : cx(t.sub, dark ? 'hover:bg-slate-800/50' : 'hover:bg-slate-50'),
              )}
            >
              <button type="button" onClick={() => { setSelectedId(c.id); setSidebarOpen(false); }} className="min-w-0 flex-1 truncate text-left">
                {c.title || c.anchor_ticker || tr('chat.hub.untitled')}
              </button>
              <button type="button" aria-label="rename" onClick={() => rename(c)} className={cx('hidden shrink-0 rounded p-1 group-hover:block', t.faint)}>
                <Pencil size={12} />
              </button>
              <button type="button" aria-label="delete" onClick={() => remove(c.id)} className={cx('hidden shrink-0 rounded p-1 group-hover:block', t.faint, 'hover:text-rose-500')}>
                <Trash2 size={12} />
              </button>
            </div>
          ))
        )}
      </div>
      <div className={cx('border-t p-2.5', t.border)}>
        <button
          type="button"
          onClick={togglePersonalData}
          aria-pressed={personalData}
          className={cx('flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left', dark ? 'hover:bg-slate-800/50' : 'hover:bg-slate-50')}
        >
          <ShieldCheck size={14} className={personalData ? (dark ? 'text-emerald-400' : 'text-emerald-600') : t.faint} />
          <span className={cx('flex-1 text-[12px] font-medium', t.sub)}>{tr('chat.hub.privacy')}</span>
          <span className={cx('relative h-4 w-7 shrink-0 rounded-full transition-colors', personalData ? (dark ? 'bg-emerald-500' : 'bg-emerald-500') : dark ? 'bg-slate-700' : 'bg-slate-300')}>
            <span className={cx('absolute top-0.5 h-3 w-3 rounded-full bg-white transition-all', personalData ? 'left-3.5' : 'left-0.5')} />
          </span>
        </button>
        <p className={cx('mt-1 px-2 text-[10.5px] leading-snug', t.faint)}>{tr(personalData ? 'chat.hub.privacyOn' : 'chat.hub.privacyOff')}</p>
      </div>
    </div>
  );

  return (
    <div className="mx-auto max-w-6xl tw-fade">
      <div className="mb-3 flex items-center gap-2 lg:hidden">
        <button type="button" onClick={() => setSidebarOpen(o => !o)} className={cx('inline-flex items-center gap-1 rounded-full border px-3 py-1.5 text-[12px] font-medium', t.border, t.sub)}>
          {sidebarOpen ? <X size={14} /> : <Menu size={14} />} {tr('chat.hub.title')}
        </button>
      </div>
      <div className="grid gap-4 lg:grid-cols-[260px_1fr]">
        <div className={cx('lg:block', sidebarOpen ? 'block' : 'hidden', 'h-[60vh] lg:h-[72vh]')}>{Sidebar}</div>
        <div className={cx('h-[72vh] rounded-2xl border p-4', t.card, t.border)}>
          {selected ? (
            <ChatThreadPanel source={{kind: 'conversation', id: selected.id, anchorTicker: selected.anchor_ticker}} onActivity={refresh} />
          ) : (
            <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
              <Sparkles size={22} className={dark ? 'text-violet-300' : 'text-violet-500'} />
              <p className={cx('text-[14px] font-semibold', t.text)}>{tr('chat.hub.startTitle')}</p>
              <p className={cx('max-w-md text-[12.5px]', t.sub)}>{tr('chat.hub.startBody')}</p>
              <button type="button" onClick={newChat} className={cx('mt-1 inline-flex items-center gap-1 rounded-full px-4 py-1.5 text-[12.5px] font-semibold', btnPrimary(dark))}>
                <Plus size={13} /> {tr('chat.hub.new')}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function Center({children}: {children: React.ReactNode}) {
  return <div className="mx-auto flex min-h-[50vh] max-w-2xl items-center justify-center tw-fade">{children}</div>;
}

function Gate({dark, t, icon, title, body, cta, href}: {dark: boolean; t: Tokens; icon: React.ReactNode; title: string; body: string; cta: string; href: string}) {
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
