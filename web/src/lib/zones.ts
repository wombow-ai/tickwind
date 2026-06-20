/**
 * Curated investment-theme "专区" (zones) — a HYBRID engine (see docs/zones-plan.md):
 * one zone shape, instantiated as the AI flagship (deep, multi-layer, chokepoint-
 * flagged) and later as lighter 10x theme siblings.
 *
 * ANTI-HALLUCINATION CONTRACT: this config is pure EDITORIAL STRUCTURE — layer names,
 * company names, rationales, and chokepoint flags. It contains ZERO market numbers.
 * Every price / % change / market cap is fetched LIVE from the Go backend at render
 * time (useQuotes / getBarsBatch). Tickers are real, US-listed (or US ADRs) and are
 * validated against /v1/symbols; foreign-listed or private chokepoint companies (SK
 * Hynix, Samsung, Schneider, OpenAI, Anthropic) are mentioned by NAME ONLY — never
 * given an invented US symbol.
 */

/** A real, US-listed (or US-ADR) ticker with its editorial rationale. */
export interface ZoneTicker {
  ticker: string;
  company: string;
  /** One-line "why it's here" (English; EN-first). */
  rationale: string;
  /** Owns/controls a supply-chain chokepoint → badge. */
  chokepoint?: boolean;
  /** Pre-revenue / recent-IPO / binary-outcome name → speculative badge. */
  speculative?: boolean;
}

/** A foreign-listed or private company named in a layer but with NO tradable US ticker. */
export interface ZoneNamed {
  company: string;
  note: string;
}

/** One layer (AI flagship) or sub-theme (10x sibling) of a zone. */
export interface ZoneLayer {
  key: string;
  titleEn: string;
  titleZh: string;
  blurbEn: string;
  blurbZh: string;
  /** Layer-level chokepoint (the pick-and-shovel bottleneck). */
  chokepoint?: boolean;
  tickers: ZoneTicker[];
  /** Text-only mentions (no US ticker). */
  named?: ZoneNamed[];
}

/** A top-level curated zone. */
export interface Zone {
  key: string;
  kind: 'ai-flagship' | 'tenx-theme';
  titleEn: string;
  titleZh: string;
  taglineEn: string;
  taglineZh: string;
  descEn: string;
  descZh: string;
  /** Whole-zone speculative banner (e.g. quantum / gene-editing). */
  speculative?: boolean;
  layers: ZoneLayer[];
}

const AI_ZONE: Zone = {
  key: 'ai',
  kind: 'ai-flagship',
  titleEn: 'The AI Stack',
  titleZh: 'AI 产业链专区',
  taglineEn: "Jensen's five-layer cake — Energy → Chips → Infrastructure → Models → Applications",
  taglineZh: '黄仁勋「五层蛋糕」:供电 → 芯片 → 基础设施 → 大模型 → 应用',
  descEn:
    "The AI build-out, layer by layer — from the power that feeds it, to the chips that compute it, to the models and apps that capture the value. Each layer's key public companies, with the supply-chain chokepoints flagged. Curated structure; live delayed prices from public data. Not investment advice.",
  descZh:
    'AI 产业链,一层一层看 —— 从供电、算力芯片,到大模型与应用价值捕获。每层的关键上市公司,并标出供应链「卡脖子」环节。层级为人工策展,价格为公开数据实时(延迟)行情。非投资建议。',
  layers: [
    {
      key: 'energy',
      titleEn: 'Energy — Power & Cooling',
      titleZh: '能源 —— 供电与散热',
      blurbEn:
        'The hard physical floor: power generation, electrical distribution gear (transformers, switchgear, UPS), and liquid cooling for the 50–130 kW racks of an AI factory. Power availability caps how much intelligence a region can produce.',
      blurbZh:
        '硬性物理地基:发电、配电设备(变压器、开关柜、UPS),以及为 AI 工厂 50–130kW 机柜散热的液冷。电力供给决定一个地区能产出多少智能。',
      tickers: [
        {ticker: 'VRT', company: 'Vertiv Holdings', rationale: 'Cleanest public pure-play on AI data-center power management + liquid cooling.'},
        {ticker: 'ETN', company: 'Eaton', rationale: 'Broad electrical portfolio (power distribution, UPS/PDU) + liquid cooling; data-center orders surging.'},
        {ticker: 'GEV', company: 'GE Vernova', rationale: 'Supplies the electrons — gas turbines + grid electrification gear for AI campuses.'},
        {ticker: 'PWR', company: 'Quanta Services', rationale: 'Builds + connects the grid/substation power infrastructure feeding new AI data centers.'},
        {ticker: 'CEG', company: 'Constellation Energy', rationale: 'Largest US merchant/nuclear generator; signs power deals directly with hyperscalers.'},
        {ticker: 'VST', company: 'Vistra', rationale: 'Independent power producer (gas + nuclear) selling firm capacity to data-center load growth.'},
        {ticker: 'TLN', company: 'Talen Energy', rationale: 'Nuclear-heavy IPP; pioneered co-located data-center-at-the-reactor power deals.'},
      ],
      named: [
        {company: 'Schneider Electric', note: 'Major power/cooling systems vendor — French-listed (Euronext SU.PA), no clean US common ticker.'},
      ],
    },
    {
      key: 'chips',
      titleEn: 'Chips — Compute Silicon',
      titleZh: '芯片 —— 算力硅',
      chokepoint: true,
      blurbEn:
        'The pick-and-shovel bottleneck: merchant GPUs and custom AI ASICs, plus the upstream nobody can route around — TSMC’s leading-edge foundry + ~90% of CoWoS advanced packaging, ASML’s EUV-litho monopoly, and the sold-out HBM oligopoly. Supply, not demand, gates accelerator volume here.',
      blurbZh:
        '卖铲子的瓶颈:GPU 与定制 AI ASIC,加上谁都绕不开的上游 —— 台积电先进制程 + 约 90% 的 CoWoS 先进封装、ASML 的 EUV 光刻垄断、供不应求的 HBM 寡头。这一层由供给(而非需求)决定加速器产量。',
      tickers: [
        {ticker: 'NVDA', company: 'NVIDIA', rationale: 'Dominant merchant AI GPU + CUDA + NVLink; the reference platform for the whole stack.'},
        {ticker: 'AMD', company: 'Advanced Micro Devices', rationale: 'Instinct MI GPUs + EPYC CPUs; the credible second source in training/inference accelerators.'},
        {ticker: 'AVGO', company: 'Broadcom', rationale: 'Co-designs custom AI ASICs (XPUs) for the hyperscalers; ~70% of the custom-accelerator design market.'},
        {ticker: 'MRVL', company: 'Marvell Technology', rationale: 'Other custom-ASIC house (Trainium, Maia) + data-center networking/optical silicon.'},
        {ticker: 'TSM', company: 'Taiwan Semiconductor', rationale: 'THE chokepoint: sole leading-edge foundry + ~90% of CoWoS packaging that gates every accelerator.', chokepoint: true},
        {ticker: 'ASML', company: 'ASML Holding', rationale: 'EUV-lithography monopoly — no advanced AI logic chip exists without its machines.', chokepoint: true},
        {ticker: 'MU', company: 'Micron Technology', rationale: 'Only US-listed HBM supplier; HBM is a sold-out 3-player oligopoly gating GPU memory bandwidth.', chokepoint: true},
        {ticker: 'ARM', company: 'Arm Holdings', rationale: 'CPU instruction-set IP under NVIDIA Grace + most data-center/edge CPUs — a royalty toll on compute.'},
        {ticker: 'AMAT', company: 'Applied Materials', rationale: 'Largest wafer-fab equipment maker; arms the foundries expanding leading-edge + packaging capacity.'},
        {ticker: 'LRCX', company: 'Lam Research', rationale: 'Etch/deposition leader critical to HBM stacking + advanced-node and packaging buildout.'},
      ],
      named: [
        {company: 'SK Hynix', note: 'HBM market leader (~62%) — Korea-listed (KRX 000660), no liquid US common ticker.'},
        {company: 'Samsung Electronics', note: 'Third HBM supplier + foundry — Korea-listed (KRX 005930), only foreign/GDR lines.'},
      ],
    },
    {
      key: 'infrastructure',
      titleEn: 'Infrastructure — The AI Factory',
      titleZh: '基础设施 —— AI 工厂',
      blurbEn:
        'Wiring tens of thousands of chips into one machine: GPU server/rack integration, in-rack connectivity silicon, optical interconnect that beats the “copper wall” between racks, and the switch fabric. A single degraded interconnect port stalls a whole training job.',
      blurbZh:
        '把几万颗芯片连成一台机器:GPU 服务器/机柜集成、机柜内互联芯片、突破机柜间「铜墙」的光互联、交换网络。一个互联端口降速就能拖住整个训练任务。',
      tickers: [
        {ticker: 'SMCI', company: 'Super Micro Computer', rationale: 'Leading GPU-server / liquid-cooled rack integrator assembling silicon into deployable AI-factory racks.'},
        {ticker: 'DELL', company: 'Dell Technologies', rationale: 'Tier-1 AI server + storage integrator shipping full GPU rack systems to hyperscalers + enterprises.'},
        {ticker: 'HPE', company: 'Hewlett Packard Enterprise', rationale: 'AI servers + Cray supercomputing systems and the orchestration to run them at scale.'},
        {ticker: 'ANET', company: 'Arista Networks', rationale: 'Ethernet switching leader for AI back-end fabrics — the merchant alternative to InfiniBand.'},
        {ticker: 'ALAB', company: 'Astera Labs', rationale: 'In-rack connectivity silicon pure-play (PCIe/CXL retimers + fabric controllers) wiring GPUs to CPUs/memory.'},
        {ticker: 'CRDO', company: 'Credo Technology', rationale: 'Active Electrical Cables + SerDes connecting GPUs within/between racks; rack-scale connectivity pure-play.'},
        {ticker: 'COHR', company: 'Coherent', rationale: 'Optical transceivers + silicon photonics connecting servers across the data center (the copper wall).'},
        {ticker: 'LITE', company: 'Lumentum Holdings', rationale: 'Optical components / lasers (EMLs) for 1.6T transceivers; named supplier in next-gen interconnect.'},
        {ticker: 'CIEN', company: 'Ciena', rationale: 'Coherent optical / DWDM systems for campus-to-campus data-center interconnect; record backlog.'},
      ],
    },
    {
      key: 'models',
      titleEn: 'Models — Foundation Models',
      titleZh: '大模型 —— 基础模型',
      blurbEn:
        'The foundation models across language, biology, physics, and robotics. A breakthrough at the top drives demand all the way down the stack. The frontier leaders are largely private — public exposure comes via their compute + cloud partners.',
      blurbZh:
        '语言、生物、物理、机器人领域的基础大模型。顶层突破把需求一路向下传导。前沿领跑者多为非上市公司 —— 公开敞口主要通过其算力与云合作方获得。',
      tickers: [
        {ticker: 'GOOGL', company: 'Alphabet (Google DeepMind)', rationale: 'Owns Gemini + DeepMind + the TPU stack — the cleanest public pure-frontier-model proxy.'},
        {ticker: 'META', company: 'Meta Platforms', rationale: 'Open-weight Llama family + massive in-house AI compute; the public open-model proxy.'},
        {ticker: 'MSFT', company: 'Microsoft', rationale: 'Deep OpenAI partnership + in-house models + Azure model hosting — public exposure to frontier models.'},
      ],
      named: [
        {company: 'OpenAI', note: 'Frontier-model leader, PRIVATE — no public ticker (reference editorially via MSFT).'},
        {company: 'Anthropic', note: 'Frontier lab (Claude), PRIVATE — no public ticker.'},
      ],
    },
    {
      key: 'applications',
      titleEn: 'Applications — Value Capture',
      titleZh: '应用 —— 价值捕获',
      blurbEn:
        'Where the economic value is captured: copilots, agents, autonomy, and applied AI on enterprise data. Higher-multiple than the layers below — value here depends on AI-budget share, not a physical chokepoint.',
      blurbZh:
        '价值落地层:Copilot、智能体、自动驾驶,以及企业数据上的应用 AI。估值倍数高于下层 —— 价值取决于 AI 预算份额,而非物理瓶颈。',
      tickers: [
        {ticker: 'MSFT', company: 'Microsoft', rationale: 'Copilot across M365/Dynamics + GitHub Copilot — the broadest enterprise AI-app distribution.'},
        {ticker: 'CRM', company: 'Salesforce', rationale: 'Einstein / Agentforce — agentic AI embedded in the dominant CRM workflow.'},
        {ticker: 'NOW', company: 'ServiceNow', rationale: 'Now Assist — AI agents automating IT / enterprise service workflows.'},
        {ticker: 'PLTR', company: 'Palantir', rationale: 'AIP operationalizes models against enterprise/government data — an applied-AI deployment platform.'},
        {ticker: 'SNOW', company: 'Snowflake', rationale: 'Data + AI app platform (Cortex) where enterprises build/run AI on their own data.'},
        {ticker: 'TSLA', company: 'Tesla', rationale: 'Physical AI — FSD autonomy + Optimus humanoid robotics (Huang’s self-driving + robots example).'},
        {ticker: 'DDOG', company: 'Datadog', rationale: 'Observability for AI apps — the picks-and-shovels of running AI in production.'},
      ],
    },
  ],
};

const SPACE_ZONE: Zone = {
  key: 'space',
  kind: 'tenx-theme',
  titleEn: 'Launch & Space',
  titleZh: '航天专区',
  taglineEn: 'Launch · satellites · defense-space primes · eVTOL',
  taglineZh: '发射 · 卫星 · 防务航天巨头 · eVTOL',
  descEn:
    'The space economy across four sub-themes — launch & infrastructure, satellites & direct-to-device, defense-space primes, and eVTOL air mobility. The defense primes are the quality anchors; many new-space names are recent IPOs or pre-revenue (flagged). Curated structure; live delayed prices. Not investment advice.',
  descZh:
    '航天经济四大子主题 —— 发射与基础设施、卫星与直连手机、防务航天巨头、eVTOL 飞行汽车。防务巨头是质量锚,多数新航天公司为近期 IPO 或尚未盈利(已标注)。层级为人工策展,价格为公开数据实时(延迟)行情。非投资建议。',
  layers: [
    {
      key: 'launch',
      titleEn: 'Launch & Space Infrastructure',
      titleZh: '发射与基础设施',
      blurbEn: 'Getting mass to orbit + the in-space infrastructure to use it. Several names IPO\'d in 2025 with short trading histories — higher risk.',
      blurbZh: '把载荷送上轨道 + 在轨基础设施。多家公司 2025 年才 IPO、交易历史短 —— 风险更高。',
      tickers: [
        {ticker: 'RKLB', company: 'Rocket Lab', rationale: 'Most diversified small/mid-cap space pure-play: launch + satellite systems + defense.'},
        {ticker: 'LUNR', company: 'Intuitive Machines', rationale: 'Lunar landers + NASA/defense backlog; closest to profitability among new-space names (execution risk).', speculative: true},
        {ticker: 'FLY', company: 'Firefly Aerospace', rationale: 'Launch + lunar; IPO\'d Aug 2025, large backlog but very short trading history.', speculative: true},
        {ticker: 'VOYG', company: 'Voyager Technologies', rationale: 'Starlab commercial space station + in-space infrastructure; IPO\'d Jun 2025, pre-scale.', speculative: true},
        {ticker: 'KRMN', company: 'Karman Holdings', rationale: 'Payload/propulsion systems for missiles + launch; IPO\'d Feb 2025.', speculative: true},
      ],
    },
    {
      key: 'satellites',
      titleEn: 'Satellites & Direct-to-Device',
      titleZh: '卫星与直连手机',
      blurbEn: 'Earth-observation data + satellite-to-phone connectivity. Big TAMs, but some are binary on technical execution.',
      blurbZh: '对地观测数据 + 卫星直连手机连接。市场空间大,但部分公司成败取决于技术能否规模化(二元)。',
      tickers: [
        {ticker: 'ASTS', company: 'AST SpaceMobile', rationale: 'Direct satellite-to-standard-smartphone connectivity; binary but enormous TAM if it scales.', speculative: true},
        {ticker: 'PL', company: 'Planet Labs', rationale: 'Large Earth-observation fleet with recurring data revenue + growing backlog — the space-data anchor.'},
        {ticker: 'GSAT', company: 'Globalstar', rationale: 'Satellite connectivity with a large anchor-customer (Apple emergency SOS) relationship.', speculative: true},
      ],
    },
    {
      key: 'defense-primes',
      titleEn: 'Defense-Space Primes',
      titleZh: '防务航天巨头',
      blurbEn: 'The quality anchors: profitable defense primes with major space + missile-defense exposure, dividends, lower beta.',
      blurbZh: '质量锚:盈利的防务巨头,航天与导弹防御敞口大、有分红、波动较低。',
      tickers: [
        {ticker: 'LMT', company: 'Lockheed Martin', rationale: 'Defense prime with major space + missile-defense exposure; quality anchor with a dividend.'},
        {ticker: 'RTX', company: 'RTX Corporation', rationale: 'Defense prime (missiles/sensors/space sensors); diversified aerospace-defense anchor.'},
        {ticker: 'NOC', company: 'Northrop Grumman', rationale: 'Space systems + strategic missile/defense prime; strong space-segment exposure.'},
        {ticker: 'LHX', company: 'L3Harris Technologies', rationale: 'Space sensors/electronics + defense tech; higher-margin electronics tilt vs pure primes.'},
        {ticker: 'KTOS', company: 'Kratos Defense', rationale: 'Drones, hypersonics propulsion, satcom; revenue inflection expected (higher-growth tilt).', speculative: true},
        {ticker: 'LDOS', company: 'Leidos Holdings', rationale: 'Government IT/services supporting space (ground systems, mission software) for NASA/Space Force.'},
      ],
    },
    {
      key: 'evtol',
      titleEn: 'eVTOL — Advanced Air Mobility',
      titleZh: 'eVTOL 飞行汽车',
      blurbEn: 'Electric vertical-takeoff aircraft. Both leaders are pre-commercial-revenue and certification-gated — speculative.',
      blurbZh: '电动垂直起降飞行器。两家龙头均尚未商业化、受适航认证制约 —— 投机。',
      tickers: [
        {ticker: 'JOBY', company: 'Joby Aviation', rationale: 'Leading eVTOL developer, well-capitalized; pre-commercial-revenue, certification-gated.', speculative: true},
        {ticker: 'ACHR', company: 'Archer Aviation', rationale: 'eVTOL (Midnight) with FAA compliance progress + a Palantir defense-systems tie-up.', speculative: true},
      ],
    },
  ],
};

const GLP1_ZONE: Zone = {
  key: 'glp1-obesity',
  kind: 'tenx-theme',
  titleEn: 'GLP-1 & Obesity',
  titleZh: 'GLP-1 减肥药专区',
  taglineEn: 'The profitable obesity-drug duopoly + the oral challenger',
  taglineZh: '盈利的减肥药双寡头 + 口服挑战者',
  descEn:
    'The obesity / GLP-1 drug wave — the most fundamentally-grounded 10x theme, anchored by two profitable mega-cap leaders with real, large revenue today, plus a high-risk clinical-stage challenger. Curated structure; live delayed prices. Not investment advice.',
  descZh:
    '肥胖症 / GLP-1 减肥药浪潮 —— 最有基本面支撑的 10 倍股主题:两家盈利的大盘龙头(今天就有真实、可观营收)+ 一家高风险临床期挑战者。层级为人工策展,价格为公开数据实时(延迟)行情。非投资建议。',
  layers: [
    {
      key: 'leaders',
      titleEn: 'The Duopoly Leaders',
      titleZh: '双寡头龙头',
      blurbEn: 'Two profitable mega-caps that own today\'s GLP-1 market and are racing on next-gen oral formulations.',
      blurbZh: '两家盈利的大盘股,主导当下 GLP-1 市场,并在下一代口服剂型上竞速。',
      tickers: [
        {ticker: 'LLY', company: 'Eli Lilly', rationale: 'Co-leader (Mounjaro/Zepbound) pulling ahead on oral orforglipron; profitable mega-cap.'},
        {ticker: 'NVO', company: 'Novo Nordisk (ADR)', rationale: 'Co-leader (semaglutide / Wegovy / Ozempic) defending its franchise with oral amycretin; profitable.'},
      ],
    },
    {
      key: 'challenger',
      titleEn: 'The Clinical-Stage Challenger',
      titleZh: '临床期挑战者',
      blurbEn: 'The high-risk, high-reward third entrant — binary on Phase III trial readouts.',
      blurbZh: '高风险高回报的第三玩家 —— 成败取决于三期试验数据(二元)。',
      tickers: [
        {ticker: 'VKTX', company: 'Viking Therapeutics', rationale: 'Dual GIPR/GLP-1 agonist VK2735 (oral + injectable) heading to Phase III — the high-beta challenger.', speculative: true},
      ],
    },
  ],
};

/** The zone catalog. AI is the flagship; 10x theme siblings reuse the same engine. */
export const ZONES: readonly Zone[] = [AI_ZONE, SPACE_ZONE, GLP1_ZONE] as const;

/** Resolves a zone by its URL slug, or undefined. */
export function zoneByKey(key: string): Zone | undefined {
  return ZONES.find(z => z.key === key);
}

/** Every distinct real ticker in a zone (for one batched live-quote fetch). */
export function zoneTickers(z: Zone): string[] {
  const seen = new Set<string>();
  for (const layer of z.layers) for (const t of layer.tickers) seen.add(t.ticker);
  return [...seen];
}
