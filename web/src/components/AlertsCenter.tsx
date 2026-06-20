'use client';

import {Bell, RotateCcw, Trash2} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useCallback, useEffect, useState} from 'react';
import {deleteAlert, getAlerts, reactivateAlert, type Alert} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {useToast} from '@/components/ui/Toast';
import {describeAlert} from '@/lib/alerts';

/**
 * Cross-stock alerts hub (the `/alerts` page): every alert the signed-in user
 * has, across all tickers — split into Triggered (re-armable) and Active —
 * unlike the per-stock AlertsPanel. So a fired NVDA alert is visible without
 * opening NVDA. Re-arm clears the trigger so a one-shot alert can be reused.
 */
export function AlertsCenter() {
  const {user, getToken} = useAuth();
  const {toast} = useToast();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(() => {
    getToken().then(token =>
      getAlerts(token).then(
        r => {
          setAlerts(r.alerts ?? []);
          setLoaded(true);
        },
        () => setLoaded(true),
      ),
    );
  }, [getToken]);
  useEffect(() => {
    if (user) load();
  }, [user, load]);

  async function remove(a: Alert) {
    setAlerts(prev => prev.filter(x => x.id !== a.id)); // optimistic
    try {
      await deleteAlert(await getToken(), a.id);
    } catch {
      load();
    }
  }

  async function rearm(a: Alert) {
    setAlerts(prev => prev.map(x => (x.id === a.id ? {...x, triggered_at: undefined, active: true} : x)));
    try {
      await reactivateAlert(await getToken(), a.id);
      toast(tr('alerts.rearmed'), {tone: 'ok'});
    } catch {
      load();
    }
  }

  if (!user) {
    return (
      <div className="mx-auto max-w-2xl">
        <Header t={t} tr={tr} dark={dark} />
        <p className={cx('rounded-2xl border p-8 text-center text-[13px]', t.card, t.border, t.soft, t.sub)}>
          {tr('alerts.loginToView')}
        </p>
      </div>
    );
  }

  const triggered = alerts.filter(a => a.triggered_at);
  const active = alerts.filter(a => !a.triggered_at);

  const row = (a: Alert, isTriggered: boolean) => (
    <div
      key={a.id}
      className={cx('group flex items-center gap-2 rounded-2xl border p-3', t.card, t.border, t.soft)}
    >
      <Bell size={14} className={dark ? 'text-amber-300' : 'text-amber-500'} />
      <Link
        href={`/stock/${encodeURIComponent(a.ticker)}`}
        className={cx('shrink-0 rounded-md px-1.5 py-0.5 text-[11px] font-bold', t.chip, t.accentText)}
      >
        {a.ticker}
      </Link>
      <span className={cx('min-w-0 flex-1 truncate text-[13.5px] font-medium', t.text)}>{describeAlert(a, tr)}</span>
      {isTriggered && (
        <button
          onClick={() => rearm(a)}
          className={cx(
            'inline-flex shrink-0 items-center gap-1 rounded-lg border px-2 py-1 text-[11.5px] font-semibold transition',
            t.border,
            dark ? 'text-teal-300 hover:border-teal-400' : 'text-teal-600 hover:border-teal-400',
          )}
        >
          <RotateCcw size={12} /> {tr('alerts.rearm')}
        </button>
      )}
      <button
        onClick={() => remove(a)}
        aria-label={tr('alerts.delete')}
        className={cx('shrink-0 opacity-0 transition group-hover:opacity-100', dark ? 'text-rose-400' : 'text-rose-500')}
      >
        <Trash2 size={13} />
      </button>
    </div>
  );

  return (
    <div className="mx-auto max-w-2xl">
      <Header t={t} tr={tr} dark={dark} />

      {loaded && alerts.length === 0 && (
        <p className={cx('rounded-2xl border p-8 text-center text-[13px]', t.card, t.border, t.soft, t.sub)}>
          {tr('alerts.centerEmpty')}
        </p>
      )}

      {triggered.length > 0 && (
        <section className="mb-5">
          <h2 className={cx('mb-2 text-[12.5px] font-bold uppercase tracking-wide', dark ? 'text-emerald-400' : 'text-emerald-600')}>
            {tr('alerts.triggered')} · {triggered.length}
          </h2>
          <div className="space-y-2">{triggered.map(a => row(a, true))}</div>
        </section>
      )}

      {active.length > 0 && (
        <section>
          <h2 className={cx('mb-2 text-[12.5px] font-bold uppercase tracking-wide', t.faint)}>
            {tr('alerts.active')} · {active.length}
          </h2>
          <div className="space-y-2">{active.map(a => row(a, false))}</div>
        </section>
      )}

      <p className={cx('mt-4 text-[10.5px]', t.faint)}>{tr('alerts.deliveryNote')}</p>
    </div>
  );
}

function Header({t, tr, dark}: {t: ReturnType<typeof tok>; tr: (k: string) => string; dark: boolean}) {
  return (
    <header className="mb-5">
      <h1 className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}>
        <Bell size={20} className={dark ? 'text-amber-300' : 'text-amber-500'} />
        {tr('alerts.centerTitle')}
      </h1>
      <p className={cx('mt-1 text-[13px]', t.sub)}>{tr('alerts.centerSubtitle')}</p>
    </header>
  );
}
