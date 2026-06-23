package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChannelBreakerState struct {
	ChannelID             int     `json:"-" gorm:"column:channel_id;primaryKey"`
	BreakerPressure       float64 `json:"-" gorm:"column:breaker_pressure;default:0"`
	BreakerUpdatedAt      int64   `json:"-" gorm:"column:breaker_updated_at;bigint;default:0"`
	BreakerFailStreak     int     `json:"-" gorm:"column:breaker_fail_streak;default:0"`
	BreakerCooldownAt     int64   `json:"-" gorm:"column:breaker_cooldown_at;bigint;default:0"`
	BreakerLastFailure    string  `json:"-" gorm:"column:breaker_last_failure;type:varchar(64);default:''"`
	BreakerHP             float64 `json:"-" gorm:"column:breaker_hp"`
	BreakerTripCount      int     `json:"-" gorm:"column:breaker_trip_count;default:0"`
	BreakerRecentRequests float64 `json:"-" gorm:"column:breaker_recent_requests;default:0"`
	BreakerRecentFailures float64 `json:"-" gorm:"column:breaker_recent_failures;default:0"`
	BreakerRecentTimeouts float64 `json:"-" gorm:"column:breaker_recent_timeouts;default:0"`
}

func (ChannelBreakerState) TableName() string {
	return "channel_breaker_states"
}

type ChannelTestConfig struct {
	ChannelID             int     `json:"-" gorm:"column:channel_id;primaryKey"`
	TestCase              *string `json:"test_case" gorm:"type:text"`
	ExpectedAnswer        *string `json:"expected_answer" gorm:"type:text"`
	MaxFirstTokenLatency  *int    `json:"max_first_token_latency" gorm:"column:max_first_token_latency"`
	ScheduledTestInterval *int    `json:"scheduled_test_interval" gorm:"column:scheduled_test_interval;default:0"`
}

func (ChannelTestConfig) TableName() string {
	return "channel_test_configs"
}

type legacyChannelExternalColumns struct {
	BreakerPressure       float64 `gorm:"column:breaker_pressure"`
	BreakerUpdatedAt      int64   `gorm:"column:breaker_updated_at"`
	BreakerFailStreak     int     `gorm:"column:breaker_fail_streak"`
	BreakerCooldownAt     int64   `gorm:"column:breaker_cooldown_at"`
	BreakerLastFailure    string  `gorm:"column:breaker_last_failure"`
	BreakerHP             float64 `gorm:"column:breaker_hp"`
	BreakerTripCount      int     `gorm:"column:breaker_trip_count"`
	BreakerRecentRequests float64 `gorm:"column:breaker_recent_requests"`
	BreakerRecentFailures float64 `gorm:"column:breaker_recent_failures"`
	BreakerRecentTimeouts float64 `gorm:"column:breaker_recent_timeouts"`
	TestCase              *string `gorm:"column:test_case"`
	ExpectedAnswer        *string `gorm:"column:expected_answer"`
	MaxFirstTokenLatency  *int    `gorm:"column:max_first_token_latency"`
	ScheduledTestInterval *int    `gorm:"column:scheduled_test_interval"`
}

func (legacyChannelExternalColumns) TableName() string {
	return "channels"
}

func (channel *Channel) applyDefaultExternalFields() {
	if channel == nil {
		return
	}
	channel.BreakerHP = -1
}

func (channel *Channel) applyBreakerState(state ChannelBreakerState) {
	if channel == nil {
		return
	}
	channel.BreakerPressure = state.BreakerPressure
	channel.BreakerUpdatedAt = state.BreakerUpdatedAt
	channel.BreakerFailStreak = state.BreakerFailStreak
	channel.BreakerCooldownAt = state.BreakerCooldownAt
	channel.BreakerLastFailure = state.BreakerLastFailure
	channel.BreakerHP = state.BreakerHP
	channel.BreakerTripCount = state.BreakerTripCount
	channel.BreakerRecentRequests = state.BreakerRecentRequests
	channel.BreakerRecentFailures = state.BreakerRecentFailures
	channel.BreakerRecentTimeouts = state.BreakerRecentTimeouts
}

func (channel *Channel) applyTestConfig(config ChannelTestConfig) {
	if channel == nil {
		return
	}
	channel.TestCase = config.TestCase
	channel.ExpectedAnswer = config.ExpectedAnswer
	channel.MaxFirstTokenLatency = config.MaxFirstTokenLatency
	channel.ScheduledTestInterval = config.ScheduledTestInterval
}

func (channel *Channel) breakerStateRecord() ChannelBreakerState {
	hp := channel.BreakerHP
	if channel.BreakerPressure == 0 &&
		channel.BreakerUpdatedAt == 0 &&
		channel.BreakerFailStreak == 0 &&
		channel.BreakerCooldownAt == 0 &&
		channel.BreakerLastFailure == "" &&
		channel.BreakerTripCount == 0 &&
		channel.BreakerRecentRequests == 0 &&
		channel.BreakerRecentFailures == 0 &&
		channel.BreakerRecentTimeouts == 0 &&
		hp == 0 {
		hp = -1
	}
	return ChannelBreakerState{
		ChannelID:             channel.Id,
		BreakerPressure:       channel.BreakerPressure,
		BreakerUpdatedAt:      channel.BreakerUpdatedAt,
		BreakerFailStreak:     channel.BreakerFailStreak,
		BreakerCooldownAt:     channel.BreakerCooldownAt,
		BreakerLastFailure:    channel.BreakerLastFailure,
		BreakerHP:             hp,
		BreakerTripCount:      channel.BreakerTripCount,
		BreakerRecentRequests: channel.BreakerRecentRequests,
		BreakerRecentFailures: channel.BreakerRecentFailures,
		BreakerRecentTimeouts: channel.BreakerRecentTimeouts,
	}
}

func (channel *Channel) testConfigRecord() ChannelTestConfig {
	return ChannelTestConfig{
		ChannelID:             channel.Id,
		TestCase:              channel.TestCase,
		ExpectedAnswer:        channel.ExpectedAnswer,
		MaxFirstTokenLatency:  channel.MaxFirstTokenLatency,
		ScheduledTestInterval: channel.ScheduledTestInterval,
	}
}

func shouldPersistChannelTestConfig(channel *Channel) bool {
	if channel == nil {
		return false
	}
	return channel.TestCase != nil ||
		channel.ExpectedAnswer != nil ||
		channel.MaxFirstTokenLatency != nil ||
		channel.ScheduledTestInterval != nil
}

func loadChannelExternalFieldsTx(tx *gorm.DB, channels []*Channel) error {
	if tx == nil {
		return errors.New("transaction is nil")
	}
	if len(channels) == 0 {
		return nil
	}

	ids := make([]int, 0, len(channels))
	byID := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		if channel == nil || channel.Id == 0 {
			continue
		}
		channel.applyDefaultExternalFields()
		ids = append(ids, channel.Id)
		byID[channel.Id] = channel
	}
	if len(ids) == 0 {
		return nil
	}

	var states []ChannelBreakerState
	if err := tx.Where("channel_id in ?", ids).Find(&states).Error; err != nil {
		return err
	}
	for _, state := range states {
		if channel, ok := byID[state.ChannelID]; ok {
			channel.applyBreakerState(state)
		}
	}

	var configs []ChannelTestConfig
	if err := tx.Where("channel_id in ?", ids).Find(&configs).Error; err != nil {
		return err
	}
	for _, config := range configs {
		if channel, ok := byID[config.ChannelID]; ok {
			channel.applyTestConfig(config)
		}
	}
	return nil
}

func LoadChannelExternalFields(channels ...*Channel) error {
	return loadChannelExternalFieldsTx(DB, channels)
}

func saveChannelTestConfigTx(tx *gorm.DB, channel *Channel) error {
	if tx == nil {
		return errors.New("transaction is nil")
	}
	if channel == nil {
		return errors.New("channel is nil")
	}
	if shouldPersistChannelTestConfig(channel) {
		config := channel.testConfigRecord()
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "channel_id"}},
			UpdateAll: true,
		}).Create(&config).Error
	}
	return tx.Where("channel_id = ?", channel.Id).Delete(&ChannelTestConfig{}).Error
}

func saveChannelExternalFieldsTx(tx *gorm.DB, channel *Channel) error {
	state := channel.breakerStateRecord()
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}},
		DoNothing: true,
	}).Create(&state).Error; err != nil {
		return err
	}
	if err := saveChannelTestConfigTx(tx, channel); err != nil {
		return err
	}
	return nil
}

func deleteChannelExternalFieldsTx(tx *gorm.DB, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Where("channel_id in ?", ids).Delete(&ChannelBreakerState{}).Error; err != nil {
		return err
	}
	return tx.Where("channel_id in ?", ids).Delete(&ChannelTestConfig{}).Error
}

func UpdateChannelBreakerStateTx(tx *gorm.DB, channel *Channel) error {
	if tx == nil {
		return errors.New("transaction is nil")
	}
	if channel == nil {
		return errors.New("channel is nil")
	}
	state := channel.breakerStateRecord()
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
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
		}),
	}).Create(&state).Error
}

func UpdateChannelBreakerState(channel *Channel) error {
	if channel == nil {
		return errors.New("channel is nil")
	}
	if err := UpdateChannelBreakerStateTx(DB, channel); err != nil {
		return err
	}
	CacheUpdateChannelBreakerState(channel)
	return nil
}

func channelColumnName(name string) string {
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return fmt.Sprintf(`"%s"`, name)
	}
	return fmt.Sprintf("`%s`", name)
}

func migrateChannelExternalTables() error {
	breakerCols := []string{
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
	}
	testCols := []string{
		"test_case",
		"expected_answer",
		"max_first_token_latency",
		"scheduled_test_interval",
	}

	hasBreakerCols := false
	for _, col := range breakerCols {
		if DB.Migrator().HasColumn(&legacyChannelExternalColumns{}, col) {
			hasBreakerCols = true
			break
		}
	}
	hasTestCols := false
	for _, col := range testCols {
		if DB.Migrator().HasColumn(&legacyChannelExternalColumns{}, col) {
			hasTestCols = true
			break
		}
	}

	if hasBreakerCols {
		selectCols := "id"
		insertCols := make([]string, 0, len(breakerCols))
		for _, col := range breakerCols {
			if DB.Migrator().HasColumn(&legacyChannelExternalColumns{}, col) {
				selectCols += ", " + channelColumnName(col)
				insertCols = append(insertCols, col)
			}
		}
		sql := fmt.Sprintf("INSERT INTO %s (channel_id, %s) SELECT %s FROM %s ON CONFLICT (channel_id) DO NOTHING",
			channelColumnName("channel_breaker_states"),
			strings.Join(quoteColumns(insertCols), ", "),
			selectCols,
			channelColumnName("channels"),
		)
		if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			sql = fmt.Sprintf("INSERT OR IGNORE INTO %s (channel_id, %s) SELECT %s FROM %s",
				channelColumnName("channel_breaker_states"),
				strings.Join(quoteColumns(insertCols), ", "),
				selectCols,
				channelColumnName("channels"),
			)
		} else if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
			sql = fmt.Sprintf("INSERT IGNORE INTO %s (channel_id, %s) SELECT %s FROM %s",
				channelColumnName("channel_breaker_states"),
				strings.Join(quoteColumns(insertCols), ", "),
				selectCols,
				channelColumnName("channels"),
			)
		}
		if err := DB.Exec(sql).Error; err != nil {
			return err
		}
	}

	if hasTestCols {
		selectCols := "id"
		insertCols := make([]string, 0, len(testCols))
		for _, col := range testCols {
			if DB.Migrator().HasColumn(&legacyChannelExternalColumns{}, col) {
				selectCols += ", " + channelColumnName(col)
				insertCols = append(insertCols, col)
			}
		}
		sql := fmt.Sprintf("INSERT INTO %s (channel_id, %s) SELECT %s FROM %s ON CONFLICT (channel_id) DO NOTHING",
			channelColumnName("channel_test_configs"),
			strings.Join(quoteColumns(insertCols), ", "),
			selectCols,
			channelColumnName("channels"),
		)
		if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			sql = fmt.Sprintf("INSERT OR IGNORE INTO %s (channel_id, %s) SELECT %s FROM %s",
				channelColumnName("channel_test_configs"),
				strings.Join(quoteColumns(insertCols), ", "),
				selectCols,
				channelColumnName("channels"),
			)
		} else if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
			sql = fmt.Sprintf("INSERT IGNORE INTO %s (channel_id, %s) SELECT %s FROM %s",
				channelColumnName("channel_test_configs"),
				strings.Join(quoteColumns(insertCols), ", "),
				selectCols,
				channelColumnName("channels"),
			)
		}
		if err := DB.Exec(sql).Error; err != nil {
			return err
		}
	}

	legacyCols := append(breakerCols, testCols...)
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return dropSQLiteChannelColumns(legacyCols)
	}
	for _, col := range append(breakerCols, testCols...) {
		if DB.Migrator().HasColumn(&legacyChannelExternalColumns{}, col) {
			if err := DB.Migrator().DropColumn(&legacyChannelExternalColumns{}, col); err != nil {
				return err
			}
		}
	}
	return nil
}

type sqliteTableColumn struct {
	Cid       int     `gorm:"column:cid"`
	Name      string  `gorm:"column:name"`
	Type      string  `gorm:"column:type"`
	NotNull   int     `gorm:"column:notnull"`
	Default   *string `gorm:"column:dflt_value"`
	PrimaryPK int     `gorm:"column:pk"`
}

func dropSQLiteChannelColumns(dropCols []string) error {
	dropSet := make(map[string]struct{}, len(dropCols))
	for _, col := range dropCols {
		dropSet[col] = struct{}{}
	}
	var cols []sqliteTableColumn
	if err := DB.Raw("PRAGMA table_info(`channels`)").Scan(&cols).Error; err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	kept := make([]sqliteTableColumn, 0, len(cols))
	hasDrop := false
	for _, col := range cols {
		if _, ok := dropSet[col.Name]; ok {
			hasDrop = true
			continue
		}
		kept = append(kept, col)
	}
	if !hasDrop {
		return nil
	}
	if len(kept) == 0 {
		return errors.New("cannot drop all channels columns")
	}

	columnDefs := make([]string, 0, len(kept))
	columnNames := make([]string, 0, len(kept))
	for _, col := range kept {
		name := channelColumnName(col.Name)
		columnNames = append(columnNames, name)
		typ := strings.TrimSpace(col.Type)
		if typ == "" {
			typ = "text"
		}
		def := name + " " + typ
		if col.PrimaryPK > 0 {
			def += " PRIMARY KEY"
		}
		if col.NotNull > 0 {
			def += " NOT NULL"
		}
		if col.Default != nil {
			def += " DEFAULT " + *col.Default
		}
		columnDefs = append(columnDefs, def)
	}

	tmpTable := "channels_external_migration_tmp"
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DROP TABLE IF EXISTS `" + tmpTable + "`").Error; err != nil {
			return err
		}
		if err := tx.Exec("CREATE TABLE `" + tmpTable + "` (" + strings.Join(columnDefs, ", ") + ")").Error; err != nil {
			return err
		}
		colsSQL := strings.Join(columnNames, ", ")
		if err := tx.Exec("INSERT INTO `" + tmpTable + "` (" + colsSQL + ") SELECT " + colsSQL + " FROM `channels`").Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP TABLE `channels`").Error; err != nil {
			return err
		}
		if err := tx.Exec("ALTER TABLE `" + tmpTable + "` RENAME TO `channels`").Error; err != nil {
			return err
		}
		return nil
	})
}

func quoteColumns(cols []string) []string {
	quoted := make([]string, 0, len(cols))
	for _, col := range cols {
		quoted = append(quoted, channelColumnName(col))
	}
	return quoted
}
