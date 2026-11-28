//go:build embed

package web

import (
	"bytes"
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// page_metadata_embed.go describes the public SPA destinations used by initial
// HTML, sitemaps, and machine-readable site guidance. A route becomes indexable
// through this explicit list or a published legal identity, keeping account
// pages and unknown application routes outside public discovery. Known client
// routes retain their application shell, while unknown addresses receive 404.
//
// Canonical addresses collapse the homepage and pricing aliases and discard
// request queries and fragments. Page copy is selected from the same language
// envelope as the client configuration. Structured data describes the website
// and current public page only; model inventory, commercial offers, private
// accounts, and unverified performance claims are outside this renderer's scope.

var (
	// Metadata patterns recognize the bundle's quoted head attributes. Replacing
	// complete owned elements prevents static defaults from competing with resolved copy.
	publicTitlePattern      = regexp.MustCompile(`(?is)<title\b[^>]*>.*?</title\s*>`)
	publicMetaPattern       = regexp.MustCompile(`(?is)<meta\b[^>]*\b(?:name\s*=\s*["'](?:description|robots|twitter:[^"']+)["']|property\s*=\s*["']og:[^"']+["'])[^>]*>`)
	publicCanonicalPattern  = regexp.MustCompile(`(?is)<link\b[^>]*\brel\s*=\s*["']canonical["'][^>]*>`)
	publicStructuredPattern = regexp.MustCompile(`(?is)<script\b[^>]*\btype\s*=\s*["']application/ld\+json["'][^>]*>.*?</script\s*>`)
)

// publicDocumentSummary identifies legal copy already published through public settings.
type publicDocumentSummary struct {
	ID    string `json:"id"`    // published legal identity
	Title string `json:"title"` // localized policy heading
}

// publicPageMetadata describes indexing and display copy for one canonical route.
type publicPageMetadata struct {
	Path        string // canonical application path
	Title       string // localized document title
	Description string // localized page purpose
	Kind        string // structured page type
	Indexable   bool   // public discovery eligibility
}

// pageMetadata resolves one application address against the explicit public route list.
// Unknown and account destinations retain branded titles with noindex. Backend
// mode also excludes pages whose client navigation redirects anonymous visitors.
func (s socialSettings) pageMetadata(path string) publicPageMetadata {
	knownRoute := knownSPARoute(path)
	path = strings.TrimRight(path, "/")
	if path == "" || path == "/home" {
		path = "/"
	}
	if path == "/pricing" {
		path = "/models"
	}
	page := publicPageMetadata{Path: path, Title: s.siteName(), Kind: "WebPage"}
	if !knownRoute {
		return page
	}
	chinese := strings.HasPrefix(s.Locale, "zh")
	var title string
	switch path {
	case "/":
		page.Title = strings.TrimSpace(s.SiteTitle)
		if page.Title == "" || strings.EqualFold(page.Title, s.siteName()) {
			page.Title = s.siteName() + " | AI API gateway"
			if chinese {
				page.Title = s.siteName() + " | AI API 中转站与多模型网关"
			}
		}
		page.Description = s.homeDescription()
		page.Indexable = !s.BackendMode
		return page
	case "/models":
		title = "Models & pricing"
		page.Description = "Browse model offers, supported endpoints, and pricing. Availability depends on the selected group and API key."
		if chinese {
			title = "模型与定价"
			page.Description = "查看模型报价、支持的接口与价格。可用模型取决于所选分组和 API 密钥。"
		}
		page.Kind = "CollectionPage"
	case "/docs":
		title = "API connection guide"
		page.Description = "Find the gateway address, list available models, and make an API request with your key."
		if chinese {
			title = "API 接入指南"
			page.Description = "查看网关地址，使用 API 密钥查询可用模型并发起请求。"
		}
	case "/model-quality":
		title = "Model assessments"
		page.Description = "Browse published model assessment reports and their recorded outcomes."
		if chinese {
			title = "模型评测"
			page.Description = "查看已发布的模型评测报告与记录结果。"
		}
		if s.BackendMode {
			return page
		}
		page.Kind = "CollectionPage"
	default:
		if !strings.HasPrefix(path, "/legal/") {
			return page
		}
		id := strings.TrimPrefix(path, "/legal/")
		if !validPublicDocumentID(id) {
			return page
		}
		for _, document := range s.Documents {
			if document.ID == id {
				title = strings.TrimSpace(document.Title)
				break
			}
		}
		if title == "" {
			return page
		}
		page.Description = "Read " + title + " for " + s.siteName() + "."
		if chinese {
			page.Description = "阅读 " + s.siteName() + " 的" + title + "。"
		}
	}
	page.Title = title + " | " + s.siteName()
	page.Indexable = true
	return page
}

// knownSPARoute matches declared client destinations, aliases, and plugin route families.
// Matching follows Vue Router's case-insensitive paths and optional trailing slash;
// dynamic identifiers occupy one segment except the declared plugin subpath wildcard.
// Keep this list aligned with frontend/src/router/routes and pluginAccountPages.ts;
// recognizing a route preserves browser startup and grants no access to its data.
func knownSPARoute(path string) bool {
	path = strings.ToLower(strings.TrimSuffix(path, "/"))
	switch path {
	case "", "/home", "/models", "/pricing", "/docs", "/model-quality",
		"/setup", "/login", "/register", "/email-verify", "/key-usage",
		"/forgot-password", "/reset-password",
		"/auth/callback", "/auth/oauth/callback", "/auth/linuxdo/callback",
		"/auth/qq/callback", "/auth/phone/callback", "/auth/wechat/callback",
		"/auth/wechat/payment/callback", "/auth/dingtalk/callback",
		"/auth/dingtalk/email-completion", "/auth/telegram/callback",
		"/auth/discord/callback", "/auth/oidc/callback":
		return true
	case "/dashboard", "/keys", "/team", "/creative", "/usage", "/usage-ranking",
		"/redeem", "/affiliate", "/profile", "/subscriptions", "/purchase", "/orders",
		"/monitoring", "/chat", "/playground", "/chat-history", "/chat2link",
		"/support-tickets", "/task-history", "/usage-logs/task",
		"/payment/qrcode", "/payment/result", "/payment/stripe", "/payment/airwallex",
		"/payment/stripe-popup", "/account/verification", "/account/bindings":
		return true
	case "/admin", "/admin/profit", "/admin/model-quality", "/admin/support-tickets",
		"/support-tickets-admin", "/admin/task-history", "/task-records",
		"/admin/permissions", "/permissions", "/admin/plugins", "/admin/plugins/sources",
		"/admin/mcp/authorize", "/admin/mcp", "/admin/dashboard", "/admin/ops",
		"/admin/audit-logs", "/admin/users", "/admin/teams", "/admin/groups",
		"/admin/model-attributes", "/admin/pricing", "/admin/subscriptions",
		"/admin/providers", "/admin/announcements", "/admin/proxies", "/admin/redeem",
		"/admin/promo-codes", "/admin/settings", "/admin/risk-control", "/admin/usage",
		"/admin/requests", "/admin/affiliates", "/admin/affiliates/invites",
		"/admin/affiliates/rebates", "/admin/affiliates/transfers",
		"/admin/orders/dashboard", "/admin/orders", "/admin/orders/plans":
		return true
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	switch {
	case len(segments) == 2 && segments[1] != "":
		switch segments[0] {
		case "chat", "support-tickets", "custom", "ext", "legal":
			return true
		default:
			return false
		}
	case len(segments) >= 3 && segments[0] == "ext" && segments[1] != "":
		return true
	case len(segments) == 3 && segments[0] == "admin" && segments[2] != "":
		return segments[1] == "plugins" || segments[1] == "support-tickets" || segments[1] == "ext"
	case len(segments) >= 4 && segments[0] == "admin" && segments[1] == "ext" && segments[2] != "":
		return true
	default:
		return false
	}
}

// siteName supplies the configured public brand with surrounding whitespace removed.
// An empty setting selects TokenRouter, so page titles and structured website
// identities retain a display name.
func (s socialSettings) siteName() string {
	if name := strings.TrimSpace(s.SiteName); name != "" {
		return name
	}
	return "TokenRouter"
}

// homeDescription selects the public sharing description before the homepage subtitle.
// When both configured values are empty, the selected interface language supplies
// the same gateway summary as the public footer.
func (s socialSettings) homeDescription() string {
	if description := strings.TrimSpace(s.Description); description != "" {
		return description
	}
	if subtitle := strings.TrimSpace(s.SiteSubtitle); subtitle != "" {
		return subtitle
	}
	if strings.HasPrefix(s.Locale, "zh") {
		return publicHomeSummaryChinese
	}
	return publicHomeSummaryEnglish
}

// validPublicDocumentID accepts a single published path segment without URL delimiters.
func validPublicDocumentID(id string) bool {
	return id != "" && id != "." && id != ".." && strings.TrimSpace(id) == id && !strings.ContainsAny(id, "/\\?#")
}

// publicAbsoluteURL resolves a root-relative page against a public HTTP(S) address.
// Bases containing credentials or missing hosts return empty. Query parameters
// and fragments never enter canonical links, including when present in configuration.
func publicAbsoluteURL(base, path string) string {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.ResolveReference(&url.URL{Path: path}).String()
}

// structuredPageMetadata serializes website and public-page identities as inert JSON-LD.
// Empty canonical addresses and private pages produce no structured data. The
// caller substitutes the normal CSP nonce after completing response validation.
func (s socialSettings) structuredPageMetadata(page publicPageMetadata, base string) string {
	canonical := publicAbsoluteURL(base, page.Path)
	if !page.Indexable || canonical == "" {
		return ""
	}
	home := publicAbsoluteURL(base, "/")
	document := map[string]any{
		"@context": "https://schema.org",
		"@graph": []map[string]any{
			{
				"@type": "WebSite", "@id": home + "#website", "url": home,
				"name": s.siteName(), "description": s.homeDescription(), "inLanguage": s.Locale,
			},
			{
				"@type": page.Kind, "@id": canonical + "#webpage", "url": canonical,
				"name": page.Title, "description": page.Description, "inLanguage": s.Locale,
				"isPartOf": map[string]string{"@id": home + "#website"},
			},
		},
	}
	// The document contains only strings, maps, and slices constructed above.
	// Encoding cannot encounter unsupported values, and JSON escapes script delimiters.
	encoded, _ := json.Marshal(document)
	return `<script id="site-structured-data" type="application/ld+json" nonce="` + NonceHTMLPlaceholder + `">` + string(encoded) + "</script>\n"
}

// replacePageMetadata replaces owned head elements while preserving application startup markup.
// The embedded bundle supplies a head element with quoted attributes. Only its
// head is rewritten, leaving legal source and application-host content untouched.
func replacePageMetadata(page []byte, title, tags string) []byte {
	start := bytes.Index(page, []byte("<head>"))
	end := bytes.Index(page, []byte("</head>"))
	if start < 0 || end <= start {
		return page
	}
	start += len("<head>")
	head := publicTitlePattern.ReplaceAll(page[start:end], nil)
	head = publicMetaPattern.ReplaceAll(head, nil)
	head = publicCanonicalPattern.ReplaceAll(head, nil)
	head = publicStructuredPattern.ReplaceAll(head, nil)
	var rendered bytes.Buffer
	// Buffer writes retain the unmodified document segments around the rewritten
	// head in memory and always return nil errors, so their unused results are discarded.
	_, _ = rendered.Write(page[:start])
	_, _ = rendered.Write(head)
	_, _ = rendered.WriteString("<title>" + html.EscapeString(title) + "</title>\n" + tags)
	_, _ = rendered.Write(page[end:])
	return rendered.Bytes()
}
