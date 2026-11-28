//go:build embed

package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// public_pages_scenario_test.go checks initial public delivery across
// embed_on.go and public_pages_embed.go. Requests use the real response writer
// with cached startup settings and a controlled legal lookup. The fixtures
// avoid account middleware so successful responses demonstrate that public
// information is available without a browser session or JavaScript execution.
//
// One startup snapshot is reused across home, login, and document requests to
// expose accidental cache contamination. Changed legal bodies must change the
// final validator even when that snapshot stays the same. Other cases protect
// language headers, unavailable documents, inert operator markup, and retained
// application scripts after the mount host's children are replaced.

// publicPageFixture contains startup bytes shared across serial HTTP requests.
type publicPageFixture struct {
	// Snapshot is passed by value while its bytes stay shared between requests.
	// Tests compare those bytes after rendering to detect in-place body replacement.
	Snapshot CachedHTML      // shared startup envelope
	Server   *FrontendServer // controlled page renderer
}

// newPublicPageFixture constructs a renderer with one reusable startup envelope.
// Its lookup is invoked only for legal addresses, and no authentication state or
// network service is required for the returned fixture's serial HTTP requests.
func newPublicPageFixture(lookup PublicDocumentLookup) publicPageFixture {
	return publicPageFixture{
		Snapshot: CachedHTML{
			Content: []byte(`<html lang="en"><head><title>Demo Gateway</title></head><body>` +
				`<div id="app"><div>old mount content</div></div>` +
				`<script nonce="__CSP_NONCE_VALUE__">window.ready=true;</script></body></html>`),
			SettingsJSON: []byte(`{"locale":"en","site_name":"Demo Gateway","site_subtitle":"Public gateway","login_agreement_documents":[{"id":"privacy","title":"Privacy policy"}]}`),
			ETag:         `"startup"`,
		},
		Server: &FrontendServer{publicDocuments: lookup},
	}
}

// publicPageResponse renders one fixture request through the actual HTML writer.
// The optional validator comes from a previous response; callers retain the
// same fixture snapshot so conditional requests also exercise shared-byte safety.
func publicPageResponse(fixture publicPageFixture, path, validator string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
	if validator != "" {
		ctx.Request.Header.Set("If-None-Match", validator)
	}
	fixture.Server.writeIndexHTML(ctx, fixture.Snapshot, "fixture-nonce")
	// Direct handler fixtures must flush pending status, matching Gin's response
	// finalization so conditional responses reach the recorder before assertions.
	ctx.Writer.WriteHeaderNow()
	return recorder
}

// TestPublicPagesAnonymousHome requests the landing page without account state.
// Initial HTML uses configured sharing copy before the subtitle and default
// summary. Each response retains the brand, gateway purpose, policy links, and
// application script while replacing old children in its own document.
func TestPublicPagesAnonymousHome(t *testing.T) {
	pages := []struct {
		name        string // description selection scenario
		description string // preferred sharing description
		subtitle    string // configured homepage subtitle
		summary     string // expected visible summary
	}{
		{"subtitle", "", "Public gateway", "Public gateway"},
		{"sharing_description", "Preferred gateway summary", "Public gateway", "Preferred gateway summary"},
		{"default_summary", "", "", publicHomeSummaryEnglish},
	}
	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			fixture := newPublicPageFixture(func(context.Context, string) (PublicDocument, error) {
				t.Fatal("home rendering must not look up a legal body")
				return PublicDocument{}, nil
			})
			var settings map[string]any
			require.NoError(t, json.Unmarshal(fixture.Snapshot.SettingsJSON, &settings))
			settings["site_share_description"] = page.description
			settings["site_subtitle"] = page.subtitle
			settingsJSON, err := json.Marshal(settings)
			require.NoError(t, err)
			fixture.Snapshot.SettingsJSON = settingsJSON
			before := string(fixture.Snapshot.Content)
			response := publicPageResponse(fixture, "/", "")

			require.Equal(t, http.StatusOK, response.Code)
			require.Contains(t, response.Body.String(), `<h1 class="text-3xl font-semibold">Demo Gateway | AI API gateway</h1>`)
			require.Contains(t, response.Body.String(), `<p class="text-base leading-relaxed">`+page.summary+`</p>`)
			require.Contains(t, response.Body.String(), `name="description" content="`+page.summary+`"`)
			require.Contains(t, response.Body.String(), `href="/legal/privacy"`)
			require.Contains(t, response.Body.String(), `nonce="fixture-nonce"`)
			require.NotContains(t, response.Body.String(), "old mount content")
			require.Equal(t, before, string(fixture.Snapshot.Content))
		})
	}
}

// TestPublicPagesLegalText requests published copy before application startup.
// The HTML response must expose the selected policy source and language without
// converting operator markup into executable elements. The retained application
// script and the public structured-data script keep distinct content and roles.
func TestPublicPagesLegalText(t *testing.T) {
	fixture := newPublicPageFixture(func(ctx context.Context, id string) (PublicDocument, error) {
		require.Equal(t, "privacy", id)
		_, hasDeadline := ctx.Deadline()
		require.True(t, hasDeadline)
		return PublicDocument{
			Title:    "Privacy policy",
			Markdown: "# Privacy policy\n\nAccount records are described here.\n<script>unexpected()</script>",
			Locale:   "en",
		}, nil
	})
	response := publicPageResponse(fixture, "/legal/privacy", "")

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "en", response.Header().Get("Content-Language"))
	require.Contains(t, response.Body.String(), "Account records are described here.")
	require.Contains(t, response.Body.String(), "&lt;script&gt;unexpected()&lt;/script&gt;")
	// Escaped policy text stays outside script elements. Inspect both trusted
	// script roles separately so structured metadata cannot mask lost startup code.
	scripts := strings.Split(response.Body.String(), "<script")
	require.Len(t, scripts, 3)
	appScripts := 0
	structuredScripts := 0
	for _, element := range scripts[1:] {
		attributes, content, hasContent := strings.Cut(element, ">")
		require.True(t, hasContent)
		script, _, hasClosingTag := strings.Cut(content, "</script>")
		require.True(t, hasClosingTag)
		require.Contains(t, attributes, `nonce="fixture-nonce"`)
		if strings.Contains(attributes, `type="application/ld+json"`) {
			structuredScripts++
			require.Contains(t, attributes, `id="site-structured-data"`)
			require.True(t, json.Valid([]byte(script)))
			require.Contains(t, script, `"url":"http://example.com/legal/privacy"`)
			require.Contains(t, script, `"name":"Privacy policy | Demo Gateway"`)
			continue
		}
		appScripts++
		require.NotContains(t, attributes, "type=")
		require.Equal(t, "window.ready=true;", script)
	}
	require.Equal(t, 1, appScripts)
	require.Equal(t, 1, structuredScripts)
}

// TestPublicPagesKeepLoginSnapshot renders home before the account entry route.
// The shared startup envelope must keep its original children so a cached home
// response cannot become the login route's initial application content.
func TestPublicPagesKeepLoginSnapshot(t *testing.T) {
	fixture := newPublicPageFixture(func(context.Context, string) (PublicDocument, error) {
		return PublicDocument{}, nil
	})
	require.Equal(t, http.StatusOK, publicPageResponse(fixture, "/home", "").Code)
	response := publicPageResponse(fixture, "/login", "")

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "old mount content")
	require.NotContains(t, response.Body.String(), publicHomeSummaryEnglish)
}

// TestPublicPagesLegalValidator changes a policy behind an unchanged startup cache.
// A validator from the first body must receive the replacement body with HTTP
// 200, while the replacement's own validator permits a not-modified response.
func TestPublicPagesLegalValidator(t *testing.T) {
	markdown := "First policy revision."
	fixture := newPublicPageFixture(func(context.Context, string) (PublicDocument, error) {
		return PublicDocument{Title: "Privacy policy", Markdown: markdown, Locale: "en"}, nil
	})
	first := publicPageResponse(fixture, "/legal/privacy", "")
	markdown = "Second policy revision."
	second := publicPageResponse(fixture, "/legal/privacy", first.Header().Get("ETag"))

	require.Equal(t, http.StatusOK, second.Code)
	require.Contains(t, second.Body.String(), markdown)
	require.NotEqual(t, first.Header().Get("ETag"), second.Header().Get("ETag"))
	third := publicPageResponse(fixture, "/legal/privacy", second.Header().Get("ETag"))
	require.Equal(t, http.StatusNotModified, third.Code)
}

// TestPublicPagesMissingDocument supplies an absent policy through the lookup contract.
// The public HTML endpoint must return a missing-page status without caching a
// successful application shell under that legal address.
func TestPublicPagesMissingDocument(t *testing.T) {
	fixture := newPublicPageFixture(func(context.Context, string) (PublicDocument, error) {
		return PublicDocument{}, ErrPublicDocumentNotFound
	})
	response := publicPageResponse(fixture, "/legal/privacy", "")

	require.Equal(t, http.StatusNotFound, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
}

// TestPublicPagesFailedLookup supplies a storage failure instead of published copy.
// The endpoint must distinguish that failure from absence and expose a plain
// unavailable message without including the internal diagnostic in its body.
func TestPublicPagesFailedLookup(t *testing.T) {
	fixture := newPublicPageFixture(func(context.Context, string) (PublicDocument, error) {
		return PublicDocument{}, errors.New("private storage diagnostic")
	})
	response := publicPageResponse(fixture, "/legal/privacy", "")

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.NotContains(t, response.Body.String(), "private storage diagnostic")
	require.Contains(t, response.Body.String(), "currently unavailable")
}
