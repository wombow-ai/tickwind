import type {MetadataRoute} from 'next';
import {SITE_URL} from '@/lib/config';

/** robots.txt — allow crawling of public pages; point to the sitemap. */
export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: '*',
      allow: '/',
      // Auth + personal areas aren't useful to crawl.
      disallow: ['/login', '/signup', '/auth', '/settings', '/designs'],
    },
    sitemap: `${SITE_URL}/sitemap.xml`,
    host: SITE_URL,
  };
}
