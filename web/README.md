# Tickwind — web

Next.js (App Router, TypeScript, Tailwind) frontend for Tickwind, configured for
**static export** (`output: 'export'`) and deployed to Cloudflare Pages
(output dir `web/out`).

## Develop

```bash
npm install
npm run dev          # http://localhost:3000
```

Point it at the API with `NEXT_PUBLIC_API_BASE` (default `http://localhost:8080`):

```bash
cp .env.example .env.local   # then edit if needed
```

Run the Go backend separately (`make run` in the repo root) for live data.

## Build (static export)

```bash
npm run build        # emits ./out
```

`NEXT_PUBLIC_API_BASE` is read at build time and baked into the export, so set it
before building for production (e.g. `https://api.tickwind.com`).

## Routes

- `/` — watchlist board (default tickers in `src/lib/config.ts`).
- `/stock?ticker=AAPL` — security header + SEC filings timeline. Query params are
  used instead of dynamic segments to keep the static export simple.

## Layout

```
src/
  app/            routes (layout, home, /stock) + global styles
  components/     reusable UI (cards, badges, timeline, states)
  lib/            typed API client, config, hooks, formatters
```
