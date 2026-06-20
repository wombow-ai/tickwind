'use client';

import {ArrowLeft, ArrowRight, Loader2, Lock, Sparkles} from 'lucide-react';
import Link from '@/components/LocalLink';
import {ChatThreadPanel} from '@/components/ChatThreadPanel';
import {useAuth} from '@/lib/auth';
import {useEntitlement} from '@/lib/entitlement';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok, type Tokens} from '@/lib/ui';

/**
 * ChatThread — the per-stock AI chat (Pro-gated), rendered on a stock's Research tab and
 * the /stock/{t}/chat route. It owns the auth/Pro gate + page chrome and delegates the
 * thread (messages + composer) to the shared ChatThreadPanel (also used by the hub).
 */
export function ChatThread({ticker}: {ticker: string}) {
  const norm = ticker.toUpperCase();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {user, loading: authLoading} = useAuth();
  const {isPro, loading: entLoading} = useEntitlement();

  if (authLoading || entLoading) {
    return (
      <Wrap dark={dark} t={t} tr={tr} norm={norm}>
        <div className={cx('flex items-center gap-2 rounded-2xl border p-6 text-[13px]', t.card, t.border, t.sub)}>
          <Loader2 size={15} className="animate-spin" /> {tr('chat.thinking')}
        </div>
      </Wrap>
    );
  }
  if (!user) {
    return (
      <Wrap dark={dark} t={t} tr={tr} norm={norm}>
        <Gate dark={dark} t={t} icon={<Lock size={20} />} title={tr('chat.gate.login.title')} body={tr('chat.gate.login.body')} cta={tr('chat.login')} href="/login" />
      </Wrap>
    );
  }
  if (!isPro) {
    return (
      <Wrap dark={dark} t={t} tr={tr} norm={norm}>
        <Gate dark={dark} t={t} icon={<Sparkles size={20} />} title={tr('chat.gate.pro.title')} body={tr('chat.gate.pro.body').replace('{t}', norm)} cta={tr('chat.gate.cta')} href="/pro" />
      </Wrap>
    );
  }
  return (
    <Wrap dark={dark} t={t} tr={tr} norm={norm}>
      <ChatThreadPanel source={{kind: 'stock', ticker: norm}} />
    </Wrap>
  );
}

function Wrap({dark, t, tr, norm, children}: {dark: boolean; t: Tokens; tr: (k: string) => string; norm: string; children: React.ReactNode}) {
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
        <Link href="/chat" className={cx('mt-1 shrink-0 rounded-full border px-2.5 py-1 text-[11px] font-medium', t.border, t.faint, dark ? 'hover:bg-slate-800/60' : 'hover:bg-slate-50')}>
          {tr('chat.hub.all')}
        </Link>
      </div>
      <p className={cx('mb-4 text-[12.5px]', t.sub)}>{tr('chat.subtitle').replace('{t}', norm)}</p>
      {children}
    </div>
  );
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
