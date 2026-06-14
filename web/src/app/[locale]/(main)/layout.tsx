import {Footer} from '@/components/Footer';
import {TopNav} from '@/components/TopNav';

/** Chrome for the main app: sticky nav + content + footer. */
export default function MainLayout({
  children,
}: Readonly<{children: React.ReactNode}>) {
  return (
    <div className="flex min-h-screen flex-col">
      <TopNav />
      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8 sm:px-6">
        {children}
      </main>
      <Footer />
    </div>
  );
}
