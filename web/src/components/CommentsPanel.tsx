'use client';

import {Flag, Heart, MessageSquare, Pencil, Trash2} from 'lucide-react';
import Link from 'next/link';
import {useCallback, useEffect, useState} from 'react';
import {
  deleteComment,
  getComments,
  likeComment,
  postComment,
  reportComment,
  updateComment,
  type Comment,
} from '@/lib/api';
import {useAuth} from '@/lib/auth';
import {useT} from '@/lib/i18n';
import {useDark} from '@/lib/theme';
import {btnPrimary, cx, timeAgo, tok} from '@/lib/ui';
import {useToast} from '@/components/ui/Toast';
import {Markdown} from '@/components/Markdown';

type Tokens = ReturnType<typeof tok>;

/**
 * Public comments surface, reused by the stock detail "Comments" tab (pass
 * `ticker`) and the standalone /community page (no `ticker` → the global board,
 * with ticker chips deep-linking to each stock). Reading is public; posting,
 * editing (own), and liking require sign-in. §230 neutral-host safeguards
 * (disclaimer, report, delete-own) live here; rate-limiting + moderation are
 * enforced server-side. Bodies render as safe Markdown.
 */
export function CommentsPanel({ticker}: {ticker?: string}) {
  const {user, getToken} = useAuth();
  const {toast} = useToast();
  const dark = useDark();
  const t = tok(dark);
  const tr = useT();
  const [comments, setComments] = useState<Comment[]>([]);
  // On a stock page the composer starts with the stock's cashtag, so the post
  // carries it by default (deletable). Posting just the bare tag is disabled.
  const prefix = ticker ? `$${ticker.toUpperCase()} ` : '';
  const [draft, setDraft] = useState(prefix);
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

  // True while the draft says nothing beyond the auto-inserted cashtag.
  const draftEmpty = draft.trim() === '' || draft.trim() === prefix.trim();

  async function add() {
    const body = draft.trim();
    if (draftEmpty || busy || !user) return;
    setBusy(true);
    try {
      const token = await getToken();
      const c = await postComment(token, ticker ? {body, ticker} : {body});
      setComments(prev => [c, ...prev]);
      setDraft(prefix);
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

  // Persist an edit into the list so it survives re-renders.
  function applyEdit(updated: Comment) {
    setComments(prev => prev.map(x => (x.id === updated.id ? {...x, ...updated} : x)));
  }
  function applyLike(id: string, likes: number) {
    setComments(prev => prev.map(x => (x.id === id ? {...x, likes} : x)));
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
          <div className="mt-1 flex items-center justify-between">
            <span className={cx('text-[11px]', t.faint)}>{tr('comments.mdHint')}</span>
            <button
              onClick={add}
              disabled={draftEmpty || busy}
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
              canInteract={!!user}
              getToken={getToken}
              toast={toast}
              onReport={() => report(c)}
              onDelete={() => remove(c)}
              onEdited={applyEdit}
              onLiked={applyLike}
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
  canInteract,
  getToken,
  toast,
  onReport,
  onDelete,
  onEdited,
  onLiked,
}: {
  c: Comment;
  dark: boolean;
  t: Tokens;
  tr: (k: string) => string;
  showTicker: boolean;
  own: boolean;
  canInteract: boolean;
  getToken: () => Promise<string | null>;
  toast: (m: string) => void;
  onReport: () => void;
  onDelete: () => void;
  onEdited: (updated: Comment) => void;
  onLiked: (id: string, likes: number) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [editDraft, setEditDraft] = useState(c.body);
  const [savingEdit, setSavingEdit] = useState(false);
  const [liked, setLiked] = useState(c.liked ?? false); // server returns the viewer's like state
  const [likeBusy, setLikeBusy] = useState(false);

  async function saveEdit() {
    const body = editDraft.trim();
    if (!body || savingEdit) return;
    setSavingEdit(true);
    try {
      const token = await getToken();
      const updated = await updateComment(token, c.id, body);
      onEdited(updated);
      setEditing(false);
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Failed to save');
    } finally {
      setSavingEdit(false);
    }
  }

  async function toggleLike() {
    if (!canInteract || likeBusy) return;
    setLikeBusy(true);
    try {
      const token = await getToken();
      const r = await likeComment(token, c.id);
      setLiked(r.liked);
      onLiked(c.id, r.likes);
    } catch {
      // best-effort
    } finally {
      setLikeBusy(false);
    }
  }

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
          {c.edited_at ? `${tr('comments.edited')} · ` : ''}
          {timeAgo(c.created_at)} {tr('common.ago')}
        </span>
      </div>

      {editing ? (
        <div>
          <textarea
            value={editDraft}
            onChange={e => setEditDraft(e.target.value)}
            rows={3}
            className={cx(
              'w-full resize-none rounded-lg border bg-transparent p-2 text-[13.5px] outline-none',
              t.border,
              dark ? 'text-slate-100' : 'text-slate-900',
            )}
          />
          <div className="mt-1.5 flex items-center gap-2">
            <button
              onClick={saveEdit}
              disabled={!editDraft.trim() || savingEdit}
              className={cx('rounded-lg px-3 py-1 text-[12px] font-semibold transition disabled:opacity-50', btnPrimary(dark))}
            >
              {tr('comments.save')}
            </button>
            <button
              onClick={() => {
                setEditDraft(c.body);
                setEditing(false);
              }}
              className={cx('text-[12px] font-medium hover:opacity-80', t.sub)}
            >
              {tr('comments.cancel')}
            </button>
          </div>
        </div>
      ) : (
        <Markdown>{c.body}</Markdown>
      )}

      <div className="mt-2 flex items-center gap-3">
        <button
          onClick={toggleLike}
          disabled={!canInteract || likeBusy}
          aria-pressed={liked}
          className={cx(
            'inline-flex items-center gap-1 text-[11.5px] font-medium transition hover:opacity-80 disabled:opacity-50',
            liked ? (dark ? 'text-rose-400' : 'text-rose-500') : t.faint,
          )}
        >
          <Heart size={12} fill={liked ? 'currentColor' : 'none'} /> {c.likes > 0 ? c.likes : ''}
        </button>
        {!editing && (
          <div className="flex items-center gap-3 opacity-0 transition group-hover:opacity-100">
            {own && (
              <button
                onClick={() => {
                  setEditDraft(c.body);
                  setEditing(true);
                }}
                className={cx('inline-flex items-center gap-1 text-[11px] font-medium hover:opacity-80', t.faint)}
              >
                <Pencil size={11} /> {tr('comments.edit')}
              </button>
            )}
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
        )}
      </div>
    </div>
  );
}
