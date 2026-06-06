import {createServerClient} from '@supabase/ssr';
import {cookies} from 'next/headers';

const url = process.env.NEXT_PUBLIC_SUPABASE_URL ?? '';
const anonKey = process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY ?? '';

/**
 * Server-side Supabase client bound to the request cookies (Server Components,
 * Route Handlers, Server Actions). Token refresh is handled by middleware.
 */
export async function createClient() {
  const cookieStore = await cookies();
  return createServerClient(url, anonKey, {
    cookies: {
      getAll() {
        return cookieStore.getAll();
      },
      setAll(cookiesToSet) {
        try {
          for (const {name, value, options} of cookiesToSet) {
            cookieStore.set(name, value, options);
          }
        } catch {
          // Called from a Server Component (read-only cookies); the middleware
          // refresh path handles writing the updated session.
        }
      },
    },
  });
}
