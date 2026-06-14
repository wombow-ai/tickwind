'use client';

import Link from '@/components/LocalLink';
import {Logo} from '@/components/ui/atoms';

/** Centered, chrome-free layout for the sign-in / sign-up screens. */
export default function AuthLayout({
  children,
}: Readonly<{children: React.ReactNode}>) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center px-4 py-10">
      <Link href="/" className="mb-7" aria-label="Tickwind home">
        <Logo size={34} />
      </Link>
      {children}
      <Link
        href="/"
        className="mt-6 text-[12.5px] text-slate-400 hover:text-slate-500"
      >
        ← Back to the board
      </Link>
    </div>
  );
}
