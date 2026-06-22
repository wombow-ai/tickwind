import snapshotData from '@/data/topicsSnapshot.json';
import type {HotTopic, TopicsResponse} from '@/lib/api';

/**
 * A BUNDLED snapshot of the trending-topics set, read synchronously with no network call.
 *
 * Why bundled: the trending-topic pSEO pages (`/topic/[key]`) are SERVER-rendered for crawlers,
 * but Vercel's build AND runtime fetch of `/v1/topics` through the Cloudflare tunnel is
 * unreliable — the cold hop intermittently returns a 200 with an EMPTY body. On-demand rendering
 * did NOT fix it: the first runtime render that hit the empty reply called `notFound()`, and Next
 * then cached that 404 for the whole `revalidate` window (~30 min) — so every page stayed blank.
 * Same class as the indicator catalog (which we bundled in `lib/catalog.ts`).
 *
 * The topic THEMES (semis / AI / crypto / IPO / …) and their representative tickers are stable
 * enough for a pSEO landing page, so a bundled snapshot guarantees the pages always render real
 * content server-side. The page + sitemap still PREFER a live `getTopics()` (fresher) and only
 * fall back to this snapshot when the live fetch is empty/fails — so when the backend is
 * reachable the pages stay current, and they are NEVER blank.
 *
 * This module is server-only (imported by the topic page + sitemap), so the JSON stays out of the
 * client bundle. The client `TopicsStrip` keeps using `getTopics` (a browser fetch, which reaches
 * api.tickwind.com directly via CORS and is unaffected by the Vercel-runtime tunnel limitation).
 *
 * REGENERATE on a meaningful topic-set change:
 *   curl -s https://api.tickwind.com/v1/topics > web/src/data/topicsSnapshot.json
 */
const SNAPSHOT = snapshotData as unknown as TopicsResponse;

/** The bundled topics snapshot — sync, never throws. */
export function localTopics(): TopicsResponse {
  return SNAPSHOT;
}

/** The bundled topic for a key, or null if the key isn't in the snapshot. */
export function localTopic(key: string): HotTopic | null {
  return SNAPSHOT.topics.find(t => t.key === key) ?? null;
}

/** All bundled topic keys (drives the prerendered set so every known page bakes with content). */
export function localTopicKeys(): string[] {
  return SNAPSHOT.topics.map(t => t.key);
}
