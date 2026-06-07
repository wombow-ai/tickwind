'use client';

import {Bell, Trash2} from 'lucide-react';
import {useCallback, useEffect, useState} from 'react';
import {createAlert, deleteAlert, getAlerts, type Alert} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

const KINDS = ['price_above', 'price_below', 'pct_move', 'new_filing'] as const;

function kindLabelKey(kind: string): string {
  switch (kind) {
    case 'price_above':
      return 'alerts.priceAbove';
    case 'price_below':
      return 'alerts.priceBelow';
    case 'pct_move':
      return 'alerts.pctMove';
    default:
      return 'alerts.newFiling';
  }
}

/**
 * Per-stock price/event alerts for the signed-in user (the StockView "Alerts"
 * tab; auth is assumed — the tab only renders for logged-in users). Create/list/
 * delete via /v1/alerts, scoped to the current ticker. Evaluation + delivery
 * (in-app, then web-push) land in later increments.
 */
export function AlertsPanel({ticker}: {ticker: string}) {
  const {getToken} = useAuth();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [kind, setKind] = useState<string>('price_above');
  const [threshold, setThreshold] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    getToken().then(token =>
      getAlerts(token).then(
        r => setAlerts((r.alerts ?? []).filter(a => a.ticker === ticker)),
        () => setAlerts([]),
      ),
    );
  }, [getToken, ticker]);
  useEffect(() => {
    load();
  }, [load]);

  const needsThreshold = kind !== 'new_filing';
  const canAdd = !busy && (!needsThreshold || parseFloat(threshold) > 0);

  async function add() {
    if (!canAdd) return;
    setBusy(true);
    try {
      const token = await getToken();
      const a = await createAlert(token, {
        ticker,
        kind,
        threshold: needsThreshold ? parseFloat(threshold) : 0,
      });
      setAlerts(prev => [a, ...prev]);
      setThreshold('');
    } catch {
      // keep the form so the user can retry
    } finally {
      setBusy(false);
    }
  }

  async function remove(a: Alert) {
    setAlerts(prev => prev.filter(x => x.id !== a.id));
    try {
      const token = await getToken();
      await deleteAlert(token, a.id);
    } catch {
      load();
    }
  }

  function describe(a: Alert): string {
    const k = tr(kindLabelKey(a.kind));
    if (a.kind === 'new_filing') return k;
    if (a.kind === 'pct_move') return `${k} ${a.threshold}%`;
    return `${k} $${a.threshold}`;
  }

  return (
    <div className="tw-fade">
      <div className={cx('mb-4 rounded-2xl border p-3', t.card, t.border, t.soft)}>
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1">
            <span className={cx('text-[11px]', t.faint)}>{tr('alerts.kind')}</span>
            <select
              value={kind}
              onChange={e => setKind(e.target.value)}
              className={cx('rounded-lg border bg-transparent px-2.5 py-1.5 text-[13px]', t.border, t.text)}
            >
              {KINDS.map(k => (
                <option key={k} value={k} className={dark ? 'bg-slate-800' : ''}>
                  {tr(kindLabelKey(k))}
                </option>
              ))}
            </select>
          </label>
          {needsThreshold && (
            <label className="flex flex-col gap-1">
              <span className={cx('text-[11px]', t.faint)}>
                {kind === 'pct_move' ? tr('alerts.percent') : tr('alerts.threshold')}
              </span>
              <input
                type="number"
                inputMode="decimal"
                value={threshold}
                onChange={e => setThreshold(e.target.value)}
                placeholder={kind === 'pct_move' ? '5' : '200'}
                className={cx('w-24 rounded-lg border bg-transparent px-2.5 py-1.5 text-[13px]', t.border, t.text)}
              />
            </label>
          )}
          <button
            onClick={add}
            disabled={!canAdd}
            className={cx('rounded-lg px-3.5 py-1.5 text-[13px] font-semibold transition disabled:opacity-50', btnPrimary(dark))}
          >
            {tr('alerts.add')}
          </button>
        </div>
        <p className={cx('mt-2 text-[11px]', t.faint)}>{tr('alerts.hint')}</p>
      </div>

      {alerts.length === 0 ? (
        <p className={cx('py-6 text-center text-[12.5px]', t.faint)}>{tr('alerts.empty')}</p>
      ) : (
        <div className="space-y-2">
          {alerts.map(a => (
            <div
              key={a.id}
              className={cx('group flex items-center gap-2 rounded-2xl border p-3', t.card, t.border, t.soft)}
            >
              <Bell size={14} className={dark ? 'text-amber-300' : 'text-amber-500'} />
              <span className={cx('min-w-0 flex-1 truncate text-[13.5px] font-medium', t.text)}>{describe(a)}</span>
              <span className={cx('shrink-0 rounded-md px-1.5 py-0.5 text-[10.5px] font-bold', t.chip, t.accentText)}>
                {a.ticker}
              </span>
              <button
                onClick={() => remove(a)}
                aria-label={tr('alerts.delete')}
                className={cx('shrink-0 opacity-0 transition group-hover:opacity-100', dark ? 'text-rose-400' : 'text-rose-500')}
              >
                <Trash2 size={13} />
              </button>
            </div>
          ))}
        </div>
      )}
      <p className={cx('mt-3 text-[10.5px]', t.faint)}>{tr('alerts.deliveryNote')}</p>
    </div>
  );
}
