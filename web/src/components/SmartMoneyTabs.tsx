'use client';

import {useState} from 'react';
import {CongressBoard} from '@/components/CongressBoard';
import {InstitutionalBoard} from '@/components/InstitutionalBoard';
import {ThirteenFBoard} from '@/components/ThirteenFBoard';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

export type SmartMoneyTab = 'institutional' | 'congress' | '13f';

/**
 * The merged "smart money" page: SEC 13D/13G institutional/activist stakes and
 * Congress trading disclosures as two tabs of one follow-the-big-money board
 * (they were separate nav pages; merged 2026-06-10 per owner). Each board keeps
 * its own header, filters and data fetching — this is just the switcher.
 */
export function SmartMoneyTabs({initial}: {initial: SmartMoneyTab}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [tab, setTab] = useState<SmartMoneyTab>(initial);

  const tabs: {id: SmartMoneyTab; label: string}[] = [
    {id: '13f', label: tr('13f.tab')},
    {id: 'congress', label: tr('nav.congress')},
    {id: 'institutional', label: tr('nav.institutional')},
  ];

  return (
    <div className="mx-auto max-w-3xl">
      <div
        className={cx(
          'mb-4 inline-flex items-center rounded-xl border p-1 text-[12.5px] font-semibold',
          t.border,
          t.card,
        )}
      >
        {tabs.map(x => (
          <button
            key={x.id}
            onClick={() => {
              setTab(x.id);
              // Keep the URL shareable/bookmarkable without a Next re-render.
              window.history.replaceState(null, '', `/smart-money?tab=${x.id}`);
            }}
            aria-pressed={tab === x.id}
            className={cx(
              'rounded-lg px-3 py-1 transition',
              tab === x.id
                ? dark
                  ? 'bg-slate-700 text-white'
                  : 'bg-slate-900 text-white'
                : t.sub,
            )}
          >
            {x.label}
          </button>
        ))}
      </div>
      {tab === 'institutional' && <InstitutionalBoard />}
      {tab === 'congress' && <CongressBoard />}
      {tab === '13f' && <ThirteenFBoard />}
    </div>
  );
}
