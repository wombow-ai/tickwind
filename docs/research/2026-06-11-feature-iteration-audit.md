# Tickwind 现有功能迭代审计(2026-06-11)

> Subagent 报告(老功能迭代方向)。已读 ROADMAP、internal/api/api.go 路由面、全部 30+ 前端组件及相关后端包,结论均有代码出处。

## 一、逐功能审计表

| 功能 | 现状一句话 | 最值得的下一步 | 努力 | 影响 |
|---|---|---|---|---|
| 自选 Watchlist (Board.tsx) | 横向卡片条,仅按添加顺序,无排序/分组 | 排序(涨跌幅/代码)+ 紧凑表格视图 | M | 中高 |
| 持仓 Portfolio | 单档持仓表+总盈亏;无当日盈亏/占比/图表;混币种直接相加 | 当日盈亏(quote 已带 prev_close,纯前端)+ 占比列 | S | 高 |
| 个股价格卡 (StockView) | 盘前后分行+实时 WS+诚实时间戳,完整;缺 52 周高低/当日区间/量 | 52 周高低+当日区间(daily candles 在手) | S | 中 |
| K 线 (KLineChart) | 7 周期+4 指标+实时缝合;悬浮无 OHLC 读数、无对比线 | 十字光标 OHLC 图例 + SPY 对比 overlay | M | 中高 |
| 基本面卡 | 最新财年 6 格;无 TTM/同比/趋势 | 多年营收/净利 mini 趋势条(XBRL 历年已含) | M | 中 |
| 财报 | 个股只有"下次财报"chip;**全市场日历端点 live(332 条),前端 getEarnings 写好但零调用方** | 新建 /earnings 日历页(按日分组+自选高亮) | S | 高 |
| FINRA 轧空 (ShortChip) | 个股 chip 完整;1.6 万行全市场数据只服务单股查询 | "轧空榜"页或并入筛选器(数据全在内存) | M | 中 |
| 新闻/社交 | 英文标题直出、无加载更多、无来源过滤 | AI 中文标题翻译/摘要(enrich 插件已建,等 LLM 充值) | M | 高(待预算) |
| 筛选器 (Screener) | 仅价格/涨跌幅/盘段 3 维;代码注释自认"市值/成交量是 later" | 市值维度(SEC shares 缓存×现价)+ 行内"加自选" | M | 高 |
| 搜索 (symbols.go) | 排名式匹配扎实但**纯英文**:tokenize 只认 a-z0-9,搜"苹果/腾讯"零结果 | 热门股中文别名表 + CJK 匹配 | S/M | 高 |
| 热榜/飙升 (HotList) | 只有提及数+增速,**无股价/涨跌幅** | getHot 服务端 join universe Snapshot() 补价格列 | S | 中高 |
| 机会榜 | explainer 后端拼英文句(中文界面也英文) | explainer 改前端 i18n 拼装(字段已分开下发) | S | 中 |
| 聪明钱 13D/G | **标的只是公司名,没链到个股页**(有 CIK 无 ticker) | CIK→ticker 反查(SEC company_tickers 自带)让行可点 | S | 中高 |
| 聪明钱 国会 | 只有谁/何时/PDF 链接,无 ticker 级明细 | PTR PDF 解析出个股+方向(量大格式杂) | L | 中 |
| 大事件 | relative()(Today/in N days)硬编码英文 | 相对时间 i18n + 与财报日历互链 | S | 低中 |
| 话题页 | 话题冷却后页面变空 | GET /v1/topics/{key} 保冷链接 | M | 低 |
| 大V rail | 仅 Substack 数家;ago() 硬编码英文 | 扩 1-2 个中文 KOL 源 + i18n | S | 低中 |
| 指数条 | 美股三大指数;中文用户没有恒指 | 加 ^HSI(Yahoo 通路已验证) | S | 中 |
| 提醒 Alerts | 每 2 分钟评估在工作,但**触发后唯一可见处=该股 Alerts tab**;不可重武装、无全局列表 | 顶栏铃铛 + /alerts 中心 + 一键重新激活 | S/M | 高 |
| 笔记 | Markdown+置顶+日历,超预期完整 | 全文搜索(不急) | S | 低 |
| 评论 | 发/编/赞/举报齐;**列表不回传"我是否已赞"**(刷新红心复位);无回复 | ListComments 回传 liked;下一步单层回复 | S/M | 中 |
| i18n 全局 | 零散硬编码英文(events 相对时间、opp explainer、guru ago、Board signup 文案) | 一次性扫尾 | S | 中 |

## 二、TOP-8 迭代(按优先级)

1. **提醒中心:顶栏铃铛 + /alerts 全局页 + 重新武装(S/M,高)** — "建了但等于没建"的最大反差:NVDA 提醒触发了,除非恰好打开 NVDA 的 Alerts tab,否则永远不知道。TopNav 铃铛(轮询数 triggered 未读)+ 跨股票管理页 + 重新激活(store 已有 Active 字段)。不碰 web-push 红线。
2. **财报日历页 /earnings(S,高)** — 后端 live(332 条)、前端 getEarnings 写好、**grep 证实零组件调用**:已付 95% 成本缺最后 5%。按日分组、BMO/AMC、EPS 预估、自选高亮。
3. **搜索中文化:别名 + CJK 匹配(S/M,高)** — 中文优先产品搜不了"苹果/英伟达/台积电"。Symbol 加 Aliases,200-500 热门股中文名表(LLM 起稿人工校对),CJK 走别名子串匹配。
4. **筛选器加市值维度 + 行内加自选(M,高)** — 代码注释自认欠着;SEC shares bulk(~3 req/day)× universe 现价 → minCap/maxCap + 市值列。
5. **组合页:当日盈亏 + 配置占比(S,高)** — 当日盈亏 = Σ shares×(price−prev_close),纯前端一行公式;中式炒股 App 每天第一眼。顺修混币种相加 bug。
6. **热榜补价格/涨跌幅(S,中高)** — 榜单没股价,判断"涨出来还是跌出来的"要逐个点。universe 缓存已注入 api,join 两个字段。
7. **K 线:十字光标 OHLC 图例 + SPY 对比线(M,中高)** — subscribeCrosshairMove 画 legend;normalize 首日=100 的对比 LineSeries。
8. **自选排序/分组(M,中)** — 排序纯前端;紧凑表格视图;分组要动后端,做第二切片。

### 替补小票
13D/G 榜 ticker 化(S)· 评论"已赞"状态(S)· i18n 扫尾(S)· 指数条加 ^HSI(S)· AI 摘要按钮点亮(等 LLM key)。
