import type {MetadataRoute} from 'next';
import {SITE_URL} from '@/lib/config';

/** robots.txt — allow crawling of public pages; point to the sitemap. */
export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: '*',
      allow: '/',
      // Auth + personal areas aren't useful to crawl. Pages are now locale-
      // prefixed (`/en/login`, `/zh/login`, …), so list both locales; `/auth`
      // (the non-localized OAuth callback) stays bare.
      disallow: [
        '/en/login',
        '/zh/login',
        '/en/signup',
        '/zh/signup',
        '/en/settings',
        '/zh/settings',
        '/en/designs',
        '/zh/designs',
        '/auth',
      ],
    },
    sitemap: `${SITE_URL}/sitemap.xml`,
    host: SITE_URL,
  };
}
