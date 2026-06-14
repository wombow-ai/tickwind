import {createServerClient} from '@supabase/ssr';
import {type NextRequest, NextResponse} from 'next/server';
import {DEFAULT_LOCALE, isLocale} from '@/lib/locale';

/**
 * Picks the locale for a bare (un-prefixed) path: the `tw-lang` cookie wins, else
 * an `Accept-Language` that starts with `zh`, else the default (`en`).
 */
function detectLocale(request: NextRequest): 'en' | 'zh' {
  const cookie = request.cookies.get('tw-lang')?.value;
  if (isLocale(cookie)) return cookie;
  const accept = request.headers.get('accept-language') ?? '';
  if (accept.trim().toLowerCase().startsWith('zh')) return 'zh';
  return DEFAULT_LOCALE;
}

/**
 * Next 16's `proxy` (formerly `middleware`). Two responsibilities, in order:
 *
 *   1. Locale routing — a bare page path (no `/en` or `/zh` prefix) is 308-
 *      redirected to the detected locale. Non-page paths (sitemap, robots,
 *      `/api/*`, `/auth/*`, files with extensions) are NEVER prefixed. Already-
 *      prefixed paths fall straight through, so there is no redirect loop.
 *   2. Supabase session refresh on every (non-redirected) request, so Server
 *      Components see a current user and cookies stay fresh. No-ops without the
 *      Supabase env vars.
 */
export async function proxy(request: NextRequest) {
  const {pathname, search} = request.nextUrl;

  // Paths that must stay un-localized. The `proxy` matcher already excludes
  // `_next` static + image files; this also guards sitemap/robots/api/auth and
  // real static assets. A STATIC-ASSET ALLOWLIST (not a blanket trailing-dot
  // test) is used so dotted tickers like `/stock/BRK.B` and `/stock/0700.HK`
  // are still locale-prefixed (a bare visit must 308 → `/en/stock/BRK.B`).
  const isExcluded =
    pathname.startsWith('/api/') ||
    pathname.startsWith('/auth/') ||
    pathname === '/sitemap.xml' ||
    pathname === '/robots.txt' ||
    pathname === '/favicon.ico' ||
    /\.(?:txt|xml|json|webmanifest|map|css|js|mjs|woff2?|ttf|otf|eot|ico|png|jpe?g|gif|svg|webp|avif|mp4|webm|pdf)$/i.test(
      pathname,
    );

  // The leading path segment — if it's a locale, the path is already prefixed.
  const firstSeg = pathname.split('/', 2)[1] ?? '';
  const hasLocale = isLocale(firstSeg);

  if (!isExcluded && !hasLocale) {
    const locale = detectLocale(request);
    const url = request.nextUrl.clone();
    url.pathname = `/${locale}${pathname === '/' ? '' : pathname}`;
    // search is preserved by cloning nextUrl; set explicitly for clarity.
    url.search = search;
    return NextResponse.redirect(url, 308);
  }

  let response = NextResponse.next({request});

  const url = process.env.NEXT_PUBLIC_SUPABASE_URL ?? '';
  const anonKey = process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY ?? '';
  if (!url || !anonKey) {
    return response;
  }

  const supabase = createServerClient(url, anonKey, {
    cookies: {
      getAll() {
        return request.cookies.getAll();
      },
      setAll(cookiesToSet) {
        for (const {name, value} of cookiesToSet) {
          request.cookies.set(name, value);
        }
        response = NextResponse.next({request});
        for (const {name, value, options} of cookiesToSet) {
          response.cookies.set(name, value, options);
        }
      },
    },
  });

  // Touch the user to trigger a token refresh when needed.
  await supabase.auth.getUser();
  return response;
}

export const config = {
  matcher: [
    // Run on all paths except static assets and image files.
    '/((?!_next/static|_next/image|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp|ico)$).*)',
  ],
};
