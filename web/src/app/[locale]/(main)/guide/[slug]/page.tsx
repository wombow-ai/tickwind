import type {Metadata} from 'next';
import Link from '@/components/LocalLink';
import {notFound} from 'next/navigation';
import {SITE_URL, langAlternates} from '@/lib/config';
import {GUIDES, guideBySlug} from '@/lib/guides';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';

/** Pre-render every guide × locale at build time. */
export function generateStaticParams() {
  return LOCALES.flatMap(locale => GUIDES.map(g => ({locale, slug: g.slug})));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string; slug: string}>;
}): Promise<Metadata> {
  const {locale, slug} = await params;
  const g = guideBySlug(slug);
  if (!g) return {title: 'Guide'};
  const loc = isLocale(locale) ? locale : 'en';
  return {
    // Locale-matched tab title; Chinese keywords in description/keywords.
    title: {absolute: loc === 'zh' ? g.titleZh : g.titleEn},
    description: g.descZh,
    keywords: g.keywords,
    alternates: langAlternates(`/guide/${g.slug}`, loc),
    openGraph: {
      type: 'article',
      title: loc === 'zh' ? g.titleZh : g.titleEn,
      description: loc === 'zh' ? g.descZh : g.descEn,
      url: `${SITE_URL}/${loc}/guide/${g.slug}`,
      images: [
        ogImageMeta({
          lang: loc,
          eyebrow: loc === 'zh' ? '指南' : 'Guide',
          title: loc === 'zh' ? g.h1Zh : g.h1En,
          subtitle: (loc === 'zh' ? g.descZh : g.descEn).slice(0, 54),
        }),
      ],
    },
  };
}

/**
 * SEO landing page for one keyword cluster: a single-locale explainer + FAQ
 * (with FAQPage structured data) + a CTA into the live board + cross-links.
 * Server-rendered so crawlers get the full content; only the active locale's
 * copy (chosen from the route segment) is emitted, so /en and /zh are distinct.
 */
export default async function GuideRoute({
  params,
}: {
  params: Promise<{locale: string; slug: string}>;
}) {
  const {locale, slug} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const g = guideBySlug(slug);
  if (!g) notFound();
  const zh = loc === 'zh';

  const ld = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'FAQPage',
        mainEntity: g.faq.map(f => ({
          '@type': 'Question',
          name: zh ? f.qZh : f.qEn,
          acceptedAnswer: {'@type': 'Answer', text: zh ? f.aZh : f.aEn},
        })),
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          {'@type': 'ListItem', position: 1, name: 'Tickwind', item: `${SITE_URL}/${loc}`},
          {'@type': 'ListItem', position: 2, name: zh ? '指南' : 'Guides', item: `${SITE_URL}/${loc}/guide`},
          {'@type': 'ListItem', position: 3, name: zh ? g.titleZh : g.titleEn, item: `${SITE_URL}/${loc}/guide/${g.slug}`},
        ],
      },
    ],
  };

  const related = g.related.map(guideBySlug).filter((r): r is NonNullable<typeof r> => Boolean(r));

  return (
    <article className="mx-auto max-w-3xl">
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <nav className="mb-4 text-[12px] text-slate-500 dark:text-slate-400" aria-label="Breadcrumb">
        <Link href="/" className="hover:underline">
          Tickwind
        </Link>
        <span className="mx-1.5">/</span>
        <Link href="/guide" className="hover:underline">
          {zh ? '指南' : 'Guides'}
        </Link>
      </nav>

      <h1 className="text-[26px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
        {zh ? g.h1Zh : g.h1En}
      </h1>

      <div className="mt-4 space-y-3.5">
        {(zh ? g.bodyZh : g.bodyEn).map((p, i) => (
          <p
            key={i}
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
          {zh ? g.cta.labelZh : g.cta.labelEn}
          <span aria-hidden>→</span>
        </Link>
      </div>

      <section className="mt-10 border-t border-slate-200 pt-6 dark:border-slate-800">
        <h2 className="text-[17px] font-bold text-slate-900 dark:text-slate-100">
          {zh ? '常见问题' : 'FAQ'}
        </h2>
        <dl className="mt-4 space-y-4">
          {g.faq.map((f, i) => (
            <div key={i}>
              <dt className="text-[14px] font-semibold text-slate-800 dark:text-slate-100">
                {zh ? f.qZh : f.qEn}
              </dt>
              <dd className="mt-1 text-[13.5px] leading-relaxed text-slate-600 dark:text-slate-400">
                {zh ? f.aZh : f.aEn}
              </dd>
            </div>
          ))}
        </dl>
      </section>

      {related.length > 0 && (
        <section className="mt-10 border-t border-slate-200 pt-6 dark:border-slate-800">
          <h2 className="text-[15px] font-bold text-slate-900 dark:text-slate-100">
            {zh ? '相关指南' : 'Related guides'}
          </h2>
          <ul className="mt-3 grid gap-2 sm:grid-cols-2">
            {related.map(r => (
              <li key={r.slug}>
                <Link
                  href={`/guide/${r.slug}`}
                  className="block rounded-xl border border-slate-200 p-3 text-[13px] font-semibold text-slate-800 hover:border-teal-400 dark:border-slate-800 dark:text-slate-100"
                >
                  {zh ? r.h1Zh : r.h1En}
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      <p className="mt-8 text-[11.5px] text-slate-400 dark:text-slate-500">
        {zh
          ? '数据来自 SEC、FINRA、Cboe 等公开来源,可能延迟,仅供参考,不构成投资建议。'
          : 'Data from public sources (SEC, FINRA, Cboe); may be delayed; for information only, not investment advice.'}
      </p>
    </article>
  );
}
