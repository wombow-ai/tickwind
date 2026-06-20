import type {CSSProperties} from 'react';

/**
 * Chat-hub palette — the owner's finalized "warm" design (from the Claude Design handoff),
 * light + dark, applied as CSS custom properties on the chat root. Components read them via
 * `var(--token)`. Mirrors the prototype's themeVars; light/dark follows the app theme (useDark).
 *
 * Tokens: bg (canvas) · surface (cards) · surface2 (insets) · border/border2 · text/2/3 ·
 * accent (gold) + accent2 (gold text) + accent-soft/-fill/-line (soft button + chips) ·
 * bubble/-line (the user message bubble) · up/down (gains/losses).
 */
export function chatVars(dark: boolean): CSSProperties {
  const light = {
    '--bg': '#f3f1ee', '--surface': '#ffffff', '--surface2': '#f7f5f2',
    '--border': 'rgba(20,22,28,.09)', '--border2': 'rgba(20,22,28,.15)',
    '--text': '#191c22', '--text2': '#596170', '--text3': '#8b919c',
    '--accent': '#cf9a33', '--accent2': '#9c6f18', '--accent-soft': 'rgba(207,154,51,.12)',
    '--accent-fill': 'rgba(207,154,51,.14)', '--accent-line': 'rgba(207,154,51,.36)',
    '--bubble': '#ffffff', '--bubble-line': 'rgba(20,22,28,.10)',
    '--up': '#1a8f3c', '--down': '#d23b4a',
  };
  const darkv = {
    '--bg': '#0d0f13', '--surface': '#13161c', '--surface2': '#181c23',
    '--border': 'rgba(255,255,255,.07)', '--border2': 'rgba(255,255,255,.12)',
    '--text': '#e9eaee', '--text2': '#9aa1ad', '--text3': '#646b78',
    '--accent': '#e0a948', '--accent2': '#f0c578', '--accent-soft': 'rgba(224,169,72,.14)',
    '--accent-fill': 'rgba(224,169,72,.16)', '--accent-line': 'rgba(224,169,72,.34)',
    '--bubble': 'rgba(255,255,255,.06)', '--bubble-line': 'rgba(255,255,255,.11)',
    '--up': '#3fb950', '--down': '#f0616d',
  };
  return (dark ? darkv : light) as CSSProperties;
}

/** Monospace stack used for tickers / figures, matching the prototype. */
export const CHAT_MONO = "'IBM Plex Mono','SF Mono',ui-monospace,monospace";
