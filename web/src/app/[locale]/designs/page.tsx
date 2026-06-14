import Link from '@/components/LocalLink';

const VARIANTS = [
  {
    slug: 'aurora',
    name: 'Aurora',
    blurb: 'Light, airy, premium — soft sky/teal gradients, Linear/Stripe polish.',
  },
  {
    slug: 'horizon',
    name: 'Horizon',
    blurb:
      'Ultra-clean editorial minimal — near-monochrome with one indigo accent.',
  },
  {
    slug: 'breeze',
    name: 'Breeze',
    blurb: 'Friendly consumer-fintech — warm, rounded, approachable (coming).',
  },
];

/** Internal index for comparing landing-page design directions. */
export default function DesignsIndex() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight text-zinc-50">
          Landing design directions
        </h1>
        <p className="mt-1 text-sm text-zinc-400">
          Compare the variants, then we ship the one you like.
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {VARIANTS.map(v => (
          <Link
            key={v.slug}
            href={`/designs/${v.slug}`}
            className="group rounded-2xl border border-white/10 bg-white/[0.03] p-6 transition hover:border-sky-400/40 hover:bg-white/[0.06]"
          >
            <h2 className="text-lg font-semibold text-zinc-100">{v.name}</h2>
            <p className="mt-1 text-sm text-zinc-400">{v.blurb}</p>
            <span className="mt-3 inline-block text-xs font-medium text-sky-400/80 group-hover:text-sky-300">
              View →
            </span>
          </Link>
        ))}
      </div>
    </div>
  );
}
