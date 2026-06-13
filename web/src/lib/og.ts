import {SITE_URL} from '@/lib/config';

/** Params for the dynamic share-card / OG image (rendered by /api/og/[kind]). */
export interface OgParams {
  kind?: 'page' | 'stock';
  /** Small teal label above the title (e.g. a section name). */
  eyebrow?: string;
  title: string;
  subtitle?: string;
  /** Optional big figure (e.g. a price or %); pair with `tone` for color. */
  stat?: string;
  tone?: 'up' | 'down';
  /** Footer tagline; defaults to the product's pillars. */
  tag?: string;
  /** UI language for the card chrome (brand badge + default footer tag). Defaults to 'zh' on the route. */
  lang?: 'zh' | 'en';
}

/**
 * Builds the absolute URL of the dynamic branded card for a page or share image.
 * Used for `metadata.openGraph.images` (rich link previews on X/Telegram/微信)
 * and for one-tap "save image" share buttons. Absolute (not relative) so it's
 * valid in OG tags consumed by external crawlers.
 */
export function ogImage(p: OgParams): string {
  const q = new URLSearchParams();
  if (p.eyebrow) q.set('eyebrow', p.eyebrow);
  q.set('title', p.title);
  if (p.subtitle) q.set('subtitle', p.subtitle);
  if (p.stat) q.set('stat', p.stat);
  if (p.tone) q.set('tone', p.tone);
  if (p.tag) q.set('tag', p.tag);
  if (p.lang) q.set('lang', p.lang);
  return `${SITE_URL}/api/og/${p.kind ?? 'page'}?${q.toString()}`;
}

/** OG image descriptor (URL + dimensions) for metadata. The card is rendered at
 * 2× the 1200×630 design (= 2400×1260) for crisp output; the 1.91:1 ratio is the
 * standard OG aspect, and crawlers downscale as needed. */
export function ogImageMeta(p: OgParams): {url: string; width: number; height: number} {
  return {url: ogImage(p), width: 2400, height: 1260};
}
