package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

// user_monitoring_test.go verifies aggregate projection and unavailable measurements.
// SQL expectations keep group authorization arguments on every query, while
// mixed samples protect success-rate arithmetic and idle groups remain unpriced
// in health terms rather than receiving invented zero latency or full success.
// TestUserMonitoringProjection combines success, failure, and empty group observations.
// Only the supplied group set is queried; nullable timing and absent sample
// rates survive conversion into the user-safe response structure.
func TestUserMonitoringProjection(t *testing.T) {
	db, mock := newSQLMock(t)
	start := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	mock.ExpectQuery("WITH observations AS").WithArgs("{4,5}", start, end).WillReturnRows(sqlmock.NewRows([]string{"group_id", "success", "errors", "duration", "ttft", "input_tokens", "cache_creation_tokens", "cache_read_tokens", "tps", "active_models"}).AddRow(4, 3, 1, 200, nil, 20, 20, 60, 25.5, 1))
	mock.ExpectQuery("SELECT group_id,bucket").WithArgs("{4,5}", start, end, 5).WillReturnRows(sqlmock.NewRows([]string{"group_id", "bucket", "success", "errors"}).AddRow(4, start, 3, 1))
	mock.ExpectQuery("WITH models AS").WithArgs("{4,5}", start, end).WillReturnRows(sqlmock.NewRows([]string{"group_id", "model", "requests", "duration", "ttft", "tps"}).AddRow(4, "visible-model", 3, 200, nil, 25.5))
	groups, err := NewUserMonitoringStore(db).ReadUserMonitoring(context.Background(), []ops.MonitoringGroup{{ID: 4}, {ID: 5}}, start, end)
	require.NoError(t, err)
	require.Equal(t, 75.0, *groups[0].SuccessRate)
	require.Equal(t, 0.6, *groups[0].CacheHitRate)
	require.Equal(t, 25.5, *groups[0].AvgTPS)
	require.Equal(t, int64(1), groups[0].ActiveModels)
	require.Nil(t, groups[1].AvgTPS)
	require.Nil(t, groups[1].CacheHitRate)
	require.Nil(t, groups[0].AvgTTFTMS)
	require.Len(t, groups[0].Models, 1)
	require.Nil(t, groups[1].SuccessRate)
	require.Nil(t, groups[1].AvgDurationMS)
	require.Empty(t, groups[1].Trend)
	require.NoError(t, mock.ExpectationsWereMet())
}
