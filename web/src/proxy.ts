import {createServerClient} from '@supabase/ssr';
import {type NextRequest, NextResponse} from 'next/server';

/**
 * Refreshes the Supabase auth session on every navigation so Server Components
 * see a current user and cookies stay fresh. No-ops when Supabase env vars are
 * unset (e.g. local builds without auth configured).
 *
 * Next 16's `proxy` convention (formerly `middleware`).
 */
export async function proxy(request: NextRequest) {
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
