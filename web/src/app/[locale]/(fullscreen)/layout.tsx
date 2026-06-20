/**
 * Full-screen, chrome-free shell (no TopNav / Footer) for immersive app surfaces —
 * the AI chat hub lives here so it feels like a standalone app while still inheriting
 * the locale providers (Theme / Auth / Toast / Lang) from the [locale] layout.
 */
export default function FullscreenLayout({
  children,
}: Readonly<{children: React.ReactNode}>) {
  return <div className="h-[100dvh] w-full overflow-hidden">{children}</div>;
}
