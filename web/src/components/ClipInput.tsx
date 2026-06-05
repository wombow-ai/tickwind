'use client';

import {useState, type FormEvent} from 'react';
import {ApiError, clipLink, type Post} from '@/lib/api';

interface ClipInputProps {
  ticker: string;
  /** Called with the created post after a successful save. */
  onClipped: (post: Post) => void;
}

/**
 * A paste box that saves a link (X, Xiaohongshu, TikTok, …) to the ticker's
 * feed. The backend fetches the page title; the created post is reported via
 * {@link ClipInputProps.onClipped} so the caller can show it immediately.
 */
export function ClipInput({ticker, onClipped}: ClipInputProps) {
  const [url, setUrl] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const link = url.trim();
    if (link === '' || busy) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const post = await clipLink(ticker, link);
      setUrl('');
      onClipped(post);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to save link');
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-1.5">
      <div className="flex gap-2">
        <input
          type="url"
          value={url}
          onChange={event => setUrl(event.target.value)}
          placeholder="Paste a link — X, Xiaohongshu, TikTok…"
          className="flex-1 rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-zinc-200 placeholder:text-zinc-600 focus:border-sky-500/50 focus:outline-none"
        />
        <button
          type="submit"
          disabled={busy || url.trim() === ''}
          className="rounded-lg bg-sky-500/90 px-4 py-2 text-sm font-medium text-white transition hover:bg-sky-400 disabled:opacity-40"
        >
          {busy ? 'Saving…' : 'Save'}
        </button>
      </div>
      {error ? <p className="text-xs text-red-400">{error}</p> : null}
    </form>
  );
}
