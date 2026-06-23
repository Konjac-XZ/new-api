package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
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

func TestMigrateChannelExternalTablesUpdatesExistingExternalRowsFromLegacyColumns(t *testing.T) {
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
	test_case text,
	max_first_token_latency integer
)`).Error)
	require.NoError(t, db.Exec(`
INSERT INTO channels (
	id, name, `+"`key`"+`, breaker_pressure, breaker_updated_at, test_case, max_first_token_latency
) VALUES (
	1, 'legacy-existing-row', 'sk-test', 4.5, 222, 'legacy prompt', 12
)`).Error)
	require.NoError(t, db.AutoMigrate(&ChannelBreakerState{}, &ChannelTestConfig{}))
	require.NoError(t, db.Create(&ChannelBreakerState{
		ChannelID:        1,
		BreakerPressure:  1.5,
		BreakerUpdatedAt: 111,
	}).Error)
	oldPrompt := "old prompt"
	oldLatency := 3
	require.NoError(t, db.Create(&ChannelTestConfig{
		ChannelID:                1,
		TestCase:                 &oldPrompt,
		MaxFirstTokenLatency:     &oldLatency,
		MaxRetryAttempts:         7,
		TreatEmptyReplyAsFailure: true,
	}).Error)

	require.NoError(t, migrateChannelExternalTables())

	var breakerState ChannelBreakerState
	require.NoError(t, db.First(&breakerState, "channel_id = ?", 1).Error)
	require.Equal(t, 4.5, breakerState.BreakerPressure)
	require.Equal(t, int64(222), breakerState.BreakerUpdatedAt)

	var testConfig ChannelTestConfig
	require.NoError(t, db.First(&testConfig, "channel_id = ?", 1).Error)
	require.NotNil(t, testConfig.TestCase)
	require.Equal(t, "legacy prompt", *testConfig.TestCase)
	require.NotNil(t, testConfig.MaxFirstTokenLatency)
	require.Equal(t, 12, *testConfig.MaxFirstTokenLatency)
	require.Equal(t, 7, testConfig.MaxRetryAttempts)
	require.True(t, testConfig.TreatEmptyReplyAsFailure)
}

func TestMigrateChannelExternalTablesMovesFeatureSettingsOutOfChannelSetting(t *testing.T) {
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

	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelBreakerState{}, &ChannelTestConfig{}))
	coeff := 2.5
	settingBytes, err := common.Marshal(dto.ChannelSettings{
		ForceFormat:              true,
		MaxRetryAttempts:         3,
		TreatEmptyReplyAsFailure: true,
		DynamicCircuitBreaker:    true,
		ToleranceCoefficient:     &coeff,
	})
	require.NoError(t, err)
	channel := Channel{
		Name:    "legacy-setting",
		Key:     "sk-test",
		Setting: common.GetPointer(string(settingBytes)),
	}
	require.NoError(t, db.Create(&channel).Error)

	require.NoError(t, migrateChannelExternalTables())

	var testConfig ChannelTestConfig
	require.NoError(t, db.First(&testConfig, "channel_id = ?", channel.Id).Error)
	require.Equal(t, 3, testConfig.MaxRetryAttempts)
	require.True(t, testConfig.TreatEmptyReplyAsFailure)

	var breakerState ChannelBreakerState
	require.NoError(t, db.First(&breakerState, "channel_id = ?", channel.Id).Error)
	require.True(t, breakerState.DynamicCircuitBreaker)
	require.NotNil(t, breakerState.ToleranceCoefficient)
	require.Equal(t, coeff, *breakerState.ToleranceCoefficient)

	var migrated Channel
	require.NoError(t, db.First(&migrated, "id = ?", channel.Id).Error)
	require.NotNil(t, migrated.Setting)
	var migratedSetting map[string]any
	require.NoError(t, common.Unmarshal([]byte(*migrated.Setting), &migratedSetting))
	require.Equal(t, true, migratedSetting["force_format"])
	require.NotContains(t, migratedSetting, "max_retry_attempts")
	require.NotContains(t, migratedSetting, "treat_empty_reply_as_failure")
	require.NotContains(t, migratedSetting, "dynamic_circuit_breaker")
	require.NotContains(t, migratedSetting, "tolerance_coefficient")
}

func TestMigrateChannelExternalTablesMergesFeatureSettingsWithoutOverwritingTestConfig(t *testing.T) {
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

	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelBreakerState{}, &ChannelTestConfig{}))
	settingBytes, err := common.Marshal(dto.ChannelSettings{
		MaxRetryAttempts:         3,
		TreatEmptyReplyAsFailure: true,
	})
	require.NoError(t, err)
	channel := Channel{
		Name:    "legacy-setting-merge",
		Key:     "sk-test",
		Setting: common.GetPointer(string(settingBytes)),
	}
	require.NoError(t, db.Create(&channel).Error)
	testCase := "hello"
	expectedAnswer := "world"
	maxLatency := 9
	interval := 15
	require.NoError(t, db.Create(&ChannelTestConfig{
		ChannelID:             channel.Id,
		TestCase:              &testCase,
		ExpectedAnswer:        &expectedAnswer,
		MaxFirstTokenLatency:  &maxLatency,
		ScheduledTestInterval: &interval,
	}).Error)

	require.NoError(t, migrateChannelExternalTables())

	var testConfig ChannelTestConfig
	require.NoError(t, db.First(&testConfig, "channel_id = ?", channel.Id).Error)
	require.NotNil(t, testConfig.TestCase)
	require.Equal(t, testCase, *testConfig.TestCase)
	require.NotNil(t, testConfig.ExpectedAnswer)
	require.Equal(t, expectedAnswer, *testConfig.ExpectedAnswer)
	require.NotNil(t, testConfig.MaxFirstTokenLatency)
	require.Equal(t, maxLatency, *testConfig.MaxFirstTokenLatency)
	require.NotNil(t, testConfig.ScheduledTestInterval)
	require.Equal(t, interval, *testConfig.ScheduledTestInterval)
	require.Equal(t, 3, testConfig.MaxRetryAttempts)
	require.True(t, testConfig.TreatEmptyReplyAsFailure)
}

func TestMigrateChannelExternalTablesMovesAccidentalFeatureConfigTable(t *testing.T) {
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

	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelBreakerState{}, &ChannelTestConfig{}, &legacyChannelFeatureConfig{}))
	channel := Channel{Name: "feature-table", Key: "sk-test"}
	require.NoError(t, db.Create(&channel).Error)
	coeff := 1.8
	require.NoError(t, db.Create(&legacyChannelFeatureConfig{
		ChannelID:                channel.Id,
		MaxRetryAttempts:         4,
		TreatEmptyReplyAsFailure: true,
		DynamicCircuitBreaker:    true,
		ToleranceCoefficient:     &coeff,
	}).Error)

	require.NoError(t, migrateChannelExternalTables())

	var breakerState ChannelBreakerState
	require.NoError(t, db.First(&breakerState, "channel_id = ?", channel.Id).Error)
	require.True(t, breakerState.DynamicCircuitBreaker)
	require.NotNil(t, breakerState.ToleranceCoefficient)
	require.Equal(t, coeff, *breakerState.ToleranceCoefficient)

	var testConfig ChannelTestConfig
	require.NoError(t, db.First(&testConfig, "channel_id = ?", channel.Id).Error)
	require.Equal(t, 4, testConfig.MaxRetryAttempts)
	require.True(t, testConfig.TreatEmptyReplyAsFailure)
	require.False(t, db.Migrator().HasTable(&legacyChannelFeatureConfig{}))
}
