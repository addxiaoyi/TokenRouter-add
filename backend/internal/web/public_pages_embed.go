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
// the home and legal routes. Visitors and document readers receive brand and
// policy text before JavaScript runs. The configured lookup supplies legal copy
// in the request language, while the home summary uses the existing gateway
// description and the published policy identities from public settings.
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
    <a href="/home" class="text-xl font-semibold text-primary-500">{{.SiteName}}</a>
    <a href="/login" class="font-medium text-primary-500">{{.SignIn}}</a>
  </header>
  {{if .DocumentTitle}}
  <article class="space-y-4">
    <h1 class="text-3xl font-semibold">{{.DocumentTitle}}</h1>
    <div class="whitespace-pre-wrap break-words text-base leading-relaxed">{{.Markdown}}</div>
  </article>
  {{else}}
  <section class="space-y-4">
    <h1 class="text-3xl font-semibold">{{.SiteName}}</h1>
    {{if .Subtitle}}<p class="text-base leading-relaxed">{{.Subtitle}}</p>{{end}}
    <p class="text-base leading-relaxed">{{.Summary}}</p>
    <a href="/models" class="font-medium text-primary-500">{{.Models}}</a>
  </section>
  {{end}}
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
	Subtitle string // configured public subtitle
	Summary  string // localized gateway description
	SignIn   string // account entry label
	Models   string // model directory label
	Policies string // policy navigation label
	// DocumentTitle and Markdown describe one resolved legal document. An empty
	// DocumentTitle selects the home summary rather than an incomplete document view.
	DocumentTitle string             // resolved legal heading
	Markdown      string             // inert legal source
	Links         []publicPolicyLink // published policy destinations
}

// publicPageSnapshot adds initial content only for home and published legal routes.
// The supplied snapshot stays unchanged; unrelated routes reuse its original bytes.
// Legal reads use the request language and a bounded context, distinguishing
// missing copy from a failed read before the caller writes response headers.
//
// The returned content is request-specific and must not be published to the
// language cache. Document bytes remain available to final metadata hashing, so
// a cached settings envelope cannot hide a newly selected policy body behind an
// unchanged validator. Failed rendering returns an explicit HTTP status and error.
// @project-doc docs/architecture/frontend_modules.md#public_page_delivery
func (s *FrontendServer) publicPageSnapshot(c *gin.Context, snapshot CachedHTML) (CachedHTML, int, error) {
	if s.publicDocuments == nil {
		return snapshot, http.StatusOK, nil
	}
	path := c.Request.URL.Path
	isHome := path == "/" || path == "/home" || path == "/home/"
	isLegal := strings.HasPrefix(path, "/legal/")
	if !isHome && !isLegal {
		return snapshot, http.StatusOK, nil
	}

	var settings struct {
		Name      string `json:"site_name"`     // selected public brand
		Subtitle  string `json:"site_subtitle"` // selected public subtitle
		Locale    string `json:"locale"`        // selected interface language
		Documents []struct {
			ID    string `json:"id"`    // published document identity
			Title string `json:"title"` // selected document heading
		} `json:"login_agreement_documents"` // published policy summaries
	}
	if err := json.Unmarshal(snapshot.SettingsJSON, &settings); err != nil {
		return snapshot, http.StatusInternalServerError, fmt.Errorf("web/public_pages_embed.go: decode public settings: %w", err)
	}
	copy := publicPageCopy{SiteName: settings.Name, Subtitle: settings.Subtitle}
	if copy.SiteName == "" {
		copy.SiteName = "TokenRouter"
	}
	copy.Summary = publicHomeSummaryEnglish
	copy.SignIn = "Log in"
	copy.Models = "Models & pricing"
	copy.Policies = "Policies"
	if strings.HasPrefix(settings.Locale, "zh") {
		copy.Summary = publicHomeSummaryChinese
		copy.SignIn = "登录"
		copy.Models = "模型与定价"
		copy.Policies = "政策与条款"
	}
	for _, document := range settings.Documents {
		if document.ID == "" || document.Title == "" {
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
		if id == "" || strings.Contains(id, "/") {
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
	page, err := replacePublicAppContent(snapshot.Content, body.Bytes())
	if err != nil {
		return snapshot, http.StatusInternalServerError, err
	}
	snapshot.Content = page
	return snapshot, http.StatusOK, nil
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
