package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/identity"
)

// qq_oauth.go exchanges QQ互联 authorization codes and extracts stable
// subjects and profile suggestions from provider replies.
// QQ互联 answers its token and identity endpoints with JSON when the
// fmt=json parameter is supplied, but error replies can still arrive as
// form-encoded text or as JSON carrying a numeric code without an access
// token, so parsing accepts both success shapes and treats a coded reply
// as an exchange failure.
//
// The stable subject prefers the unionid the identity endpoint returns
// for applications with cross-application identity enabled and falls back
// to the application-scoped openid. QQ supplies no email; the nickname
// becomes the display suggestion, and the username suggestion reuses the
// nickname only when it fits the local username alphabet, so arbitrary
// nickname text never reaches account creation unchecked.

type (
	// QQTokenResponse aliases token fields consumed by the identity OAuth flow.
	QQTokenResponse = identity.QQTokenResponse
	// QQTokenExchangeError aliases provider failure details retained for callback handling.
	QQTokenExchangeError = identity.QQTokenExchangeError
)

// QQClient exchanges authorization codes and retrieves QQ profiles.
type QQClient struct{}

// QQOptions aliases the resolved QQ authorization settings supplied by identity.
type QQOptions = identity.QQOAuthOptions

// ExchangeCode delegates code exchange with the callback supplied by the
// authorization flow. Failures return the underlying exchange error.
func (QQClient) ExchangeCode(ctx context.Context, o QQOptions, code, redirect string) (*QQTokenResponse, error) {
	return QQExchangeCode(ctx, o, code, redirect)
}

// FetchUserInfo delegates identity and profile retrieval with a nonnil
// token and the supplied QQ options. Successful output includes the
// stable subject and profile suggestions; the email result is always
// empty because QQ互联 does not share one.
func (QQClient) FetchUserInfo(ctx context.Context, o QQOptions, t *QQTokenResponse) (string, string, string, string, string, error) {
	return QQFetchUserInfo(ctx, o, t)
}

// QQExchangeCode submits a code grant with a thirty-second request
// budget. QQ answers a successful grant as JSON under fmt=json or as
// form-encoded text; a reply carrying a provider code but no access
// token is an exchange failure retaining status and body.
func QQExchangeCode(
	ctx context.Context,
	cfg QQOptions,
	code string,
	redirectURI string,
) (*QQTokenResponse, error) {
	query := url.Values{}
	query.Set("grant_type", "authorization_code")
	query.Set("client_id", cfg.ClientID)
	query.Set("client_secret", cfg.ClientSecret)
	query.Set("code", code)
	query.Set("redirect_uri", redirectURI)
	query.Set("fmt", "json")
	endpoint := appendQuery(cfg.TokenURL, query)

	statusCode, bodyBytes, err := doIdentityOAuthRequest(ctx, http.MethodGet, endpoint, "", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("request token: %w", err)
	}
	body := strings.TrimSpace(string(bodyBytes))
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		providerErr, providerDesc := ParseOAuthProviderError(body)
		return nil, &QQTokenExchangeError{
			StatusCode:          statusCode,
			ProviderError:       providerErr,
			ProviderDescription: providerDesc,
			Body:                body,
		}
	}
	tokenResp, ok := ParseQQTokenResponse(body)
	if !ok || strings.TrimSpace(tokenResp.AccessToken) == "" {
		providerErr, providerDesc := ParseOAuthProviderError(body)
		return nil, &QQTokenExchangeError{
			StatusCode:          statusCode,
			ProviderError:       providerErr,
			ProviderDescription: providerDesc,
			Body:                body,
		}
	}
	return tokenResp, nil
}

// QQFetchUserInfo resolves the caller's stable subject from the identity
// endpoint and then reads the profile. The subject prefers unionid over
// openid and must pass the synthetic-address safety check before it can
// participate in local identity binding.
func QQFetchUserInfo(
	ctx context.Context,
	cfg QQOptions,
	token *QQTokenResponse,
) (email string, username string, subject string, displayName string, avatarURL string, err error) {
	if token == nil || strings.TrimSpace(token.AccessToken) == "" {
		return "", "", "", "", "", errors.New("missing access_token")
	}
	meQuery := url.Values{}
	meQuery.Set("access_token", token.AccessToken)
	meQuery.Set("fmt", "json")
	// Unionid is returned only when the application enabled it; requesting
	// it costs nothing for applications that did not.
	meQuery.Set("unionid", "1")
	statusCode, bodyBytes, err := doIdentityOAuthRequest(ctx, http.MethodGet, appendQuery(cfg.MeURL, meQuery), "", nil, nil)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("request identity: %w", err)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return "", "", "", "", "", fmt.Errorf("identity status=%d", statusCode)
	}
	openID, unionID, err := ParseQQMeResponse(string(bodyBytes))
	if err != nil {
		return "", "", "", "", "", err
	}
	subject = identity.OAuthFirstNonEmpty(unionID, openID)
	if subject == "" {
		return "", "", "", "", "", errors.New("identity missing openid field")
	}
	if !identity.OAuthIsSafeQQSubject(subject) {
		return "", "", "", "", "", errors.New("identity returned invalid subject field")
	}

	infoQuery := url.Values{}
	infoQuery.Set("access_token", token.AccessToken)
	infoQuery.Set("oauth_consumer_key", cfg.ClientID)
	infoQuery.Set("openid", openID)
	statusCode, bodyBytes, err = doIdentityOAuthRequest(ctx, http.MethodGet, appendQuery(cfg.UserInfoURL, infoQuery), "", nil, nil)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("request userinfo: %w", err)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return "", "", "", "", "", fmt.Errorf("userinfo status=%d", statusCode)
	}
	nickname, avatar, err := ParseQQUserInfo(string(bodyBytes))
	if err != nil {
		return "", "", "", "", "", err
	}
	displayName = strings.TrimSpace(nickname)
	username = displayName
	if !isQQUsernameSuggestion(username) {
		// Nicknames are free-form text; only a nickname that already fits
		// the local username alphabet may suggest the account username.
		username = identity.QQFallbackUsername(subject)
	}
	if displayName == "" {
		displayName = username
	}
	return "", username, subject, displayName, strings.TrimSpace(avatar), nil
}

// ParseQQTokenResponse decodes JSON or form-encoded token fields while
// requiring a nonempty access_token in either representation. A JSON
// reply carrying a provider error code yields no token, so coded failures
// never parse as grants.
func ParseQQTokenResponse(body string) (*QQTokenResponse, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, false
	}
	if doc, ok := decodeOAuthJSONObject(body); ok {
		accessToken := strings.TrimSpace(lookupOAuthJSONString(doc, "access_token"))
		if accessToken == "" {
			return nil, false
		}
		return &QQTokenResponse{
			AccessToken:  accessToken,
			ExpiresIn:    lookupOAuthJSONInt64(doc, "expires_in"),
			RefreshToken: strings.TrimSpace(lookupOAuthJSONString(doc, "refresh_token")),
		}, true
	}
	values, err := url.ParseQuery(body)
	if err != nil {
		return nil, false
	}
	accessToken := strings.TrimSpace(values.Get("access_token"))
	if accessToken == "" {
		return nil, false
	}
	expiresIn := int64(0)
	if raw := strings.TrimSpace(values.Get("expires_in")); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			expiresIn = v
		}
	}
	return &QQTokenResponse{
		AccessToken:  accessToken,
		ExpiresIn:    expiresIn,
		RefreshToken: strings.TrimSpace(values.Get("refresh_token")),
	}, true
}

// ParseQQMeResponse extracts the openid and optional unionid from an
// identity-endpoint reply. The reply is JSON under fmt=json; the legacy
// JSONP wrapper is also accepted so either deployment shape resolves.
func ParseQQMeResponse(body string) (openID string, unionID string, err error) {
	body = strings.TrimSpace(body)
	// Strip the JSONP callback wrapper when the endpoint ignored fmt=json.
	// The provider terminates the call with a closing parenthesis and a
	// semicolon, so both marks come off before decoding.
	if strings.HasPrefix(body, "callback(") {
		body = strings.TrimSpace(strings.TrimPrefix(body, "callback("))
		body = strings.TrimSpace(strings.TrimSuffix(body, ";"))
		body = strings.TrimSpace(strings.TrimSuffix(body, ")"))
	}
	doc, ok := decodeOAuthJSONObject(body)
	if !ok {
		return "", "", errors.New("identity response is not a JSON object")
	}
	if code := lookupOAuthJSONString(doc, "code"); code != "" {
		return "", "", fmt.Errorf("identity error code=%s", code)
	}
	openID = strings.TrimSpace(lookupOAuthJSONString(doc, "openid"))
	unionID = strings.TrimSpace(lookupOAuthJSONString(doc, "unionid"))
	if openID == "" {
		return "", "", errors.New("identity missing openid field")
	}
	return openID, unionID, nil
}

// ParseQQUserInfo extracts the nickname and best avatar from a profile
// reply. A nonzero ret field is a provider failure, not an empty profile.
func ParseQQUserInfo(body string) (nickname string, avatarURL string, err error) {
	doc, ok := decodeOAuthJSONObject(strings.TrimSpace(body))
	if !ok {
		return "", "", errors.New("userinfo response is not a JSON object")
	}
	if ret := lookupOAuthJSONInt64(doc, "ret"); ret != 0 {
		return "", "", fmt.Errorf("userinfo error ret=%d", ret)
	}
	nickname = strings.TrimSpace(lookupOAuthJSONString(doc, "nickname"))
	avatarURL = identity.OAuthFirstNonEmpty(
		lookupOAuthJSONString(doc, "figureurl_qq_2"),
		lookupOAuthJSONString(doc, "figureurl_qq_1"),
		lookupOAuthJSONString(doc, "figureurl_2"),
		lookupOAuthJSONString(doc, "figureurl_1"),
		lookupOAuthJSONString(doc, "figureurl"),
	)
	return nickname, avatarURL, nil
}

// isQQUsernameSuggestion reports whether nickname text can serve as a
// local username suggestion: 2-32 bytes of ASCII letters, digits,
// underscores, dots, and hyphens.
func isQQUsernameSuggestion(value string) bool {
	if len(value) < 2 || len(value) > 32 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '_' || r == '.' || r == '-':
		default:
			return false
		}
	}
	return true
}

// appendQuery merges encoded parameters into an endpoint that may
// already carry a query string.
func appendQuery(endpoint string, query url.Values) string {
	separator := "?"
	if strings.Contains(endpoint, "?") {
		separator = "&"
	}
	return endpoint + separator + query.Encode()
}
