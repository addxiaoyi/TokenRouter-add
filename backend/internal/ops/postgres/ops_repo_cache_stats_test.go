package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

// TestOpsRepositoryGetCacheStats_WithFilters supplies platform and group
// filters with one overall row and two group rows. It protects normalized
// filter arguments, the group limit argument, and hit rates computed from the
// summed counters of each slice.
func TestOpsRepositoryGetCacheStats_WithFilters(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &Store{db: db}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	groupID := int64(9)

	filter := &ops.OpsCacheStatsFilter{
		TimeRange: "1d",
		StartTime: start,
		EndTime:   end,
		Platform:  " Anthropic ",
		GroupID:   &groupID,
	}

	overallRows := sqlmock.NewRows([]string{
		"request_count",
		"input_tokens",
		"cache_creation_tokens",
		"cache_read_tokens",
		"total_input_tokens",
	}).AddRow(int64(40), int64(1000), int64(200), int64(300), int64(1500))

	mock.ExpectQuery(`FROM \(SELECT \* FROM usage_logs WHERE request_source = 'user'\) ul`).
		WithArgs(start, end, groupID, "anthropic").
		WillReturnRows(overallRows)

	groupRows := sqlmock.NewRows([]string{
		"group_id",
		"group_name",
		"request_count",
		"input_tokens",
		"cache_creation_tokens",
		"cache_read_tokens",
		"total_input_tokens",
	}).
		AddRow(int64(9), "Claude", int64(30), int64(600), int64(150), int64(250), int64(1000)).
		AddRow(int64(12), "GPT", int64(10), int64(400), int64(50), int64(50), int64(500))

	mock.ExpectQuery(`LEFT JOIN groups g ON g\.id = ul\.group_id[\s\S]+LIMIT \$5`).
		WithArgs(start, end, groupID, "anthropic", cacheStatsGroupLimit).
		WillReturnRows(groupRows)

	resp, err := repo.GetCacheStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "anthropic", resp.Platform)
	require.NotNil(t, resp.GroupID)
	require.Equal(t, groupID, *resp.GroupID)

	require.Equal(t, int64(40), resp.Overall.RequestCount)
	require.Equal(t, int64(1500), resp.Overall.TotalInputTokens)
	require.NotNil(t, resp.Overall.HitRate)
	require.InDelta(t, 0.2, *resp.Overall.HitRate, 0.0001)

	require.Len(t, resp.Groups, 2)
	require.Equal(t, int64(9), resp.Groups[0].GroupID)
	require.Equal(t, "Claude", resp.Groups[0].GroupName)
	require.NotNil(t, resp.Groups[0].HitRate)
	require.InDelta(t, 0.25, *resp.Groups[0].HitRate, 0.0001)
	require.Equal(t, int64(12), resp.Groups[1].GroupID)
	require.NotNil(t, resp.Groups[1].HitRate)
	require.InDelta(t, 0.1, *resp.Groups[1].HitRate, 0.0001)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestOpsRepositoryGetCacheStats_EmptyWindow supplies a window whose overall
// row contains only zeros and whose group query returns no rows. It protects
// the absent overall hit rate and the empty, nonnil group slice.
func TestOpsRepositoryGetCacheStats_EmptyWindow(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &Store{db: db}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	filter := &ops.OpsCacheStatsFilter{
		TimeRange: "1h",
		StartTime: start,
		EndTime:   end,
	}

	overallRows := sqlmock.NewRows([]string{
		"request_count",
		"input_tokens",
		"cache_creation_tokens",
		"cache_read_tokens",
		"total_input_tokens",
	}).AddRow(int64(0), int64(0), int64(0), int64(0), int64(0))

	mock.ExpectQuery(`FROM \(SELECT \* FROM usage_logs WHERE request_source = 'user'\) ul`).
		WithArgs(start, end).
		WillReturnRows(overallRows)

	groupRows := sqlmock.NewRows([]string{
		"group_id",
		"group_name",
		"request_count",
		"input_tokens",
		"cache_creation_tokens",
		"cache_read_tokens",
		"total_input_tokens",
	})

	mock.ExpectQuery(`LEFT JOIN groups g ON g\.id = ul\.group_id[\s\S]+LIMIT \$3`).
		WithArgs(start, end, cacheStatsGroupLimit).
		WillReturnRows(groupRows)

	resp, err := repo.GetCacheStats(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int64(0), resp.Overall.RequestCount)
	require.Equal(t, int64(0), resp.Overall.TotalInputTokens)
	require.Nil(t, resp.Overall.HitRate)
	require.NotNil(t, resp.Groups)
	require.Len(t, resp.Groups, 0)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestOpsRepositoryGetCacheStats_InvalidFilter submits a nil filter and a
// filter with a zero start time. Both calls must fail before any query is
// issued against the mocked database.
func TestOpsRepositoryGetCacheStats_InvalidFilter(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &Store{db: db}

	_, err := repo.GetCacheStats(context.Background(), nil)
	require.Error(t, err)

	_, err = repo.GetCacheStats(context.Background(), &ops.OpsCacheStatsFilter{
		EndTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	require.Error(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}
