//go:build embed

package web

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/pkg/locale"
	"github.com/TokenFlux/TokenRouter/internal/server/middleware"
)

// discovery_embed.go serves crawler instructions and public-page directories
// before embedded asset lookup can turn those addresses into application HTML.
// Discovery uses the same language-selected public settings and legal lookup as
// initial page delivery. Account state, model inventory, provider configuration,
// and authentication credentials never enter these responses. Application shells
// remain crawlable so their response-level indexing restrictions can be read.
//
// The sitemap and machine-readable guide share one list of published pages.
// Legal links require a readable, nonempty published document, while unavailable
// storage returns an explicit temporary failure instead of a partial directory.
// Absolute links use the configured frontend address or the request origin
// selected by the server's trusted-proxy policy, with queries and fragments removed.

// staticDiscoveryPages describes the Chinese guide pages shipped with the
// frontend. Discovery advertises each destination only when its embedded file exists.
var staticDiscoveryPages = []publicPageMetadata{
	{Path: "/seo/claude-zhongzhuan.html", Title: "Claude API 接入", Description: "了解 Claude 模型的客户端接入方式、API 密钥与接口选择。"},
	{Path: "/seo/gpt-zhongzhuan.html", Title: "GPT API 接入", Description: "了解 GPT 模型的网关地址、API 密钥与请求方式。"},
	{Path: "/seo/gemini-zhongzhuan.html", Title: "Gemini API 接入", Description: "了解 Gemini 模型的 API 接入与客户端配置。"},
	{Path: "/seo/deepseek-api.html", Title: "DeepSeek API 接入", Description: "了解 DeepSeek 模型的接入步骤与可用模型查询方式。"},
	{Path: "/seo/grok-zhongzhuan.html", Title: "Grok API 接入", Description: "了解 Grok 模型的 API 接入与接口选择。"},
	{Path: "/seo/free-api.html", Title: "Free 组使用说明", Description: "查看 Free 组的使用条件与接入说明，以当前账户和分组状态为准。"},
	{Path: "/seo/claude-code-guide.html", Title: "Claude Code 接入指南", Description: "查看 Claude Code 的网关地址与 API 密钥配置说明。"},
	{Path: "/seo/gpt-vs-claude.html", Title: "GPT 与 Claude 对比", Description: "了解 GPT 与 Claude 的接口和客户端选择，并按具体模型确认能力。"},
	{Path: "/seo/faq.html", Title: "常见问题", Description: "查阅 API 接入、模型选择和请求错误的常见问题。"},
	{Path: "/seo/guide.html", Title: "AI API 中转选站指南", Description: "按协议、当前计价、账号权限与实际请求结果选择 API 接入方式。"},
}

// sitemapDocument represents the standard sitemap envelope and its public URLs.
type sitemapDocument struct {
	XMLName xml.Name       `xml:"urlset"`     // sitemap root element
	XMLNS   string         `xml:"xmlns,attr"` // sitemap namespace URI
	URLs    []sitemapEntry `xml:"url"`        // published page destinations
}

// sitemapEntry contains the canonical address of one published public page.
type sitemapEntry struct {
	Location string `xml:"loc"` // absolute canonical address
}

// serveDiscovery handles the three generated discovery addresses before SPA fallback.
// Other paths return false. Recognized paths accept GET and HEAD only; settings
// or document failures return an uncached error without exposing internal details.
func (s *FrontendServer) serveDiscovery(c *gin.Context) bool {
	path := c.Request.URL.Path
	if path != "/robots.txt" && path != "/sitemap.xml" && path != "/llms.txt" {
		return false
	}
	c.Header("Cache-Control", "no-cache")
	c.Writer.Header().Add("Vary", "Accept-Language, Cookie")
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		c.Header("Allow", "GET, HEAD")
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return true
	}

	settings, err := s.discoverySettings(c)
	if err != nil {
		s.writeDiscoveryError(c, err)
		return true
	}
	base := strings.TrimSpace(settings.FrontendURL)
	if base == "" {
		base = middleware.GetRequestOrigin(c)
	}
	if publicAbsoluteURL(base, "/") == "" {
		s.writeDiscoveryError(c, errors.New("web/discovery_embed.go: public frontend address is unavailable"))
		return true
	}
	c.Header("Content-Language", settings.Locale)

	var body []byte
	contentType := "text/plain; charset=utf-8"
	if path == "/robots.txt" {
		body = []byte(settings.robotsText(base))
	} else {
		pages, pageErr := s.discoveryPages(c, settings)
		if pageErr != nil {
			s.writeDiscoveryError(c, pageErr)
			return true
		}
		if path == "/sitemap.xml" {
			contentType = "application/xml; charset=utf-8"
			document := sitemapDocument{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
			for _, page := range pages {
				document.URLs = append(document.URLs, sitemapEntry{Location: publicAbsoluteURL(base, page.Path)})
			}
			body, err = xml.Marshal(document)
			if err != nil {
				s.writeDiscoveryError(c, fmt.Errorf("web/discovery_embed.go: encode sitemap: %w", err))
				return true
			}
			body = append([]byte(xml.Header), body...)
		} else {
			body = []byte(settings.llmsText(base, pages))
		}
	}
	writeDiscoveryBody(c, http.StatusOK, contentType, body)
	return true
}

// discoverySettings reads the current language envelope without publishing crawler output.
// A cached HTML settings snapshot is reusable; a cache miss reads public settings
// within the same bounded query budget used by initial public-page rendering.
func (s *FrontendServer) discoverySettings(c *gin.Context) (socialSettings, error) {
	language := locale.FromContext(c.Request.Context())
	var encoded []byte
	if snapshot, _ := s.cache.Snapshot(language); snapshot != nil {
		encoded = snapshot.SettingsJSON
	} else {
		ctx, cancel := context.WithTimeout(c.Request.Context(), publicDocumentBudget)
		defer cancel()
		settings, err := s.settings.GetPublicSettingsForInjection(ctx)
		if err != nil {
			return socialSettings{}, fmt.Errorf("web/discovery_embed.go: read public settings: %w", err)
		}
		encoded, err = json.Marshal(settings)
		if err != nil {
			return socialSettings{}, fmt.Errorf("web/discovery_embed.go: encode public settings: %w", err)
		}
	}
	var settings socialSettings
	if err := json.Unmarshal(encoded, &settings); err != nil {
		return socialSettings{}, fmt.Errorf("web/discovery_embed.go: decode public settings: %w", err)
	}
	if settings.Locale == "" {
		settings.Locale = language
	}
	return settings, nil
}

// discoveryPages lists built-in public destinations and readable published policies.
// Legal lookups share one request-derived deadline. A missing policy is omitted;
// other lookup errors abort directory generation so storage failure remains visible.
func (s *FrontendServer) discoveryPages(c *gin.Context, settings socialSettings) ([]publicPageMetadata, error) {
	var pages []publicPageMetadata
	for _, path := range []string{"/home", "/models", "/docs", "/model-quality"} {
		page := settings.pageMetadata(path)
		if page.Indexable {
			pages = append(pages, page)
		}
	}
	for _, page := range staticDiscoveryPages {
		if !s.fileExists(strings.TrimPrefix(page.Path, "/")) {
			continue
		}
		page.Title += " | " + settings.siteName()
		page.Kind = "WebPage"
		page.Indexable = true
		pages = append(pages, page)
	}
	if s.publicDocuments == nil {
		return pages, nil
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), publicDocumentBudget)
	defer cancel()
	seen := make(map[string]bool)
	for _, summary := range settings.Documents {
		if !validPublicDocumentID(summary.ID) || strings.TrimSpace(summary.Title) == "" || seen[summary.ID] {
			continue
		}
		seen[summary.ID] = true
		document, err := s.publicDocuments(ctx, summary.ID)
		if errors.Is(err, ErrPublicDocumentNotFound) {
			// A policy can disappear after the settings snapshot was cached. Omit
			// that stale summary rather than advertising a destination which returns 404.
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("web/discovery_embed.go: read published policy: %w", err)
		}
		if strings.TrimSpace(document.Title) == "" || strings.TrimSpace(document.Markdown) == "" {
			continue
		}
		page := settings.pageMetadata("/legal/" + summary.ID)
		pages = append(pages, page)
	}
	return pages, nil
}

// robotsText allows crawling of public content and noindex application shells.
// API, gateway, and authentication callback paths remain excluded, while public
// share images stay available within the API prefix. Existing authentication and
// authorization middleware continue to control access independently of these rules.
func (s socialSettings) robotsText(base string) string {
	var body strings.Builder
	// Builder writes remain in memory and always return nil errors. Discard their
	// unused results while assembling the crawler instructions as one response body.
	_, _ = body.WriteString("User-agent: *\nAllow: /\nAllow: /api/v1/settings/share-image/\n")
	for _, path := range []string{
		"/api/", "/v1/", "/v1beta/", "/backend-api/", "/auth/", "/setup/",
		"/mcp", "/responses", "/images/", "/videos/", "/antigravity/", "/alpha/", "/health",
	} {
		_, _ = body.WriteString("Disallow: " + path + "\n")
	}
	_, _ = body.WriteString("\nSitemap: " + publicAbsoluteURL(base, "/sitemap.xml") + "\n")
	return body.String()
}

// llmsText describes only the same public pages advertised by the sitemap.
// Operator text is kept on one Markdown line and escaped in link labels, so
// document names cannot introduce additional sections or destinations.
func (s socialSettings) llmsText(base string, pages []publicPageMetadata) string {
	var body strings.Builder
	// Builder writes stay in memory with nil errors, so only their completed text
	// enters the public guide and the unused write results can be discarded.
	_, _ = body.WriteString("# " + publicMarkdownText(s.siteName()) + "\n\n")
	_, _ = body.WriteString("> " + publicMarkdownText(s.homeDescription()) + "\n\n")
	_, _ = body.WriteString("## Public pages\n\n")
	for _, page := range pages {
		_, _ = body.WriteString("- [" + publicMarkdownText(page.Title) + "](" + publicAbsoluteURL(base, page.Path) + "): " + publicMarkdownText(page.Description) + "\n")
	}
	_, _ = body.WriteString("\n## API access\n\n")
	_, _ = body.WriteString("API requests require an API key. Use GET /v1/models with that key to discover the models available to it. The public marketplace describes model offers; availability and pricing can change.\n")
	return body.String()
}

// publicMarkdownText escapes text used in single-line Markdown headings and links.
func publicMarkdownText(value string) string {
	return strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`").Replace(strings.Join(strings.Fields(value), " "))
}

// writeDiscoveryError records diagnostics and returns a plain uncached temporary failure.
func (s *FrontendServer) writeDiscoveryError(c *gin.Context, err error) {
	// Keep the detailed cause in Gin's error collection for request logging while
	// the public response carries only the temporary-failure message.
	_ = c.Error(err)
	c.Header("Cache-Control", "no-store")
	writeDiscoveryBody(c, http.StatusServiceUnavailable, "text/plain; charset=utf-8", []byte("Public site information is currently unavailable.\n"))
}

// writeDiscoveryBody emits the selected status, content type, and GET payload length.
// HEAD requests receive the same headers with an empty body, and either method
// ends further Gin processing.
func writeDiscoveryBody(c *gin.Context, status int, contentType string, body []byte) {
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", strconv.Itoa(len(body)))
	if c.Request.Method == http.MethodHead {
		c.Status(status)
	} else {
		c.Data(status, contentType, body)
	}
	c.Abort()
}
