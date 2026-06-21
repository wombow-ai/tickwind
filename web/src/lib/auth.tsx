'use client';

import type {SupabaseClient, User} from '@supabase/supabase-js';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';
import {createClient} from '@/lib/supabase/client';

/** Authentication state shared across the app. */
interface AuthState {
  /** The signed-in user, or `null` when anonymous. */
  user: User | null;
  /** True until the initial session check resolves. */
  loading: boolean;
  /** The browser Supabase client (for sign-in/sign-up forms). */
  supabase: SupabaseClient;
  /** Resolves the current (auto-refreshed) access token, or `null`. */
  getToken: () => Promise<string | null>;
  /** Signs the user out and clears the session. */
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

/**
 * Whether two user snapshots refer to the same signed-in identity. Supabase mints a
 * FRESH `session.user` object on every token refresh — which fires on tab refocus
 * (its visibility handler re-validates the session). Treating those as a new `user`
 * would churn the reference and re-run every effect keyed on it (entitlement refetch,
 * conversation re-fetch) → a visible "refresh" when you tab back into the chat. Compare
 * by id + email so a routine token refresh keeps the SAME reference, while a real
 * sign-in/out or email change still updates it.
 */
function sameUser(a: User | null, b: User | null): boolean {
  if (a === b) return true;
  if (!a || !b) return false;
  return a.id === b.id && a.email === b.email;
}

/** Tracks the Supabase session and exposes a token getter for API calls. */
export function AuthProvider({children}: {children: React.ReactNode}) {
  // A single browser client instance for the app's lifetime.
  const [supabase] = useState<SupabaseClient>(() => createClient());
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    // Stable updater: keep the prior reference when only the token changed (same id+email),
    // so token-refresh events (incl. tab refocus) don't ripple a re-render/refetch storm.
    const apply = (next: User | null) => setUser(prev => (sameUser(prev, next) ? prev : next));
    supabase.auth.getSession().then(({data}) => {
      if (!active) return;
      apply(data.session?.user ?? null);
      setLoading(false);
    });
    const {data: sub} = supabase.auth.onAuthStateChange((_event, session) => {
      apply(session?.user ?? null);
      setLoading(false);
    });
    return () => {
      active = false;
      sub.subscription.unsubscribe();
    };
  }, [supabase]);

  const getToken = useCallback(async () => {
    const {data} = await supabase.auth.getSession();
    return data.session?.access_token ?? null;
  }, [supabase]);

  const signOut = useCallback(async () => {
    await supabase.auth.signOut();
  }, [supabase]);

  const value = useMemo<AuthState>(
    () => ({user, loading, supabase, getToken, signOut}),
    [user, loading, supabase, getToken, signOut],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

/** Returns auth state. Must be used under an {@link AuthProvider}. */
export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return ctx;
}
