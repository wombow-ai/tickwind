/** A monospace badge for an SEC filing form type, color-coded by family. */

interface FormBadgeProps {
  form: string;
}

/**
 * Maps a form type to a Tailwind style. Lookup is by the uppercased form
 * string first, then by a coarse prefix so variants like `10-K/A` inherit
 * their family's color.
 */
function styleForForm(form: string): string {
  const f = form.toUpperCase();
  if (f.startsWith('8-K')) {
    return 'bg-amber-500/15 text-amber-300 ring-amber-500/30';
  }
  if (f.startsWith('10-K') || f.startsWith('10-Q')) {
    return 'bg-emerald-500/15 text-emerald-300 ring-emerald-500/30';
  }
  if (f.startsWith('S-') || f.startsWith('424')) {
    return 'bg-fuchsia-500/15 text-fuchsia-300 ring-fuchsia-500/30';
  }
  // Ownership forms (3/4/5) and their amendments.
  if (/^(3|4|5)(\/A)?$/.test(f)) {
    return 'bg-sky-500/15 text-sky-300 ring-sky-500/30';
  }
  return 'bg-zinc-500/15 text-zinc-300 ring-zinc-500/30';
}

/** Renders the form type (e.g. `8-K`) as a compact monospace pill. */
export function FormBadge({form}: FormBadgeProps) {
  return (
    <span
      className={`inline-flex items-center rounded-md px-2 py-0.5 font-mono text-xs font-semibold ring-1 ring-inset ${styleForForm(
        form,
      )}`}
    >
      {form}
    </span>
  );
}
