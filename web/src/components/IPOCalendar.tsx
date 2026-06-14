'use client';

import {Rocket} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useEffect, useState} from 'react';
import {getIPO, type IPO} from '@/lib/api';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, timeAgo, tok} from '@/lib/ui';
import {EmptyState, ErrorState, FeedSkeleton} from '@/components/ui/states';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'error';

/** A short "—" for empty source fields, so cells never render blank. */
function dash(v: string): string {
  const s = v.trim();
  return s === '' ? '—' : s;
}

/**
 * The US IPO calendar: recently-priced, upcoming, and newly-filed offerings,
 * grouped by section. Sourced from Nasdaq's public calendar (delayed,
 * display-only) — each ticker links to its stock page; not investment advice.
 */
export function IPOCalendar() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [status, setStatus] = useState<Status>('loading');
  const [priced, setPriced] = useState<IPO[]>([]);
  const [upcoming, setUpcoming] = useState<IPO[]>([]);
  const [filed, setFiled] = useState<IPO[]>([]);
  const [updatedAt, setUpdatedAt] = useState('');

  useEffect(() => {
    const ctrl = new AbortController();
    setStatus('loading');
    getIPO(ctrl.signal).then(
      r => {
        setUpcoming(r.upcoming ?? []);
        setPriced(r.priced ?? []);
        setFiled(r.filed ?? []);
        setUpdatedAt(r.updated_at ?? '');
        setStatus('ready');
      },
      () => {
        if (!ctrl.signal.aborted) setStatus('error');
      },
    );
    return () => ctrl.abort();
  }, []);

  const total = upcoming.length + priced.length + filed.length;

  return (
    <div className="w-full">
      <header className="mb-4">
        <h1 className={cx('flex items-center gap-2 text-[22px] font-bold tracking-tight', t.text)}>
          <Rocket size={20} className={dark ? 'text-sky-300' : 'text-sky-600'} />
          {tr('ipo.title')}
        </h1>
        <p className={cx('mt-1 text-[13.5px]', t.sub)}>{tr('ipo.subtitle')}</p>
        {updatedAt && timeAgo(updatedAt) && (
          <p className={cx('mt-1 text-[11.5px]', t.faint)}>
            {tr('ipo.updated').replace('{t}', timeAgo(updatedAt))}
          </p>
        )}
      </header>

      <div className={cx('mb-4 rounded-xl border p-3 text-[12px]', t.border, dark ? 'bg-slate-900' : 'bg-slate-50', t.sub)}>
        {tr('ipo.disclaimer')}
      </div>

      {status === 'loading' && <FeedSkeleton />}
      {status === 'error' && <ErrorState onRetry={() => location.reload()} />}
      {status === 'ready' && total === 0 && (
        <EmptyState label={tr('ipo.emptyAll')} sub={tr('ipo.emptyAllSub')} icon={Rocket} />
      )}
      {status === 'ready' && total > 0 && (
        <div className="tw-fade space-y-6">
          <Section title={tr('ipo.upcoming')} rows={upcoming} kind="upcoming" t={t} dark={dark} tr={tr} />
          <Section title={tr('ipo.priced')} rows={priced} kind="priced" t={t} dark={dark} tr={tr} />
          <Section title={tr('ipo.filed')} rows={filed} kind="filed" t={t} dark={dark} tr={tr} />
        </div>
      )}

      <p className={cx('mt-4 text-center text-[11px]', t.faint)}>{tr('ipo.footer')}</p>
    </div>
  );
}

/** One calendar section (Upcoming / Recently priced / Newly filed). */
function Section({
  title,
  rows,
  kind,
  t,
  dark,
  tr,
}: {
  title: string;
  rows: IPO[];
  kind: IPO['kind'];
  t: Tokens;
  dark: boolean;
  tr: (k: string) => string;
}) {
  // The amount column carries little for not-yet-priced filings; the status
  // column is only meaningful for priced/upcoming deals. Keep the table compact.
  const showAmount = kind !== 'filed';
  const showStatus = kind === 'priced';
  const showDate = kind !== 'filed';

  return (
    <section>
      <h2 className={cx('mb-2 text-[14px] font-bold', t.text)}>
        {title}
        <span className={cx('ml-2 text-[12px] font-medium', t.faint)}>{rows.length}</span>
      </h2>
      {rows.length === 0 ? (
        <div className={cx('rounded-2xl border px-4 py-6 text-center text-[12.5px]', t.card, t.border, t.faint)}>
          {tr('ipo.empty')}
        </div>
      ) : (
        <div className={cx('overflow-x-auto rounded-2xl border', t.card, t.border, t.soft)}>
          <table className="w-full text-left text-[13px]">
            <thead>
              <tr className={cx('border-b', t.border, t.faint)}>
                <th className="px-3 py-2.5 font-semibold">{tr('ipo.colCompany')}</th>
                <th className="px-3 py-2.5 font-semibold">{tr('ipo.colExchange')}</th>
                <th className="px-3 py-2.5 text-right font-semibold">{tr('ipo.colPrice')}</th>
                <th className="px-3 py-2.5 text-right font-semibold">{tr('ipo.colShares')}</th>
                {showAmount && (
                  <th className="px-3 py-2.5 text-right font-semibold">{tr('ipo.colAmount')}</th>
                )}
                {showDate && <th className="px-3 py-2.5 font-semibold">{tr('ipo.colDate')}</th>}
                {showStatus && <th className="px-3 py-2.5 font-semibold">{tr('ipo.colStatus')}</th>}
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr
                  key={`${r.ticker}-${r.company}-${i}`}
                  className={cx(i > 0 ? 'border-t' : '', t.hair)}
                >
                  <td className="px-3 py-2.5">
                    <div className="flex flex-col">
                      {r.ticker ? (
                        <Link
                          href={`/stock/${encodeURIComponent(r.ticker)}`}
                          className={cx('font-semibold hover:underline', t.accentText)}
                        >
                          {r.ticker}
                        </Link>
                      ) : (
                        <span className={cx('font-semibold', t.faint)}>—</span>
                      )}
                      <span className={cx('text-[12px]', t.sub)}>{dash(r.company)}</span>
                    </div>
                  </td>
                  <td className={cx('px-3 py-2.5 text-[12.5px]', t.sub)}>{dash(r.exchange)}</td>
                  <td className={cx('px-3 py-2.5 text-right tabular-nums', t.text)}>{dash(r.price)}</td>
                  <td className={cx('px-3 py-2.5 text-right tabular-nums', t.sub)}>{dash(r.shares)}</td>
                  {showAmount && (
                    <td className={cx('px-3 py-2.5 text-right tabular-nums', t.sub)}>{dash(r.amount)}</td>
                  )}
                  {showDate && (
                    <td className={cx('px-3 py-2.5 whitespace-nowrap text-[12.5px] tabular-nums', t.sub)}>
                      {dash(r.date)}
                    </td>
                  )}
                  {showStatus && (
                    <td className={cx('px-3 py-2.5 text-[12.5px]', t.sub)}>{dash(r.status)}</td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
