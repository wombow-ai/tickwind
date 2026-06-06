import {NextResponse} from 'next/server';
import {createClient} from '@/lib/supabase/server';

/**
 * OAuth callback: Supabase redirects here with a `code` after the provider
 * (e.g. Google) authenticates the user. We exchange it for a session (which
 * sets the auth cookies) and redirect home. On failure, back to /login.
 */
export async function GET(request: Request) {
  const {searchParams, origin} = new URL(request.url);
  const code = searchParams.get('code');
  const next = searchParams.get('next') ?? '/';

  if (code) {
    const supabase = await createClient();
    const {error} = await supabase.auth.exchangeCodeForSession(code);
    if (!error) {
      return NextResponse.redirect(`${origin}${next}`);
    }
  }
  return NextResponse.redirect(`${origin}/login?error=oauth`);
}
