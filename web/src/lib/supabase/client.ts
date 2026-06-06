import {createBrowserClient} from '@supabase/ssr';

const url = process.env.NEXT_PUBLIC_SUPABASE_URL ?? '';
const anonKey = process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY ?? '';

/** Browser-side Supabase client (anon key — safe to expose). */
export function createClient() {
  return createBrowserClient(url, anonKey);
}
