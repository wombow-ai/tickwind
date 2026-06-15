# ⏳ 待 owner 确认队列（Claude 记录，owner 回来拍板）

> 这些点需要你（owner）的决策才能继续；Claude 在等待期间**只做不需确认的事**（AI 研报增量 2/3、登录门控、roadmap 功能/bug-check/数据校验/UI 优化）。

## ✅ owner 决策 (2026-06-15 owner 回来后) + 待确认开发计划
- **#1 Yahoo/paywall**:owner=移除 Yahoo 依赖,**paywall 推迟到用户起量**(没用户开浪费钱)。⚠️**仍待 owner 拍**:移除 Yahoo=港股(腾讯0700/智谱2513/MiniMax0100)报价下线(无￥0 授权替代)——Claude 荐**留到 paywall 前再移除**(现移除丢港股却无收益);owner 定:现移除 / 留到 paywall 前。
- **#2 深度研报**:(a) 模型=Claude 荐 **DeepSeek-R1**(`deepseek/deepseek-r1` 付费,强且性价比,~$0.02-0.04/篇)或 V3 付费(~$0.01/篇);(b) 配额=**每用户每月 1 篇**(付费更多)[从每天1篇改];(c) owner 充了 **$5** 测试——设付费模型即不限流(限流是免费档)。
- **#5 CF Timeout**:owner 调到 **120s**(原30s)。Claude:够(per-call 超时 25/60s),异步生成后更无所谓。
- **#6 dual-class**:owner=暂不建管线,放 roadmap + 标原因(已做)。
- **#7 异步生成**:owner=**异步生成 + 轮询**(确认)。
- **待 owner 点头的开发计划(顺序 A→D→C→B)**:**A** 深度研报升级(LLM_DEEP_MODEL=付费R1 + 配额日→月 + 异步生成+前端轮询);**D** 个股页分 Tab(概览/详情,owner"在考虑");**C** 反爬(app层限流已上线 c7eae11;荐 owner 开 CF 边缘 Rate-Limiting + Bot Fight);**B** Yahoo 移除(待 #1 港股取舍)。
- **已修(owner 报的4问题,本批,LIVE 验证)**:新IPO价兜底、movement 中英、Material/Insider 下移、per-IP 限流。rate-cut 移出首页。

---

## 1. 付费上线 + Yahoo 移除（最关键，互相绑定）
法律调研结论（`docs/research/2026-06-14-monetization-legal-risk.md`）：**一旦开始收费，整个产品即"商用"**，而 ToS 看产品是否商用、不看哪个功能收费 → "免费数据 + 付费 AI" **不**能为 non-commercial-only 数据源开脱。
- **必须在开 paywall 前移除 Yahoo Finance**（明确禁商用 + 爬非官方端点）。Yahoo 当前载重:美股报价兜底、**港股延迟报价**、指数。移除后:美股走 Alpaca(OK);**港股报价很可能要砍**(￥0 预算下无授权替代源);指数另寻。
- 还需:Nasdaq 符号表 → SEC(琐碎)、弃 StockTwits、少依赖 Finnhub 免费。
- **请确认:**
  - (a) 是否同意移除 Yahoo + **接受港股报价可能下线**?(或保留港股、推迟收费?)
  - (b) 何时开 paywall?(研报功能本身可先建好不收费,paywall 是最后一步)

## 2. AI 深度研报:模型 + 成本 + 免费/付费边界
增量 1 已上线:研报用 `LLM_DEEP_MODEL`(env),**当前未设 = fallback 现免费模型(零成本)**。付费时应换更强模型。
- **观察:当前免费 DeepSeek/OpenRouter 限流**,全平台 AI 间歇性降级 data-only(已正确降级,非 bug)。这正说明付费研报需要更可靠的(付费)模型。预算记忆里"LLM credits (pending)"。
- **请确认:**
  - (a) 深度研报用哪个 OpenRouter 模型 / 成本上限?(我可提一个"强且性价比"默认 + 单篇 token 估算供你选)
  - (b) 免费边界:免费=每用户每天 1 篇(配额已建),付费=更多/PDF?还是免费=0、纯付费?
  - (c) 是否给 LLM 充值(OpenRouter credits),让免费功能也不再限流?

## 3.（FYI，不阻塞）登录拉新门控的范围
你说"核心功能未登录看部分、完整需登录,样式我设计"。我会**保守**地设计(如:指标面板未登录显示前若干项 + 引导登录看全部,不打扰式),先做出来你回来可调。若你对"哪些核心功能该门控 / 露出多少"有具体想法,回来告诉我。

## 4.（FYI，已自主处理）opportunity board 偏小
已修空板 bug(429 clobber)。dei 股本覆盖缺口致候选被 `sh<=0` gate 掉、板子偏小(实测 4 行)。**已自主加 us-gaap 股本兜底**(commit e864dce,只对 dei 缺失的 CIK 兜底、保持 450 天 staleness + 0/1 股垃圾值守护、dei 仍为权威源、insufficient-not-wrong)放大覆盖。**✅已部署 LIVE 验证:板子 4→13 行、全部市值在带内、216 个 dei 缺失 CIK 由兜底解析。** 不需你确认。

## 5.（FYI，需你 CF 面板）冷门股研报首请求间歇性 ~3s 空响应
发现(2026-06-15):未缓存的冷门股 on-demand 端点(如 `/v1/stocks/{t}/research`)**首次**请求**间歇性**在 **~3.0s** 被重置/空响应(curl exit 52/16,无 CF 错误头、无 body),立即重试即成功(数据已缓存)。**已定位到 Cloudflare Tunnel 那一跳,不是代码 bug**(Go 无 WriteTimeout、无 3s 字面量、容器无 panic;CF 边缘超时会回 524+cf-ray,这里都没有)。`cloudflared` 是 token 隧道,ingress/超时在 **CF Zero-Trust 面板**配置,VPS 上无本地 config 可调。**对深度研报实际影响低**(用户从已预热的 /stock 页进入→装配快→不触发)。**缓解方案(待定/可选):** (a) 前端对网络/空响应错误**重试一次**(便宜,我可自主做,下一轮);(b) 你在 CF 面板调隧道 HTTP 超时;(c) 异步生成研报(返回 data-only 即时+后台预热)。详见记忆 tickwind-cold-research-3s-reset。

## 6.（已调研,owner 拍板:做不做)dual-class 正确总市值
BRK.A/BRK.B 现 `market_cap=insufficient`(stale-shares 守护正确零化了 2011 冻结的股本)。**已 investigate-first 调研**:companyfacts(app 唯一 XBRL 源)**无维度信息、且 BRK 无任何当前股本**(仅 2011 冻结值;frames API 对 member 路径 404)。per-class 当前股本只存在于 **raw inline-XBRL 实例文档**(app 不抓)。**GOOGL/GOOG 已正确**(companyfacts 有当前聚合股本 12.116B × 类价≈$4.37T;A/B/C 价相近故聚合×类价≈真总值;实测 GOOGL quote $360.87 真实非翻倍)。**故只有"无当前聚合"的双类发行人(BRK 这类)受影响。** 修需**新建 raw-XBRL 抓取+解析管线**(FilingSummary→封面实例→按 `StatementClassOfStockAxis` 维度+scale+`TradingSymbol`/`NoTradingSymbolFlag` 解析,排除债券行)+ 非交易类代理定价(如 Alphabet Class B 836M 股无 ticker,~$150B/7%)——bespoke、per-issuer、低通用性,为少数高知名度票。数学验证可行(BRK $1.066T、GOOGL $2.19T 均吻合)。**建议:defer**——`insufficient` 已诚实满足质量线,ROI 对少数票偏低,且新管线是可观工程面。**请你定:值得为 Berkshire 等建这条管线吗?**

## 7.（owner 拍板:做不做+怎么做)冷门股研报同步生成慢(付费旗舰 UX)
未缓存研报(尤其 `?depth=deep`)同步 assemble+LLM 生成,LLM 慢时阻塞 10-60s。已缓解急性问题:retry-once(c5560d4,治冷门股 Cloudflare 3s 重置)+ LLM per-call 超时(88eb75c,慢则快降 data-only)。剩余:首次未缓存请求的 10-60s 等待对**付费**旗舰首印象不佳。**根治=异步生成**:返回 data-only 即时 +"AI 分析生成中" + 后台生成 prose + 前端轮询/SSE 直到 prose 就绪。多数研报命中热门/已缓存票(快),仅冷门首次慢。**这是付费旗舰的架构/UX 决策**(轮询 vs SSE、loading 体验、是否值得),故记此待你拍板而非擅自大改。**请你定:要做异步生成吗?偏好轮询还是 SSE?**

---
*更新于 2026-06-15。Claude 在等待期间持续推进不需确认的 roadmap/数据质量工作(三次数据审计+pSEO /stocks 目录等);#1、#2 待你回来拍板;#6、#7 为已调研的 owner-facing options。*
