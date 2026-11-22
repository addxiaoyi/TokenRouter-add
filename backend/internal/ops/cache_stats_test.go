package ops

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// cache_stats_test.go verifies cache-statistics request validation and the
// hit-rate calculation over summed usage counters.
// The service tests use a capturing repository stub, so validation failures
// are checked before storage is reached. The rate tests pin the documented
// formula and the absent-rate result for slices without input-side tokens.

// cacheStatsRepoStub captures cache-statistics filters and supplies configured
// response or error results.
type cacheStatsRepoStub struct {
	// OpsRepository leaves unrelated methods unconfigured through its nil
	// embedded interface. These tests invoke the GetCacheStats override and need
	// another implementation for other calls.
	OpsRepository // unused embedded repository methods
	// captured retains the supplied filter before err is inspected. A nonnil err
	// wins over resp; otherwise resp is returned when present and an absent
	// response produces an empty statistics result.
	resp     *OpsCacheStatsResponse // configured successful statistics response
	err      error                  // configured repository failure result
	captured *OpsCacheStatsFilter   // last supplied filter pointer
}

// GetCacheStats retains the supplied filter pointer before returning the
// configured error or response. A nil response produces an empty successful
// object, and subsequent caller mutation remains visible through captured.
func (s *cacheStatsRepoStub) GetCacheStats(ctx context.Context, filter *OpsCacheStatsFilter) (*OpsCacheStatsResponse, error) {
	s.captured = filter
	if s.err != nil {
		return nil, s.err
	}
	if s.resp != nil {
		return s.resp, nil
	}
	return &OpsCacheStatsResponse{}, nil
}

// TestCacheHitRate supplies empty, unread, partial, and fully cached slices.
// It checks the cache-read share of the summed input side and the nil result
// reserved for slices without any input-side tokens.
func TestCacheHitRate(t *testing.T) {
	tests := []struct {
		name     string  // rate case display name
		read     int64   // cache-read token input
		input    int64   // fresh input token input
		creation int64   // cache-creation token input
		want     float64 // expected rate when wantNil is false
		wantNil  bool    // whether an absent rate is expected
	}{
		{name: "no tokens", read: 0, input: 0, creation: 0, wantNil: true},
		{name: "no cache reads", read: 0, input: 100, creation: 50, want: 0},
		{name: "quarter of input side", read: 50, input: 100, creation: 50, want: 0.25},
		{name: "all reads", read: 10, input: 0, creation: 0, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CacheHitRate(tt.read, tt.input, tt.creation)
			if tt.wantNil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.InDelta(t, tt.want, *got, 0.0001)
		})
	}
}

// TestOpsServiceGetCacheStats_Validation supplies absent or invalid times and
// group identity. Each subtest checks the service HTTP status and specific
// error reason for its listed input.
func TestOpsServiceGetCacheStats_Validation(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name       string               // validation case display name
		filter     *OpsCacheStatsFilter // submitted invalid filter fixture
		wantCode   int                  // expected HTTP status code
		wantReason string               // expected application error reason
	}{
		{
			name:       "filter 不能为空",
			filter:     nil,
			wantCode:   400,
			wantReason: "OPS_FILTER_REQUIRED",
		},
		{
			name: "start_time/end_time 必填",
			filter: &OpsCacheStatsFilter{
				StartTime: time.Time{},
				EndTime:   now,
			},
			wantCode:   400,
			wantReason: "OPS_TIME_RANGE_REQUIRED",
		},
		{
			name: "start_time 不能晚于 end_time",
			filter: &OpsCacheStatsFilter{
				StartTime: now,
				EndTime:   now.Add(-1 * time.Minute),
			},
			wantCode:   400,
			wantReason: "OPS_TIME_RANGE_INVALID",
		},
		{
			name: "group_id 必须大于 0",
			filter: &OpsCacheStatsFilter{
				StartTime: now.Add(-time.Hour),
				EndTime:   now,
				GroupID:   int64Ptr(0),
			},
			wantCode:   400,
			wantReason: "OPS_GROUP_ID_INVALID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &OpsService{
				opsRepo: &cacheStatsRepoStub{},
			}

			_, err := svc.GetCacheStats(context.Background(), tt.filter)
			require.Error(t, err)
			require.Equal(t, tt.wantCode, httpx.ErrorCode(err))
			require.Equal(t, tt.wantReason, apperror.Reason(err))
		})
	}
}

// TestOpsServiceGetCacheStats_Success requests a one-hour window with platform
// and group filters. It checks the repository receives the supplied filter
// unchanged and the configured response is returned to the caller.
func TestOpsServiceGetCacheStats_Success(t *testing.T) {
	now := time.Now().UTC()
	groupID := int64(9)
	want := &OpsCacheStatsResponse{
		Overall: OpsCacheStatsOverall{RequestCount: 12, TotalInputTokens: 300},
	}
	repo := &cacheStatsRepoStub{resp: want}
	svc := &OpsService{opsRepo: repo}

	filter := &OpsCacheStatsFilter{
		TimeRange: "1h",
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
		Platform:  "anthropic",
		GroupID:   &groupID,
	}
	resp, err := svc.GetCacheStats(context.Background(), filter)
	require.NoError(t, err)
	require.Same(t, want, resp)
	require.Same(t, filter, repo.captured)
}

// TestOpsServiceGetCacheStats_RepoUnavailable requests a valid window from a
// service with no repository. It protects the unavailable HTTP status and
// repository-unavailable reason.
func TestOpsServiceGetCacheStats_RepoUnavailable(t *testing.T) {
	now := time.Now().UTC()
	svc := &OpsService{}

	_, err := svc.GetCacheStats(context.Background(), &OpsCacheStatsFilter{
		TimeRange: "1h",
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
	})
	require.Error(t, err)
	require.Equal(t, 503, httpx.ErrorCode(err))
	require.Equal(t, "OPS_REPO_UNAVAILABLE", apperror.Reason(err))
}
