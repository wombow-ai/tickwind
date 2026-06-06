import type {MetadataRoute} from 'next';
import {POPULAR_TICKERS, SITE_URL} from '@/lib/config';

/**
 * Sitemap of the public, indexable pages: the board, announcements, and a stock
 * page per popular ticker. Per-user and auth routes are intentionally omitted.
 */
export default function sitemap(): MetadataRoute.Sitemap {
  const staticPages: MetadataRoute.Sitemap = [
    {url: `${SITE_URL}/`, changeFrequency: 'hourly', priority: 1},
    {url: `${SITE_URL}/hot`, changeFrequency: 'hourly', priority: 0.7},
    {url: `${SITE_URL}/opportunities`, changeFrequency: 'daily', priority: 0.7},
    {url: `${SITE_URL}/events`, changeFrequency: 'daily', priority: 0.6},
    {url: `${SITE_URL}/news`, changeFrequency: 'hourly', priority: 0.6},
    {url: `${SITE_URL}/discussion`, changeFrequency: 'hourly', priority: 0.6},
    {url: `${SITE_URL}/announcements`, changeFrequency: 'weekly', priority: 0.5},
  ];
  const stockPages: MetadataRoute.Sitemap = POPULAR_TICKERS.map(ticker => ({
    url: `${SITE_URL}/stock/${ticker}`,
    changeFrequency: 'hourly',
    priority: 0.8,
  }));
  return [...staticPages, ...stockPages];
}
