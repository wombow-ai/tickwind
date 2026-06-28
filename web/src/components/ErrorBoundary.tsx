'use client';

import {RefreshCw} from 'lucide-react';
import {Component, type ReactNode} from 'react';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/**
 * Compact fallback shown when a panel inside {@link PanelBoundary} throws.
 * Functional so it can read locale + theme via hooks (the boundary itself must
 * be a class component — those are the only React API that catches render
 * errors). Mirrors the calm rose accent of the shared {@link ErrorState}.
 */
function PanelFallback({onRetry}: {onRetry: () => void}) {
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  return (
    <div
      role="alert"
      className={cx(
        'mb-6 flex items-center gap-3 rounded-2xl border p-4 text-left',
        t.border,
        dark ? 'bg-rose-500/[0.06]' : 'bg-rose-50/70',
      )}
    >
      <div className="min-w-0 flex-1">
        <p className={cx('text-[13px] font-semibold', t.text)}>{tr('boundary.panelTitle')}</p>
        <p className={cx('mt-0.5 text-[12px]', t.sub)}>{tr('boundary.panelSub')}</p>
      </div>
      <button
        onClick={onRetry}
        className={cx(
          'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1.5 text-[12px] font-medium',
          t.border,
          t.ghost,
        )}
      >
        <RefreshCw size={12} /> {tr('states.retry')}
      </button>
    </div>
  );
}

type Props = {children: ReactNode; resetKey?: string | number};
type State = {error: boolean};

/**
 * A small client-side error boundary for an individual panel or tab. When a
 * child throws during render, only this subtree degrades to a compact
 * "couldn't load · retry" card — the rest of the page keeps working (no white
 * screen). Wrapped around StockView's top-level tabs and its heavy data panels
 * so one bad-data card can't take down the whole route. The route-level
 * `(main)/error.tsx` is the backstop above this.
 *
 * `resetKey` (e.g. the ticker) auto-clears a stuck error when the wrapped
 * subtree's identity changes, so a stale error from the previous symbol doesn't
 * persist across an in-place navigation.
 */
export class PanelBoundary extends Component<Props, State> {
  state: State = {error: false};

  static getDerivedStateFromError(): State {
    return {error: true};
  }

  componentDidCatch(error: unknown) {
    // Swallow so it never propagates to a full-route white-screen. Surface in
    // dev for debugging; production stays quiet (no PII, no telemetry).
    if (process.env.NODE_ENV !== 'production') {
      // eslint-disable-next-line no-console
      console.error('[PanelBoundary] caught', error);
    }
  }

  componentDidUpdate(prev: Props) {
    if (this.state.error && prev.resetKey !== this.props.resetKey) {
      this.setState({error: false});
    }
  }

  render() {
    if (this.state.error) {
      return <PanelFallback onRetry={() => this.setState({error: false})} />;
    }
    return this.props.children;
  }
}
