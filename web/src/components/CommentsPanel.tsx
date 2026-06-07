'use client';

import {Flag, MessageSquare, Trash2} from 'lucide-react';
import Link from 'next/link';
import {useCallback, useEffect, useState} from 'react';
import {
  deleteComment,
  getComments,
  postComment,
  reportComment,
  type Comment,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, timeAgo, tok} from '@/lib/ui';
import {useToast} from '@/components/ui/Toast';

type Tokens = ReturnType<typeof tok>;

/**
 * Public comments surface, reused by the stock detail "Comments" tab (pass
 * `ticker`) and the standalone /community page (no `ticker` → the global board,
 * with ticker chips deep-linking to each stock). Reading is public; posting
 * requires sign-in. §230 neutral-host safeguards (disclaimer, report, delete-own)
 * live here; rate-limiting + moderation are enforced server-side.
 */
export function CommentsPanel({ticker}: {ticker?: string}) {
  const {user, getToken} = useAuth();
  const {toast} = useToast();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [comments, setComments] = useState<Comment[]>([]);
  const [draft, setDraft] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    getComments(ticker ?? '').then(
      r => setComments(r.comments ?? []),
      () => setComments([]),
    );
  }, [ticker]);
  useEffect(() => {
    load();
  }, [load]);

  async function add() {
    const body = draft.trim();
    if (!body || busy || !user) return;
    setBusy(true);
    try {
      const token = await getToken();
      const c = await postComment(token, ticker ? {body, ticker} : {body});
      setComments(prev => [c, ...prev]);
      setDraft('');
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Failed to post');
    } finally {
      setBusy(false);
    }
  }

  async function remove(c: Comment) {
    setComments(prev => prev.filter(x => x.id !== c.id)); // optimistic
    try {
      const token = await getToken();
      await deleteComment(token, c.id);
    } catch {
      load();
    }
  }

  async function report(c: Comment) {
    try {
      const token = await getToken();
      await reportComment(token, c.id);
      toast(tr('comments.reported'), {tone: 'ok'});
    } catch {
      // best-effort
    }
  }

  return (
    <div className="tw-fade">
      <p className={cx('mb-3 rounded-xl border px-3 py-2 text-[11.5px]', t.border, t.faint)}>
        {tr('comments.disclaimer')}
      </p>

      {user ? (
        <div className={cx('mb-4 rounded-2xl border p-3', t.card, t.border, t.soft)}>
          <textarea
            value={draft}
            onChange={e => setDraft(e.target.value)}
            onKeyDown={e => {
              if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') add();
            }}
            placeholder={tr('comments.placeholder')}
            rows={2}
            className={cx(
              'w-full resize-none bg-transparent text-[13.5px] outline-none',
              dark ? 'text-slate-100 placeholder:text-slate-500' : 'text-slate-900 placeholder:text-slate-400',
            )}
          />
          <div className="mt-1 flex justify-end">
            <button
              onClick={add}
              disabled={!draft.trim() || busy}
              className={cx(
                'rounded-lg px-3.5 py-1.5 text-[12.5px] font-semibold transition disabled:opacity-50',
                btnPrimary(dark),
              )}
            >
              {tr('comments.post')}
            </button>
          </div>
        </div>
      ) : (
        <p className={cx('mb-4 text-[13px]', t.sub)}>{tr('comments.loginToPost')}</p>
      )}

      {comments.length === 0 ? (
        <div className={cx('rounded-2xl border p-8 text-center', t.card, t.border, t.soft)}>
          <MessageSquare className={cx('mx-auto mb-2', dark ? 'text-teal-300' : 'text-teal-600')} size={22} />
          <p className={cx('text-[13px]', t.sub)}>{tr('comments.empty')}</p>
        </div>
      ) : (
        <div className="space-y-2.5">
          {comments.map(c => (
            <CommentCard
              key={c.id}
              c={c}
              dark={dark}
              t={t}
              tr={tr}
              showTicker={!ticker}
              own={!!user && c.user_id === user.id}
              onReport={() => report(c)}
              onDelete={() => remove(c)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function CommentCard({
  c,
  dark,
  t,
  tr,
  showTicker,
  own,
  onReport,
  onDelete,
}: {
  c: Comment;
  dark: boolean;
  t: Tokens;
  tr: (k: string) => string;
  showTicker: boolean;
  own: boolean;
  onReport: () => void;
  onDelete: () => void;
}) {
  return (
    <div className={cx('group rounded-2xl border p-3.5', t.card, t.border, t.soft)}>
      <div className="mb-1 flex flex-wrap items-center gap-2">
        <span className={cx('text-[12.5px] font-bold', t.text)}>{c.author || 'anon'}</span>
        {showTicker && c.ticker && (
          <Link
            href={`/stock/${encodeURIComponent(c.ticker)}`}
            className={cx('rounded-md px-1.5 py-0.5 text-[10.5px] font-bold', t.chip, t.accentText)}
          >
            {c.ticker}
          </Link>
        )}
        <span className={cx('ml-auto text-[11px]', t.faint)}>
          {timeAgo(c.created_at)} {tr('common.ago')}
        </span>
      </div>
      <p className={cx('whitespace-pre-wrap break-words text-[13.5px]', t.text)}>{c.body}</p>
      <div className="mt-2 flex items-center gap-3 opacity-0 transition group-hover:opacity-100">
        <button
          onClick={onReport}
          className={cx('inline-flex items-center gap-1 text-[11px] font-medium hover:opacity-80', t.faint)}
        >
          <Flag size={11} /> {tr('comments.report')}
        </button>
        {own && (
          <button
            onClick={onDelete}
            className={cx(
              'inline-flex items-center gap-1 text-[11px] font-medium hover:opacity-80',
              dark ? 'text-rose-400' : 'text-rose-500',
            )}
          >
            <Trash2 size={11} /> {tr('notes.delete')}
          </button>
        )}
      </div>
    </div>
  );
}
