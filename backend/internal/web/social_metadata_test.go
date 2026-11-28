//go:build embed

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// social_metadata_test.go verifies request-specific sharing markup against a
// reusable language snapshot. Requests vary their destination and conditional
// validators while metadata fixtures include hostile text and missing-cover fallbacks.
// This protects raw crawler-visible HTML without depending on browser JavaScript.

// TestRenderSocialMetadata checks escaped homepage sharing copy and route-specific titles.
// Reusing a language snapshot across paths or hosts must change the ETag without
// leaking query parameters into canonical URLs or sharing fields onto private pages.
func TestRenderSocialMetadata(t *testing.T) {
	snapshot := CachedHTML{Content: []byte("<html><head></head></html>"), SettingsJSON: []byte(`{"site_name":"Site","site_title":"Home","site_subtitle":"Subtitle","site_share_title":"Share\"<tag>","site_share_image_url":"/api/v1/settings/share-image/abc","site_share_image_type":"image/png","site_share_image_width":1200,"site_share_image_height":630}`)}
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "https://one.test/?token=private", nil)
	first, etag := renderSocialMetadata(context, snapshot)
	require.Contains(t, string(first), `<title>Home</title>`)
	require.Contains(t, string(first), `property="og:title" content="Share&#34;&lt;tag&gt;"`)
	require.Contains(t, string(first), `name="twitter:title" content="Share&#34;&lt;tag&gt;"`)
	require.Contains(t, string(first), `content="summary_large_image"`)
	require.Contains(t, string(first), `content="https://one.test/api/v1/settings/share-image/abc"`)
	require.Contains(t, string(first), `content="https://one.test/"`)
	require.NotContains(t, string(first), "private")
	context.Request = httptest.NewRequest(http.MethodGet, "https://two.test/", nil)
	_, secondTag := renderSocialMetadata(context, snapshot)
	require.NotEqual(t, etag, secondTag)
	context.Request = httptest.NewRequest(http.MethodGet, "https://one.test/models", nil)
	models, thirdTag := renderSocialMetadata(context, snapshot)
	require.NotEqual(t, etag, thirdTag)
	require.Contains(t, string(models), `<title>Models &amp; pricing | Site</title>`)
	require.Contains(t, string(models), `property="og:title" content="Models &amp; pricing | Site"`)
	require.Contains(t, string(models), `content="https://one.test/models"`)
	require.NotContains(t, string(models), `content="Share&#34;&lt;tag&gt;"`)
	context.Request = httptest.NewRequest(http.MethodGet, "https://one.test/other", nil)
	unknown, _ := renderSocialMetadata(context, snapshot)
	require.Equal(t, "noindex, nofollow", context.Writer.Header().Get("X-Robots-Tag"))
	require.Contains(t, string(unknown), `name="robots" content="noindex, nofollow"`)
	require.NotContains(t, string(unknown), `rel="canonical"`)
	require.NotContains(t, string(unknown), `property="og:url"`)
	require.NotContains(t, string(snapshot.Content), "og:")
}

// TestSocialSettingsFallback checks inherited copy and a configured canonical domain.
// Missing cover metadata emits a summary card and no image tags, preserving usable previews for installations without an upload.
func TestSocialSettingsFallback(t *testing.T) {
	tags := (socialSettings{SiteName: "Site", SiteTitle: "Home", SiteSubtitle: "Subtitle"}).tags("https://canonical.test", "/models")
	require.Contains(t, tags, `name="twitter:title" content="Home"`)
	require.Contains(t, tags, `content="summary"`)
	require.Contains(t, tags, `content="https://canonical.test/models"`)
	require.NotContains(t, tags, `property="og:image"`)
	require.Equal(t, 1, strings.Count(tags, `name="twitter:card"`))
}
