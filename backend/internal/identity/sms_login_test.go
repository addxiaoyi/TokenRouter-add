package identity_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity"
	settingscore "github.com/TokenFlux/TokenRouter/internal/settings"
)

// sms_login_test.go covers phone normalization, SMS challenge budgets,
// and code verification against in-memory doubles.
// The challenge rules mirror the email flow — save before send, cooldown,
// daily cap, bounded attempts — so the tests pin each budget boundary
// and the single-use success path.

// smsCacheDouble keeps one challenge record and one daily counter in
// memory for challenge tests.
type smsCacheDouble struct {
	mu     sync.Mutex                     // fixture state lock
	record *identity.VerificationCodeData // saved challenge record
	count  int64                          // current daily send counter
}

// smsSenderDouble records the last delivery request instead of sending.
type smsSenderDouble struct {
	calls int    // observed delivery count
	key   string // last provider key argument
	tpl   string // last template argument
	phone string // last number argument
	code  string // last code argument
}

// GetSMSCode returns the saved record or an absence error.
func (c *smsCacheDouble) GetSMSCode(context.Context, string) (*identity.VerificationCodeData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.record == nil {
		return nil, errors.New("sms code not found")
	}
	return c.record, nil
}

// SetSMSCode replaces the saved record.
func (c *smsCacheDouble) SetSMSCode(_ context.Context, _ string, data *identity.VerificationCodeData, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.record = data
	return nil
}

// DeleteSMSCode clears the saved record.
func (c *smsCacheDouble) DeleteSMSCode(context.Context, string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.record = nil
	return nil
}

// IncrementSMSDailyCount advances and returns the daily counter.
func (c *smsCacheDouble) IncrementSMSDailyCount(context.Context, string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	return c.count, nil
}

// SendSMSCode records the delivery arguments and succeeds.
func (s *smsSenderDouble) SendSMSCode(_ context.Context, key, templateID, phone, code string) error {
	s.calls++
	s.key = key
	s.tpl = templateID
	s.phone = phone
	s.code = code
	return nil
}

// newSMSChallengesForTest builds challenges over the supplied settings
// values with fresh doubles.
func newSMSChallengesForTest(values map[string]string) (*identity.SMSChallenges, *smsCacheDouble, *smsSenderDouble) {
	runtime := identity.NewRuntimeSettings(&captchaSettingsStore{values: values}, settingscore.ErrSettingNotFound)
	cache := &smsCacheDouble{}
	sender := &smsSenderDouble{}
	return identity.NewSMSChallenges(cache, runtime, sender), cache, sender
}

// smsEnabledSettings returns a fully configured SMS settings set.
func smsEnabledSettings() map[string]string {
	return map[string]string{
		identity.SettingKeySMSLoginEnabled:   "true",
		identity.SettingKeySMSJuheKey:        "KEY-1",
		identity.SettingKeySMSJuheTemplateID: "TPL-7",
	}
}

// TestNormalizeSMSPhoneNumber accepts eleven-digit mainland mobiles and
// rejects prefixed, separated, and non-mobile shapes without rewriting.
func TestNormalizeSMSPhoneNumber(t *testing.T) {
	phone, ok := identity.NormalizeSMSPhoneNumber(" 13800138000 ")
	require.True(t, ok)
	require.Equal(t, "13800138000", phone)

	for _, raw := range []string{"+8613800138000", "8613800138000", "1380013800", "138001380000", "12345678901", "138-0013-8000", ""} {
		_, ok := identity.NormalizeSMSPhoneNumber(raw)
		require.False(t, ok, "input %q must be rejected", raw)
	}
}

// TestSMSChallengesSendCodeGates checks the feature switch and provider
// configuration gates before any delivery happens.
func TestSMSChallengesSendCodeGates(t *testing.T) {
	challenges, _, sender := newSMSChallengesForTest(map[string]string{})
	err := challenges.SendCode(context.Background(), "13800138000")
	require.ErrorIs(t, err, identity.ErrSMSLoginDisabled)
	require.Zero(t, sender.calls)

	challenges, _, sender = newSMSChallengesForTest(map[string]string{identity.SettingKeySMSLoginEnabled: "true"})
	err = challenges.SendCode(context.Background(), "13800138000")
	require.ErrorIs(t, err, identity.ErrSMSNotConfigured)
	require.Zero(t, sender.calls)
}

// TestSMSChallengesSendAndVerify runs one full challenge: the saved code
// is the delivered code, a resend inside the cooldown is refused, a
// wrong code consumes an attempt, and the right code succeeds once.
func TestSMSChallengesSendAndVerify(t *testing.T) {
	ctx := context.Background()
	challenges, cache, sender := newSMSChallengesForTest(smsEnabledSettings())

	require.NoError(t, challenges.SendCode(ctx, "13800138000"))
	require.Equal(t, 1, sender.calls)
	require.Equal(t, "KEY-1", sender.key)
	require.Equal(t, "TPL-7", sender.tpl)
	require.NotNil(t, cache.record)
	require.Equal(t, sender.code, cache.record.Code)
	require.Len(t, sender.code, 6)

	err := challenges.SendCode(ctx, "13800138000")
	require.ErrorIs(t, err, identity.ErrVerifyCodeTooFrequent)

	wrong := "000000"
	if sender.code == wrong {
		wrong = "999999"
	}
	err = challenges.VerifyCode(ctx, "13800138000", wrong)
	require.ErrorIs(t, err, identity.ErrInvalidVerifyCode)
	require.Equal(t, 1, cache.record.Attempts)

	require.NoError(t, challenges.VerifyCode(ctx, "13800138000", sender.code))
	require.Nil(t, cache.record)

	err = challenges.VerifyCode(ctx, "13800138000", sender.code)
	require.ErrorIs(t, err, identity.ErrInvalidVerifyCode)
}

// TestSMSChallengesDailyLimit refuses the send that would exceed the
// per-number daily cap before any provider call.
func TestSMSChallengesDailyLimit(t *testing.T) {
	challenges, cache, sender := newSMSChallengesForTest(smsEnabledSettings())
	cache.count = identity.SMSCodeDailyLimit

	err := challenges.SendCode(context.Background(), "13800138000")
	require.ErrorIs(t, err, identity.ErrSMSDailyLimit)
	require.Zero(t, sender.calls)
}

// TestSMSChallengesAttemptLimit locks the challenge once the saved
// record reaches the attempt ceiling, even for the correct code.
func TestSMSChallengesAttemptLimit(t *testing.T) {
	ctx := context.Background()
	challenges, cache, _ := newSMSChallengesForTest(smsEnabledSettings())
	cache.record = &identity.VerificationCodeData{
		Code:      "123456",
		Attempts:  identity.SMSCodeMaxAttempts,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(identity.SMSCodeTTL),
	}

	err := challenges.VerifyCode(ctx, "13800138000", "123456")
	require.ErrorIs(t, err, identity.ErrVerifyCodeMaxAttempts)
}

// TestGenerateSMSCode pins the six-digit decimal shape.
func TestGenerateSMSCode(t *testing.T) {
	code, err := identity.GenerateSMSCode()
	require.NoError(t, err)
	require.Len(t, code, 6)
	for _, r := range code {
		require.True(t, r >= '0' && r <= '9')
	}
}
