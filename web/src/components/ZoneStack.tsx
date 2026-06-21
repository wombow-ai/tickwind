'use client';

import {Lock} from 'lucide-react';
import {type Zone} from '@/lib/zones';

// A funnel / "layer-cake" overview of an AI-flagship zone (e.g. Jensen's five-layer cake):
// the curated stack from the foundation (widest band, bottom) up to value capture (narrowest,
// top). Tap a band to jump to that layer's detail below. Pure editorial structure — it carries
// NO price or number (those live in the live ticker cards), so it's anti-hallucination-safe.

/** "Energy — Power & Cooling" → "Energy"; "能源 —— 供电与散热" → "能源". */
function shortTitle(s: string): string {
  return s.split('—')[0].trim();
}

export function ZoneStack({zone, zh}: {zone: Zone; zh: boolean}) {
  const layers = zone.layers;
  const n = layers.length;
  if (n < 2) return null;
  // Display top→bottom = last layer (value capture) on top, first (foundation) at the base.
  const rows = layers.map((layer, i) => ({layer, i})).reverse();
  return (
    <div className="mb-6">
      <div className="flex flex-col items-center gap-1.5">
        {rows.map(({layer, i}, r) => {
          const widthPct = 56 + (100 - 56) * (r / (n - 1)); // top narrow → bottom wide (funnel)
          const shade = 0.95 - 0.4 * (r / (n - 1)); // top vivid → base softer
          return (
            <a
              key={layer.key}
              href={`#layer-${layer.key}`}
              title={zh ? layer.titleZh : layer.titleEn}
              style={{width: `${widthPct}%`, background: `rgba(124,58,237,${shade.toFixed(3)})`}}
              className="group flex items-center justify-center gap-1.5 rounded-lg px-3 py-2.5 text-center shadow-sm transition hover:brightness-110 hover:-translate-y-px"
            >
              <span className="truncate text-[12.5px] font-semibold text-white">
                {i + 1}. {zh ? shortTitle(layer.titleZh) : shortTitle(layer.titleEn)}
              </span>
              {layer.chokepoint && <Lock size={11} className="shrink-0 text-amber-200" />}
            </a>
          );
        })}
      </div>
      <p className="mt-2 text-center text-[11px] text-slate-400 dark:text-slate-500">
        {zh
          ? '价值自下而上 —— 供电/算力是地基,模型与应用捕获更高溢价 · 点击跳到对应层'
          : 'Value flows up — power & compute are the base; models & apps capture the higher multiple · tap a layer'}
      </p>
    </div>
  );
}
