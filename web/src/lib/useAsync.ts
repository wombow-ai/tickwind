'use client';

import {useEffect, useState} from 'react';
import {ApiError} from '@/lib/api';

/** Discriminated state of an in-flight or settled async request. */
export type AsyncState<T> =
  | {status: 'loading'}
  | {status: 'success'; data: T}
  | {status: 'error'; error: string};

/**
 * Runs an abortable async function and exposes its lifecycle as a
 * discriminated {@link AsyncState}. The request re-runs whenever `key`
 * changes; the previous request is aborted on cleanup, and the state resets to
 * `loading` while a new request is in flight.
 *
 * `key` is a stable primitive identifying the request (e.g. the ticker). It is
 * the sole dependency, which keeps the effect honest and the reset logic simple
 * compared to threading an array of deps through.
 *
 * @param fn Receives an {@link AbortSignal}; should reject on failure.
 * @param key Primitive that identifies the current request.
 */
export function useAsync<T>(
  fn: (signal: AbortSignal) => Promise<T>,
  key: string,
): AsyncState<T> {
  const [state, setState] = useState<AsyncState<T>>({status: 'loading'});

  // Reset to `loading` synchronously when `key` changes, following React's
  // "adjusting state when a prop changes" pattern (state-held previous key, no
  // ref, no effect setState — so it stays clear of cascading-render lint).
  const [activeKey, setActiveKey] = useState(key);
  if (key !== activeKey) {
    setActiveKey(key);
    setState({status: 'loading'});
  }

  useEffect(() => {
    const controller = new AbortController();

    fn(controller.signal).then(
      data => {
        if (!controller.signal.aborted) {
          setState({status: 'success', data});
        }
      },
      (error: unknown) => {
        if (!controller.signal.aborted) {
          setState({status: 'error', error: messageOf(error)});
        }
      },
    );

    return () => controller.abort();
    // `fn` is an inline closure recreated each render; `key` is the explicit
    // re-run trigger.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  return state;
}

/** Extracts a human-readable message from an unknown thrown value. */
function messageOf(error: unknown): string {
  if (error instanceof ApiError) {
    return error.status === 404
      ? 'Not tracked yet'
      : error.message || 'Request failed';
  }
  if (error instanceof Error) {
    return error.message;
  }
  return 'Unexpected error';
}
