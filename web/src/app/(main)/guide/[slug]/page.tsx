import type {Metadata} from 'next';
import Link from 'next/link';
import {notFound} from 'next/navigation';
import {SITE_URL, langAlternates} from '@/lib/config';
import {GUIDES, guideBySlug} from '@/lib/guides';
import {ogImageMeta} from '@/lib/og';
import {LocalizedTitle} from '@/components/LocalizedTitle';

/** Pre-render every guide at build time. */
export function generateStaticParams() {
  return GUIDES.map(g => ({slug: g.slug}));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{slug: string}>;
}): Promise<Metadata> {
  const {slug} = await params;
  const g = guideBySlug(slug);
  if (!g) return {title: 'Guide'};
  return {
    // English-default tab title (LocalizedTitle swaps zh); Chinese keywords in
    // description/keywords for the targeting.
    title: {absolute: g.titleEn},
    description: g.descZh,
    keywords: g.keywords,
    alternates: langAlternates(`/guide/${g.slug}`),
    openGraph: {
      type: 'article',
      title: g.titleEn,
      description: g.descEn,
      url: `${SITE_URL}/guide/${g.slug}`,
      images: [ogImageMeta({eyebrow: '指南', title: g.h1Zh, subtitle: g.descZh.slice(0, 54)})],
    },
  };
}

/**
 * SEO landing page for one keyword cluster: bilingual explainer + FAQ (with
 * FAQPage structured data) + a CTA into the live board + cross-links. Server-
 * rendered so crawlers get the full content; the inactive language is hidden by
 * the [data-i18n] CSS keyed to <html lang>.
 */
export default async function GuideRoute({params}: {params: Promise<{slug: string}>}) {
  const {slug} = await params;
  const g = guideBySlug(slug);
  if (!g) notFound();

  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'FAQPage',
        mainEntity: g.faq.map(f => ({
          '@type': 'Question',
          name: f.qZh,
          acceptedAnswer: {'@type': 'Answer', text: f.aZh},
        })),
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: SITE_URL},
          {'@type': 'ListItem', position: 2, name: 'Guides', item: `${SITE_URL}/guide`},
          {'@type': 'ListItem', position: 3, name: g.titleEn, item: `${SITE_URL}/guide/${g.slug}`},
        ],
      },
    ],
  };

  const related = g.related.map(guideBySlug).filter((r): r is NonNullable<typeof r> => Boolean(r));

  return (
    <article className="mx-auto max-w-3xl">
      <LocalizedTitle en={g.titleEn} zh={g.titleZh} />
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <nav className="mb-4 text-[12px] text-slate-500 dark:text-slate-400" aria-label="Breadcrumb">
        <Link href="/" className="hover:underline">
          Tickwind
        </Link>
        <span className="mx-1.5">/</span>
        <Link href="/guide" className="hover:underline">
          <span data-i18n="zh">指南</span>
          <span data-i18n="en">Guides</span>
        </Link>
      </nav>

      <h1 className="text-[26px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
        <span data-i18n="zh">{g.h1Zh}</span>
        <span data-i18n="en">{g.h1En}</span>
      </h1>

      <div className="mt-4 space-y-3.5">
        {g.bodyZh.map((p, i) => (
          <p
            key={`zh${i}`}
            data-i18n="zh"
            className="text-[14px] leading-relaxed text-slate-600 dark:text-slate-300"
          >
            {p}
          </p>
        ))}
        {g.bodyEn.map((p, i) => (
          <p
            key={`en${i}`}
            data-i18n="en"
            className="text-[14px] leading-relaxed text-slate-600 dark:text-slate-300"
          >
            {p}
          </p>
        ))}
      </div>

      <div className="mt-6">
        <Link
          href={g.cta.href}
          className="inline-flex items-center gap-1.5 rounded-full bg-teal-600 px-5 py-2.5 text-[14px] font-semibold text-white hover:bg-teal-700 dark:bg-teal-500 dark:text-slate-950 dark:hover:bg-teal-400"
        >
          <span data-i18n="zh">{g.cta.labelZh}</span>
          <span data-i18n="en">{g.cta.labelEn}</span>
          <span aria-hidden>→</span>
        </Link>
      </div>

      <section className="mt-10 border-t border-slate-200 pt-6 dark:border-slate-800">
        <h2 className="text-[17px] font-bold text-slate-900 dark:text-slate-100">
          <span data-i18n="zh">常见问题</span>
          <span data-i18n="en">FAQ</span>
        </h2>
        <dl className="mt-4 space-y-4">
          {g.faq.map((f, i) => (
            <div key={i}>
              <dt className="text-[14px] font-semibold text-slate-800 dark:text-slate-100">
                <span data-i18n="zh">{f.qZh}</span>
                <span data-i18n="en">{f.qEn}</span>
              </dt>
              <dd className="mt-1 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-400">
                <span data-i18n="zh">{f.aZh}</span>
                <span data-i18n="en">{f.aEn}</span>
              </dd>
            </div>
          ))}
        </dl>
      </section>

      {related.length > 0 && (
        <section className="mt-10 border-t border-slate-200 pt-6 dark:border-slate-800">
          <h2 className="text-[15px] font-bold text-slate-900 dark:text-slate-100">
            <span data-i18n="zh">相关指南</span>
            <span data-i18n="en">Related guides</span>
          </h2>
          <ul className="mt-3 grid gap-2 sm:grid-cols-2">
            {related.map(r => (
              <li key={r.slug}>
                <Link
                  href={`/guide/${r.slug}`}
                  className="block rounded-xl border border-slate-200 p-3 text-[13px] font-semibold text-slate-800 hover:border-teal-400 dark:border-slate-800 dark:text-slate-100"
                >
                  <span data-i18n="zh">{r.h1Zh}</span>
                  <span data-i18n="en">{r.h1En}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      <p className="mt-8 text-[11.5px] text-slate-400 dark:text-slate-500">
        <span data-i18n="zh">数据来自 SEC、FINRA、Cboe 等公开来源,可能延迟,仅供参考,不构成投资建议。</span>
        <span data-i18n="en">
          Data from public sources (SEC, FINRA, Cboe); may be delayed; for information only, not
          investment advice.
        </span>
      </p>
    </article>
  );
}
