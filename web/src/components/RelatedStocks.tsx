import Link from 'next/link';
import {pairSlug} from '@/lib/compare';
import {peersFor, tickerToZones} from '@/lib/related';

/**
 * Server-rendered "Related" footer for a /stock page. Emits crawlable internal
 * links (in the initial HTML — no client hooks) so the ~6,000 stock pages cross-
 * link the /compare engine, /zone hubs, and sector peers; this is what lets
 * Google discover those built-but-otherwise-orphaned pages.
 *
 * Renders nothing for an obscure ticker with no curated peers and no zone (avoids
 * a thin/empty section). Locale prefix is built explicitly (`/{locale}/…`) so the
 * links are correct + present at SSR without depending on a client pathname hook.
 */
export function RelatedStocks({
  ticker,
  locale,
}: {
  ticker: string;
  locale: string;
}) {
  const T = ticker.toUpperCase();
  const zh = locale === 'zh';
  const peers = peersFor(T, 6);
  const zones = tickerToZones(T);
  const comparePeers = peers.slice(0, 4);

  if (peers.length === 0 && zones.length === 0) return null;

  const px = (path: string) => `/${locale}${path}`;
  const chip =
    'inline-flex items-center gap-1.5 rounded-full border border-slate-200 px-3 py-1.5 text-[12.5px] font-medium text-slate-700 transition hover:border-teal-300 hover:bg-slate-50 dark:border-slate-800 dark:text-slate-300 dark:hover:border-teal-500/40 dark:hover:bg-slate-900';
  const label =
    'mb-2 text-[11px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500';

  return (
    <nav
      aria-label={zh ? '相关页面' : 'Related pages'}
      className="mt-10 border-t border-slate-200 pt-6 dark:border-slate-800"
    >
      <h2 className="mb-4 text-[13px] font-bold text-slate-900 dark:text-slate-100">
        {zh ? `与 ${T} 相关` : `Related to ${T}`}
      </h2>

      <div className="space-y-5">
        {comparePeers.length > 0 && (
          <section>
            <p className={label}>{zh ? '对比' : 'Compare'}</p>
            <div className="flex flex-wrap gap-2">
              {comparePeers.map(p => (
                <Link key={p} href={px(`/compare/${pairSlug(T, p)}`)} className={chip}>
                  {T} <span className="text-slate-400 dark:text-slate-500">vs</span> {p}
                </Link>
              ))}
            </div>
          </section>
        )}

        {peers.length > 0 && (
          <section>
            <p className={label}>{zh ? '同类股票' : 'Related stocks'}</p>
            <div className="flex flex-wrap gap-2">
              {peers.map(p => (
                <Link key={p} href={px(`/stock/${p}`)} className={chip}>
                  {p}
                </Link>
              ))}
            </div>
          </section>
        )}

        {zones.length > 0 && (
          <section>
            <p className={label}>{zh ? '主题专区' : 'Theme zones'}</p>
            <div className="flex flex-wrap gap-2">
              {zones.map(z => (
                <Link key={z.key} href={px(`/zone/${z.key}`)} className={chip}>
                  {zh ? z.titleZh : z.titleEn}
                </Link>
              ))}
            </div>
          </section>
        )}
      </div>
    </nav>
  );
}

export default RelatedStocks;
