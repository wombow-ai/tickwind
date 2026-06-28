'use client';

import {useEffect} from 'react';

/**
 * App-root error boundary — the absolute last resort. It only fires when the
 * root/locale layout itself throws (the providers, the <html>/<body> shell),
 * which the segment-level `(main)/error.tsx` cannot catch. It REPLACES the root
 * layout, so it must render its own <html>/<body>, and globals.css / the theme +
 * i18n providers are unavailable here — hence inline styles and English-only
 * copy (the product is English-first; this screen is exceedingly rare).
 */
export default function GlobalError({
  error,
  reset,
}: {
  error: Error & {digest?: string};
  reset: () => void;
}) {
  useEffect(() => {
    if (process.env.NODE_ENV !== 'production') {
      // eslint-disable-next-line no-console
      console.error('[global error]', error);
    }
  }, [error]);

  return (
    <html lang="en">
      <body
        style={{
          margin: 0,
          minHeight: '100vh',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          padding: '24px',
          textAlign: 'center',
          background: '#f8fafc',
          color: '#0f172a',
          fontFamily:
            'ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, Helvetica, Arial, sans-serif',
        }}
      >
        <div style={{maxWidth: 360}}>
          <h1 style={{margin: '0 0 8px', fontSize: 18, fontWeight: 600}}>
            Something went wrong
          </h1>
          <p style={{margin: '0 0 20px', fontSize: 14, color: '#64748b', lineHeight: 1.5}}>
            The page failed to load. Your data is safe — please try again.
          </p>
          <button
            onClick={() => reset()}
            style={{
              cursor: 'pointer',
              border: '1px solid #cbd5e1',
              borderRadius: 9999,
              background: '#0f766e',
              color: '#fff',
              padding: '8px 18px',
              fontSize: 13,
              fontWeight: 600,
            }}
          >
            Try again
          </button>
        </div>
      </body>
    </html>
  );
}
