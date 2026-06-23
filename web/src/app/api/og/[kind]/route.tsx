import {ImageResponse} from 'next/og';
import type {NextRequest} from 'next/server';

export const runtime = 'nodejs';

/**
 * Dynamic Open Graph / share-card renderer. One route serves every card kind via
 * `?` params, so links shared to X / Telegram / Discord / 微信 render a rich
 * branded card, and boards can offer a one-tap "save image" for 小红书/群聊.
 *
 * CJK is the hard part of satori (next/og): it ships no Chinese font. We fetch a
 * **subset** containing only the glyphs this card renders, from Google Fonts'
 * `css2?...&text=` endpoint (tiny + fast), forcing a TrueType src that satori
 * supports. Falls back to a latin-only render if the font fetch fails, so the
 * route never 500s.
 */

// The card is DESIGNED at 1200×630 but RENDERED at 2× (2400×1260) for crisp
// "save image" output on high-DPI screens / 小红书 reposts. The design is scaled
// up via a transform wrapper so no per-element sizes change.
const BASE_W = 1200;
const BASE_H = 630;
const SCALE = 2;
const WIDTH = BASE_W * SCALE;
const HEIGHT = BASE_H * SCALE;

// Force a non-woff2 (TrueType/OpenType) src — satori cannot parse woff2.
const OLD_UA =
  'Mozilla/5.0 (Windows NT 6.1; rv:6.0) Gecko/20110814 Firefox/6.0';

async function loadFontSubset(family: string, weight: number, text: string): Promise<ArrayBuffer | null> {
  try {
    const url = `https://fonts.googleapis.com/css2?family=${encodeURIComponent(
      family,
    )}:wght@${weight}&text=${encodeURIComponent(text)}`;
    const css = await fetch(url, {headers: {'User-Agent': OLD_UA}}).then(r => r.text());
    const m = css.match(/src:\s*url\(([^)]+)\)\s*format\(['"]?(?:opentype|truetype|woff)['"]?\)/);
    if (!m) return null;
    const res = await fetch(m[1]);
    if (!res.ok) return null;
    return await res.arrayBuffer();
  } catch {
    return null;
  }
}

/** Clamp + sanitize a query string so a card can't be abused for huge renders. */
function clamp(s: string | null, max: number): string {
  if (!s) return '';
  return s.length > max ? s.slice(0, max) : s;
}

export async function GET(req: NextRequest, ctx: {params: Promise<{kind: string}>}) {
  const {kind} = await ctx.params;
  const sp = req.nextUrl.searchParams;

  const eyebrow = clamp(sp.get('eyebrow'), 40);
  const title = clamp(sp.get('title'), 90) || 'Tickwind';
  const subtitle = clamp(sp.get('subtitle'), 140);
  const stat = clamp(sp.get('stat'), 24); // optional big figure (e.g. a price / %)
  const statTone = sp.get('tone'); // 'up' | 'down' | null
  const lang = sp.get('lang') === 'en' ? 'en' : 'zh'; // UI language for the card chrome
  // Brand badge + default footer tag follow the UI language (single-language
  // values default to English for the EN UI; Chinese for the zh UI).
  const badge = lang === 'en' ? 'Data-first US stocks' : '中文美股数据台';
  const defaultTag =
    lang === 'en'
      ? 'Congress · 13F · Options flow · Insider buys'
      : '国会交易 · 13F · 期权异动 · 内部人买入';
  const tag = clamp(sp.get('tag'), 60) || defaultTag;

  const isStock = kind === 'stock';
  const accent = '#0d9488'; // teal-600
  const statColor = statTone === 'down' ? '#e11d48' : statTone === 'up' ? '#059669' : '#0f172a';

  // Subset must cover every glyph we draw. Include BOTH language candidates for
  // the badge + default tag so the subset always covers the rendered chrome
  // regardless of `lang` (the extra latin glyphs are harmless).
  const allText =
    'Tickwind tickwind.com 中文美股数据台 Data-first US stocks' +
    '国会交易 · 13F · 期权异动 · 内部人买入 Congress · Options flow · Insider buys' +
    eyebrow +
    title +
    subtitle +
    stat +
    tag;
  const [reg, bold] = await Promise.all([
    loadFontSubset('Noto Sans SC', 400, allText),
    loadFontSubset('Noto Sans SC', 700, allText),
  ]);

  const fonts = [
    reg && {name: 'Noto Sans SC', data: reg, weight: 400 as const, style: 'normal' as const},
    bold && {name: 'Noto Sans SC', data: bold, weight: 700 as const, style: 'normal' as const},
  ].filter(Boolean) as {name: string; data: ArrayBuffer; weight: 400 | 700; style: 'normal'}[];

  return new ImageResponse(
    (
      <div style={{display: 'flex', width: '100%', height: '100%'}}>
        <div
          style={{
            width: BASE_W,
            height: BASE_H,
            transform: `scale(${SCALE})`,
            transformOrigin: 'top left',
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'space-between',
            padding: 72,
            background: 'linear-gradient(135deg, #ecfeff 0%, #eff6ff 55%, #f5f3ff 100%)',
            fontFamily: '"Noto Sans SC"',
          }}
        >
        {/* header */}
        <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between'}}>
          <div style={{display: 'flex', alignItems: 'center'}}>
            {/* The Tickwind "streams" mark, inlined as an SVG data-URI (Satori rasterizes
                it via resvg — reliable, unlike CSS currentColor): navy streams + green
                leader/node, matching the main-site LogoMark on this light card. */}
            <img
              width={56}
              height={56}
              style={{marginRight: 18}}
              src={`data:image/svg+xml;base64,${Buffer.from(
                '<svg xmlns="http://www.w3.org/2000/svg" viewBox="12 13 76 76" fill="none">' +
                  '<path d="M18 67 C42 67 54 60 67 46" stroke="#0E1B2E" stroke-width="6" stroke-linecap="round"/>' +
                  '<path d="M18 76 C38 76 48 71 58 60" stroke="#0E1B2E" stroke-width="6" stroke-linecap="round"/>' +
                  '<path d="M18 58 C44 58 57 50 76 32" stroke="#00C08B" stroke-width="6" stroke-linecap="round"/>' +
                  '<circle cx="76" cy="32" r="4.4" fill="#00C08B"/>' +
                  '<circle cx="76" cy="32" r="8.4" stroke="#00C08B" stroke-width="1.5"/>' +
                  '</svg>',
              ).toString('base64')}`}
            />
            <div style={{display: 'flex', fontSize: 40, fontWeight: 700}}>
              <span style={{color: accent}}>Tick</span>
              <span style={{color: '#0f172a'}}>wind</span>
            </div>
          </div>
          <div
            style={{
              display: 'flex',
              padding: '8px 20px',
              borderRadius: 999,
              background: 'rgba(13,148,136,0.10)',
              color: accent,
              fontSize: 22,
              fontWeight: 700,
            }}
          >
            {badge}
          </div>
        </div>

        {/* body */}
        <div style={{display: 'flex', flexDirection: 'column'}}>
          {eyebrow ? (
            <div style={{display: 'flex', color: accent, fontSize: 28, fontWeight: 700, marginBottom: 16}}>
              {eyebrow}
            </div>
          ) : null}
          <div
            style={{
              display: 'flex',
              color: '#0f172a',
              fontSize: isStock ? 84 : 60,
              fontWeight: 700,
              lineHeight: 1.15,
            }}
          >
            {title}
          </div>
          {stat ? (
            <div style={{display: 'flex', color: statColor, fontSize: 64, fontWeight: 700, marginTop: 18}}>
              {stat}
            </div>
          ) : null}
          {subtitle ? (
            <div style={{display: 'flex', color: '#475569', fontSize: 30, marginTop: 20, lineHeight: 1.4}}>
              {subtitle}
            </div>
          ) : null}
        </div>

        {/* footer */}
        <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between'}}>
          <div style={{display: 'flex', color: accent, fontSize: 28, fontWeight: 700}}>tickwind.com</div>
          <div style={{display: 'flex', color: '#94a3b8', fontSize: 22}}>{tag}</div>
        </div>
        </div>
      </div>
    ),
    {width: WIDTH, height: HEIGHT, fonts: fonts.length ? fonts : undefined},
  );
}
