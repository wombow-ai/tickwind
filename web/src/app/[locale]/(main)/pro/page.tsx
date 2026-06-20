'use client';

import {ArrowRight, Check, Crown, Loader2, Settings as SettingsIcon, ShieldCheck, Sparkles} from 'lucide-react';
import {useState} from 'react';
import Link from '@/components/LocalLink';
import {createCheckout, createPortal} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useEntitlement} from '@/lib/entitlement';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

/**
 * Tickwind Pro pricing page. Honest, value-forward (no dark patterns): names exactly
 * what Pro unlocks, leads with the anti-hallucination trust story, annual default with
 * the real saving shown. The CTA opens a Stripe Checkout session (test mode until the
 * owner flips live keys); logged-out users are routed to sign in first.
 */
export default function ProPage() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {user, getToken} = useAuth();
  const {isPro, loading: entLoading} = useEntitlement();
  const [interval, setInterval] = useState<'month' | 'year'>('year');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(false);

  const onSubscribe = async () => {
    setBusy(true);
    setErr(false);
    try {
      const token = await getToken();
      const url = await createCheckout(interval, token);
      window.location.assign(url);
    } catch {
      setErr(true);
      setBusy(false);
    }
  };

  const onManage = async () => {
    setBusy(true);
    setErr(false);
    try {
      const token = await getToken();
      window.location.assign(await createPortal(token));
    } catch {
      setErr(true);
      setBusy(false);
    }
  };

  // Already-Pro users see a Manage card (portal); free/anon see the pricing + Subscribe flow.
  const features = [tr('pro.feat1'), tr('pro.feat2'), tr('pro.feat3'), tr('pro.feat4')];

  return (
    <div className="mx-auto max-w-lg">
      <div className="mb-6 text-center">
        <h1 className={cx('flex items-center justify-center gap-2 text-[26px] font-bold tracking-tight', t.text)}>
          <Sparkles size={22} className={dark ? 'text-violet-300' : 'text-violet-500'} />
          {tr('pro.title')}
        </h1>
        <p className={cx('mt-2 text-[14px]', t.sub)}>{tr('pro.subtitle')}</p>
      </div>

      {!entLoading && isPro ? (
        <section className={cx('rounded-3xl border p-6 text-center', t.card, t.border, t.soft)}>
          <Crown size={28} className={cx('mx-auto', dark ? 'text-amber-300' : 'text-amber-500')} />
          <h2 className={cx('mt-3 text-[18px] font-bold', t.text)}>{tr('pro.alreadyPro')}</h2>
          <p className={cx('mt-1.5 text-[13.5px]', t.sub)}>{tr('pro.alreadyProSub')}</p>
          <ul className="mt-5 space-y-2.5 text-left">
            {features.map((f, i) => (
              <li key={i} className={cx('flex items-start gap-2 text-[13.5px]', t.text)}>
                <Check size={16} className={cx('mt-0.5 shrink-0', dark ? 'text-emerald-400' : 'text-emerald-500')} />
                {f}
              </li>
            ))}
          </ul>
          <button
            onClick={onManage}
            disabled={busy}
            className={cx(
              'mt-5 flex w-full items-center justify-center gap-1.5 rounded-full border px-4 py-2.5 text-[14px] font-semibold disabled:opacity-60',
              t.border,
              t.ghost,
            )}
          >
            {busy ? <Loader2 size={16} className="animate-spin" /> : <SettingsIcon size={15} />}
            {tr('pro.manageSub')}
          </button>
          {err && <p className={cx('mt-2 text-[12px]', dark ? 'text-rose-400' : 'text-rose-500')}>{tr('pro.err')}</p>}
        </section>
      ) : (
      <section className={cx('rounded-3xl border p-6', t.card, t.border, t.soft)}>
        {/* monthly / annual toggle */}
        <div className={cx('mx-auto mb-5 flex w-fit rounded-full border p-1', t.border)}>
          {(['year', 'month'] as const).map(opt => (
            <button
              key={opt}
              onClick={() => setInterval(opt)}
              className={cx(
                'rounded-full px-4 py-1.5 text-[12.5px] font-semibold transition',
                interval === opt ? btnPrimary(dark) : t.sub,
              )}
            >
              {opt === 'year' ? tr('pro.annual') : tr('pro.monthly')}
              {opt === 'year' && <span className="ml-1 opacity-90">· {tr('pro.save')}</span>}
            </button>
          ))}
        </div>

        {/* price */}
        <div className="text-center">
          <div className={cx('text-[34px] font-bold tracking-tight tabular-nums', t.text)}>
            {interval === 'year' ? '$8.25' : '$12.99'}
            <span className={cx('text-[15px] font-medium', t.sub)}> /{tr('pro.mo')}</span>
          </div>
          <p className={cx('mt-1 text-[12.5px]', t.faint)}>
            {interval === 'year' ? tr('pro.billedAnnually') : tr('pro.billedMonthly')}
          </p>
        </div>

        {/* unlocks */}
        <ul className="mt-5 space-y-2.5">
          {features.map((f, i) => (
            <li key={i} className={cx('flex items-start gap-2 text-[13.5px]', t.text)}>
              <Check size={16} className={cx('mt-0.5 shrink-0', dark ? 'text-emerald-400' : 'text-emerald-500')} />
              {f}
            </li>
          ))}
        </ul>

        {/* trust line */}
        <p className={cx('mt-5 flex items-start gap-1.5 rounded-xl p-3 text-[12px]', t.surf2, t.sub)}>
          <ShieldCheck size={14} className={cx('mt-0.5 shrink-0', dark ? 'text-emerald-400' : 'text-emerald-500')} />
          {tr('pro.trust')}
        </p>

        {/* CTA */}
        {user ? (
          <button
            onClick={onSubscribe}
            disabled={busy}
            className={cx(
              'mt-5 flex w-full items-center justify-center gap-1.5 rounded-full px-4 py-2.5 text-[14px] font-semibold disabled:opacity-60',
              btnPrimary(dark),
            )}
          >
            {busy ? <Loader2 size={16} className="animate-spin" /> : null}
            {tr('pro.cta')}
            {!busy && <ArrowRight size={15} />}
          </button>
        ) : (
          <Link
            href="/login"
            className={cx(
              'mt-5 flex w-full items-center justify-center gap-1.5 rounded-full px-4 py-2.5 text-[14px] font-semibold',
              btnPrimary(dark),
            )}
          >
            {tr('pro.ctaLogin')}
            <ArrowRight size={15} />
          </Link>
        )}
        {err && <p className={cx('mt-2 text-center text-[12px]', dark ? 'text-rose-400' : 'text-rose-500')}>{tr('pro.err')}</p>}
        <p className={cx('mt-3 text-center text-[11px]', t.faint)}>{tr('pro.cancel')}</p>
      </section>
      )}

      <p className={cx('mt-4 text-center text-[10.5px]', t.faint)}>{tr('pro.disclaimer')}</p>
    </div>
  );
}
