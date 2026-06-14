'use client';

import {
  ArrowLeft,
  CalendarClock,
  ExternalLink,
  Loader2,
  Lock,
  Sparkles,
} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useEffect, useState} from 'react';
import {
  ApiError,
  getDeepResearch,
  getResearch,
  type ResearchReportResponse,
  type ResearchSection,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';
import {Markdown} from '@/components/Markdown';
import {
  BullBearList,
  CitationChip,
  FactCell,
  type Tokens,
} from '@/components/ResearchReport';
import {KLineChart} from '@/components/KLineChart';
import {ShareCardButton} from '@/components/ShareCardButton';
import {type OgParams} from '@/lib/og';

/**
 * The state machine for the gated deep-research fetch:
 * - `auth-wait`  — still resolving the Supabase session (don't flash the gate);
 * - `anon`       — not logged in → render the login gate (never calls the API);
 * - `loading`    — authed, deep generation in flight (can take 10–60s);
 * - `ready`      — got a report (prose when `llm`, data-only when not);
 * - `quota`      — 429: today's 1/day generation is spent;
 * - `notfound`   — 404: unknown symbol;
 * - `error`      — network / 5xx / other.
 */
type State = 'auth-wait' | 'anon' | 'loading' | 'ready' | 'quota' | 'notfound' | 'error';

/**
 * The dedicated **AI Deep Research** report view (the gated, login-required deep
 * report). Reached from the AI-Digest module's top-right entry button. Reuses the
 * R2 report's fact/prose/citation/bull-bear pieces ({@link ResearchReport})
 * but lays them out with FIXED styling: an executive overview on top, then each
 * section as a clean Go-fact TABLE + prose + source/原文 links, with the price
 * chart where it clarifies. Bilingual single-locale; never renders a number the
 * LLM invented (the facts are Go-owned).
 *
 * Gating UX (the deep endpoint is auth + 1/day-quota gated):
 * - anon → a tasteful "log in to unlock" card (+ a public-research preview); the
 *   gated endpoint is NEVER called when logged out;
 * - logged-in → fetch `?depth=deep` with the Bearer token, show a clear
 *   generating spinner;
 * - 429 → a friendly "daily limit reached" note;
 * - llm:false / data-only → render the Go-owned facts/tables + a small
 *   "AI analysis unavailable — showing public data" note. Never a broken UI.
 */
export function DeepResearchView({ticker}: {ticker: string}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const {user, loading: authLoading, getToken} = useAuth();

  const [state, setState] = useState<State>('auth-wait');
  const [data, setData] = useState<ResearchReportResponse | null>(null);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    // Wait for the session check before deciding anon vs fetch, so we never
    // flash the login gate to a user who's actually signed in.
    if (authLoading) {
      setState('auth-wait');
      return;
    }
    if (!user) {
      setState('anon'); // do NOT call the gated endpoint when logged out
      return;
    }

    const c = new AbortController();
    let active = true;
    setState('loading');
    (async () => {
      try {
        const token = await getToken();
        const r = await getDeepResearch(ticker, token, lang, c.signal);
        if (!active) return;
        if (!r) {
          setState('notfound');
          return;
        }
        setData(r);
        setState('ready');
      } catch (e) {
        if (!active) return;
        if (c.signal.aborted) return;
        if (e instanceof ApiError && e.status === 401) {
          setState('anon'); // token expired / rejected → treat as logged out
        } else if (e instanceof ApiError && e.status === 429) {
          setState('quota');
        } else {
          setState('error');
        }
      }
    })();
    return () => {
      active = false;
      c.abort();
    };
  }, [ticker, lang, user, authLoading, getToken, reload]);

  // ---- chrome: a header that's shared across every state ----
  const header = <DeepHeader ticker={ticker} dark={dark} t={t} tr={tr} report={data} lang={lang} />;

  if (state === 'auth-wait' || state === 'loading') {
    return (
      <Shell header={header}>
        <DeepLoading dark={dark} t={t} tr={tr} authWait={state === 'auth-wait'} />
      </Shell>
    );
  }

  if (state === 'anon') {
    return (
      <Shell header={header}>
        <DeepGate ticker={ticker} dark={dark} t={t} tr={tr} lang={lang} />
      </Shell>
    );
  }

  if (state === 'quota') {
    return (
      <Shell header={header}>
        <Notice
          tone="amber"
          icon={<CalendarClock size={18} />}
          title={tr('deep.quota.title')}
          body={tr('deep.quota.body')}
          dark={dark}
          t={t}
        />
      </Shell>
    );
  }

  if (state === 'notfound') {
    return (
      <Shell header={header}>
        <div className={cx('rounded-2xl border p-6 text-center text-[13px]', t.card, t.border, t.soft, t.sub)}>
          {tr('research.empty')}
        </div>
      </Shell>
    );
  }

  if (state === 'error' || !data) {
    return (
      <Shell header={header}>
        <div className={cx('rounded-2xl border p-6 text-center', t.card, t.border, t.soft)}>
          <p className={cx('text-[13.5px] font-semibold', t.text)}>{tr('deep.error.title')}</p>
          <button
            onClick={() => setReload(n => n + 1)}
            className={cx('mt-3 rounded-full px-4 py-1.5 text-[12.5px] font-semibold', btnPrimary(dark))}
          >
            {tr('deep.error.retry')}
          </button>
        </div>
      </Shell>
    );
  }

  // ---- ready: the full fixed-styling report ----
  const sections = data.sections ?? [];
  const overview = sections.find(s => s.key === 'overview');
  const body = sections.filter(s => s.key !== 'overview');
  const anchorBase = `/stock/${encodeURIComponent(ticker)}`;

  return (
    <Shell header={header}>
      {/* data-only note: the LLM was off / over cap / failed (no prose) */}
      {!data.llm && (
        <Notice
          tone="slate"
          icon={<Sparkles size={16} />}
          title={tr('research.dataOnly')}
          dark={dark}
          t={t}
        />
      )}

      {/* executive overview at the very top */}
      {overview && (
        <section className={cx('mb-4 rounded-2xl border p-5', t.card, t.border, t.soft)}>
          <h2 className={cx('mb-2 text-[15px] font-bold', t.text)}>{tr('deep.overview')}</h2>
          {overview.prose.trim() ? (
            <Markdown>{overview.prose}</Markdown>
          ) : (
            data.price_label && <p className={cx('text-[12.5px] tabular-nums', t.sub)}>{data.price_label}</p>
          )}
          {((overview.bull?.length ?? 0) > 0 || (overview.bear?.length ?? 0) > 0) && (
            <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
              <BullBearList points={overview.bull ?? []} tone="bull" title={tr('research.bull')} dark={dark} t={t} />
              <BullBearList points={overview.bear ?? []} tone="bear" title={tr('research.bear')} dark={dark} t={t} />
            </div>
          )}
          {(overview.citations?.length ?? 0) > 0 && (
            <Citations citations={overview.citations} anchorBase={anchorBase} dark={dark} t={t} tr={tr} />
          )}
        </section>
      )}

      {/* the report sections (估值/基本面/技术/资金面/情绪面) */}
      <div className="space-y-4">
        {body.length === 0 ? (
          <div className={cx('rounded-2xl border p-6 text-center text-[13px]', t.card, t.border, t.soft, t.sub)}>
            {tr('research.empty')}
          </div>
        ) : (
          body.map(sec => (
            <DeepSection
              key={sec.key}
              sec={sec}
              ticker={ticker}
              anchorBase={anchorBase}
              dark={dark}
              t={t}
              tr={tr}
              lang={lang}
            />
          ))
        )}
      </div>

      {/* mandatory disclaimer */}
      <p className={cx('mt-5 text-center text-[10.5px]', t.faint)}>
        {data.disclaimer || tr('research.label')}
      </p>
    </Shell>
  );
}

/** The page container (constant max width + the shared header above the body). */
function Shell({header, children}: {header: React.ReactNode; children: React.ReactNode}) {
  return (
    <div className="mx-auto max-w-3xl tw-fade">
      {header}
      {children}
    </div>
  );
}

/** The branded report header: back link, AI badge, title, share card, as-of. */
function DeepHeader({
  ticker,
  dark,
  t,
  tr,
  report,
  lang,
}: {
  ticker: string;
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
  report: ResearchReportResponse | null;
  lang: string;
}) {
  const shareCard: OgParams = {
    kind: 'page',
    eyebrow: lang === 'en' ? 'Deep Research' : '深度研报',
    title: report?.name || ticker,
    subtitle:
      (report?.sections?.find(s => s.key === 'overview')?.prose || report?.price_label || '')
        .slice(0, 110) || undefined,
  };
  return (
    <div className="mb-5">
      <Link
        href={`/stock/${encodeURIComponent(ticker)}`}
        className={cx('mb-3 inline-flex items-center gap-1 text-[12.5px] font-medium hover:underline', t.sub)}
      >
        <ArrowLeft size={14} />
        {tr('deep.back').replace('{t}', ticker)}
      </Link>
      <div className={cx('rounded-2xl border p-5', t.card, t.border, t.soft)}>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className={cx('flex items-center gap-1.5 text-[19px] font-bold tracking-tight', t.text)}>
            <Sparkles size={18} className={dark ? 'text-violet-300' : 'text-violet-500'} />
            {tr('deep.title')}
          </h1>
          <span
            className={cx(
              'rounded-md px-1.5 py-0.5 text-[10px] font-bold',
              dark ? 'bg-violet-500/15 text-violet-300' : 'bg-violet-50 text-violet-600',
            )}
          >
            {tr('ai.badge')}
          </span>
          <div className="ml-auto flex items-center gap-2">
            {report?.as_of && (
              <span className={cx('text-[10.5px]', t.faint)}>
                {tr('research.asOf').replace('{d}', report.as_of)}
              </span>
            )}
            <ShareCardButton card={shareCard} />
          </div>
        </div>
        <p className={cx('mt-1.5 text-[12.5px]', t.sub)}>
          {tr('deep.subtitle').replace('{t}', report?.name || ticker)}
        </p>
        {report?.price_label && (
          <p className={cx('mt-1 text-[11.5px] tabular-nums', t.faint)}>{report.price_label}</p>
        )}
      </div>
    </div>
  );
}

/** The generating spinner — the deep call can take 10–60s while the LLM is busy. */
function DeepLoading({
  dark,
  t,
  tr,
  authWait,
}: {
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
  authWait: boolean;
}) {
  return (
    <div className={cx('rounded-2xl border p-6', t.card, t.border, t.soft)}>
      <div className={cx('flex items-center gap-2 text-[14px] font-semibold', t.text)}>
        <Loader2 size={16} className="animate-spin" />
        {tr('deep.generating')}
      </div>
      {!authWait && <p className={cx('mt-1.5 text-[12px]', t.sub)}>{tr('deep.generatingSub')}</p>}
      <div className="mt-4 space-y-4" aria-hidden>
        {Array.from({length: 2}).map((_, s) => (
          <div key={s}>
            <div className={cx('mb-2 h-4 w-28 rounded', t.skel)} />
            <div className="mb-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
              {Array.from({length: 4}).map((_, i) => (
                <div key={i} className={cx('h-9 rounded-lg', t.skel)} />
              ))}
            </div>
            <div className="space-y-1.5">
              <div className={cx('h-3 rounded', t.skel)} style={{width: '92%'}} />
              <div className={cx('h-3 rounded', t.skel)} style={{width: '80%'}} />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

/**
 * The anon login gate: a tasteful value card + a login CTA. Below it, a public
 * (un-gated) research PREVIEW teaser so the page is never empty for anon — the
 * value is visible, the depth is behind the login.
 */
function DeepGate({
  ticker,
  dark,
  t,
  tr,
  lang,
}: {
  ticker: string;
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
  lang: string;
}) {
  const [preview, setPreview] = useState<ResearchReportResponse | null>(null);

  useEffect(() => {
    const c = new AbortController();
    // The PUBLIC research (no depth) is open — a safe teaser above the gate.
    getResearch(ticker, lang, c.signal).then(
      r => setPreview(r),
      () => setPreview(null),
    );
    return () => c.abort();
  }, [ticker, lang]);

  return (
    <>
      <div
        className={cx(
          'rounded-2xl border p-6 text-center',
          t.border,
          dark ? 'bg-teal-500/5' : 'bg-teal-50/70',
        )}
      >
        <span
          className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-2xl"
          style={{background: dark ? 'rgba(20,184,166,.14)' : 'rgba(13,148,136,.1)'}}
        >
          <Lock size={22} className={dark ? 'text-teal-300' : 'text-teal-600'} />
        </span>
        <h2 className={cx('text-[16px] font-bold', t.text)}>{tr('deep.gate.title')}</h2>
        <p className={cx('mx-auto mt-2 max-w-md text-[13px] leading-relaxed', t.sub)}>
          {tr('deep.gate.body').replace('{t}', ticker)}
        </p>
        <Link
          href="/login"
          className={cx(
            'mt-4 inline-flex items-center justify-center rounded-full px-5 py-2 text-[13px] font-semibold',
            btnPrimary(dark),
          )}
        >
          {tr('deep.gate.cta')}
        </Link>
      </div>

      {/* public-research preview teaser (renders only when it has content) */}
      {preview && (preview.sections?.length ?? 0) > 0 && (
        <div className="mt-5">
          <div className={cx('mb-2 flex items-center gap-2 text-[11.5px] font-semibold uppercase tracking-wide', t.faint)}>
            <span>{tr('deep.gate.preview')}</span>
            <span className={cx('h-px flex-1', dark ? 'bg-slate-800' : 'bg-slate-200')} />
          </div>
          {/* Show only the overview prose as a light teaser; the full sectioned
              report stays behind the gate. */}
          {(() => {
            const ov = preview.sections.find(s => s.key === 'overview') ?? preview.sections[0];
            return (
              <div className={cx('rounded-2xl border p-5 opacity-90', t.card, t.border, t.soft)}>
                {ov?.prose?.trim() ? (
                  <Markdown>{ov.prose}</Markdown>
                ) : (
                  <p className={cx('text-[12.5px]', t.sub)}>{preview.price_label}</p>
                )}
                <p className={cx('mt-3 text-[11px]', t.faint)}>{tr('deep.gate.previewNote')}</p>
              </div>
            );
          })()}
        </div>
      )}
    </>
  );
}

/**
 * One report section, fixed styling: title → Go-fact TABLE (metric · value ·
 * source/as-of) → prose → bull/bear (if any) → citations. The price section
 * additionally gets the K-line chart, since price materially clarifies it.
 */
function DeepSection({
  sec,
  ticker,
  anchorBase,
  dark,
  t,
  tr,
  lang,
}: {
  sec: ResearchSection;
  ticker: string;
  anchorBase: string;
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
  lang: string;
}) {
  const title = lang === 'zh' ? sec.title_zh || sec.title_en : sec.title_en || sec.title_zh;
  const facts = sec.facts ?? [];
  // The chart materially clarifies the valuation / technical sections (price
  // history); show it once, on the technical section when present, else valuation.
  const showChart = sec.key === 'technical' || sec.key === 'valuation';

  return (
    <section id={sec.key} className={cx('scroll-mt-20 rounded-2xl border p-5', t.card, t.border, t.soft)}>
      <h2 className={cx('mb-3 text-[15px] font-bold', t.text)}>{title}</h2>

      {/* Go-owned facts as a clean label · value · source/as-of table */}
      {facts.length > 0 && (
        <div className="mb-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
          {facts.map(f => (
            <FactCell key={f.key} fact={f} dark={dark} t={t} tr={tr} lang={lang} />
          ))}
        </div>
      )}

      {/* chart where it clarifies (price), lean — optional polish */}
      {showChart && (
        <div className="mb-3">
          <p className={cx('mb-1.5 text-[11px] font-semibold uppercase tracking-wide', t.faint)}>
            {tr('deep.priceChart')}
          </p>
          <KLineChart ticker={ticker} />
        </div>
      )}

      {sec.prose.trim() && <Markdown>{sec.prose}</Markdown>}

      {((sec.bull?.length ?? 0) > 0 || (sec.bear?.length ?? 0) > 0) && (
        <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
          <BullBearList points={sec.bull ?? []} tone="bull" title={tr('research.bull')} dark={dark} t={t} />
          <BullBearList points={sec.bear ?? []} tone="bear" title={tr('research.bear')} dark={dark} t={t} />
        </div>
      )}

      {(sec.citations?.length ?? 0) > 0 && (
        <Citations citations={sec.citations} anchorBase={anchorBase} dark={dark} t={t} tr={tr} />
      )}
    </section>
  );
}

/** A section's source / 原文 citations footer (reuses the F3 deep-link anchors). */
function Citations({
  citations,
  anchorBase,
  dark,
  t,
  tr,
}: {
  citations: ResearchSection['citations'];
  anchorBase: string;
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
}) {
  return (
    <div className={cx('mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 border-t pt-2.5', t.hair)}>
      <span className={cx('text-[10.5px] font-semibold', t.faint)}>{tr('research.sources')}</span>
      {(citations ?? []).map((cite, i) => (
        <CitationChip key={i} cite={cite} dark={dark} t={t} anchorBase={anchorBase} />
      ))}
    </div>
  );
}

/** A small tinted notice card (data-only / quota), tone-keyed. */
function Notice({
  tone,
  icon,
  title,
  body,
  dark,
  t,
}: {
  tone: 'amber' | 'slate';
  icon: React.ReactNode;
  title: string;
  body?: string;
  dark: boolean;
  t: Tokens;
}) {
  const tint =
    tone === 'amber'
      ? dark
        ? 'bg-amber-500/5'
        : 'bg-amber-50/70'
      : dark
        ? 'bg-slate-800/40'
        : 'bg-slate-50';
  const accent =
    tone === 'amber'
      ? dark
        ? 'text-amber-300'
        : 'text-amber-600'
      : dark
        ? 'text-slate-300'
        : 'text-slate-500';
  return (
    <div className={cx('mb-4 flex items-start gap-3 rounded-2xl border p-4', t.border, tint)}>
      <span className={cx('mt-0.5 shrink-0', accent)}>{icon}</span>
      <div className="min-w-0">
        <p className={cx('text-[13px] font-semibold', t.text)}>{title}</p>
        {body && <p className={cx('mt-0.5 text-[12px] leading-relaxed', t.sub)}>{body}</p>}
      </div>
    </div>
  );
}
