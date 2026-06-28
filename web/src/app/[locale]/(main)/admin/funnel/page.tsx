'use client';

import {Loader2} from 'lucide-react';
import {useEffect, useState} from 'react';
import {ApiError, type FunnelStat, getFunnel} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/**
 * Admin-only conversion-funnel dashboard (GET /v1/admin/funnel). Not linked in the nav — the
 * operator navigates here directly. Non-admins get a 403 from the API and see "Admin only".
 * First-party data; the funnel: paywall_view → /pro view → checkout_started → subscription_active.
 */
export default function AdminFunnelPage() {
  const dark = useDark();
  const t = tok(dark);
  const {getToken, loading: authLoading} = useAuth();
  const [days, setDays] = useState(30);
  const [stats, setStats] = useState<FunnelStat[] | null>(null);
  const [err, setErr] = useState<'' | 'forbidden' | 'error'>('');

  useEffect(() => {
    if (authLoading) return;
    let active = true;
    setStats(null);
    setErr('');
    (async () => {
      try {
        const d = await getFunnel(await getToken(), days);
        if (active) setStats(d.stats);
      } catch (e) {
        if (active) setErr(e instanceof ApiError && e.status === 403 ? 'forbidden' : 'error');
      }
    })();
    return () => {
      active = false;
    };
  }, [getToken, days, authLoading]);

  const sum = (event: string) => (stats ?? []).filter(s => s.event === event).reduce((a, s) => a + s.count, 0);
  const paywall = sum('paywall_view');
  const proView = sum('pro_view');
  const checkout = sum('checkout_started');
  const active = sum('subscription_active');
  const pct = (a: number, b: number) => (b > 0 ? Math.round((a / b) * 100) : 0);
  const bySurface = (stats ?? []).filter(s => s.event === 'paywall_view').sort((a, b) => b.count - a.count);

  const stages = [
    {label: 'Paywall views', n: paywall, conv: null as number | null},
    {label: '/pro views', n: proView, conv: pct(proView, paywall)},
    {label: 'Checkout started', n: checkout, conv: pct(checkout, proView)},
    {label: 'Subscribed', n: active, conv: pct(active, checkout)},
  ];

  return (
    <main className={cx('mx-auto max-w-2xl px-4 py-8', t.text)}>
      <div className="mb-5 flex flex-wrap items-center gap-3">
        <h1 className="text-[20px] font-bold">Conversion funnel</h1>
        <div className="ml-auto flex gap-1">
          {[7, 30, 90].map(d => (
            <button
              key={d}
              type="button"
              onClick={() => setDays(d)}
              className={cx('rounded-lg px-2.5 py-1 text-[12px] font-semibold', days === d ? cx(t.card, t.border, 'border') : t.sub)}
            >
              {d}d
            </button>
          ))}
        </div>
      </div>

      {err === 'forbidden' && <p className={t.sub}>Admin only.</p>}
      {err === 'error' && <p className={t.sub}>Couldn’t load the funnel. Try again.</p>}
      {!err && stats === null && <Loader2 className="animate-spin" size={18} />}

      {!err && stats !== null && (
        <>
          <div className="space-y-2">
            {stages.map((s, i) => (
              <div key={s.label} className={cx('rounded-xl border p-3', t.card, t.border)}>
                <div className="flex items-baseline justify-between">
                  <span className="text-[13px] font-semibold">{s.label}</span>
                  <span className="text-[18px] font-bold tabular-nums">{s.n.toLocaleString()}</span>
                </div>
                {i > 0 && (
                  <div className={cx('mt-0.5 text-[11.5px]', t.sub)}>
                    {s.conv}% from previous step
                  </div>
                )}
              </div>
            ))}
          </div>

          <h2 className="mb-2 mt-7 text-[14px] font-bold">Paywall views by surface</h2>
          {bySurface.length === 0 ? (
            <p className={t.sub}>No paywall views in this window.</p>
          ) : (
            <div className="space-y-1.5">
              {bySurface.map(s => (
                <div key={s.surface} className="flex items-center justify-between text-[13px]">
                  <span>{s.surface || '(unknown)'}</span>
                  <span className="font-semibold tabular-nums">{s.count.toLocaleString()}</span>
                </div>
              ))}
            </div>
          )}
          <p className={cx('mt-6 text-[11px]', t.sub)}>
            Overall: {pct(active, paywall)}% of paywall views convert to a subscription (last {days}d).
          </p>
        </>
      )}
    </main>
  );
}
