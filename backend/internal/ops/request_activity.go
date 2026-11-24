package ops

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"hash/maphash"
	"math/bits"
	"sort"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// request_activity.go observes clusters of similar authenticated generation
// requests over ten minutes. The process-local keyed token fingerprints only
// live in memory. After clustering, callers receive account identifiers and
// counts without request content, token fingerprints, upstream routes, API
// keys, or per-request metadata.
//
// Profiles are scoped to the submitting account. The tracker scans at most
// fifty profiles for a near match and owns a globally bounded least-recently
// used queue, so one account cannot retain unbounded state. Cluster identity
// and last-seen time change together under the same mutex. This is independent
// of scheduler admission, so recording or skipping an observation never
// changes whether a request reaches its provider.

// These limits preserve the legacy Hamming distance and profile caps while
// bounding the cost of processing request bodies on the gateway path.
const (
	RequestActivityWindow     = 10 * time.Minute
	RequestActivityUserCap    = 50
	RequestActivityGlobalCap  = 1000
	RequestActivityThreshold  = 5
	requestActivityDistance   = 5
	requestActivityBodyBytes  = 32 << 10
	requestActivityTokenLimit = 256
	requestActivityRate       = 1000
	requestActivityBurst      = 250
	requestActivityRefresh    = 10 * time.Second
	requestActivityHistoryCap = 25
)

// RequestActivity holds process-local request similarity clusters and snapshots.
type RequestActivity struct {
	// lru orders profiles by last observation. Per-account member maps reference
	// the same elements so expiry and eviction can remove both indexes together.
	mu      sync.Mutex                // profile and enablement mutex
	lru     list.List                 // ordered live activity profiles
	users   map[int64][]*list.Element // bounded lists of each account's profiles
	enabled bool                      // whether observations are accepted

	seed   maphash.Seed         // process-local token hashing key
	budget *rate.Limiter        // CPU admission bound independent of request processing
	store  RequestActivityStore // aggregate-only history storage
}

// requestActivityProfile contains the current cluster representative and observation time.
type requestActivityProfile struct {
	userID int64     // authenticated submitting account
	hash   uint64    // keyed representative held in memory
	seen   time.Time // latest member observation
}

// RequestActivityCount describes one account's observed cluster count.
type RequestActivityCount struct {
	UserID   int64 `json:"user_id"`         // current account identity
	Profiles int   `json:"active_profiles"` // count of current request clusters
}

// RequestActivitySnapshot contains an in-memory observation without request fingerprints.
type RequestActivitySnapshot struct {
	ObservedAt  time.Time              `json:"observed_at"`     // aggregate collection time
	Profiles    int                    `json:"global_profiles"` // total retained cluster count
	UserLimit   int                    `json:"user_limit"`      // maximum tracked profiles per account
	GlobalLimit int                    `json:"global_limit"`    // maximum tracked profiles per process
	Accounts    []RequestActivityCount `json:"accounts"`        // activity ranked by count
}

// RequestActivityEntry stores one persisted threshold hit or imported legacy summary.
type RequestActivityEntry struct {
	ID             int64     `json:"id"`                       // stored observation identifier
	ObservedAt     time.Time `json:"observed_at"`              // timestamp of source observation
	UserID         *int64    `json:"user_id,omitempty"`        // linked account when still known
	LegacyUserID   *int64    `json:"legacy_user_id,omitempty"` // original account identity
	Profiles       int       `json:"active_profiles"`          // active profile count for the account
	GlobalProfiles int       `json:"global_profiles"`          // active profile count across accounts
	UserLimit      int       `json:"user_limit"`               // per-account cap from source configuration
	GlobalLimit    int       `json:"global_limit"`             // total cap from source configuration
}

// RequestActivityPage carries a bounded history page and its exclusive cursor.
type RequestActivityPage struct {
	Items  []RequestActivityEntry `json:"items"`             // newest matching threshold records
	NextID int64                  `json:"next_id,omitempty"` // previous page's final observation ID
}

// RequestActivityView combines current clustered activity with persisted counts.
type RequestActivityView struct {
	Current RequestActivitySnapshot `json:"current"` // current process observation
	History RequestActivityPage     `json:"history"` // paginated threshold history
}

// RequestActivityStore persists aggregate account counts without execution metadata.
type RequestActivityStore interface {
	// Save writes one sampled bucket and its threshold rows together. A repeat of
	// the same native account and bucket may be ignored; existing rows stay intact.
	Save(context.Context, RequestActivitySnapshot) error
	// List returns the newest matching threshold records beyond the optional cursor.
	List(context.Context, int64, int64, int) (RequestActivityPage, error)
	// Prune removes at most one bounded batch of expired native observations.
	Prune(context.Context, time.Time) error
}

// NewRequestActivity creates one tracker whose ephemeral key dies with the process.
func NewRequestActivity(store RequestActivityStore) *RequestActivity {
	return &RequestActivity{
		users:  make(map[int64][]*list.Element),
		seed:   maphash.MakeSeed(),
		budget: rate.NewLimiter(requestActivityRate, requestActivityBurst),
		store:  store,
	}
}

// Observe clusters one authenticated JSON generation request. The body is only
// read in this call. Disabled observation, invalid JSON, oversized requests,
// and exhausted observation capacity leave the traffic path untouched.
func (a *RequestActivity) Observe(userID int64, body []byte) {
	if a == nil || userID <= 0 || len(body) == 0 || len(body) > requestActivityBodyBytes {
		return
	}
	a.mu.Lock()
	enabled := a.enabled
	a.mu.Unlock()
	if !enabled || !a.budget.Allow() {
		return
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return
	}
	hash, ok := a.fingerprint(trimmed)
	if ok {
		a.record(userID, hash)
	}
}

// Snapshot copies ranked current counts under one lock after expiring old profiles.
// Optional user selection limits visible accounts while preserving the total count.
func (a *RequestActivity) Snapshot(userID int64) RequestActivitySnapshot {
	a.mu.Lock()
	now := time.Now().UTC()
	a.expire(now)
	snapshot := RequestActivitySnapshot{
		ObservedAt: now, Profiles: a.lru.Len(), UserLimit: RequestActivityUserCap,
		GlobalLimit: RequestActivityGlobalCap, Accounts: make([]RequestActivityCount, 0, len(a.users)),
	}
	for id, members := range a.users {
		if userID == 0 || id == userID {
			snapshot.Accounts = append(snapshot.Accounts, RequestActivityCount{UserID: id, Profiles: len(members)})
		}
	}
	a.mu.Unlock()
	sort.Slice(snapshot.Accounts, func(i, j int) bool {
		left, right := snapshot.Accounts[i], snapshot.Accounts[j]
		if left.Profiles != right.Profiles {
			return left.Profiles > right.Profiles
		}
		return left.UserID < right.UserID
	})
	return snapshot
}

// Read returns bounded private history and current activity in one projection.
// The caller must validate administrator scope and positive supplied filters.
func (a *RequestActivity) Read(ctx context.Context, userID, beforeID int64) (RequestActivityView, error) {
	page, err := a.store.List(ctx, userID, beforeID, requestActivityHistoryCap)
	if err != nil {
		return RequestActivityView{}, err
	}
	current := a.Snapshot(userID)
	current.Accounts = current.Accounts[:min(len(current.Accounts), requestActivityHistoryCap)]
	return RequestActivityView{Current: current, History: page}, nil
}

// SetEnabled opens observation admission or discards all ephemeral profiles.
// Disable and record share the lock so a late hash cannot add data after disable.
func (a *RequestActivity) SetEnabled(enabled bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.enabled = enabled
	if !enabled {
		a.lru.Init()
		clear(a.users)
	}
}

// fingerprint calculates an equal-weight 64-bit SimHash from keyed token hashes.
// Oversized token lists are discarded instead of matching on a shared prefix.
func (a *RequestActivity) fingerprint(body []byte) (uint64, bool) {
	var weights [64]int
	count := 0
	for token := range bytes.FieldsSeq(body) {
		count++
		if count > requestActivityTokenLimit {
			return 0, false
		}
		hashed := maphash.Bytes(a.seed, token)
		for bit := range weights {
			weights[bit] += (int(hashed>>bit&1) * 2) - 1
		}
	}
	var hash uint64
	for bit, weight := range weights {
		if weight >= 0 {
			hash |= uint64(1) << bit
		}
	}
	return hash, count > 0
}

// record refreshes the nearest cluster within the Hamming threshold or admits a
// new profile. It selects time after acquiring the lock so LRU order is also
// timestamp order under concurrent observations. A near match uses the newest
// hash as its representative, preserving the legacy similarity-chain behavior.
// When a cap is reached, the oldest profile in that scope yields its place.
// Recording never holds the tracker lock while hashing the body.
func (a *RequestActivity) record(userID int64, hash uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.enabled {
		return
	}
	now := time.Now().UTC()
	a.expire(now)
	members := a.users[userID]
	var nearest, oldest *list.Element
	distance := requestActivityDistance + 1
	for _, elem := range members {
		p := elem.Value.(*requestActivityProfile)
		if oldest == nil || p.seen.Before(oldest.Value.(*requestActivityProfile).seen) {
			oldest = elem
		}
		if delta := bits.OnesCount64(p.hash ^ hash); delta < distance {
			nearest, distance = elem, delta
		}
	}
	if nearest != nil {
		p := nearest.Value.(*requestActivityProfile)
		p.hash, p.seen = hash, now
		a.lru.MoveToBack(nearest)
		return
	}
	if len(members) >= RequestActivityUserCap {
		a.remove(oldest)
	}
	if a.lru.Len() >= RequestActivityGlobalCap {
		a.remove(a.lru.Front())
	}
	member := a.lru.PushBack(&requestActivityProfile{userID: userID, hash: hash, seen: now})
	a.users[userID] = append(a.users[userID], member)
}

// expire drops every profile outside the ten-minute window under the tracker lock.
func (a *RequestActivity) expire(now time.Time) {
	cutoff := now.Add(-RequestActivityWindow)
	for front := a.lru.Front(); front != nil && !front.Value.(*requestActivityProfile).seen.After(cutoff); front = a.lru.Front() {
		a.remove(front)
	}
}

// remove discards a profile from both indexes while the caller holds the lock.
// The member slice need not keep a sorted order because record selects its
// oldest observation from timestamps and the global list owns expiry.
func (a *RequestActivity) remove(elem *list.Element) {
	id := elem.Value.(*requestActivityProfile).userID
	members := a.users[id]
	for i, candidate := range members {
		if candidate != elem {
			continue
		}
		members[i] = members[len(members)-1]
		members[len(members)-1] = nil
		members = members[:len(members)-1]
		break
	}
	if len(members) == 0 {
		delete(a.users, id)
	} else {
		a.users[id] = members
	}
	a.lru.Remove(elem)
}
