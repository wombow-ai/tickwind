import type {NextConfig} from 'next';

/**
 * Tickwind web frontend — server-rendered (deployed on Vercel).
 *
 * Public pages (landing, /stock/[ticker], announcements) are server-rendered
 * for SEO; the app fetches from the Go API at NEXT_PUBLIC_API_BASE. Supabase
 * handles auth.
 */
/** Baseline security headers applied to every route. */
const securityHeaders = [
  {key: 'X-Content-Type-Options', value: 'nosniff'},
  {key: 'X-Frame-Options', value: 'SAMEORIGIN'},
  {key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin'},
  {key: 'X-DNS-Prefetch-Control', value: 'on'},
  {
    key: 'Permissions-Policy',
    value: 'camera=(), microphone=(), geolocation=()',
  },
  {
    key: 'Strict-Transport-Security',
    value: 'max-age=63072000; includeSubDomains',
  },
];

const nextConfig: NextConfig = {
  async headers() {
    return [{source: '/:path*', headers: securityHeaders}];
  },
  // Permanent (308) redirects from the pre-IA-merge URLs to their new homes, so
  // existing links, bookmarks and indexed pages keep working after the merge.
  async redirects() {
    return [
      // Unified calendar (earnings · macro · ipo)
      {source: '/earnings', destination: '/calendar/earnings', permanent: true},
      {source: '/events', destination: '/calendar/macro', permanent: true},
      {source: '/ipo', destination: '/calendar/ipo', permanent: true},
      // Personal hub (/me tabs)
      {source: '/watchlist', destination: '/me?tab=watchlist', permanent: true},
      {source: '/portfolio', destination: '/me?tab=holdings', permanent: true},
      {source: '/notes', destination: '/me?tab=notes', permanent: true},
      {source: '/alerts', destination: '/me?tab=alerts', permanent: true},
      // Community folded into the discussion shell
      {source: '/community', destination: '/discussion?tab=community', permanent: true},
    ];
  },
};

export default nextConfig;
