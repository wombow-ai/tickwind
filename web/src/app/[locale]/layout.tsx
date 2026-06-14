import {notFound} from 'next/navigation';
import {AuthProvider} from '@/lib/auth';
import {LangProvider} from '@/lib/i18n';
import {isLocale, LOCALES} from '@/lib/locale';
import {ThemeProvider, themeNoFlashScript} from '@/lib/theme';
import {ToastProvider} from '@/components/ui/Toast';

/** Pre-render both locales so `/en` and `/zh` are static shells. */
export function generateStaticParams() {
  return LOCALES.map(locale => ({locale}));
}

/**
 * The locale-scoped root layout. Renders `<html lang={locale}>` (the locale now
 * comes from the URL/SSR — no pre-paint language script), the theme no-flash
 * script, `<body>`, and the provider stack (Theme → Auth → Toast) wrapped in
 * {@link LangProvider} so the whole client tree reads the URL locale. An unknown
 * locale param → `notFound()`.
 */
export default async function LocaleLayout({
  children,
  params,
}: Readonly<{children: React.ReactNode; params: Promise<{locale: string}>}>) {
  const {locale} = await params;
  if (!isLocale(locale)) notFound();

  return (
    <html lang={locale} suppressHydrationWarning>
      <head>
        {/* Apply the persisted theme before paint to avoid a light→dark flash. */}
        <script dangerouslySetInnerHTML={{__html: themeNoFlashScript}} />
      </head>
      <body className="antialiased">
        <LangProvider lang={locale}>
          <ThemeProvider>
            <AuthProvider>
              <ToastProvider>{children}</ToastProvider>
            </AuthProvider>
          </ThemeProvider>
        </LangProvider>
      </body>
    </html>
  );
}
