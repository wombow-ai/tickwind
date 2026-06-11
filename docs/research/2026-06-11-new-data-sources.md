# Tickwind 新数据源调研(2026-06-11,全部 curl 实测)

> Subagent 报告(新数据源方向)。风险色:🟢=官方公开 🟡=灰色但普遍被容忍(免费展示 OK,别转售) 🔴=牌照墙/明确禁止。

## TOP 10

1. **Cboe 延迟期权链(免费无鉴权 CDN JSON)** — `https://cdn.cboe.com/api/global/delayed_quotes/options/{TICKER}.json`(指数加下划线 `_SPX.json`/`_VIX.json`;标的快照在 `/delayed_quotes/quotes/`)。15 分钟延迟全链:bid/ask、volume、OI、IV、希腊字母(实测 AAPL/SPX 返回 delta/gamma/theo);`_VIX` 实时快照(实测 22.22)。无需注册。🟡(OPRA 实时/转售是红线,延迟+自算指标低风险)。功能:期权异动榜、个股 P/C、Max Pain、IV 曲线、VIX 仪表盘。努力 M。备援:Alpaca Basic 免费档期权 indicative feed。
2. **SEC Form 4 卖出 + 10b5-1 计划标记** — ownershipDocument XML 的 `<aff10b5One>` 字段(2023-04 起强制)。区分"例行计划单"与"主动砸盘",中文产品几乎没有。🟢。努力 **S**(现有解析器加两字段)。
3. **SEC 13F 大佬持仓 + OpenFIGI CUSIP 映射** — 13F 结构化季度数据集(实测 200)+ EDGAR 13F-HR XML;CUSIP→ticker 用 `POST https://api.openfigi.com/v3/mapping`(实测**无 key 可用** ~25 次/分)。🟢。功能:大佬季度增减仓 diff、机构抱团榜。努力 M。
4. **FRED + 美债官方收益率** — FRED 免费 key 120 req/min;财政部日度收益率 XML + FiscalData API 无鉴权(均实测 200)。🟢。功能:中文宏观仪表盘(CPI/非农/利率/2s10s 倒挂)联动 BLS 日历。努力 S。
5. **FINRA ATS 暗池周报** — `GET https://api.finra.org/data/group/otcMarket/name/weeklySummary`(实测匿名 200)。每股每周暗池成交量/笔数(Tier1 延迟 2 周)。🟢(官方条款:可免费向终端用户再分发)。功能:个股暗池占比曲线+周榜。努力 S(已有 FINRA 集成)。
6. **IBKR 借券费率/可借量** — FTP `ftp3.interactivebrokers.com`(用户 `shortstock` 无密码,~15 分钟更新);镜像 `https://www.iborrowdesk.com/api/ticker/{SYM}`(需浏览器 UA+跟随 301;实测 GME 数据新到 2026-06-10)。🟡。功能:**轧空雷达三信号合成**(借券费↑+可借量↓+空头利息↑)。努力 S/M(FTP 需上 VPS 验证)。
7. **Wikipedia 页面浏览量** — `wikimedia.org/api/rest_v1/metrics/pageviews/per-article/...`(实测,zh 同理)。🟢。功能:散户关注度 z-score 异动,补充 WSB 热度。努力 S。
8. **Finnhub 免费档增量端点(现有 key)** — `/calendar/ipo` + `/stock/recommendation`(均确认免费档;**price-target 付费,别碰**)。功能:IPO 日历页 + 个股评级趋势条。努力 S。备援:api.nasdaq.com IPO 日历(需浏览器头,🟡)。
9. **Alpaca 公司行动 API(现有 key)** — `/v1/corporate-actions?types=cash_dividend,forward_split`。分红/拆股/并购,2020-04 起。🟢。功能:分红拆股日历+自选除权提醒。努力 S(用现有 key 调一次确认套餐)。
10. **CoinGecko + alternative.me 恐惧贪婪** — 均实测可用(当前 FNG=12 Extreme Fear)。CoinGecko Demo 免费 10,000 次/月(需署名+链接)。功能:加密页签+币股联动(COIN/MSTR/RIOT vs BTC)+恐贪角标。努力 S。

## 次级备选
FDA AdComm 日历+openFDA+ClinicalTrials v2(🟢,拼装生物医药催化日历,L)· GitHub star 增速(🟢,S,小众)· Apple 榜单 RSS 新址 `rss.marketingtools.apple.com`(🟡,M)· Greenhouse/Lever 招聘 JSON(🟡,L)· SEC N-PORT ETF 持仓(🟢,~60 天延迟,M/L)。

## 查过但不行(别再研究)
- CNN Fear & Greed:实测 418 "You're a bot",已封服务器抓取 🔴(用 VIX+P/C 自算或只展示加密版)
- Cboe 经典全市场 P/C CSV:冻结在 2019/2012,已死
- OPRA 实时期权 / CME 期货:牌照墙 🔴(期货点位用 Yahoo ES=F/NQ=F 凑,实测可用 🟡)
- Google Trends 官方 API:仍 alpha 邀请制;pytrends 429 频繁+ToS 🔴
- 分析师目标价:Finnhub/Nasdaq 付费,TipRanks/MarketBeat 🔴 反爬
- ETF 资金流:全付费墙 🔴
- Citi 经济意外指数:专有 🔴
- IEX Cloud:2024-08-31 已关停
- Alpha Vantage:免费档 25 次/天撑不起功能
- Nasdaq Data Link:免费数据集停更
- PatentsView:legacy API 已关停、迁移动荡中,弃
- Twitter/X API:$200/月起 🔴

**一句话**:前 5 名(期权异动、内部人卖出+10b5-1、13F、宏观仪表盘、暗池占比)约 2-3 周工作量、全 🟢/🟡 零数据成本,能立起"机构/内部人行为 + 期权情绪"差异化产品线。
