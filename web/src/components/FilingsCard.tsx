'use client';

import {ExternalLink, FileText} from 'lucide-react';
import {useEffect, useState} from 'react';
import {
  getMaterialEvents,
  type MaterialEvent,
  type MaterialEventsResponse,
} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

type Status = 'loading' | 'ready' | 'hidden';

/**
 * The 8-K material-events card: a company's recent current-report filings —
 * material corporate events (M&A, exec changes, earnings pre-announcements,
 * bankruptcies, …) — with an optional AI-written plain-language summary.
 *
 * Anti-hallucination split (mirrors MovementCard): every FACT shown (form, dates,
 * the parsed item-code labels, the source link) is Go-owned; only the per-filing
 * `summary` is AI-written, and is omitted when the LLM is off or the source was
 * too thin — the item labels alone still render. The labels are bilingual; we pick
 * `label_en` / `label_zh` by the current UI language.
 *
 * Hides entirely (renders null) when the symbol is unknown (404 → null) or the
 * fetch fails. An existing company with zero recent 8-Ks shows a subtle empty
 * line rather than vanishing, so the section reads as "checked, nothing recent".
 */
export function FilingsCard({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [data, setData] = useState<MaterialEventsResponse | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getMaterialEvents(ticker, lang, c.signal).then(
      r => {
        if (!r) {
          setStatus('hidden'); // unknown symbol (404) → hide
          return;
        }
        setData(r);
        setStatus('ready');
      },
      () => setStatus('hidden'), // error → hide
    );
    return () => c.abort();
  }, [ticker, lang]);

  if (status === 'hidden') return null;

  const Header = (
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <h2 className={cx('flex items-center gap-1.5 text-[14px] font-bold', t.text)}>
        <FileText size={15} className={dark ? 'text-sky-300' : 'text-sky-500'} />
        {tr('filings.title')}
      </h2>
      {data?.llm && (
        <span
          className={cx(
            'rounded-md px-1.5 py-0.5 text-[10px] font-bold',
            dark ? 'bg-sky-500/15 text-sky-300' : 'bg-sky-50 text-sky-600',
          )}
        >
          {tr('filings.aiBadge')}
        </span>
      )}
      <span className={cx('ml-auto text-[10.5px]', t.faint)}>
        {tr('filings.source')}
        {data?.generated_at ? ` · ${tr('filings.asOf')} ${data.generated_at.slice(0, 10)}` : ''}
      </span>
    </div>
  );

  if (status === 'loading') {
    return (
      <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        {Header}
        <div className="space-y-2" aria-hidden>
          <div className={cx('h-3 rounded', t.skel)} style={{width: '70%'}} />
          <div className={cx('h-3 rounded', t.skel)} style={{width: '52%'}} />
        </div>
      </section>
    );
  }

  // status === 'ready' — data is non-null.
  const filings = data?.filings ?? [];

  return (
    <section className={cx('mb-6 rounded-2xl border p-4', t.card, t.border, t.soft)}>
      {Header}

      {filings.length === 0 ? (
        <p className={cx('text-[12.5px]', t.faint)}>{tr('filings.empty')}</p>
      ) : (
        <ul className="flex flex-col gap-3">
          {filings.map((f, i) => (
            <FilingRow key={i} filing={f} lang={lang} dark={dark} />
          ))}
        </ul>
      )}

      {data?.llm && (
        <p className={cx('mt-3 text-[10.5px]', t.faint)}>{tr('filings.disclaimer')}</p>
      )}
    </section>
  );
}

/** One 8-K filing row: Go-owned facts (form/dates/item labels + source link) and
 * the optional AI summary. */
function FilingRow({
  filing,
  lang,
  dark,
}: {
  filing: MaterialEvent;
  lang: string;
  dark: boolean;
}) {
  const t = tok(dark);
  const tr = useT();

  return (
    <li className={cx('rounded-xl border p-3', t.border)}>
      <div className="mb-1.5 flex flex-wrap items-center gap-2">
        {/* Go-owned form type + amendment flag. */}
        <span className={cx('text-[13px] font-bold', t.text)}>{filing.form}</span>
        {filing.amendment && (
          <span className={cx('rounded-md px-1.5 py-0.5 text-[10px] font-semibold', t.chip, t.chipText)}>
            {tr('filings.amendment')}
          </span>
        )}
        {/* Filed date (Go-owned fact). */}
        <span className={cx('text-[11.5px] tabular-nums', t.sub)}>{filing.filed_date}</span>
        {filing.report_date && filing.report_date !== filing.filed_date && (
          <span className={cx('text-[10.5px] tabular-nums', t.faint)}>
            {tr('filings.reportDate')} {filing.report_date}
          </span>
        )}
        {/* Source link to the SEC filing index page. */}
        {filing.accession_url && (
          <a
            href={filing.accession_url}
            target="_blank"
            rel="noopener noreferrer"
            className={cx(
              'ml-auto inline-flex items-center gap-1 text-[11px] transition-colors',
              t.sub,
              dark ? 'hover:text-sky-300' : 'hover:text-sky-600',
            )}
          >
            <ExternalLink size={12} />
            {tr('filings.viewSource')}
          </a>
        )}
      </div>

      {/* Go-owned item-code labels (the anti-hallucination anchor) — chips, one per
          parsed item code, in the current UI language. */}
      {filing.items.length > 0 && (
        <div className="mb-1.5 flex flex-wrap gap-1.5">
          {filing.items.map(it => (
            <span
              key={it.code}
              title={it.code}
              className={cx(
                'inline-flex max-w-full items-center gap-1 rounded-lg border px-2 py-0.5 text-[11px]',
                t.border,
                t.sub,
              )}
            >
              <span className={cx('font-semibold tabular-nums', t.faint)}>{it.code}</span>
              <span className="truncate">{lang === 'en' ? it.label_en : it.label_zh}</span>
            </span>
          ))}
        </div>
      )}

      {/* Optional AI plain-language summary (absent when LLM off / source too thin). */}
      {filing.summary && (
        <p className={cx('text-[12.5px] leading-relaxed', t.text)}>{filing.summary}</p>
      )}
    </li>
  );
}
