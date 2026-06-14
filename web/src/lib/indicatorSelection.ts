/**
 * Global indicator-selection model for the per-stock {@link IndicatorsPanel}.
 *
 * The backend computes the FULL available set per stock; the frontend renders a
 * user-chosen SUBSET in a chosen order. The selection is a tiny ordered list of
 * indicator ids — a pure VIEW filter + ordering over the already-computed
 * payload, so it never drops a real value and silently grows as the catalog
 * grows (Phase B). It is a GLOBAL preference (the same indicators across every
 * stock), persisted anonymously to {@link STORAGE_KEY} in localStorage.
 *
 * The source is abstracted behind {@link loadSelection}/{@link saveSelection} so
 * a signed-in server-prefs source can slot in later (a later increment) without
 * touching the panel/picker.
 */

import type {PrefsBlob, StockIndicator} from './api';

/**
 * Versioned localStorage key holding the anonymous selection. Bump the suffix on
 * a shape change so a stale blob is a clean reset rather than a crash.
 */
export const STORAGE_KEY = 'tickwind.indicators.v1';

/** The persisted selection shape: selected ids in display order. */
export interface SelectionBlob {
  ids: string[];
}

/** Display order of the domain groups (technical → fundamental → sentiment). */
const DOMAIN_ORDER: Record<string, number> = {
  technical: 0,
  fundamental: 1,
  sentiment: 2,
};

/**
 * Derives the DEFAULT selection from the fetched payload: every indicator whose
 * `priority === 'P0'`, in domain order (technical, fundamental, sentiment). Within
 * a domain the payload's own order is preserved (the backend already orders ok →
 * insufficient). Deriving from the payload — never a hardcoded list — keeps the
 * default equal to today's panel and correct as the catalog grows. `unsupported`
 * are excluded so the default never offers a non-computable row.
 */
export function defaultSelection(indicators: StockIndicator[]): string[] {
  return indicators
    .map((ind, i) => ({ind, i}))
    .filter(({ind}) => ind.priority === 'P0' && ind.status !== 'unsupported')
    .sort((a, b) => {
      const da = DOMAIN_ORDER[a.ind.domain] ?? 9;
      const db = DOMAIN_ORDER[b.ind.domain] ?? 9;
      if (da !== db) return da - db;
      return a.i - b.i; // stable: keep the payload's intra-domain order
    })
    .map(({ind}) => ind.id);
}

/**
 * Resolves the effective selection for a payload: the saved selection when one
 * exists, otherwise the P0 default. The saved ids are FILTERED to those the
 * payload actually includes (so a removed/renamed id can never linger) and
 * de-duplicated while preserving order. Returns the default when no valid saved
 * id remains, so the panel never renders empty from a stale blob.
 *
 * Abstracted from the storage read ({@link loadSelection}) so a future
 * server-prefs source can supply `saved` instead.
 */
export function resolveSelection(
  indicators: StockIndicator[],
  saved: string[] | null,
): string[] {
  const available = new Set(indicators.map(i => i.id));
  if (saved && saved.length > 0) {
    const seen = new Set<string>();
    const kept = saved.filter(id => {
      if (!available.has(id) || seen.has(id)) return false;
      seen.add(id);
      return true;
    });
    if (kept.length > 0) return kept;
  }
  return defaultSelection(indicators);
}

/**
 * Reads the anonymous saved selection from localStorage. Robust to a missing or
 * malformed blob (try/catch → `null`, the caller then falls back to the default).
 * Returns the raw saved ids (not yet filtered to the payload — that's
 * {@link resolveSelection}'s job).
 */
export function loadSelection(): string[] | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<SelectionBlob> | null;
    if (!parsed || !Array.isArray(parsed.ids)) return null;
    const ids = parsed.ids.filter((x): x is string => typeof x === 'string');
    return ids.length > 0 ? ids : null;
  } catch {
    return null;
  }
}

/**
 * Persists the selection to localStorage as a tiny `{ids}` blob (order = the
 * array). Silently no-ops when storage is unavailable (private mode), so a write
 * failure never breaks the UI.
 */
export function saveSelection(ids: string[]): void {
  if (typeof window === 'undefined') return;
  try {
    const blob: SelectionBlob = {ids};
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(blob));
  } catch {
    // Storage disabled / quota: the selection still applies for this session.
  }
}

/** Clears the saved selection → the panel falls back to the P0 default. */
export function clearSelection(): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Ignore — nothing to clear if storage is unavailable.
  }
}

/**
 * Extracts the selected indicator ids from a server prefs blob's `indicators`
 * sub-key, or `null` when the blob has no (valid) indicator selection. Mirrors
 * {@link loadSelection}'s shape contract so {@link resolveSelection} can consume
 * either source. Defensive against a malformed blob (returns `null`).
 */
export function idsFromPrefs(prefs: PrefsBlob | null | undefined): string[] | null {
  const ids = prefs?.indicators?.ids;
  if (!Array.isArray(ids)) return null;
  const clean = ids.filter((x): x is string => typeof x === 'string');
  return clean.length > 0 ? clean : null;
}

/**
 * Wraps a selection in the namespaced prefs shape the server stores
 * (`{indicators: {ids}}`). Only the `indicators` sub-key is written; the backend
 * shallow-merges, so a future sibling pref key is never clobbered.
 */
export function prefsFromIds(ids: string[]): PrefsBlob {
  return {indicators: {ids}};
}
