'use client';

import {Check} from 'lucide-react';
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
} from 'react';
import {useDark} from '@/lib/theme';
import {cx, tok} from '@/lib/ui';

/** An optional inline action on a toast (e.g. "Undo"). */
interface ToastAction {
  label: string;
  fn: () => void;
}

interface ToastOptions {
  /** `ok` shows a success check; omit for a neutral toast. */
  tone?: 'ok';
  action?: ToastAction;
}

interface Toast extends ToastOptions {
  id: number;
  msg: string;
}

interface ToastApi {
  /** Shows a transient toast. */
  toast: (msg: string, opts?: ToastOptions) => void;
}

const ToastContext = createContext<ToastApi | null>(null);

/** Provides {@link useToast} and renders the toaster overlay. */
export function ToastProvider({children}: {children: React.ReactNode}) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const idRef = useRef(0);

  const dismiss = useCallback((id: number) => {
    setToasts(prev => prev.filter(x => x.id !== id));
  }, []);

  const toast = useCallback(
    (msg: string, opts: ToastOptions = {}) => {
      const id = ++idRef.current;
      setToasts(prev => [...prev, {id, msg, ...opts}]);
      setTimeout(() => dismiss(id), opts.action ? 4200 : 2400);
    },
    [dismiss],
  );

  const value = useMemo<ToastApi>(() => ({toast}), [toast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <Toaster toasts={toasts} dismiss={dismiss} />
    </ToastContext.Provider>
  );
}

/** Returns the toast API. Must be used under a {@link ToastProvider}. */
export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error('useToast must be used within a ToastProvider');
  }
  return ctx;
}

function Toaster({
  toasts,
  dismiss,
}: {
  toasts: Toast[];
  dismiss: (id: number) => void;
}) {
  const dark = useDark();
  const t = tok(dark);
  return (
    <div className="pointer-events-none fixed bottom-6 left-1/2 z-[60] flex -translate-x-1/2 flex-col items-center gap-2">
      {toasts.map(to => (
        <div
          key={to.id}
          className={cx(
            'tw-toast pointer-events-auto flex items-center gap-2.5 rounded-full border py-2 pl-3.5 pr-2 shadow-lg',
            t.card,
            t.border,
          )}
        >
          {to.tone === 'ok' && <Check size={15} className="text-emerald-500" />}
          <span className={cx('text-[13px] font-medium', t.text)}>{to.msg}</span>
          {to.action && (
            <button
              onClick={() => {
                to.action!.fn();
                dismiss(to.id);
              }}
              className={cx(
                'ml-1 rounded-full px-2.5 py-1 text-[12px] font-semibold',
                dark
                  ? 'text-teal-300 hover:bg-slate-800'
                  : 'text-teal-700 hover:bg-teal-50',
              )}
            >
              {to.action.label}
            </button>
          )}
        </div>
      ))}
    </div>
  );
}
