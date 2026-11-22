package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// juhe_sms.go submits verification codes through the Juhe SMS API.
// The provider answers a JSON envelope whose error_code is zero only
// when the message was accepted for delivery; acceptance is not a
// handset-delivery guarantee. Template text and signature are approved
// in the Juhe console, so this client sends only the code variable.

// JuheSMSEndpoint is the Juhe SMS submission address. It is a variable
// so tests can substitute a local server.
var JuheSMSEndpoint = "https://v.juhe.cn/sms/send"

// JuheSMSClient submits SMS codes with caller-supplied credentials. It
// retains no state, so one instance serves every request.
type JuheSMSClient struct{}

// SendSMSCode posts one template message carrying the code variable. A
// nonzero provider error code or a transport failure returns an error
// describing the provider reason when one was supplied.
func (JuheSMSClient) SendSMSCode(ctx context.Context, key, templateID, phone, code string) error {
	form := url.Values{}
	form.Set("key", key)
	form.Set("mobile", phone)
	form.Set("tpl_id", templateID)
	form.Set("tpl_value", "#code#="+code)

	statusCode, bodyBytes, err := doIdentityOAuthRequest(ctx, http.MethodPost, JuheSMSEndpoint, "", nil, form)
	if err != nil {
		return fmt.Errorf("request sms send: %w", err)
	}
	body := strings.TrimSpace(string(bodyBytes))
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("sms send status=%d", statusCode)
	}
	doc, ok := decodeOAuthJSONObject(body)
	if !ok {
		return fmt.Errorf("sms send returned an unreadable response")
	}
	if errorCode := lookupOAuthJSONInt64(doc, "error_code"); errorCode != 0 {
		reason := strings.TrimSpace(lookupOAuthJSONString(doc, "reason"))
		return fmt.Errorf("sms send error_code=%d reason=%s", errorCode, reason)
	}
	return nil
}
