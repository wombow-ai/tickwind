package edgar

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// MaterialEvent is one 8-K (current report) filing — the SEC vehicle for material
// corporate events (M&A, executive changes, earnings pre-announcements,
// bankruptcies, etc.). Every field here is a FACT owned by Go (parsed straight
// from the SEC submissions feed); the optional plain-language Summary is the only
// LLM-written part and is filled in by a higher layer, never by this package.
type MaterialEvent struct {
	// Form is "8-K" or "8-K/A" (the amendment variant).
	Form string `json:"form"`
	// Amendment reports whether this is an 8-K/A (an amendment to a prior 8-K).
	Amendment bool `json:"amendment"`
	// FiledDate is the SEC filing date (YYYY-MM-DD).
	FiledDate string `json:"filed_date"`
	// ReportDate is the period-of-report / event date (YYYY-MM-DD), when present.
	ReportDate string `json:"report_date,omitempty"`
	// AccessionURL is the human-readable filing index page on sec.gov.
	AccessionURL string `json:"accession_url"`
	// PrimaryDocURL is the direct URL to the primary document (used to fetch the
	// summary source text); omitted from the wire shape (internal).
	PrimaryDocURL string `json:"-"`
	// Items are the parsed 8-K item codes with their canonical Go-owned labels.
	Items []EventItem `json:"items"`
	// Summary is the OPTIONAL LLM-written plain-language summary. Empty when the
	// LLM is disabled, the source text was too thin, or the summary failed — the
	// item labels alone are still useful, so the event is never dropped.
	Summary string `json:"summary,omitempty"`
}

// EventItem is one parsed 8-K item code with its canonical English and Chinese
// labels. The label mapping is OWNED BY GO (itemLabels) — the anti-hallucination
// anchor: an LLM must NEVER decide what an item code means. An unknown/future code
// falls back to a generic "Item X.XX" label (never a fabricated meaning).
type EventItem struct {
	Code    string `json:"code"`
	LabelEN string `json:"label_en"`
	LabelZH string `json:"label_zh"`
}

// itemLabel is the canonical English + Chinese label pair for one 8-K item code.
type itemLabel struct {
	en string
	zh string
}

// itemLabels is the complete, Go-owned map of standard SEC 8-K item codes to
// their canonical English and Simplified-Chinese labels. This is the
// anti-hallucination anchor: the LLM never decides what a code means. Source: the
// official SEC Form 8-K instructions (item list, Sections 1–9). Codes absent here
// (a future/unknown code) fall back to a generic "Item X.XX" label in label().
var itemLabels = map[string]itemLabel{
	// Section 1 — Registrant's Business and Operations.
	"1.01": {"Entry into a Material Definitive Agreement", "签订重大协议"},
	"1.02": {"Termination of a Material Definitive Agreement", "终止重大协议"},
	"1.03": {"Bankruptcy or Receivership", "破产或接管"},
	"1.04": {"Mine Safety – Reporting of Shutdowns and Patterns of Violations", "矿山安全事项"},
	"1.05": {"Material Cybersecurity Incidents", "重大网络安全事件"},

	// Section 2 — Financial Information.
	"2.01": {"Completion of Acquisition or Disposition of Assets", "完成资产收购或处置"},
	"2.02": {"Results of Operations and Financial Condition", "经营业绩与财务状况(财报)"},
	"2.03": {"Creation of a Direct Financial Obligation or an Obligation under an Off-Balance Sheet Arrangement", "新增重大债务或表外义务"},
	"2.04": {"Triggering Events That Accelerate or Increase a Direct Financial Obligation", "触发债务加速或增加事件"},
	"2.05": {"Costs Associated with Exit or Disposal Activities", "退出或处置活动相关成本"},
	"2.06": {"Material Impairments", "重大资产减值"},

	// Section 3 — Securities and Trading Markets.
	"3.01": {"Notice of Delisting or Failure to Satisfy a Continued Listing Rule or Standard; Transfer of Listing", "退市通知或不符合持续上市标准"},
	"3.02": {"Unregistered Sales of Equity Securities", "未注册股权证券发行"},
	"3.03": {"Material Modification to Rights of Security Holders", "证券持有人权利重大变更"},

	// Section 4 — Matters Related to Accountants and Financial Statements.
	"4.01": {"Changes in Registrant's Certifying Accountant", "更换审计机构"},
	"4.02": {"Non-Reliance on Previously Issued Financial Statements or a Related Audit Report or Completed Interim Review", "前期财报不可依赖"},

	// Section 5 — Corporate Governance and Management.
	"5.01": {"Changes in Control of Registrant", "公司控制权变更"},
	"5.02": {"Departure of Directors or Certain Officers; Election of Directors; Appointment of Certain Officers; Compensatory Arrangements of Certain Officers", "董事或高管离任/任命及薪酬安排"},
	"5.03": {"Amendments to Articles of Incorporation or Bylaws; Change in Fiscal Year", "修订公司章程或变更财年"},
	"5.04": {"Temporary Suspension of Trading Under Registrant's Employee Benefit Plans", "员工福利计划临时停牌"},
	"5.05": {"Amendment to Registrant's Code of Ethics, or Waiver of a Provision of the Code of Ethics", "修订或豁免道德准则"},
	"5.06": {"Change in Shell Company Status", "空壳公司状态变更"},
	"5.07": {"Submission of Matters to a Vote of Security Holders", "提交股东表决事项(投票结果)"},
	"5.08": {"Shareholder Director Nominations", "股东提名董事"},

	// Section 6 — Asset-Backed Securities.
	"6.01": {"ABS Informational and Computational Material", "资产支持证券信息与计算材料"},
	"6.02": {"Change of Servicer or Trustee", "更换服务商或受托人"},
	"6.03": {"Change in Credit Enhancement or Other External Support", "信用增级或外部支持变更"},
	"6.04": {"Failure to Make a Required Distribution", "未能进行约定分配"},
	"6.05": {"Securities Act Updating Disclosure", "证券法更新披露"},
	"6.06": {"Static Pool", "静态资产池"},

	// Section 7 — Regulation FD.
	"7.01": {"Regulation FD Disclosure", "公平披露(Reg FD)"},

	// Section 8 — Other Events.
	"8.01": {"Other Events", "其他事件"},

	// Section 9 — Financial Statements and Exhibits.
	"9.01": {"Financial Statements and Exhibits", "财务报表与附件"},
}

// label returns the canonical English + Chinese labels for an 8-K item code. A
// code present in itemLabels uses its canonical pair; an unknown/future code
// (e.g. a code the SEC adds later) falls back to a generic "Item X.XX" label in
// BOTH languages — never a fabricated meaning, never a crash.
func label(code string) (en, zh string) {
	if l, ok := itemLabels[code]; ok {
		return l.en, l.zh
	}
	generic := "Item " + code
	return generic, "事项 " + code
}

// parseItems splits an 8-K filing's raw items string into individual canonical
// EventItems. The SEC feed packs the codes into one string separated by commas
// and/or newlines (e.g. "5.02,9.01" or "2.02\n9.01"); each code may carry a
// leading "Item " prefix or stray whitespace. Codes are de-duplicated (keeping
// first-seen order) so a doubled code never appears twice. Each kept code is
// mapped to its Go-owned label pair (label()). An empty/whitespace string yields
// an empty slice (never nil at the wire layer — the caller coerces).
func parseItems(raw string) []EventItem {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	// Split on comma, newline, carriage return, semicolon, or tab.
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';' || r == '\t'
	})
	seen := make(map[string]bool, len(fields))
	out := make([]EventItem, 0, len(fields))
	for _, f := range fields {
		code := normalizeItemCode(f)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		en, zh := label(code)
		out = append(out, EventItem{Code: code, LabelEN: en, LabelZH: zh})
	}
	return out
}

// normalizeItemCode cleans one raw item token into a bare code like "5.02". It
// strips a leading "Item " prefix (case-insensitive) and surrounding whitespace.
// Returns "" for a blank token. The SEC feed's codes are already in N.NN form, so
// no reformatting is needed beyond trimming.
func normalizeItemCode(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// Strip a leading "Item " label if present (some feeds include it).
	if len(s) >= 5 && strings.EqualFold(s[:5], "item ") {
		s = strings.TrimSpace(s[5:])
	}
	return s
}

// materialEventsLookback bounds how far back an 8-K is considered "recent". We
// take filings within this window, capped at maxMaterialEvents, whichever binds
// first — so a chatty filer is capped and a quiet one shows everything recent.
const materialEventsLookback = 120 * 24 * time.Hour

// maxMaterialEvents caps how many 8-Ks are returned (newest first).
const maxMaterialEvents = 10

// submissions8KResp decodes only the parallel arrays we need from the SEC
// submissions feed for 8-K extraction. The feed's filings.recent object holds
// PARALLEL ARRAYS indexed by filing — form[i] pairs with filingDate[i],
// accessionNumber[i], etc. (We re-decode rather than reuse fundamentals'
// submissionsResp because that one omits items/reportDate.)
type submissions8KResp struct {
	Filings struct {
		Recent struct {
			AccessionNumber []string `json:"accessionNumber"`
			FilingDate      []string `json:"filingDate"`
			ReportDate      []string `json:"reportDate"`
			Form            []string `json:"form"`
			PrimaryDocument []string `json:"primaryDocument"`
			Items           []string `json:"items"`
		} `json:"recent"`
	} `json:"filings"`
}

// MaterialEvents returns a US ticker's recent 8-K (and 8-K/A amendment) filings,
// newest first, capped at maxMaterialEvents and bounded to the lookback window.
// It REUSES the client's CIK lookup + SEC-compliant fetch. Each returned event
// carries the Go-owned facts (form, dates, accession URL, parsed item codes +
// canonical labels); the Summary field is left empty for a higher layer to fill
// via the LLM (this package never calls an LLM). Returns an error only when the
// ticker/CIK can't be resolved or the feed fetch fails — an existing company with
// zero recent 8-Ks returns an empty slice and nil error.
func (c *Client) MaterialEvents(ctx context.Context, ticker string) ([]MaterialEvent, error) {
	info, err := c.lookup(ctx, ticker)
	if err != nil {
		return nil, err
	}
	var sub submissions8KResp
	if err := c.get(ctx, fmt.Sprintf(submissionsURL, info.CIK), &sub); err != nil {
		return nil, err
	}
	return extractMaterialEvents(sub, info.CIK), nil
}

// extractMaterialEvents filters the submissions feed's parallel arrays down to
// recent 8-K / 8-K/A filings and builds the MaterialEvent list. It is pure (no
// I/O) so it is unit-testable. Filtering: form[i] is "8-K" or "8-K/A", filed
// within the lookback window, newest first, capped at maxMaterialEvents.
func extractMaterialEvents(sub submissions8KResp, cik string) []MaterialEvent {
	r := sub.Filings.Recent
	cikTrimmed := strings.TrimLeft(cik, "0")
	cutoff := time.Now().UTC().Add(-materialEventsLookback)

	out := make([]MaterialEvent, 0, maxMaterialEvents)
	for i := 0; i < len(r.Form); i++ {
		form := strings.TrimSpace(r.Form[i])
		if form != "8-K" && form != "8-K/A" {
			continue
		}
		filed := at(r.FilingDate, i)
		// Bound to the lookback window (parse-failure → keep, so a missing/odd
		// date never silently drops a recent 8-K).
		if t, err := time.Parse("2006-01-02", filed); err == nil && t.Before(cutoff) {
			continue
		}
		acc := at(r.AccessionNumber, i)
		accNoDashes := strings.ReplaceAll(acc, "-", "")
		primaryDoc := at(r.PrimaryDocument, i)

		ev := MaterialEvent{
			Form:       form,
			Amendment:  form == "8-K/A",
			FiledDate:  filed,
			ReportDate: at(r.ReportDate, i),
			// The accession folder index page (human-readable filing index).
			AccessionURL: fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%s/%s/",
				cikTrimmed, accNoDashes),
			Items: parseItems(at(r.Items, i)),
		}
		if ev.Items == nil {
			ev.Items = []EventItem{}
		}
		if primaryDoc != "" {
			ev.PrimaryDocURL = fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%s/%s/%s",
				cikTrimmed, accNoDashes, primaryDoc)
		}
		out = append(out, ev)
		if len(out) >= maxMaterialEvents {
			break
		}
	}
	// The submissions feed is already newest-first, but sort defensively by filed
	// date (descending) so the ordering is guaranteed regardless of feed quirks.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].FiledDate > out[j].FiledDate
	})
	return out
}

// summarySourceMaxChars bounds how much primary-document text is fed to the LLM
// summarizer — a reasonable truncation that keeps the prompt cheap while covering
// the material narrative of a typical 8-K body.
const summarySourceMaxChars = 7000

// summarySourceMinChars is the floor below which the extracted text is considered
// too thin to summarize faithfully. Below it we return "" (no fabricated summary).
const summarySourceMinChars = 120

// EventSummarySource fetches the plain-text body of an 8-K's primary document for
// LLM summarization. It REUSES the client's SEC-compliant fetch (User-Agent,
// rate-limit handling), downloads the primary document, strips HTML to plain
// text, and returns a bounded truncation. It returns "" (no error) when the doc
// is unavailable or too thin — the caller must NOT fabricate a summary in that
// case (the item labels alone are still served). The returned text is intended
// purely as LLM INPUT; it is never shown to the user verbatim.
func (c *Client) EventSummarySource(ctx context.Context, ev MaterialEvent) (string, error) {
	if ev.PrimaryDocURL == "" {
		return "", nil
	}
	text, err := c.getText(ctx, ev.PrimaryDocURL)
	if err != nil {
		return "", err
	}
	plain := htmlToText(text)
	if len([]rune(plain)) < summarySourceMinChars {
		return "", nil // too thin to summarize faithfully — never fabricate
	}
	return truncateRunes(plain, summarySourceMaxChars), nil
}

// getText fetches a URL with the SEC-compliant User-Agent and returns the raw
// body as a string. Mirrors get() but reads text (HTML) rather than decoding
// JSON — used for the primary-document body of an 8-K.
func (c *Client) getText(ctx context.Context, url string) (string, error) {
	c.throttle(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("edgar: GET %s -> %s", url, resp.Status)
	}
	// Bound the read so a pathologically large exhibit can't blow up memory; the
	// summarizer only needs the first few KB anyway.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MiB
	if err != nil {
		return "", err
	}
	return string(body), nil
}

var (
	// htmlScriptStyle strips <script>/<style> blocks (case-insensitive, dotall).
	htmlScriptStyle = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	// htmlTag strips any remaining HTML/XML tag.
	htmlTag = regexp.MustCompile(`(?s)<[^>]+>`)
	// htmlComment strips HTML comments.
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	// wsRun collapses any run of whitespace to a single space.
	wsRun = regexp.MustCompile(`\s+`)
)

// htmlToText reduces an 8-K HTML document to readable plain text: it drops
// script/style/comments, replaces block-ish tags with spaces, strips the rest,
// unescapes the common entities, and collapses whitespace. It is best-effort and
// pure (stdlib regexp only — no third-party HTML parser, per the project's
// stdlib-first rule). The output is LLM input only, so light imperfections are
// acceptable; what matters is no markup leaks into the prompt.
func htmlToText(s string) string {
	s = htmlComment.ReplaceAllString(s, " ")
	s = htmlScriptStyle.ReplaceAllString(s, " ")
	s = htmlTag.ReplaceAllString(s, " ")
	s = unescapeEntities(s)
	s = wsRun.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// entityReplacer unescapes the handful of HTML entities common in EDGAR docs.
var entityReplacer = strings.NewReplacer(
	"&amp;", "&",
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", `"`,
	"&apos;", "'",
	"&#39;", "'",
	"&nbsp;", " ",
	"&#160;", " ",
	"&#8217;", "'",
	"&#8216;", "'",
	"&#8220;", `"`,
	"&#8221;", `"`,
	"&#8211;", "-",
	"&#8212;", "-",
	"&mdash;", "-",
	"&ndash;", "-",
	"&rsquo;", "'",
	"&lsquo;", "'",
	"&ldquo;", `"`,
	"&rdquo;", `"`,
)

func unescapeEntities(s string) string { return entityReplacer.Replace(s) }

// truncateRunes returns s truncated to at most max runes (not bytes — so a
// multibyte char is never split). Returns s unchanged when it already fits.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
