package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateChannelExternalTablesMovesAndDropsLegacyColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)

	previousDB := DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		initCol()
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Exec(`
CREATE TABLE channels (
	id integer PRIMARY KEY,
	name text,
	`+"`key`"+` text NOT NULL,
	breaker_pressure real DEFAULT 0,
	breaker_updated_at bigint DEFAULT 0,
	breaker_fail_streak integer DEFAULT 0,
	breaker_cooldown_at bigint DEFAULT 0,
	breaker_last_failure varchar(64) DEFAULT '',
	breaker_hp real DEFAULT -1,
	breaker_trip_count integer DEFAULT 0,
	breaker_recent_requests real DEFAULT 0,
	breaker_recent_failures real DEFAULT 0,
	breaker_recent_timeouts real DEFAULT 0,
	test_case text,
	expected_answer text,
	max_first_token_latency integer,
	scheduled_test_interval integer DEFAULT 0
)`).Error)
	require.NoError(t, db.Exec(`
INSERT INTO channels (
	id, name, `+"`key`"+`, breaker_pressure, breaker_updated_at, breaker_fail_streak,
	breaker_cooldown_at, breaker_last_failure, breaker_hp, breaker_trip_count,
	breaker_recent_requests, breaker_recent_failures, breaker_recent_timeouts,
	test_case, expected_answer, max_first_token_latency, scheduled_test_interval
) VALUES (
	1, 'legacy', 'sk-test', 2.5, 123, 3,
	456, 'timeout', 7.5, 4,
	10, 5, 2,
	'hello', 'world', 9, 15
)`).Error)

	require.NoError(t, db.AutoMigrate(&ChannelBreakerState{}, &ChannelTestConfig{}))
	require.NoError(t, migrateChannelExternalTables())

	var breakerState ChannelBreakerState
	require.NoError(t, db.First(&breakerState, "channel_id = ?", 1).Error)
	require.Equal(t, 2.5, breakerState.BreakerPressure)
	require.Equal(t, int64(123), breakerState.BreakerUpdatedAt)
	require.Equal(t, 3, breakerState.BreakerFailStreak)
	require.Equal(t, int64(456), breakerState.BreakerCooldownAt)
	require.Equal(t, "timeout", breakerState.BreakerLastFailure)
	require.Equal(t, 7.5, breakerState.BreakerHP)
	require.Equal(t, 4, breakerState.BreakerTripCount)
	require.Equal(t, 10.0, breakerState.BreakerRecentRequests)
	require.Equal(t, 5.0, breakerState.BreakerRecentFailures)
	require.Equal(t, 2.0, breakerState.BreakerRecentTimeouts)

	var testConfig ChannelTestConfig
	require.NoError(t, db.First(&testConfig, "channel_id = ?", 1).Error)
	require.NotNil(t, testConfig.TestCase)
	require.Equal(t, "hello", *testConfig.TestCase)
	require.NotNil(t, testConfig.ExpectedAnswer)
	require.Equal(t, "world", *testConfig.ExpectedAnswer)
	require.NotNil(t, testConfig.MaxFirstTokenLatency)
	require.Equal(t, 9, *testConfig.MaxFirstTokenLatency)
	require.NotNil(t, testConfig.ScheduledTestInterval)
	require.Equal(t, 15, *testConfig.ScheduledTestInterval)

	for _, column := range []string{
		"breaker_pressure",
		"breaker_updated_at",
		"breaker_fail_streak",
		"breaker_cooldown_at",
		"breaker_last_failure",
		"breaker_hp",
		"breaker_trip_count",
		"breaker_recent_requests",
		"breaker_recent_failures",
		"breaker_recent_timeouts",
		"test_case",
		"expected_answer",
		"max_first_token_latency",
		"scheduled_test_interval",
	} {
		require.False(t, db.Migrator().HasColumn(&legacyChannelExternalColumns{}, column), "legacy column %s should be dropped", column)
	}
}
