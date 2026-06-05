import type {NextConfig} from 'next';

/**
 * Tickwind web frontend.
 *
 * Configured for a fully static export (`next build` emits `web/out`) so it can
 * be served from Cloudflare Pages. Because there is no Next.js server at
 * runtime, image optimization is disabled and all data fetching happens
 * client-side against `NEXT_PUBLIC_API_BASE`.
 */
const nextConfig: NextConfig = {
  output: 'export',
  images: {unoptimized: true},
  // Emit `dir/index.html` instead of `dir.html` so Cloudflare Pages serves
  // clean URLs (`/stock/` → `/stock/index.html`) without extra rules.
  trailingSlash: true,
};

export default nextConfig;
