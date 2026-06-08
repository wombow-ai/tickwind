'use client';

import {Trash2} from 'lucide-react';
import {useCallback, useEffect, useState} from 'react';
import {createHolding, deleteHolding, getHoldings, type Holding, type Quote} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, fmtPrice, tok} from '@/lib/ui';

type Tokens = ReturnType<typeof tok>;

/**
 * Per-stock holding editor for the signed-in user (StockView "Holdings" tab).
 * One position per ticker (upsert): enter shares + average cost, and we show the
 * current value and unrealised P/L derived from the live quote (never stored, so
 * they track the price). Delete to clear the position.
 */
export function HoldingsPanel({ticker, quote, cur}: {ticker: string; quote?: Quote; cur: string}) {
  const {getToken} = useAuth();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [holding, setHolding] = useState<Holding | null>(null);
  const [shares, setShares] = useState('');
  const [avgCost, setAvgCost] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    getToken().then(token =>
      getHoldings(token).then(
        r => {
          const h = (r.holdings ?? []).find(x => x.ticker === ticker) ?? null;
          setHolding(h);
          if (h) {
            setShares(String(h.shares));
            setAvgCost(String(h.avg_cost));
          }
        },
        () => setHolding(null),
      ),
    );
  }, [getToken, ticker]);
  useEffect(() => {
    load();
  }, [load]);

  const nShares = parseFloat(shares);
  const nCost = parseFloat(avgCost);
  const canSave = !busy && nShares > 0 && nCost >= 0;

  async function save() {
    if (!canSave) return;
    setBusy(true);
    try {
      const token = await getToken();
      const h = await createHolding(token, {ticker, shares: nShares, avg_cost: nCost});
      setHolding(h);
    } catch {
      // keep the form so the user can retry
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!holding) return;
    const id = holding.id;
    setHolding(null);
    setShares('');
    setAvgCost('');
    try {
      const token = await getToken();
      await deleteHolding(token, id);
    } catch {
      load();
    }
  }

  // Derived from the live quote — never persisted, so it tracks the price.
  const price = quote?.price ?? 0;
  const have = !!holding && price > 0;
  const value = have ? holding!.shares * price : 0;
  const cost = holding ? holding.shares * holding.avg_cost : 0;
  const pl = value - cost;
  const plPct = cost > 0 ? (pl / cost) * 100 : 0;
  const up = pl >= 0;
  const plCol = up
    ? dark
      ? 'text-emerald-400'
      : 'text-emerald-600'
    : dark
      ? 'text-rose-400'
      : 'text-rose-500';

  return (
    <div className="tw-fade">
      <div className={cx('mb-4 rounded-2xl border p-3', t.card, t.border, t.soft)}>
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1">
            <span className={cx('text-[11px]', t.faint)}>{tr('holdings.shares')}</span>
            <input
              type="number"
              inputMode="decimal"
              value={shares}
              onChange={e => setShares(e.target.value)}
              placeholder="10"
              className={cx('w-24 rounded-lg border bg-transparent px-2.5 py-1.5 text-[13px]', t.border, t.text)}
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className={cx('text-[11px]', t.faint)}>{tr('holdings.avgCost')}</span>
            <input
              type="number"
              inputMode="decimal"
              value={avgCost}
              onChange={e => setAvgCost(e.target.value)}
              placeholder="150"
              className={cx('w-24 rounded-lg border bg-transparent px-2.5 py-1.5 text-[13px]', t.border, t.text)}
            />
          </label>
          <button
            onClick={save}
            disabled={!canSave}
            className={cx('rounded-lg px-3.5 py-1.5 text-[13px] font-semibold transition disabled:opacity-50', btnPrimary(dark))}
          >
            {tr('holdings.save')}
          </button>
          {holding && (
            <button
              onClick={remove}
              aria-label={tr('holdings.remove')}
              className={cx('inline-flex items-center gap-1 px-1 text-[12.5px] font-medium', dark ? 'text-rose-400' : 'text-rose-500')}
            >
              <Trash2 size={13} /> {tr('holdings.remove')}
            </button>
          )}
        </div>
        <p className={cx('mt-2 text-[11px]', t.faint)}>{tr('holdings.hint')}</p>
      </div>

      {holding ? (
        <div className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
          <div className="grid grid-cols-2 gap-y-3 sm:grid-cols-4">
            <Cell t={t} label={tr('holdings.shares')} value={String(holding.shares)} />
            <Cell t={t} label={tr('holdings.avgCost')} value={fmtPrice(cur, holding.avg_cost)} />
            <Cell t={t} label={tr('holdings.value')} value={have ? fmtPrice(cur, value) : '—'} />
            <div className="flex flex-col">
              <span className={cx('text-[11px]', t.faint)}>{tr('holdings.pl')}</span>
              {have ? (
                <span className={cx('text-[14px] font-semibold tabular-nums', plCol)}>
                  {up ? '+' : '−'}
                  {fmtPrice(cur, Math.abs(pl))} ({up ? '+' : '−'}
                  {Math.abs(plPct).toFixed(2)}%)
                </span>
              ) : (
                <span className={cx('text-[14px] font-semibold', t.faint)}>—</span>
              )}
            </div>
          </div>
        </div>
      ) : (
        <p className={cx('py-6 text-center text-[12.5px]', t.faint)}>{tr('holdings.empty')}</p>
      )}
    </div>
  );
}

function Cell({t, label, value}: {t: Tokens; label: string; value: string}) {
  return (
    <div className="flex flex-col">
      <span className={cx('text-[11px]', t.faint)}>{label}</span>
      <span className={cx('text-[14px] font-semibold tabular-nums', t.text)}>{value}</span>
    </div>
  );
}
