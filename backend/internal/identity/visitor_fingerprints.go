package identity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// visitor_fingerprints.go converts authenticated browser observations to
// independently keyed browser and IP aliases and bounds how long the aliases
// can appear in read results. Administration can correlate an exact pair
// across accounts without receiving the browser input or the client address.
//
// Import uses a distinct source namespace for the previous site's browser
// algorithm. The same digest framing can reproduce legacy pairs without
// implying that they match observations produced by the current browser code.
// Expired rows are excluded from all queries even when the cleanup worker has
// not yet removed them. The store enforces the per-account cap at each write.

const (
	// These values identify the current collection algorithm and define the
	// eligibility window for both live and imported investigation records.
	VisitorFingerprintBrowser = "browser"
	VisitorFingerprintLegacy  = "legacy"
	VisitorFingerprintTTL     = 90 * 24 * time.Hour
	VisitorFingerprintCap     = 5
	VisitorFingerprintPage    = 50
)

type (
	// VisitorFingerprint stores an observed browser and network pair for one account.
	VisitorFingerprint struct {
		ID          int64     `json:"id"`            // stable observation cursor
		UserID      int64     `json:"user_id"`       // authenticated observer account
		Source      string    `json:"source"`        // fingerprint algorithm namespace
		VisitorHash string    `json:"visitor_hash"`  // keyed browser alias
		IPHash      string    `json:"ip_hash"`       // keyed address alias
		FirstSeenAt time.Time `json:"first_seen_at"` // first observation time
		LastSeenAt  time.Time `json:"last_seen_at"`  // latest observation time
	}

	// VisitorFingerprintFilter bounds an administrator query by owner, aliases, or cursor.
	VisitorFingerprintFilter struct {
		UserID      int64  // optional account criterion
		VisitorHash string // optional keyed browser alias
		IPHash      string // optional keyed address alias
		Before      int64  // exclusive ID cursor
		Duplicates  bool   // require another visible account sharing the exact pair
	}

	// VisitorFingerprintStore reads bounded observations and enforces owner row caps.
	VisitorFingerprintStore interface {
		// ObserveVisitorFingerprint serializes writes for an account and caps retained pairs.
		ObserveVisitorFingerprint(context.Context, VisitorFingerprint) error
		// ListVisitorFingerprints reads visible observations from the supplied retention window.
		ListVisitorFingerprints(context.Context, VisitorFingerprintFilter, time.Time) ([]VisitorFingerprint, error)
	}

	// VisitorFingerprints prepares browser evidence for authenticated reads and writes.
	VisitorFingerprints struct {
		store VisitorFingerprintStore // observation persistence boundary
		key   []byte                  // immutable independent HMAC key
	}
)

// NewVisitorFingerprints decodes the persisted HMAC key and retains an independent
// byte slice. Invalid keys fail construction so observations cannot become
// unreadable after a restart with inconsistent signing material.
func NewVisitorFingerprints(store VisitorFingerprintStore, encodedKey string) (*VisitorFingerprints, error) {
	key, err := hex.DecodeString(encodedKey)
	if err != nil || len(key) != sha256.Size {
		return nil, fmt.Errorf("visitor fingerprint HMAC key must encode 32 bytes")
	}
	return &VisitorFingerprints{store: store, key: key}, nil
}

// Observe records a current browser digest with the trusted request address.
// Input is checked before querying storage. The identities remain scoped to
// the caller account, and persistence receives keyed aliases rather than raw
// request values.
func (s *VisitorFingerprints) Observe(ctx context.Context, userID int64, visitor, clientIP string) error {
	addr, err := netip.ParseAddr(clientIP)
	if userID <= 0 || !IsVisitorFingerprintHash(visitor) || err != nil || addr.Zone() != "" {
		return apperror.BadRequest("VISITOR_FINGERPRINT_INVALID", "The visit could not be recorded with these identifiers.")
	}
	now := time.Now().UTC()
	return s.store.ObserveVisitorFingerprint(ctx, VisitorFingerprint{
		UserID:      userID,
		Source:      VisitorFingerprintBrowser,
		VisitorHash: s.digest("visitor", visitor),
		IPHash:      s.digest("ip", addr.Unmap().String()),
		FirstSeenAt: now,
		LastSeenAt:  now,
	})
}

// List reads one page after checking its optional aliases. The HTTP caller
// must supply the signed-in owner on user routes. Only administrators may
// choose a different owner or request cross-account matches.
func (s *VisitorFingerprints) List(ctx context.Context, filter VisitorFingerprintFilter) ([]VisitorFingerprint, error) {
	if filter.UserID < 0 || filter.Before < 0 ||
		(filter.VisitorHash != "" && !IsVisitorFingerprintHash(filter.VisitorHash)) ||
		(filter.IPHash != "" && !IsVisitorFingerprintHash(filter.IPHash)) {
		return nil, apperror.BadRequest("VISITOR_FINGERPRINT_FILTER_INVALID", "Check the account, cursor, or alias entered.")
	}
	return s.store.ListVisitorFingerprints(ctx, filter, time.Now().UTC().Add(-VisitorFingerprintTTL))
}

// digest domains the HMAC by format, source, and input category. Colon
// delimiters separate the fixed segments, so the same value cannot identify
// both a browser and a network address in a single investigation.
func (s *VisitorFingerprints) digest(kind, value string) string {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte("visitor-fingerprint:v1:" + VisitorFingerprintBrowser + ":" + kind + ":" + value))
	return hex.EncodeToString(mac.Sum(nil))
}

// IsVisitorFingerprintHash reports whether text is a canonical SHA-256 digest.
func IsVisitorFingerprintHash(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
