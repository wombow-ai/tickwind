'use client';

import {ArrowRight, CheckCircle2} from 'lucide-react';
import Link from '@/components/LocalLink';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, tok} from '@/lib/ui';

/** Post-checkout success page. The entitlement is synced by the Stripe webhook; this
 * just confirms + sends the user back into the product. */
export default function ProSuccessPage() {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  return (
    <div className={cx('mx-auto max-w-md rounded-3xl border p-8 text-center', t.card, t.border, t.soft)}>
      <CheckCircle2 size={40} className={cx('mx-auto', dark ? 'text-emerald-400' : 'text-emerald-500')} />
      <h1 className={cx('mt-3 text-[20px] font-bold', t.text)}>{tr('pro.success.title')}</h1>
      <p className={cx('mt-1.5 text-[13.5px]', t.sub)}>{tr('pro.success.body')}</p>
      <Link
        href="/"
        className={cx(
          'mt-5 inline-flex items-center gap-1.5 rounded-full px-4 py-2 text-[13px] font-semibold',
          btnPrimary(dark),
        )}
      >
        {tr('pro.success.cta')}
        <ArrowRight size={15} />
      </Link>
    </div>
  );
}
