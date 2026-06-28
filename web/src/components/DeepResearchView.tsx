'use client';

import {
  ArrowLeft,
  ArrowRight,
  CalendarClock,
  ExternalLink,
  Loader2,
  Lock,
  Printer,
  RefreshCw,
  Sparkles,
} from 'lucide-react';
import Link from '@/components/LocalLink';
import {AnnualSaveHint} from '@/components/AnnualSaveHint';
import {useCallback, useEffect, useState} from 'react';
import {
  ApiError,
  getDeepResearch,
  getResearch,
  type ResearchReportResponse,
  type ResearchSection,
  trackEvent,
} from '@/lib/api';
import {DEMO_REPORT_PATH, isDemoReportTicker} from '@/lib/config';
import {useAuth} from '@/lib/auth';
import {useEntitlement} from '@/lib/entitlement';
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
 * - `loading`    — authed, first fetch in flight (skeleton);
 * - `ready`      — got a report (prose when ready, data-only otherwise); the data
 *                  layer always renders here, the prose affordance is keyed off
 *                  {@link ProseStatus};
 * - `quota`      — 429: the per-user generation quota is spent (no report at all);
 * - `notfound`   — 404: unknown symbol;
 * - `error`      — network / 5xx / other.
 */
type State = 'auth-wait' | 'anon' | 'loading' | 'ready' | 'quota' | 'notfound' | 'error';

/**
 * The prose-generation lifecycle of a `ready` report, driving the inline affordance:
 * - `done`       — full report (prose present OR no `prose_status` from an older,
 *                  synchronous backend OR `llm_disabled`); polling stopped;
 * - `generating` — data-only now, a background generation is in flight → polling;
 * - `slow`       — hit the poll safety cap → stopped polling, offer a manual retry;
 * - `quota`      — `quota_exhausted`: data-only is final, monthly limit note shown.
 */
type ProseStatus = 'done' | 'generating' | 'slow' | 'quota';

/** Re-fetch cadence while a background prose generation is in flight (~4s). */
const POLL_INTERVAL_MS = 4000;
/**
 * Safety cap on automatic polls before offering a manual retry. Must comfortably
 * outlast the BACKEND deep-compose budget (api.llmDeepComposeTimeout=120s) so the
 * UI keeps polling until the report is ready instead of giving up early: a premium
 * Claude model takes ~65s typical and up to ~110s at the token ceiling. 35 × 4s =
 * 140s leaves margin over the 120s backend bound.
 */
const MAX_POLLS = 35;

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
export function DeepResearchView({ticker, inline = false}: {ticker: string; inline?: boolean}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  const {user, loading: authLoading, getToken} = useAuth();
  // Pro entitlement drives the limit-reached upsell: a free user who exhausted their
  // monthly generation sees an "upgrade for on-demand reports" CTA (the conversion
  // moment); a Pro user who somehow hits the high cap just sees the neutral reset note.
  const {isPro} = useEntitlement();

  const [state, setState] = useState<State>('auth-wait');
  const [data, setData] = useState<ResearchReportResponse | null>(null);
  const [prose, setProse] = useState<ProseStatus>('done');
  const [reload, setReload] = useState(0);

  // Manual re-poll trigger for the "taking a while" affordance — bumped to resume
  // polling without re-running the whole effect from scratch (preserves the data).
  const [repoll, setRepoll] = useState(0);
  const onRetryProse = useCallback(() => setRepoll(n => n + 1), []);

  useEffect(() => {
    // Wait for the session check before deciding anon vs fetch, so we never
    // flash the login gate to a user who's actually signed in.
    if (authLoading) {
      setState('auth-wait');
      return;
    }
    if (!user && !isDemoReportTicker(ticker)) {
      setState('anon'); // do NOT call the gated endpoint when logged out (except the demo report)
      return;
    }

    // One AbortController + one timer span this effect run. `active` guards every
    // setState so a late resolve after unmount / ticker / lang change is dropped;
    // cleanup aborts the in-flight fetch AND clears the pending poll timer.
    const c = new AbortController();
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let polls = 0;

    // The first run shows the skeleton; a re-poll keeps the already-rendered
    // data-only report on screen (only `repoll>0` re-enters mid-data).
    if (repoll === 0) setState('loading');

    /**
     * Decide whether the report is final or a background generation is still in
     * flight. BACKWARD-COMPATIBILITY: an older synchronous backend returns the
     * full prose and NO `prose_status` — absent/undefined OR any section already
     * carrying prose ⇒ DONE (no poll), so the current synchronous backend renders
     * the full report immediately during the deploy window (no regression).
     */
    const resolveProse = (r: ResearchReportResponse): ProseStatus => {
      const hasProse = (r.sections ?? []).some(s => (s.prose ?? '').trim().length > 0);
      if (r.prose_status == null || hasProse) return 'done';
      switch (r.prose_status) {
        case 'generating':
          return 'generating';
        case 'quota_exhausted':
          return 'quota';
        // 'ready' (without prose, e.g. an empty report) and 'llm_disabled' are
        // both final, data-only renders.
        case 'ready':
        case 'llm_disabled':
        default:
          return 'done';
      }
    };

    const tick = async () => {
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

        const status = resolveProse(r);
        if (status === 'generating') {
          polls += 1;
          if (polls >= MAX_POLLS) {
            // Safety cap reached — stop polling, offer a manual retry instead of
            // spinning forever.
            setProse('slow');
            return;
          }
          setProse('generating');
          timer = setTimeout(tick, POLL_INTERVAL_MS); // keep polling
        } else {
          setProse(status); // 'done' | 'quota' — terminal, no further poll
        }
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
    };

    void tick();

    return () => {
      active = false;
      c.abort(); // cancel any in-flight fetch
      if (timer) clearTimeout(timer); // clear the pending poll timer
    };
  }, [ticker, lang, user, authLoading, getToken, reload, repoll]);

  // Funnel: a free viewer hit the deep-research Pro wall (the flagship paywall surface).
  useEffect(() => {
    if (data?.paywall_locked) void (async () => trackEvent('paywall_view', 'deep_research', await getToken()))();
  }, [data?.paywall_locked, getToken]);

  // ---- chrome: a header that's shared across every state ----
  const header = (
    <DeepHeader ticker={ticker} dark={dark} t={t} tr={tr} report={data} lang={lang} inline={inline} />
  );

  if (state === 'auth-wait' || state === 'loading') {
    return (
      <Shell header={header} inline={inline}>
        <DeepLoading dark={dark} t={t} tr={tr} authWait={state === 'auth-wait'} />
      </Shell>
    );
  }

  if (state === 'anon') {
    return (
      <Shell header={header} inline={inline}>
        <DeepGate ticker={ticker} dark={dark} t={t} tr={tr} lang={lang} />
      </Shell>
    );
  }

  if (state === 'quota') {
    return (
      <Shell header={header} inline={inline}>
        <Notice
          tone="amber"
          icon={<CalendarClock size={18} />}
          title={tr('deep.quota.title')}
          body={tr('deep.quota.body')}
          cta={isPro ? undefined : <QuotaUpgradeCTA dark={dark} tr={tr} />}
          dark={dark}
          t={t}
        />
      </Shell>
    );
  }

  if (state === 'notfound') {
    return (
      <Shell header={header} inline={inline}>
        <div className={cx('rounded-2xl border p-6 text-center text-[13px]', t.card, t.border, t.soft, t.sub)}>
          {tr('research.empty')}
        </div>
      </Shell>
    );
  }

  if (state === 'error' || !data) {
    return (
      <Shell header={header} inline={inline}>
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
    <Shell header={header} inline={inline}>
      {/* Prominent generating banner at the TOP: the AI prose is composed async, so
          the data sections render instantly while the narrative streams in. Without a
          clear top-of-report cue a (Pro) reader just sees static data then prose
          "pops in" with no explanation — the reported poor experience. */}
      {(prose === 'generating' || prose === 'slow') && (
        <GeneratingBanner status={prose} onRetry={onRetryProse} dark={dark} t={t} tr={tr} />
      )}

      {/* Monthly-limit note: over the per-user quota with no cached prose → the
          data-only report below is final. */}
      {prose === 'quota' && (
        <Notice
          tone="amber"
          icon={<CalendarClock size={18} />}
          title={tr('deep.proseQuota')}
          body={tr('deep.quota.body')}
          cta={isPro ? undefined : <QuotaUpgradeCTA dark={dark} tr={tr} />}
          dark={dark}
          t={t}
        />
      )}

      {/* data-only note: the LLM is off / failed (no prose, and NOT still
          generating — while generating we show the inline affordance instead, and
          while quota-exhausted we show the monthly-limit note above). */}
      {!data.llm && prose === 'done' && (
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
          ) : prose === 'generating' || prose === 'slow' ? (
            <ProseAffordance status={prose} onRetry={onRetryProse} dark={dark} t={t} tr={tr} />
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

      {/* Pro-paywall upsell: free viewers see the overview + one section, the rest
          locked. Honest, value-forward — not a dark pattern. */}
      {data.paywall_locked && <PaywallBanner dark={dark} t={t} tr={tr} />}

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

/**
 * The page container (constant max width + the shared header above the body).
 * `tw-research-print` is the print-scope hook: the @media print rules in
 * globals.css reveal ONLY this subtree (hiding TopNav/Footer/etc.) while the body
 * carries `tw-print-research`, so the browser's "Save as PDF" captures just the
 * report. No effect on normal on-screen rendering.
 */
function Shell({
  header,
  children,
  inline = false,
}: {
  header: React.ReactNode;
  children: React.ReactNode;
  inline?: boolean;
}) {
  // Inline (rendered inside the stock page's Research tab): drop the print-scope hook
  // and the export-only branding footer — both belong to the standalone /research page
  // that owns PDF export. max-w-3xl is also dropped so it fills the tab column.
  return (
    <div className={inline ? 'tw-fade' : 'tw-research-print mx-auto max-w-3xl tw-fade'}>
      {header}
      {children}
      {!inline && (
        <div className="tw-print-only mt-6 border-t border-slate-200 pt-3 text-center text-[11px] text-slate-500">
          Tickwind · tickwind.com — AI Deep Research over public data, not investment advice.
        </div>
      )}
    </div>
  );
}

/**
 * The Pro upsell banner shown on a free-tier (paywall_locked) deep report — honest
 * and value-forward (no dark patterns): names exactly what Pro unlocks, links to /pro.
 */
function PaywallBanner({dark, t, tr}: {dark: boolean; t: Tokens; tr: (key: string) => string}) {
  return (
    <section
      className={cx(
        'mb-4 rounded-2xl border p-5',
        dark ? 'border-violet-500/30 bg-violet-500/[0.06]' : 'border-violet-200 bg-violet-50/60',
      )}
    >
      <h2 className={cx('flex items-center gap-1.5 text-[15px] font-bold', t.text)}>
        <Sparkles size={16} className={dark ? 'text-violet-300' : 'text-violet-500'} />
        {tr('deep.paywall.title')}
      </h2>
      <p className={cx('mt-1.5 text-[12.5px]', t.sub)}>{tr('deep.paywall.body')}</p>
      <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2">
        <Link
          href="/pro"
          className={cx(
            'inline-flex items-center gap-1 rounded-full px-4 py-1.5 text-[12.5px] font-semibold',
            btnPrimary(dark),
          )}
        >
          {tr('deep.paywall.cta')}
          <ArrowRight size={13} />
        </Link>
        {/* Let a prospect read ONE complete report (the AAPL demo) before deciding. */}
        <Link href={DEMO_REPORT_PATH} className={cx('inline-flex items-center gap-1 text-[12.5px] font-semibold', dark ? 'text-violet-300' : 'text-violet-600')}>
          {tr('deep.paywall.demo')}
          <ArrowRight size={13} />
        </Link>
      </div>
      <AnnualSaveHint className="mt-2.5" />
    </section>
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
  inline = false,
}: {
  ticker: string;
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
  report: ResearchReportResponse | null;
  lang: string;
  inline?: boolean;
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
      {/* Back link only on the standalone /research page; in the Research tab it would
          point back to the same page the report already lives in. */}
      {!inline && (
        <Link
          href={`/stock/${encodeURIComponent(ticker)}`}
          className={cx(
            'tw-no-print mb-3 inline-flex items-center gap-1 text-[12.5px] font-medium hover:underline',
            t.sub,
          )}
        >
          <ArrowLeft size={14} />
          {tr('deep.back').replace('{t}', ticker)}
        </Link>
      )}
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
            {/* Export to PDF — dependency-free: tag <body>, let the @media print
                rules (globals.css) show only the report, and the browser's
                "Save as PDF" produces the file (captures charts/tables natively).
                `tw-no-print` keeps the button itself out of the export. */}
            <ExportPdfButton dark={dark} t={t} tr={tr} />
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

/**
 * The "Export PDF" / "导出 PDF" button. SIMPLE + dependency-free: it tags <body>
 * with `tw-print-research`, calls window.print(), and removes the tag on
 * `afterprint`. The @media print rules in globals.css then reveal only the report
 * subtree (`.tw-research-print`) and hide the chrome + this button (`tw-no-print`),
 * so the browser's native "Save as PDF" captures the charts/tables cleanly. A true
 * PNG/image export is deferred (would need html2canvas — heavier; owner: "skip if
 * not simple").
 */
function ExportPdfButton({
  dark,
  t,
  tr,
}: {
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
}) {
  const onExport = useCallback(() => {
    const body = document.body;
    body.classList.add('tw-print-research');
    const cleanup = () => {
      body.classList.remove('tw-print-research');
      window.removeEventListener('afterprint', cleanup);
    };
    window.addEventListener('afterprint', cleanup);
    window.print();
  }, []);

  return (
    <button
      type="button"
      onClick={onExport}
      className={cx(
        'tw-no-print inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[12px] font-semibold transition',
        t.border,
        t.sub,
        dark ? 'hover:bg-slate-800/60' : 'hover:bg-slate-50',
      )}
    >
      <Printer size={13} />
      {tr('deep.exportPdf')}
    </button>
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
 * The INLINE prose affordance shown inside the data-only report while a background
 * AI generation is in flight (the page already renders every Go-owned fact/table;
 * only the qualitative prose is pending):
 * - `generating` — a subtle pulsing spinner + "正在生成深度分析…" while polling;
 * - `slow`       — after the poll safety cap, a manual "生成较慢,点此重试" retry.
 */
function ProseAffordance({
  status,
  onRetry,
  dark,
  t,
  tr,
}: {
  status: 'generating' | 'slow';
  onRetry: () => void;
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
}) {
  if (status === 'slow') {
    return (
      <button
        type="button"
        onClick={onRetry}
        className={cx(
          'inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[12px] font-semibold',
          t.border,
          t.sub,
          dark ? 'hover:bg-slate-800/60' : 'hover:bg-slate-50',
        )}
      >
        <RefreshCw size={13} />
        {tr('deep.proseSlow')}
      </button>
    );
  }
  return (
    <div className={cx('flex animate-pulse items-center gap-2 text-[12.5px] font-medium', t.sub)}>
      <Loader2 size={14} className="animate-spin" />
      {tr('deep.proseGenerating')}
    </div>
  );
}

/**
 * The prominent, full-width banner shown at the TOP of the report while the AI prose
 * is still being composed (async). It tells the reader the narrative is on its way and
 * that the data below is already usable — so the prose no longer silently "pops in".
 * In the 'slow' state (poll cap reached) it offers a keep-waiting retry.
 */
function GeneratingBanner({
  status,
  onRetry,
  dark,
  t,
  tr,
}: {
  status: 'generating' | 'slow';
  onRetry: () => void;
  dark: boolean;
  t: Tokens;
  tr: (key: string) => string;
}) {
  const slow = status === 'slow';
  return (
    <section
      className={cx(
        'mb-4 flex items-center gap-3 rounded-2xl border p-4',
        dark ? 'border-violet-500/30 bg-violet-500/[0.07]' : 'border-violet-200 bg-violet-50/70',
      )}
    >
      {slow ? (
        <RefreshCw size={18} className={cx('shrink-0', dark ? 'text-violet-300' : 'text-violet-500')} />
      ) : (
        <Loader2 size={18} className={cx('shrink-0 animate-spin', dark ? 'text-violet-300' : 'text-violet-500')} />
      )}
      <div className="min-w-0 flex-1">
        <p className={cx('text-[13.5px] font-bold', t.text)}>
          {slow ? tr('deep.gen.slowTitle') : tr('deep.gen.title')}
        </p>
        <p className={cx('mt-0.5 text-[12px] leading-relaxed', t.sub)}>
          {slow ? tr('deep.gen.slowBody') : tr('deep.gen.body')}
        </p>
      </div>
      {slow && (
        <button
          type="button"
          onClick={onRetry}
          className={cx(
            'shrink-0 rounded-full px-3.5 py-1.5 text-[12px] font-semibold',
            btnPrimary(dark),
          )}
        >
          {tr('deep.gen.retry')}
        </button>
      )}
    </section>
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
        <div className="mt-4 flex flex-wrap items-center justify-center gap-2.5">
          <Link
            href="/login"
            className={cx(
              'inline-flex items-center justify-center rounded-full px-5 py-2 text-[13px] font-semibold',
              btnPrimary(dark),
            )}
          >
            {tr('deep.gate.cta')}
          </Link>
          <Link
            href="/pro"
            className={cx(
              'inline-flex items-center justify-center gap-1 rounded-full border px-4 py-2 text-[13px] font-semibold',
              dark
                ? 'border-violet-500/40 text-violet-300 hover:bg-violet-500/10'
                : 'border-violet-300 text-violet-600 hover:bg-violet-50',
            )}
          >
            {tr('deep.paywall.cta')}
            <ArrowRight size={13} />
          </Link>
        </div>
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
// lockedTeasers maps a report section key → a dict key for a ONE-LINE, number-free,
// structural description of what that section contains. Shown on the locked (free-tier)
// card so the Pro value is legible at the point of friction — "a locked box labeled with
// what's inside" rather than a bare lock. Purely structural (no figures/advice) →
// anti-hallucination-safe. A key with no entry simply shows no teaser.
const lockedTeasers: Record<string, string> = {
  valuation: 'deep.teaser.valuation',
  fundamentals: 'deep.teaser.fundamentals',
  technical: 'deep.teaser.technical',
  relative: 'deep.teaser.relative',
  flows: 'deep.teaser.flows',
  sentiment: 'deep.teaser.sentiment',
};

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

  // Pro-paywall: a locked section shows only its title + an unlock hint (the Go
  // backend already stripped its prose/facts for the free tier).
  if (sec.locked) {
    // The locked card IS the upgrade affordance — clickable straight to /pro (the
    // most natural click target at the point of friction).
    return (
      <Link
        href="/pro"
        id={sec.key}
        className={cx(
          'group block scroll-mt-20 rounded-2xl border border-dashed p-5 transition',
          t.card,
          t.border,
          t.soft,
          dark
            ? 'hover:border-violet-500/50 hover:bg-violet-500/[0.05]'
            : 'hover:border-violet-300 hover:bg-violet-50/60',
        )}
      >
        <h2 className={cx('flex items-center gap-1.5 text-[15px] font-bold', t.sub)}>
          <Lock size={14} className={dark ? 'text-violet-300' : 'text-violet-500'} />
          {title}
        </h2>
        {lockedTeasers[sec.key] && (
          <p className={cx('mt-1 text-[12.5px]', t.faint)}>{tr(lockedTeasers[sec.key])}</p>
        )}
        <p
          className={cx(
            'mt-1.5 flex items-center gap-1 text-[12.5px] font-semibold',
            dark ? 'text-violet-300' : 'text-violet-600',
          )}
        >
          {tr('deep.locked.section')}
          <ArrowRight size={13} className="transition group-hover:translate-x-0.5" />
        </p>
      </Link>
    );
  }

  const facts = sec.facts ?? [];
  // The price K-line clarifies the technical section; show it ONCE there (the
  // technical section is always emitted). Was `technical || valuation`, which
  // rendered the identical chart TWICE — once per section.
  const showChart = sec.key === 'technical';

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

/** A small tinted notice card (data-only / quota), tone-keyed. An optional `cta` renders
 *  below the body — used to attach the Pro upsell to the monthly-limit notice. */
function Notice({
  tone,
  icon,
  title,
  body,
  cta,
  dark,
  t,
}: {
  tone: 'amber' | 'slate';
  icon: React.ReactNode;
  title: string;
  body?: string;
  cta?: React.ReactNode;
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
        {cta && <div className="mt-2.5">{cta}</div>}
      </div>
    </div>
  );
}

/**
 * The Pro upsell button shown on the monthly-limit notice to a NON-Pro user — the prime
 * conversion moment (they wanted another report this month). Links to /pro; honest framing:
 * Pro lifts the per-month generation cap to on-demand. Hidden for Pro viewers.
 */
function QuotaUpgradeCTA({dark, tr}: {dark: boolean; tr: (key: string) => string}) {
  return (
    <Link
      href="/pro"
      className={cx(
        'inline-flex items-center gap-1 rounded-full px-3.5 py-1.5 text-[12px] font-semibold',
        btnPrimary(dark),
      )}
    >
      <Sparkles size={13} />
      {tr('deep.quota.upgrade')}
      <ArrowRight size={13} />
    </Link>
  );
}
