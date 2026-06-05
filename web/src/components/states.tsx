/** Reusable loading / error / empty status presenters. */

import type {ReactNode} from 'react';

/** A spinning ring indicator. */
export function Spinner({className = ''}: {className?: string}) {
  return (
    <span
      role="status"
      aria-label="Loading"
      className={`inline-block animate-spin rounded-full border-2 border-zinc-600 border-t-sky-400 ${className}`}
    />
  );
}

/** Centered loading state with a label. */
export function LoadingState({label = 'Loading…'}: {label?: string}) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-16 text-zinc-400">
      <Spinner className="h-6 w-6" />
      <p className="text-sm">{label}</p>
    </div>
  );
}

interface ErrorStateProps {
  title?: string;
  message: string;
  /** Optional action node (e.g. a retry button or link). */
  action?: ReactNode;
}

/** Card-style error presenter. */
export function ErrorState({
  title = 'Something went wrong',
  message,
  action,
}: ErrorStateProps) {
  return (
    <div className="rounded-xl border border-rose-500/30 bg-rose-500/5 p-6 text-center">
      <h2 className="text-sm font-semibold text-rose-300">{title}</h2>
      <p className="mt-1 text-sm text-zinc-400">{message}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

interface EmptyStateProps {
  title: string;
  message: string;
}

/** Muted placeholder shown when a successful response has no items. */
export function EmptyState({title, message}: EmptyStateProps) {
  return (
    <div className="rounded-xl border border-dashed border-white/10 bg-white/[0.02] p-8 text-center">
      <h2 className="text-sm font-semibold text-zinc-300">{title}</h2>
      <p className="mt-1 text-sm text-zinc-500">{message}</p>
    </div>
  );
}
