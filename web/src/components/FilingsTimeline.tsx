import type {Filing} from '@/lib/api';
import {FormBadge} from '@/components/FormBadge';
import {formatFiledDate, toDateTimeAttr} from '@/lib/format';

interface FilingsTimelineProps {
  filings: Filing[];
}

/**
 * A vertical timeline of regulatory filings, most recent first. Each entry
 * shows its form badge, title, filed date, and a link out to the SEC source.
 */
export function FilingsTimeline({filings}: FilingsTimelineProps) {
  return (
    <ol className="relative space-y-1 border-l border-white/10 pl-6">
      {filings.map((filing, index) => (
        <FilingRow
          key={`${filing.accession_no || filing.url}-${index}`}
          filing={filing}
        />
      ))}
    </ol>
  );
}

/** A single timeline entry. */
function FilingRow({filing}: {filing: Filing}) {
  const dateTime = toDateTimeAttr(filing.filed_at);
  return (
    <li className="group relative -ml-6 rounded-lg pl-6 pr-3 py-3 transition hover:bg-white/[0.03]">
      <span
        aria-hidden
        className="absolute -left-[5px] top-5 h-2.5 w-2.5 rounded-full border-2 border-zinc-950 bg-zinc-600 transition group-hover:bg-sky-400"
      />
      <div className="flex flex-wrap items-center gap-2">
        <FormBadge form={filing.form} />
        <time
          dateTime={dateTime}
          className="text-xs font-medium text-zinc-500"
        >
          {formatFiledDate(filing.filed_at)}
        </time>
      </div>
      <a
        href={filing.url}
        target="_blank"
        rel="noopener noreferrer"
        className="mt-1.5 flex items-start gap-1.5 text-sm font-medium text-zinc-200 hover:text-sky-300 focus:outline-none focus-visible:underline"
      >
        <span className="flex-1">{filing.title}</span>
        <span
          aria-hidden
          className="mt-0.5 shrink-0 text-zinc-600 transition group-hover:text-sky-400"
        >
          ↗
        </span>
      </a>
      {filing.accession_no ? (
        <p className="mt-0.5 font-mono text-[11px] text-zinc-600">
          {filing.accession_no}
        </p>
      ) : null}
    </li>
  );
}
