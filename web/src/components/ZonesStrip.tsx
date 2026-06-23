'use client';

import {Layers} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useLang, useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';
import {ZONES} from '@/lib/zones';

/**
 * The Theme Zones strip — a horizontally-scrollable row of CURATED, evergreen
 * investment-theme chips (The AI Stack, Quantum, GLP-1 …), each linking to its
 * `/zone/{key}` page.
 *
 * Deliberately sits right below the Hot Topics strip but is framed distinctly:
 * Hot Topics are DYNAMIC, news-derived trending keywords (today's buzz, with live
 * article counts); Theme Zones are STATIC, editor-curated supply-chain maps. Showing
 * both side by side with a different icon/accent (Layers/violet vs Flame/amber) makes
 * the distinction obvious — and lifts the zones out of the More▾ menu where they were
 * buried. Pure editorial structure; no market numbers here (the /zone pages fetch
 * those live).
 */
export function ZonesStrip() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const {lang} = useLang();
  if (ZONES.length === 0) return null;
  return (
    <div className="mb-6">
      <div className="mb-2 flex items-center gap-1.5">
        <Layers size={14} className={dark ? 'text-violet-300' : 'text-violet-500'} />
        <span className={cx('text-[12px] font-semibold uppercase tracking-wide', t.sub)}>
          {tr('nav.zones')}
        </span>
      </div>
      <div className="-mx-1 flex gap-2 overflow-x-auto px-1 pb-1">
        {ZONES.map(z => (
          <Link
            key={z.key}
            href={`/zone/${z.key}`}
            className={cx(
              'inline-flex shrink-0 items-center rounded-full border px-3 py-1.5 text-[12.5px] font-medium transition hover:opacity-80',
              t.border,
              dark ? 'bg-slate-900' : 'bg-white',
              t.text,
            )}
          >
            {lang === 'zh' ? z.titleZh : z.titleEn}
          </Link>
        ))}
      </div>
    </div>
  );
}
