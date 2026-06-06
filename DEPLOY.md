# Deploy — free, domain-only

Total cost **$0/month**. You only ever access `tickwind.com`; the VM's IP and
ports are never exposed to the internet.

```
        Cloudflare (DNS + Tunnel)            Vercel (frontend TLS/CDN)
 user ─▶ tickwind.com / www  ───────────────▶ Vercel  (Next.js SSR frontend)
              └─ calls https://api.tickwind.com
                          │
                  Cloudflare Tunnel (outbound-only, no open ports)
                          │
        Your VM (any 1GB+ host) ── docker compose:
           cloudflared · tickwind-api (Go) · postgres
```

## 1. A small VM (any cheap host — 1 GB RAM is enough)
The slim stack (api + postgres + cloudflared) fits in ~1 GB. Good options:
**GCP e2-micro** (always-free, US regions), **RackNerd** (~$11–24/yr),
**Hetzner CAX11** (~€3.79/mo, 4 GB), or **IONOS/OVH** (~$2–4/mo). Use Ubuntu 24.04
(x86 or ARM — images are multi-arch). Then SSH in and install Docker:
  ```bash
  curl -fsSL https://get.docker.com | sh
  sudo usermod -aG docker $USER   # re-login after this
  ```

## 2. Code on the VM
```bash
git clone https://github.com/wombow-ai/tickwind.git && cd tickwind
cp .env.example .env
# edit .env: EDGAR_USER_AGENT (your email), POSTGRES_PASSWORD, TUNNEL_TOKEN (next step)
```

## 3. Cloudflare Tunnel → api.tickwind.com
- Cloudflare dashboard → **Zero Trust → Networks → Tunnels → Create a tunnel**
  → connector **cloudflared** → name it `tickwind` → **copy the token** into
  `.env` as `TUNNEL_TOKEN=...`.
- In the tunnel's **Public Hostnames**, add:
  - Subdomain `api`, domain `tickwind.com`
  - Service: **HTTP** → `api:8080`
- This auto-creates the `api.tickwind.com` DNS record + edge TLS. No ports opened.

## 4. Start the backend
```bash
docker compose up -d --build
docker compose logs -f api      # should show "ingested filings ..."
```
Verify: `https://api.tickwind.com/healthz` → `{"status":"ok"}`

## 5. Frontend → Vercel (tickwind.com)
The frontend is a **server-rendered** Next.js app (in `web/`), so it runs on
Vercel (not the static Pages export). DNS stays on Cloudflare.

1. **Import**: [vercel.com](https://vercel.com) → log in with GitHub → **Add New
   → Project** → import `wombow-ai/tickwind` (authorize Vercel's GitHub App for
   the repo on first use).
2. **Root Directory**: click Edit → set to **`web`** (monorepo — the Go API lives
   at the repo root). Framework auto-detects **Next.js**; keep the default build.
3. **Environment Variables** (add for Production **and** Preview):
   - `NEXT_PUBLIC_API_BASE` = `https://api.tickwind.com`
   - `NEXT_PUBLIC_SUPABASE_URL` = `https://<project-ref>.supabase.co`
   - `NEXT_PUBLIC_SUPABASE_ANON_KEY` = `<anon public key>`
4. **Deploy** → open the `*.vercel.app` URL and test the board, a stock page, and
   `/login`. Every push to `main` then auto-deploys.

### Custom domain (DNS stays on Cloudflare)
5. Vercel project → **Settings → Domains** → add `tickwind.com` (and `www`).
6. In **Cloudflare → DNS**, point apex + www at Vercel with **proxy OFF (grey
   cloud / “DNS only”)** so Vercel can issue TLS without a redirect loop:

   | Type  | Name           | Value                   | Proxy    |
   |-------|----------------|-------------------------|----------|
   | A     | `tickwind.com` | `76.76.21.21`           | DNS only |
   | CNAME | `www`          | `cname.vercel-dns.com`  | DNS only |

   Leave the **`api.tickwind.com`** tunnel record untouched. (If you’d rather keep
   the apex proxied/orange, set Cloudflare **SSL/TLS → Full (strict)**; grey is
   simplest and avoids redirect loops.)

### Supabase auth redirect (so confirmation/login links resolve)
7. Supabase → **Authentication → URL Configuration**:
   - **Site URL**: `https://tickwind.com`
   - **Redirect URLs**: `https://tickwind.com/**` and `https://*.vercel.app/**`

### Optional: "Continue with Google"
The button is **hidden** until you enable it, so you can ship without it.
8. Google Cloud Console → **APIs & Services → Credentials → OAuth client ID**
   (Web). Authorized redirect URI: `https://<project-ref>.supabase.co/auth/v1/callback`.
9. Supabase → **Authentication → Providers → Google**: enable it, paste the
   Client ID + Secret. Ensure `https://tickwind.com/auth/callback` is covered by
   the Redirect URLs above.
10. Set **`NEXT_PUBLIC_GOOGLE_OAUTH=1`** in Vercel to reveal the button. The app's
    `/auth/callback` route exchanges the code for a session and redirects home.

> Recommended order: get the `*.vercel.app` URL working first, then switch the
> custom domain + Supabase URLs.
>
> Cost: Vercel **Hobby** is free for personal use; commercial traffic should move
> to **Pro** (~$20/mo). The Go backend stays $0 on your VM behind the tunnel.

> SSH can also ride the tunnel (no port 22 exposed): add a Public Hostname
> `ssh.tickwind.com` → SSH → `localhost:22`, then `cloudflared access ssh`.
