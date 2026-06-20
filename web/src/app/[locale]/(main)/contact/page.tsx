import type {Metadata} from 'next';
import {Mail, MessageSquare} from 'lucide-react';
import {SITE_URL, langAlternates} from '@/lib/config';
import {isLocale, LOCALES} from '@/lib/locale';
import {ogImageMeta} from '@/lib/og';

// The single support address. Cloudflare Email Routing forwards it (+ catch-all) to the
// owner's inbox. Keep this in one place so it's easy to change.
const SUPPORT_EMAIL = 'support@tickwind.com';

export function generateStaticParams(): {locale: string}[] {
  return LOCALES.map(locale => ({locale}));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{locale: string}>;
}): Promise<Metadata> {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';
  const title = zh ? '联系我们' : 'Contact Us';
  const desc = zh
    ? `有问题、反馈或合作?发邮件到 ${SUPPORT_EMAIL},我们会认真阅读每一封。`
    : `Questions, feedback, or anything else? Email ${SUPPORT_EMAIL} — we read every message.`;
  return {
    title: {absolute: `${title} · Tickwind`},
    description: desc,
    alternates: langAlternates('/contact', loc),
    openGraph: {
      type: 'website',
      title,
      description: desc,
      url: `${SITE_URL}/${loc}/contact`,
      images: [ogImageMeta({lang: loc, eyebrow: zh ? '联系' : 'Contact', title, subtitle: SUPPORT_EMAIL})],
    },
  };
}

/** Contact page — a single support email (Cloudflare-routed), EN-first + zh. */
export default async function ContactRoute({params}: {params: Promise<{locale: string}>}) {
  const {locale} = await params;
  const loc = isLocale(locale) ? locale : 'en';
  const zh = loc === 'zh';

  const ld = {
    '@context': 'https://schema.org',
    '@type': 'ContactPage',
    name: zh ? '联系 Tickwind' : 'Contact Tickwind',
    url: `${SITE_URL}/${loc}/contact`,
    mainEntity: {'@type': 'Organization', name: 'Tickwind', email: SUPPORT_EMAIL, url: SITE_URL},
  };

  return (
    <article className="mx-auto max-w-2xl">
      <script type="application/ld+json" dangerouslySetInnerHTML={{__html: JSON.stringify(ld)}} />

      <header className="mb-6">
        <h1 className="flex items-center gap-2 text-[24px] font-bold tracking-tight text-slate-900 dark:text-slate-100">
          <MessageSquare size={20} className="text-teal-600 dark:text-teal-300" />
          {zh ? '联系我们' : 'Contact Us'}
        </h1>
        <p className="mt-2 text-[14px] leading-relaxed text-slate-600 dark:text-slate-300">
          {zh
            ? '有任何问题、反馈、数据纠错或合作意向,欢迎随时联系。我们会认真阅读每一封邮件。'
            : 'Questions, feedback, a data correction, or a partnership idea — reach out anytime. We read every message.'}
        </p>
      </header>

      <a
        href={`mailto:${SUPPORT_EMAIL}`}
        className="flex items-center gap-3 rounded-2xl border border-slate-200 bg-white p-4 transition hover:border-teal-300 hover:shadow-sm dark:border-slate-800 dark:bg-slate-950 dark:hover:border-teal-500/40"
      >
        <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-teal-50 text-teal-600 dark:bg-teal-500/15 dark:text-teal-300">
          <Mail size={20} />
        </span>
        <span className="min-w-0">
          <span className="block text-[12px] font-semibold uppercase tracking-wide text-slate-400 dark:text-slate-500">
            {zh ? '邮箱' : 'Email'}
          </span>
          <span className="block truncate text-[16px] font-bold text-slate-900 dark:text-slate-100">{SUPPORT_EMAIL}</span>
        </span>
      </a>

      <p className="mt-4 text-[12.5px] leading-relaxed text-slate-500 dark:text-slate-400">
        {zh
          ? '同一邮箱也用于隐私、版权(DMCA)与法律事务。我们通常在 1–2 个工作日内回复。'
          : 'The same address handles privacy, copyright (DMCA), and legal matters. We typically reply within 1–2 business days.'}
      </p>

      <p className="mt-8 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh ? '数据延迟 · 仅供参考 · 非投资建议' : 'Delayed data · for reference only · not investment advice'}
      </p>
    </article>
  );
}
