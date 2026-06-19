'use client';

import {useCallback, useEffect, useState} from 'react';
import {getEntitlement, type Entitlement} from '@/lib/api';
import {useAuth} from '@/lib/auth';

/**
 * Reads the logged-in user's Pro entitlement (GET /v1/billing/me). Returns null while
 * loading or when logged out; falls back to {tier:'free'} on any error so the UI never
 * wrongly shows Pro. `refresh()` re-fetches (e.g. after returning from checkout).
 */
export function useEntitlement(): {
  entitlement: Entitlement | null;
  loading: boolean;
  isPro: boolean;
  refresh: () => void;
} {
  const {user, getToken} = useAuth();
  const [entitlement, setEntitlement] = useState<Entitlement | null>(null);
  const [loading, setLoading] = useState(false);
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    if (!user) {
      setEntitlement(null);
      return;
    }
    let active = true;
    setLoading(true);
    (async () => {
      try {
        const token = await getToken();
        const e = await getEntitlement(token);
        if (active) setEntitlement(e);
      } catch {
        if (active) setEntitlement({tier: 'free'});
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [user, getToken, nonce]);

  const refresh = useCallback(() => setNonce(n => n + 1), []);
  return {entitlement, loading, isPro: entitlement?.tier === 'pro', refresh};
}
