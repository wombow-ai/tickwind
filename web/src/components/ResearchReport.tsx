'use client';

import {ExternalLink, Loader2, Sparkles} from 'lucide-react';
import Link from 'next/link';
import {useEffect, useState} from 'react';
import {
  getResearch,
  type ResearchCitation,
  type ResearchFact,
  type ResearchReportResponse,
  type ResearchSection,
} from '@/lib/api';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {Markdown} from '@/components/Markdown';
import {ShareCardButton} from '@/components/ShareCardButton';
import {type OgParams} from '@/lib/og';

type Tokens = ReturnType<typeof tok>;
type Status = 'loading' | 'ready' | 'hidden';

/**
 * The R2 deep-research report tab: a Go-assembled, source-attributed fact-sheet
 * (every number set in Go from public structured data) plus the LLM's qualitative
 * per-section prose. Numbers and words are split by design — the LLM never sets a
 * value (the anti-hallucination contract). The report ALWAYS renders: when the LLM
 * is off / over the daily cap / the call failed the backend returns 200 with the
 * data-only report (`llm:false`, prose empty), and this shows the facts grids with
 * a small "AI summary unavailable" note. Hidden only when the symbol is unknown
 * (404). Lazy: mounted only when the tab is opened, since it's an LLM call.
 *
 * The report BODY is Chinese-by-design (the documented single-language exception,
 * design §4.2); the chrome (tab, section/fact labels) is English-default and shows
 * the Chinese label only in the Chinese UI. Mandatory "AI 生成 · 数字来自公开数据 ·
 * 非投资建议" labels ride the top and bottom.
 */
export function ResearchReport({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const [data, setData] = useState<ResearchReportResponse | null>(null);
  const [status, setStatus] = useState<Status>('loading');

  useEffect(() => {
    const c = new AbortController();
    setStatus('loading');
    getResearch(ticker, lang, c.signal).then(
      r => {
        if (!r) {
          setStatus('hidden'); // 404 — unknown symbol
          return;
        }
        setData(r);
        setStatus('ready');
      },
      () => setStatus('hidden'), // network/other error → hide the tab body
    );
    return () => c.abort();
  }, [ticker, lang]);

  if (status === 'hidden') return null;
  if (status === 'loading' || !data) {
    return <ResearchSkeleton dark={dark} t={t} tr={tr} />;
  }

  // Propagation organ: a branded, shareable card for the report. The overview
  // prose (zh, present when the LLM ran) makes the best subtitle; fall back to
  // the price label for the data-only report. Never carries a fabricated number.
  const overviewProse = data.sections.find(s => s.key === 'overview')?.prose;
  const shareCard: OgParams = {
    kind: 'page',
    eyebrow: lang === 'en' ? 'Deep Research' : '深度研报',
    title: data.name || data.ticker,
    subtitle: (overviewProse || data.price_label || '').slice(0, 110) || undefined,
  };

  return (
    <div className="tw-fade">
      {/* mandatory top label + AI badge */}
      <div className={cx('mb-4 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        <div className="mb-1.5 flex flex-wrap items-center gap-2">
          <h2 className={cx('flex items-center gap-1.5 text-[15px] font-bold', t.text)}>
            <Sparkles size={16} className={dark ? 'text-violet-300' : 'text-violet-500'} />
            {tr('research.title')}
          </h2>
          <span
            className={cx(
              'rounded-md px-1.5 py-0.5 text-[10px] font-bold',
              dark ? 'bg-violet-500/15 text-violet-300' : 'bg-violet-50 text-violet-600',
            )}
          >
            {tr('ai.badge')}
          </span>
          <div className="ml-auto flex items-center gap-2">
            {data.as_of && (
              <span className={cx('text-[10.5px]', t.faint)}>
                {tr('research.asOf').replace('{d}', data.as_of)}
              </span>
            )}
            {/* propagation organ: save a branded research card */}
            <ShareCardButton card={shareCard} />
          </div>
        </div>
        {data.price_label && (
          <p className={cx('text-[12px] tabular-nums', t.sub)}>{data.price_label}</p>
        )}
        <p className={cx('mt-1.5 text-[11px] font-medium', t.faint)}>{tr('research.label')}</p>
        {/* data-only note: prose absent (LLM off / over cap / failed) */}
        {!data.llm && (
          <p className={cx('mt-1 text-[11px]', t.faint)}>{tr('research.dataOnly')}</p>
        )}
      </div>

      {/* sections */}
      {data.sections.length === 0 ? (
        <div
          className={cx('rounded-2xl border p-6 text-center text-[13px]', t.card, t.border, t.soft, t.sub)}
        >
          {tr('research.empty')}
        </div>
      ) : (
        <div className="space-y-4">
          {data.sections.map(sec => (
            <Section key={sec.key} sec={sec} dark={dark} t={t} tr={tr} lang={lang} />
          ))}
        </div>
      )}

      {/* mandatory bottom label = the disclaimer field */}
      <p className={cx('mt-4 text-center text-[10.5px]', t.faint)}>
        {data.disclaimer || tr('research.label')}
      </p>
    </div>
  );
}

/** One report section: title + facts grid + prose + citations footer. */
function Section({
  sec,
  dark,
  t,
  tr,
  lang,
}: {
  sec: ResearchSection;
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
  lang: string;
}) {
  const title = lang === 'zh' ? sec.title_zh || sec.title_en : sec.title_en || sec.title_zh;
  return (
    <section
      id={sec.key}
      className={cx('scroll-mt-20 rounded-2xl border p-4', t.card, t.border, t.soft)}
    >
      <h3 className={cx('mb-3 text-[14px] font-bold', t.text)}>{title}</h3>

      {(sec.facts?.length ?? 0) > 0 && (
        <div className="mb-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
          {sec.facts.map(f => (
            <FactCell key={f.key} fact={f} dark={dark} t={t} tr={tr} lang={lang} />
          ))}
        </div>
      )}

      {sec.prose.trim() && <Markdown>{sec.prose}</Markdown>}

      {(sec.citations?.length ?? 0) > 0 && (
        <div className={cx('mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 border-t pt-2.5', t.hair)}>
          <span className={cx('text-[10.5px] font-semibold', t.faint)}>{tr('research.sources')}</span>
          {sec.citations.map((cite, i) => (
            <CitationChip key={i} cite={cite} dark={dark} t={t} />
          ))}
        </div>
      )}
    </section>
  );
}

/** One fact: bilingual label + a value chip (muted "数据不足" chip when not ok). */
function FactCell({
  fact,
  dark,
  t,
  tr,
  lang,
}: {
  fact: ResearchFact;
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
  lang: string;
}) {
  const label = lang === 'zh' ? fact.label_zh || fact.label_en : fact.label_en || fact.label_zh;
  const ok = fact.status === 'ok';
  // Freshness labels (source + as-of) travel with each fact and must be shown.
  const freshness = [fact.source, fact.as_of].filter(Boolean).join(' · ');

  return (
    <div
      className={cx('flex items-start justify-between gap-3 rounded-xl border px-3 py-2', t.hair, t.surf2)}
    >
      <div className="min-w-0 flex-1">
        <div className={cx('text-[12.5px] font-medium', t.sub)}>{label}</div>
        {freshness && (
          <div className={cx('mt-0.5 truncate text-[10px]', t.faint)} title={freshness}>
            {freshness}
          </div>
        )}
      </div>
      <div className="shrink-0 text-right">
        {ok ? (
          <span className={cx('text-[14px] font-bold tabular-nums', t.text)}>{fact.value}</span>
        ) : (
          <span
            className={cx(
              'inline-flex cursor-help rounded-md px-1.5 py-0.5 text-[11.5px] font-semibold',
              dark ? 'bg-slate-800 text-slate-500' : 'bg-slate-100 text-slate-400',
            )}
            title={fact.reason || tr('research.insufficient')}
          >
            {tr('research.insufficient')}
          </span>
        )}
      </div>
    </div>
  );
}

/** A citation chip: an in-page anchor link, an external link, or a plain label. */
function CitationChip({cite, dark, t}: {cite: ResearchCitation; dark: boolean; t: Tokens}) {
  const cls = cx('inline-flex items-center gap-1 text-[10.5px] hover:underline', dark ? 'text-teal-300' : 'text-teal-600');
  if (cite.anchor) {
    // In-page deep-link to the matching card on this page (e.g. "#fundamentals").
    return (
      <a href={cite.anchor} className={cls}>
        {cite.label}
      </a>
    );
  }
  if (cite.url) {
    const internal = cite.url.startsWith('/');
    if (internal) {
      return (
        <Link href={cite.url} className={cls}>
          {cite.label}
        </Link>
      );
    }
    return (
      <a href={cite.url} target="_blank" rel="noopener noreferrer" className={cls}>
        {cite.label}
        <ExternalLink size={10} />
      </a>
    );
  }
  return <span className={cx('text-[10.5px]', t.faint)}>{cite.label}</span>;
}

/** Labeled, animated placeholder (an LLM call can take a few seconds). */
function ResearchSkeleton({
  dark,
  t,
  tr,
}: {
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
}) {
  return (
    <div className="tw-fade">
      <div className={cx('mb-4 rounded-2xl border p-4', t.card, t.border, t.soft)}>
        <div className="flex flex-wrap items-center gap-2">
          <h2 className={cx('flex items-center gap-1.5 text-[15px] font-bold', t.text)}>
            <Sparkles size={16} className={dark ? 'text-violet-300' : 'text-violet-500'} />
            {tr('research.title')}
          </h2>
          <span className={cx('ml-auto inline-flex items-center gap-1.5 text-[11.5px]', t.sub)}>
            <Loader2 size={13} className="animate-spin" />
            {tr('research.loading')}
          </span>
        </div>
      </div>
      <div className="space-y-4">
        {Array.from({length: 2}).map((_, s) => (
          <section key={s} className={cx('rounded-2xl border p-4', t.card, t.border, t.soft)}>
            <div className={cx('mb-3 h-4 w-24 rounded', t.skel)} />
            <div className="mb-3 grid grid-cols-1 gap-2 sm:grid-cols-2" aria-hidden>
              {Array.from({length: 4}).map((_, i) => (
                <div key={i} className={cx('h-10 rounded-xl', t.skel)} />
              ))}
            </div>
            <div className="space-y-2" aria-hidden>
              <div className={cx('h-3 rounded', t.skel)} style={{width: '90%'}} />
              <div className={cx('h-3 rounded', t.skel)} style={{width: '80%'}} />
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}
