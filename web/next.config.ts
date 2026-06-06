import type {NextConfig} from 'next';

/**
 * Tickwind web frontend — server-rendered (deployed on Vercel).
 *
 * Public pages (landing, /stock/[ticker], announcements) are server-rendered
 * for SEO; the app fetches from the Go API at NEXT_PUBLIC_API_BASE. Supabase
 * handles auth.
 */
const nextConfig: NextConfig = {};

export default nextConfig;
