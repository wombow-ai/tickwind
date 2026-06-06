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
};

export default nextConfig;
