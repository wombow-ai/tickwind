'use client';

import {ArrowRight, Crown, Loader2, LogOut, Moon, Sun} from 'lucide-react';
import {useState} from 'react';
import Link from '@/components/LocalLink';
import {createPortal} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useEntitlement} from '@/lib/entitlement';
import {useT} from '@/lib/i18n';
import {useTheme} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

/** Account + appearance settings (requires sign-in). */
export default function SettingsPage() {
  const {user, loading, signOut, getToken} = useAuth();
  const {dark, toggle} = useTheme();
  const t = tok(dark);
  const tr = useT();
  const {isPro, loading: entLoading} = useEntitlement();
  const [portalBusy, setPortalBusy] = useState(false);

  const onManage = async () => {
    setPortalBusy(true);
    try {
      const token = await getToken();
      window.location.assign(await createPortal(token));
    } catch {
      setPortalBusy(false);
    }
  };

  if (loading) {
    return <div className={cx('h-40 rounded-3xl border', t.card, t.border, t.skel)} />;
  }

  if (!user) {
    return (
      <div
        className={cx(
          'mx-auto max-w-md rounded-3xl border p-8 text-center',
          t.card,
          t.border,
          t.soft,
        )}
      >
        <h1 className={cx('text-[18px] font-bold', t.text)}>{tr('settings.signInTitle')}</h1>
        <p className={cx('mt-1.5 text-[13.5px]', t.sub)}>{tr('settings.signInSub')}</p>
        <Link
          href="/login"
          className={cx(
            'mt-5 inline-flex rounded-full px-4 py-2 text-[13px] font-semibold',
            btnPrimary(dark),
          )}
        >
          {tr('nav.login')}
        </Link>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-xl">
      <h1 className={cx('mb-6 text-[26px] font-bold tracking-tight', t.text)}>
        {tr('nav.settings')}
      </h1>

      <section className={cx('rounded-3xl border p-6', t.card, t.border, t.soft)}>
        <h2 className={cx('text-[13px] font-semibold uppercase tracking-wide', t.faint)}>
          {tr('footer.account')}
        </h2>
        <div className="mt-3 flex items-center gap-3">
          <span
            className="flex h-11 w-11 items-center justify-center rounded-full text-[14px] font-bold text-white"
            style={{background: 'linear-gradient(135deg,#2DD4BF,#0EA5E9)'}}
          >
            {(user.email ?? 'TW').slice(0, 2).toUpperCase()}
          </span>
          <div className="min-w-0">
            <p className={cx('truncate text-[14px] font-semibold', t.text)}>
              {user.email}
            </p>
            <p className={cx('text-[12px]', t.faint)}>{tr('settings.signedIn')}</p>
          </div>
        </div>
      </section>

      <section id="subscription" className={cx('mt-5 scroll-mt-20 rounded-3xl border p-6', t.card, t.border, t.soft)}>
        <h2 className={cx('text-[13px] font-semibold uppercase tracking-wide', t.faint)}>
          {tr('settings.subscription')}
        </h2>
        <div className="mt-3 flex items-center justify-between gap-3">
          <div className="min-w-0">
            <p className={cx('flex items-center gap-1.5 text-[14px] font-semibold', t.text)}>
              {isPro && <Crown size={15} className={dark ? 'text-amber-300' : 'text-amber-500'} />}
              {entLoading ? '…' : isPro ? tr('settings.planPro') : tr('settings.planFree')}
            </p>
            <p className={cx('text-[12.5px]', t.sub)}>
              {isPro ? tr('settings.planProSub') : tr('settings.planFreeSub')}
            </p>
          </div>
          {isPro ? (
            <button
              onClick={onManage}
              disabled={portalBusy}
              className={cx(
                'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-3.5 py-2 text-[13px] font-medium disabled:opacity-60',
                t.border,
                t.ghost,
              )}
            >
              {portalBusy && <Loader2 size={14} className="animate-spin" />}
              {tr('settings.manage')}
            </button>
          ) : (
            <Link
              href="/pro"
              className={cx(
                'inline-flex shrink-0 items-center gap-1 rounded-full px-3.5 py-2 text-[13px] font-semibold',
                btnPrimary(dark),
              )}
            >
              {tr('settings.upgrade')}
              <ArrowRight size={14} />
            </Link>
          )}
        </div>
      </section>

      <section className={cx('mt-5 rounded-3xl border p-6', t.card, t.border, t.soft)}>
        <h2 className={cx('text-[13px] font-semibold uppercase tracking-wide', t.faint)}>
          {tr('settings.appearance')}
        </h2>
        <div className="mt-3 flex items-center justify-between">
          <div>
            <p className={cx('text-[14px] font-semibold', t.text)}>{tr('settings.theme')}</p>
            <p className={cx('text-[12.5px]', t.sub)}>
              {dark ? tr('settings.themeDark') : tr('settings.themeLight')} {tr('settings.themeHint')}
            </p>
          </div>
          <button
            onClick={toggle}
            className={cx(
              'inline-flex items-center gap-2 rounded-full border px-3.5 py-2 text-[13px] font-medium',
              t.border,
              t.ghost,
            )}
          >
            {dark ? <Sun size={15} /> : <Moon size={15} />}
            {dark ? tr('settings.themeLight') : tr('settings.themeDark')}
          </button>
        </div>
      </section>

      <button
        onClick={signOut}
        className={cx(
          'mt-5 inline-flex items-center gap-2 rounded-full border px-4 py-2 text-[13px] font-semibold',
          t.border,
          dark ? 'text-rose-400 hover:bg-slate-800' : 'text-rose-500 hover:bg-rose-50',
        )}
      >
        <LogOut size={15} /> {tr('nav.signout')}
      </button>
    </div>
  );
}
