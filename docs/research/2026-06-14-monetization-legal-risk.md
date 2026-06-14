# Monetization legal/ToS risk — paid AI on free data (2026-06-14)

**Question (owner):** if we charge ONLY for the AI deep-research feature while keeping the data display free, is it risky to keep using free/free-tier data sources?

**Key legal principle:** restrictive ToS gate on *"is the **product** commercial?"*, NOT on *"which feature is paid."* Any paid tier makes Tickwind a commercial product, so "free data + paid AI" does **NOT** create a safe harbor for sources whose terms say "personal/non-commercial use only." Structuring the paywall around AI (not data) only helps for the congressional-disclosure statute (news-dissemination exception).

## Per-source verdict
| Source | Use | Rating | Note |
|---|---|---|---|
| **SEC EDGAR** | filings/XBRL/Form4/13D-G | 🟢 | public domain; UA + 10 req/s only. **Paid AI on EDGAR is clearly safe.** |
| **Treasury.gov** | yields | 🟢 | public domain |
| **FINRA** | short volume | 🟢/🟡 | public dissemination; display OK, don't redistribute bulk |
| **CoinGecko** | BTC/ETH | 🟢 | commercial OK **but requires "Powered by CoinGecko" attribution**; no resale |
| **alternative.me** | crypto F&G | 🟢 | commercial OK **with attribution next to the gauge** |
| **Bluesky / AT Proto** | social | 🟢 | public API by design; display public posts is the intent |
| **Tickertick** | news/UGC | 🟢 | MIT, commercial OK; 10 req/min |
| **Alpaca (IEX free)** | prices | 🟡 | commercial allowed **WITH 30-day "User Application" written notice**; no raw redistribution. Load-bearing → resolve. |
| **US Congress trades** | disclosures | 🟡 | Ethics-in-Gov Act bars "commercial purpose" EXCEPT news dissemination to the public → **keep the display free to all** (leans on the exception); don't sell the raw data |
| **Nasdaq IPO calendar** | IPO | 🟡 | undocumented endpoint, no clear license; low-value |
| **ApeWisdom** | buzz | 🟡 | no published terms; aggregates Reddit (restricted) → display aggregate counts only |
| **Yahoo Finance** | quotes/bars/HK/indices | 🔴 | **commercial use BANNED + scraping undocumented endpoints. #1 blocker.** |
| **StockTwits** | social | 🔴 | personal/non-commercial license + anti-scraping |
| **Nasdaq Trader** | symbol lists | 🔴 | "internal non-commercial usage only" — but trivially replaceable by SEC tickers |
| **Finnhub (free tier)** | news | 🔴/🟡 | free tier = non-professional/personal; commercial needs a paid license |

## Bottom line
**Cannot safely flip on paid AI today without first removing Yahoo.** A clean, fully-safe foundation for a paid product = **EDGAR + Treasury + FINRA(display) + Tickertick + Bluesky + CoinGecko(attr) + alternative.me(attr) + Alpaca IEX(notice sent).**

### Must-fix BEFORE charging
1. **Remove Yahoo Finance** (commercial ban + scraping). It's load-bearing (US quote/bar fallback, HK delayed quotes, indices) → route through Alpaca; **HK quotes may have to drop** unless a licensed feed is found (product tradeoff for owner). **#1 blocker.**
2. **Swap Nasdaq Trader symbol files → SEC `company_tickers.json`** (public domain). Trivial, removes a clear violation. *(Note: ties into the symbols-directory work; SEC is already a source.)*

### Should-fix (low effort, real risk)
3. **Drop StockTwits** → replace social with Bluesky + Tickertick + ApeWisdom.
4. **Stop relying on Finnhub free tier** for news → lean on Tickertick + SEC, or buy a commercial tier.

### Do this week (free, cheap)
- Send Alpaca the 30-day commercial / "User Application" notice (or confirm in writing).
- Add **"Powered by CoinGecko"** + **"Data from alternative.me"** attributions.
- Add a footer/about: "Market data delayed · informational only · not investment advice."
- Keep congressional-trade **display free to all** (don't paywall the raw disclosure data).
- **Never redistribute raw bulk data** from any source — only display derived/individual views (satisfies most "no redistribution" clauses).

**Lowest-risk paid feature:** an AI deep-research report built primarily on **EDGAR + Treasury + FINRA public data** — exactly the kind of feature being planned. Prioritize public-domain sources in the report content.

**Ambiguity flags:** Alpaca (does a public web app trigger the "User Application" notice → send it / ask), Congress (news-media exception is real but fact-specific), Nasdaq-IPO/ApeWisdom (no published terms). The 🔴 items are unambiguous.

*(Full sourced assessment with ToS quotes/URLs: subagent run addc6f274b8576ff1, 2026-06-14.)*
