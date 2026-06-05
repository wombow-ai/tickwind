/** Small presentation helpers shared across components. */

/**
 * Formats an RFC 3339 timestamp as a short, human-readable date
 * (e.g. `Jun 4, 2026`). Returns the raw input if it cannot be parsed.
 */
export function formatFiledDate(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

/**
 * Returns an ISO date string (`YYYY-MM-DD`) suitable for a `<time dateTime>`
 * attribute, or `undefined` if the input is not a valid date.
 */
export function toDateTimeAttr(iso: string): string | undefined {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return undefined;
  }
  return date.toISOString().slice(0, 10);
}

/**
 * Formats a price as a fixed two-decimal string with thousands separators
 * (e.g. `1,234.50`). Currency symbols are intentionally omitted; markets carry
 * different currencies and the UI labels them elsewhere.
 */
export function formatPrice(price: number): string {
  return price.toLocaleString('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}
