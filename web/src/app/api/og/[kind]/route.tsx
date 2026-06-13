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

const WIDTH = 1200;
const HEIGHT = 630;

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
  const tag = clamp(sp.get('tag'), 60) || '国会交易 · 13F · 期权异动 · 内部人买入';

  const isStock = kind === 'stock';
  const accent = '#0d9488'; // teal-600
  const statColor = statTone === 'down' ? '#e11d48' : statTone === 'up' ? '#059669' : '#0f172a';

  // Subset must cover every glyph we draw.
  const allText =
    'Tickwind tickwind.com 中文美股数据台' + eyebrow + title + subtitle + stat + tag;
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
      <div
        style={{
          width: '100%',
          height: '100%',
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
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                width: 56,
                height: 56,
                borderRadius: 16,
                background: accent,
                color: 'white',
                fontSize: 34,
                fontWeight: 700,
                marginRight: 18,
              }}
            >
              ↗
            </div>
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
            中文美股数据台
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
    ),
    {width: WIDTH, height: HEIGHT, fonts: fonts.length ? fonts : undefined},
  );
}
