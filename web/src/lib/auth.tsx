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

/** Tracks the Supabase session and exposes a token getter for API calls. */
export function AuthProvider({children}: {children: React.ReactNode}) {
  // A single browser client instance for the app's lifetime.
  const [supabase] = useState<SupabaseClient>(() => createClient());
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    supabase.auth.getSession().then(({data}) => {
      if (!active) return;
      setUser(data.session?.user ?? null);
      setLoading(false);
    });
    const {data: sub} = supabase.auth.onAuthStateChange((_event, session) => {
      setUser(session?.user ?? null);
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
