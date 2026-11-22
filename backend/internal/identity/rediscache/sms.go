package rediscache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/TokenFlux/TokenRouter/internal/identity"
)

// sms.go stores SMS verification challenges and per-number daily send
// counters in Redis.
// Challenge records share the email verification record shape but live
// under their own prefix, so a phone number and an email address can
// never alias one credential. The daily counter key embeds the local
// calendar date and expires with the day, bounding provider sends per
// number without a cleanup job.

// These prefixes separate SMS challenges from daily send counters.
const (
	smsCodeKeyPrefix       = "sms_code:"
	smsCodeDailyKeyPrefix  = "sms_code_daily:"
	smsCodeDailyDateLayout = "20060102"
)

// smsCache implements identity SMS challenge storage with Redis commands.
type smsCache struct {
	// rdb is retained for every cache operation. The caller owns this
	// shared client and closes it after identity services stop using the
	// cache.
	rdb *redis.Client
}

// smsCodeKey builds the challenge key for a normalized phone number.
func smsCodeKey(phone string) string {
	return smsCodeKeyPrefix + phone
}

// smsCodeDailyKey builds the current-day send counter key for a number.
func smsCodeDailyKey(phone string) string {
	return smsCodeDailyKeyPrefix + phone + ":" + time.Now().Format(smsCodeDailyDateLayout)
}

// NewSMSCache binds identity SMS challenge storage to the supplied Redis
// client. The cache retains that client, and its owner keeps the
// connection available until all challenge operations finish.
func NewSMSCache(rdb *redis.Client) identity.SMSCodeCache {
	return &smsCache{rdb: rdb}
}

// GetSMSCode decodes the challenge saved for the number. A missing key
// returns the Redis error so verification treats it as unavailable.
func (c *smsCache) GetSMSCode(ctx context.Context, phone string) (*identity.VerificationCodeData, error) {
	raw, err := c.rdb.Get(ctx, smsCodeKey(phone)).Result()
	if err != nil {
		return nil, err
	}
	var data identity.VerificationCodeData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// SetSMSCode serializes the challenge before writing it under the
// supplied lifetime.
func (c *smsCache) SetSMSCode(ctx context.Context, phone string, data *identity.VerificationCodeData, ttl time.Duration) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, smsCodeKey(phone), raw, ttl).Err()
}

// DeleteSMSCode removes the number's challenge key through Redis DEL. A
// key that is already absent is successful.
func (c *smsCache) DeleteSMSCode(ctx context.Context, phone string) error {
	return c.rdb.Del(ctx, smsCodeKey(phone)).Err()
}

// IncrementSMSDailyCount atomically increments the number's current-day
// counter and returns the new total. The first increment of a day sets
// the key's expiry to the day boundary so counters never accumulate.
func (c *smsCache) IncrementSMSDailyCount(ctx context.Context, phone string) (int64, error) {
	key := smsCodeDailyKey(phone)
	count, err := c.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		// A failed expiry leaves the counter readable until Redis evicts
		// it; the challenge service still enforces its own cooldown.
		_ = c.rdb.Expire(ctx, key, 24*time.Hour).Err()
	}
	return count, nil
}
