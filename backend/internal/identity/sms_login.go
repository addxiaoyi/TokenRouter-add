package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// sms_login.go issues, sends, and verifies SMS verification codes for
// phone login and phone binding.
// A phone number is an authentication identity of provider type phone
// whose stable subject is the normalized mobile number itself. Codes
// follow the email-challenge rules: the credential is saved before the
// provider send is requested, resends observe a cooldown, failed
// comparisons consume attempts inside the original validity window, and
// a per-number daily cap bounds provider cost.
//
// Settings are read on every send so administrator changes to the
// provider key or template take effect without a restart. The transport
// is injected at assembly; this service owns challenge state and policy,
// never provider credentials.

// SMS login setting keys name the persistent feature switch and the
// Juhe provider credentials read by RuntimeSettings.
const (
	SettingKeySMSLoginEnabled         = "sms_login_enabled"
	SettingKeySMSJuheKey              = "sms_juhe_key"
	SettingKeySMSJuheTemplateID       = "sms_juhe_tpl_id"
	SMSPhoneProviderType              = "phone" // authentication identity provider type for phone numbers
	SMSPhoneProviderKey               = "juhe"  // provider key naming the configured SMS channel
	SMSCodeTTL                        = 10 * time.Minute
	SMSCodeCooldown                   = 1 * time.Minute
	SMSCodeMaxAttempts                = 5
	SMSCodeDailyLimit           int64 = 10
)

// ErrSMSLoginDisabled reports a send or login attempt while the SMS
// login feature switch is off.
var ErrSMSLoginDisabled = infraerrors.NotFound("SMS_LOGIN_DISABLED", "sms login is disabled")

// ErrSMSNotConfigured reports a send attempt without a usable provider
// key or template.
var ErrSMSNotConfigured = infraerrors.ServiceUnavailable("SMS_NOT_CONFIGURED", "sms provider is not configured")

// ErrSMSDailyLimit reports a number that already received the daily
// maximum of codes.
var ErrSMSDailyLimit = infraerrors.TooManyRequests("SMS_DAILY_LIMIT", "daily sms code limit reached")

// ErrInvalidPhoneNumber reports a value that is not a mainland mobile
// number the SMS login accepts.
var ErrInvalidPhoneNumber = infraerrors.BadRequest("INVALID_PHONE_NUMBER", "invalid phone number")

// SMSLoginSettings carries the effective SMS login configuration.
type SMSLoginSettings struct {
	Enabled    bool   // feature switch for phone code login and binding
	JuheKey    string // Juhe SMS application credential
	TemplateID string // approved Juhe verification-code template identifier
}

// Configured reports whether sends can run: the feature is enabled and
// both provider values are present.
func (s SMSLoginSettings) Configured() bool {
	return s.Enabled && strings.TrimSpace(s.JuheKey) != "" && strings.TrimSpace(s.TemplateID) != ""
}

// GetSMSLoginSettings reads the SMS login switch and provider values in
// one batch. Missing keys select disabled and empty values; a batch
// failure remains an error so senders fail closed.
func (s *RuntimeSettings) GetSMSLoginSettings(ctx context.Context) (SMSLoginSettings, error) {
	if s == nil || s.settingRepo == nil {
		return SMSLoginSettings{}, fmt.Errorf("settings store not configured")
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeySMSLoginEnabled,
		SettingKeySMSJuheKey,
		SettingKeySMSJuheTemplateID,
	})
	if err != nil {
		return SMSLoginSettings{}, fmt.Errorf("get sms login settings: %w", err)
	}
	return SMSLoginSettings{
		Enabled:    values[SettingKeySMSLoginEnabled] == "true",
		JuheKey:    strings.TrimSpace(values[SettingKeySMSJuheKey]),
		TemplateID: strings.TrimSpace(values[SettingKeySMSJuheTemplateID]),
	}, nil
}

// NormalizeSMSPhoneNumber validates a mainland mobile number and returns
// its canonical eleven-digit form. Surrounding whitespace is trimmed;
// country prefixes and separators are rejected rather than rewritten so
// the stored identity subject always matches the number the user proved.
func NormalizeSMSPhoneNumber(raw string) (string, bool) {
	phone := strings.TrimSpace(raw)
	if len(phone) != 11 || phone[0] != '1' || phone[1] < '3' || phone[1] > '9' {
		return "", false
	}
	for _, r := range phone {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return phone, true
}

// SMSCodeSender delivers one prepared code through the configured
// provider. The caller has already saved the credential; a send error
// leaves credential cleanup with the challenge service.
type SMSCodeSender interface {
	// SendSMSCode submits the code to the provider for the given number.
	// Provider rejections and transport failures remain errors.
	SendSMSCode(context.Context, string, string, string, string) error
}

// SMSCodeCache persists SMS challenge records and per-number daily send
// counters.
type SMSCodeCache interface {
	// GetSMSCode returns the saved challenge for the number, or an error
	// when absent or unreadable.
	GetSMSCode(context.Context, string) (*VerificationCodeData, error)
	// SetSMSCode saves the challenge under the supplied lifetime.
	SetSMSCode(context.Context, string, *VerificationCodeData, time.Duration) error
	// DeleteSMSCode removes the saved challenge; an absent key succeeds.
	DeleteSMSCode(context.Context, string) error
	// IncrementSMSDailyCount atomically adds one send to the number's
	// current-day counter and returns the new total.
	IncrementSMSDailyCount(context.Context, string) (int64, error)
}

// SMSChallenges prepares, verifies, and cleans up SMS credentials around
// provider sends.
type SMSChallenges struct {
	cache     SMSCodeCache     // challenge and counter persistence
	settings  *RuntimeSettings // current SMS login configuration reader
	transport SMSCodeSender    // provider delivery client
}

// NewSMSChallenges retains the cache, settings reader, and provider
// transport used by code operations. Configure all three before sending.
func NewSMSChallenges(cache SMSCodeCache, settings *RuntimeSettings, transport SMSCodeSender) *SMSChallenges {
	return &SMSChallenges{cache: cache, settings: settings, transport: transport}
}

// GenerateSMSCode returns a six-character decimal challenge using
// cryptographic random draws.
func GenerateSMSCode() (string, error) {
	// The decimal alphabet permits leading zeros in generated challenges.
	const digits = "0123456789"
	code := make([]byte, 6)
	for i := range code {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		code[i] = digits[num.Int64()]
	}
	return string(code), nil
}

// SendCode validates configuration and send budgets, saves a new
// credential, and requests its provider delivery. A recently created
// record returns ErrVerifyCodeTooFrequent, and a number at its daily cap
// returns ErrSMSDailyLimit before any provider call. Saving precedes
// delivery so a quickly arriving code can be verified; a failed send
// leaves the saved credential and its cooldown in place, matching the
// email-challenge ordering.
func (s *SMSChallenges) SendCode(ctx context.Context, phone string) error {
	cfg, err := s.settings.GetSMSLoginSettings(ctx)
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return ErrSMSLoginDisabled
	}
	if strings.TrimSpace(cfg.JuheKey) == "" || strings.TrimSpace(cfg.TemplateID) == "" {
		return ErrSMSNotConfigured
	}

	existing, err := s.cache.GetSMSCode(ctx, phone)
	if err == nil && existing != nil {
		if time.Since(existing.CreatedAt) < SMSCodeCooldown {
			return ErrVerifyCodeTooFrequent
		}
	}

	count, err := s.cache.IncrementSMSDailyCount(ctx, phone)
	if err != nil {
		return fmt.Errorf("count sms sends: %w", err)
	}
	if count > SMSCodeDailyLimit {
		return ErrSMSDailyLimit
	}

	code, err := GenerateSMSCode()
	if err != nil {
		return fmt.Errorf("generate code: %w", err)
	}
	data := &VerificationCodeData{
		Code:      code,
		Attempts:  0,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(SMSCodeTTL),
	}
	if err := s.cache.SetSMSCode(ctx, phone, data, SMSCodeTTL); err != nil {
		return fmt.Errorf("save sms code: %w", err)
	}

	return s.transport.SendSMSCode(ctx, cfg.JuheKey, cfg.TemplateID, phone, code)
}

// SendSMSCode delegates one phone challenge send to the assembled SMS
// service. A missing service reports SMS not configured.
func (s *AuthService) SendSMSCode(ctx context.Context, phone string) error {
	if s == nil || s.SMS == nil {
		return ErrSMSNotConfigured
	}
	return s.SMS.SendCode(ctx, phone)
}

// VerifySMSCode delegates one phone challenge check to the assembled SMS
// service. A missing service reports the code as invalid rather than
// accepting any credential.
func (s *AuthService) VerifySMSCode(ctx context.Context, phone, code string) error {
	if s == nil || s.SMS == nil {
		return ErrInvalidVerifyCode
	}
	return s.SMS.VerifyCode(ctx, phone, code)
}

// VerifyCode checks the saved challenge and returns the applicable
// attempt or invalid-code error. Missing or unreadable records are
// invalid, and a record already at SMSCodeMaxAttempts is rejected before
// comparison. A mismatch increments the attempt count within the
// remaining lifetime; a match attempts deletion and succeeds even when
// cleanup fails.
func (s *SMSChallenges) VerifyCode(ctx context.Context, phone, code string) error {
	data, err := s.cache.GetSMSCode(ctx, phone)
	if err != nil || data == nil {
		return ErrInvalidVerifyCode
	}

	if data.Attempts >= SMSCodeMaxAttempts {
		return ErrVerifyCodeMaxAttempts
	}

	if subtle.ConstantTimeCompare([]byte(data.Code), []byte(code)) != 1 {
		data.Attempts++
		remaining := time.Until(data.ExpiresAt)
		if remaining <= 0 {
			return ErrInvalidVerifyCode
		}
		if err := s.cache.SetSMSCode(ctx, phone, data, remaining); err != nil {
			slog.Error("failed to update sms attempt count", "error", err)
		}
		if data.Attempts >= SMSCodeMaxAttempts {
			return ErrVerifyCodeMaxAttempts
		}
		return ErrInvalidVerifyCode
	}

	if err := s.cache.DeleteSMSCode(ctx, phone); err != nil {
		slog.Error("failed to delete sms code after success", "error", err)
	}
	return nil
}
