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
  return `${SITE_URL}/api/og/${p.kind ?? 'page'}?${q.toString()}`;
}

/** OG image descriptor (URL + standard 1200×630 dimensions) for metadata. */
export function ogImageMeta(p: OgParams): {url: string; width: number; height: number} {
  return {url: ogImage(p), width: 1200, height: 630};
}
