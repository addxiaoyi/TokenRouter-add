//go:build embed

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"
)

// public_pages_embed.go puts existing public site copy into initial HTML for
// the home, model directory, connection guide, assessment, and legal routes.
// Visitors and document readers receive public descriptions and navigation
// before JavaScript runs. The configured lookup supplies legal copy in the
// request language; directory summaries describe capabilities without inventing
// model inventory, prices, assessments, or commercial claims.
//
// Request-specific content is rendered into a new document tree instead of the
// shared language cache. This keeps a policy response from becoming another
// route's startup page, and lets final ETags include the selected legal body.
// Template escaping keeps operator text inert; Markdown remains readable as
// plain source until the Vue document view applies its normal formatting.

const (
	// Public copy uses the existing home.footer.description wording for both
	// interface languages, keeping initial delivery consistent with the shipped footer.
	publicHomeSummaryEnglish = "One API for AI models, request routing, and usage management."
	publicHomeSummaryChinese = "一个入口连接多种 AI 模型，集中管理调用、额度与用量。"
	// publicDocumentBudget bounds an additional document read without giving it
	// the lifetime of a cached settings envelope or an unrelated browser session.
	publicDocumentBudget = 2 * time.Second
)

// publicPageTemplate renders public navigation and text without granting
// operator-supplied Markdown or display names permission to execute as markup.
var publicPageTemplate = template.Must(template.New("public-page").Parse(`<main class="mx-auto max-w-4xl space-y-6 px-6 py-12 text-gray-800 dark:text-dark-100">
  <header class="flex flex-wrap items-center justify-between gap-4">
    <a href="{{.HomeURL}}" class="text-xl font-semibold text-primary-500">{{.SiteName}}</a>
    <a href="/login" class="font-medium text-primary-500">{{.SignIn}}</a>
  </header>
  {{if .DocumentTitle}}
  <article class="space-y-4">
    <h1 class="text-3xl font-semibold">{{.DocumentTitle}}</h1>
    <p class="text-base leading-relaxed">{{.Summary}}</p>
    <div class="whitespace-pre-wrap break-words text-base leading-relaxed">{{.Markdown}}</div>
  </article>
  {{else}}
  <section class="space-y-4">
    <h1 class="text-3xl font-semibold">{{.Heading}}</h1>
    {{if .Subtitle}}<p class="text-base leading-relaxed">{{.Subtitle}}</p>{{end}}
    <p class="text-base leading-relaxed">{{.Summary}}</p>
  </section>
  {{range .Sections}}
  <section class="space-y-3">
    <h2 class="text-xl font-semibold">{{.Title}}</h2>
    {{range .Paragraphs}}<p class="text-base leading-relaxed">{{.}}</p>{{end}}
    {{if .Command}}<pre class="overflow-x-auto whitespace-pre-wrap break-words"><code>{{.Command}}</code></pre>{{end}}
  </section>
  {{end}}
  {{end}}
  <nav aria-label="{{.Explore}}" class="flex flex-wrap gap-4">
    {{range .Navigation}}<a href="{{.URL}}" class="text-primary-500">{{.Label}}</a>{{end}}
  </nav>
  <nav aria-label="{{.Policies}}" class="flex flex-wrap gap-4">
    {{range .Links}}<a href="{{.URL}}" class="text-primary-500">{{.Label}}</a>{{end}}
  </nav>
</main>`))

// publicPolicyLink identifies one published policy destination and display label.
type publicPolicyLink struct {
	Label string // selected policy title
	URL   string // encoded legal address
}

// publicPageCopy combines safe display values for one initial public response.
type publicPageCopy struct {
	SiteName string // configured public brand
	HomeURL  string // public brand destination
	Heading  string // selected page heading
	Subtitle string // configured public subtitle
	Summary  string // localized page description
	SignIn   string // account entry label
	Policies string // policy navigation label
	Explore  string // public navigation label
	// DocumentTitle and Markdown describe one resolved legal document. An empty
	// DocumentTitle selects the built-in page summary and its optional guide sections.
	DocumentTitle string              // resolved legal heading
	Markdown      string              // inert legal source
	Links         []publicPolicyLink  // published policy destinations
	Navigation    []publicPolicyLink  // public information destinations
	Sections      []publicPageSection // readable public guide sections
}

// publicPageSection contains explanatory paragraphs and an optional inert request example.
type publicPageSection struct {
	Title      string   // localized section heading
	Paragraphs []string // visible explanatory text
	Command    string   // public shell example
}

// publicPageSnapshot adds initial content for public information and legal routes.
// The supplied snapshot stays unchanged; private and unknown routes reuse its
// original bytes with their matching 200 or 404 status. Legal reads use the
// request language and a bounded context, distinguishing missing copy from a
// failed read before the caller writes response headers.
//
// The returned content is request-specific and must not be published to the
// language cache. Document bytes remain available to final metadata hashing, so
// a cached settings envelope cannot hide a newly selected policy body behind an
// unchanged validator. Failed rendering returns an explicit HTTP status and error.
// @project-doc docs/architecture/frontend_modules.md#public_page_delivery
func (s *FrontendServer) publicPageSnapshot(c *gin.Context, snapshot CachedHTML) (CachedHTML, int, error) {
	path := c.Request.URL.Path
	isLegal := strings.HasPrefix(path, "/legal/")
	var settings socialSettings
	if err := json.Unmarshal(snapshot.SettingsJSON, &settings); err != nil {
		return snapshot, http.StatusInternalServerError, fmt.Errorf("web/public_pages_embed.go: decode public settings: %w", err)
	}
	page := settings.pageMetadata(path)
	if !page.Indexable && !isLegal {
		if knownSPARoute(path) {
			return snapshot, http.StatusOK, nil
		}
		return snapshot, http.StatusNotFound, nil
	}
	copy := settings.publicPageCopy(page)
	for _, document := range settings.Documents {
		if !validPublicDocumentID(document.ID) || strings.TrimSpace(document.Title) == "" {
			continue
		}
		copy.Links = append(copy.Links, publicPolicyLink{
			Label: document.Title,
			URL:   "/legal/" + url.PathEscape(document.ID),
		})
	}
	if isLegal {
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/legal/"), "/")
		// A legal address must identify one document. Reject extra path segments so
		// SPA fallback cannot disguise an unavailable policy as successful published copy.
		if !validPublicDocumentID(id) || s.publicDocuments == nil {
			return snapshot, http.StatusNotFound, ErrPublicDocumentNotFound
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), publicDocumentBudget)
		defer cancel()
		document, err := s.publicDocuments(ctx, id)
		if err != nil {
			status := http.StatusServiceUnavailable
			if errors.Is(err, ErrPublicDocumentNotFound) {
				status = http.StatusNotFound
			}
			return snapshot, status, fmt.Errorf("web/public_pages_embed.go: read legal document %q: %w", id, err)
		}
		if document.Title == "" || strings.TrimSpace(document.Markdown) == "" {
			return snapshot, http.StatusNotFound, ErrPublicDocumentNotFound
		}
		copy.DocumentTitle = document.Title
		copy.Markdown = document.Markdown
		if document.Locale != "" {
			c.Header("Content-Language", document.Locale)
		}
	}
	var body bytes.Buffer
	if err := publicPageTemplate.Execute(&body, copy); err != nil {
		return snapshot, http.StatusInternalServerError, fmt.Errorf("web/public_pages_embed.go: render public body: %w", err)
	}
	rendered, err := replacePublicAppContent(snapshot.Content, body.Bytes())
	if err != nil {
		return snapshot, http.StatusInternalServerError, err
	}
	snapshot.Content = rendered
	return snapshot, http.StatusOK, nil
}

// publicPageCopy selects visible summaries, navigation, and connection instructions.
// Directory pages intentionally describe the live interfaces instead of embedding
// a model list or prices from an unrelated cache or authenticated response.
func (s socialSettings) publicPageCopy(page publicPageMetadata) publicPageCopy {
	copy := publicPageCopy{
		SiteName: s.siteName(), HomeURL: "/", Heading: strings.TrimSuffix(page.Title, " | "+s.siteName()),
		Summary: page.Description, SignIn: "Log in", Policies: "Policies", Explore: "Explore",
		Navigation: []publicPolicyLink{{Label: "Models & pricing", URL: "/models"}, {Label: "API connection guide", URL: "/docs"}},
	}
	if page.Path == "/" {
		copy.Subtitle = s.SiteSubtitle
		if strings.TrimSpace(copy.Subtitle) == strings.TrimSpace(copy.Summary) {
			copy.Subtitle = ""
		}
	}
	if s.BackendMode {
		copy.HomeURL = "/models"
	} else {
		copy.Navigation = append(copy.Navigation, publicPolicyLink{Label: "Model assessments", URL: "/model-quality"})
	}
	chinese := strings.HasPrefix(s.Locale, "zh")
	if chinese {
		copy.SignIn = "登录"
		copy.Policies = "政策与条款"
		copy.Explore = "浏览公开页面"
		copy.Navigation[0].Label = "模型与定价"
		copy.Navigation[1].Label = "API 接入指南"
		if !s.BackendMode {
			copy.Navigation[2].Label = "模型评测"
		}
	}
	switch page.Path {
	case "/models":
		copy.Sections = []publicPageSection{
			{Title: "Compare model offers", Paragraphs: []string{"The live directory shows published model offers, supported endpoints, and prices. Check the selected offer before making a request."}},
			{Title: "Choose an available model", Paragraphs: []string{"Available models depend on your API key and its group. Call GET /v1/models with the same key you will use for requests, then choose a model that supports the endpoint you need."}},
		}
		if chinese {
			copy.Sections = []publicPageSection{
				{Title: "比较模型报价", Paragraphs: []string{"实时目录展示已发布的模型报价、支持接口与价格。发起请求前，请核对所选报价。"}},
				{Title: "选择可用模型", Paragraphs: []string{"可用模型取决于 API 密钥及其所属分组。请使用实际调用的同一密钥请求 GET /v1/models，再选择支持所需接口的模型。"}},
			}
		}
	case "/docs":
		copy.Sections = []publicPageSection{
			{Title: "Connect to the gateway", Paragraphs: []string{"Create an API key for the group you want to use. Store it on your server as API_KEY. Set GATEWAY_URL to the gateway origin displayed in the interactive connection guide."}},
			{Title: "List available models", Paragraphs: []string{"Use your API key to discover model IDs. A model must support the endpoint you choose."}, Command: `curl --fail-with-body "$GATEWAY_URL/v1/models" -H "Authorization: Bearer $API_KEY"`},
			{Title: "Make a request", Paragraphs: []string{"Replace MODEL_ID with an available model. Supported request families include chat completions, responses, embeddings, image generation, and video generation; support depends on the model."}, Command: `curl --fail-with-body "$GATEWAY_URL/v1/chat/completions" -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" -d '{"model":"MODEL_ID","messages":[{"role":"user","content":"Hello"}]}'`},
			{Title: "Check the response", Paragraphs: []string{"Review usage records after the request. For errors, check the HTTP status, message, key permissions, model ID, quota, and rate limits. Remove credentials before sharing diagnostics."}},
		}
		if chinese {
			copy.Sections[0].Title = "连接网关"
			copy.Sections[0].Paragraphs = []string{"为需要使用的分组创建 API 密钥，在自己的服务器上将其保存为 API_KEY。将 GATEWAY_URL 设置为交互式接入指南中展示的网关源地址。"}
			copy.Sections[1].Title = "查询可用模型"
			copy.Sections[1].Paragraphs = []string{"使用自己的 API 密钥查询可用模型 ID，并确认模型支持所选接口。"}
			copy.Sections[2].Title = "发起请求"
			copy.Sections[2].Paragraphs = []string{"将 MODEL_ID 替换为可用模型。接口类型包括聊天补全、Responses、嵌入、图片生成和视频生成，具体支持情况取决于模型。"}
			copy.Sections[3].Title = "检查响应"
			copy.Sections[3].Paragraphs = []string{"请求后查看使用记录。遇到错误时，检查 HTTP 状态、返回消息、密钥权限、模型 ID、额度和频率限制。分享诊断信息前请移除凭证。"}
		}
	case "/model-quality":
		copy.Sections = []publicPageSection{{Title: "Published reports", Paragraphs: []string{"Open the interactive report list to read available assessment outcomes and report status. Refer to each published report for its recorded results."}}}
		if chinese {
			copy.Sections = []publicPageSection{{Title: "已发布报告", Paragraphs: []string{"在交互式报告列表中查看可用的评测结果与报告状态。具体结果以已发布报告为准。"}}}
		}
	}
	return copy
}

// replacePublicAppContent replaces the app host's children with rendered public markup.
// Supply body from the escaping page template, because parsing does not sanitize
// arbitrary HTML. The returned bytes have separate storage, and the existing mount
// host remains available for the application's later startup.
//
// Tree replacement preserves nested document structure and scripts outside the
// host; a substring replacement could stop at an inner closing div instead.
// A missing app host is an explicit rendering error rather than an empty
// successful public page, and the supplied page bytes remain unchanged.
func replacePublicAppContent(page, body []byte) ([]byte, error) {
	document, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("web/public_pages_embed.go: parse page: %w", err)
	}
	host := findPublicAppHost(document)
	if host == nil {
		return nil, errors.New("web/public_pages_embed.go: app host is missing")
	}
	children, err := html.ParseFragment(bytes.NewReader(body), host)
	if err != nil {
		return nil, fmt.Errorf("web/public_pages_embed.go: parse public body: %w", err)
	}
	for host.FirstChild != nil {
		host.RemoveChild(host.FirstChild)
	}
	for _, child := range children {
		host.AppendChild(child)
	}
	var rendered bytes.Buffer
	if err := html.Render(&rendered, document); err != nil {
		return nil, fmt.Errorf("web/public_pages_embed.go: serialize public page: %w", err)
	}
	return rendered.Bytes(), nil
}

// findPublicAppHost locates the first div whose id identifies the Vue mount host.
func findPublicAppHost(node *html.Node) *html.Node {
	if node.Type == html.ElementNode && node.Data == "div" {
		for _, attribute := range node.Attr {
			if attribute.Key == "id" && attribute.Val == "app" {
				return node
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if host := findPublicAppHost(child); host != nil {
			return host
		}
	}
	return nil
}
