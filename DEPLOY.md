# Deploy — free, domain-only

Total cost **$0/month**. You only ever access `tickwind.com`; the VM's IP and
ports are never exposed to the internet.

```
        Cloudflare (DNS + TLS + CDN + Tunnel)
 user ─▶ tickwind.com / www  ─▶ Cloudflare Pages   (Next.js frontend)
              └─ calls https://api.tickwind.com
                          │
                  Cloudflare Tunnel (outbound-only, no open ports)
                          │
        Oracle Always-Free VM ── docker compose:
           cloudflared · tickwind-api (Go) · postgres · redis
```

## 1. Oracle Always-Free VM
- Oracle Cloud → Compute → Instances → Create.
- Shape: **VM.Standard.A1.Flex** (Ampere ARM), e.g. 2 OCPU / 12 GB (Always Free
  allows up to 4 OCPU / 24 GB total). Image: **Ubuntu 24.04 (aarch64)**. Boot
  volume up to 200 GB (free).
- If you hit "Out of capacity", try another Availability Domain or region.
- SSH in, then install Docker:
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

## 5. Frontend → Cloudflare Pages (tickwind.com)
- Cloudflare → **Workers & Pages → Create → Pages → Connect to Git** →
  `wombow-ai/tickwind`.
- Build settings: framework **Next.js (Static HTML Export)**, root directory
  `web`, build command `npm run build`, output `web/out`.
- Environment variable: `NEXT_PUBLIC_API_BASE=https://api.tickwind.com`.
- **Custom domains**: add `tickwind.com` and `www.tickwind.com` (DNS auto-set).

> SSH can also ride the tunnel (no port 22 exposed): add a Public Hostname
> `ssh.tickwind.com` → SSH → `localhost:22`, then `cloudflared access ssh`.
