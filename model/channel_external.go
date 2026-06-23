package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChannelBreakerState struct {
	ChannelID             int      `json:"-" gorm:"column:channel_id;primaryKey"`
	DynamicCircuitBreaker bool     `json:"dynamic_circuit_breaker" gorm:"column:dynamic_circuit_breaker"`
	ToleranceCoefficient  *float64 `json:"tolerance_coefficient" gorm:"column:tolerance_coefficient"`
	BreakerPressure       float64  `json:"-" gorm:"column:breaker_pressure;default:0"`
	BreakerUpdatedAt      int64    `json:"-" gorm:"column:breaker_updated_at;bigint;default:0"`
	BreakerFailStreak     int      `json:"-" gorm:"column:breaker_fail_streak;default:0"`
	BreakerCooldownAt     int64    `json:"-" gorm:"column:breaker_cooldown_at;bigint;default:0"`
	BreakerLastFailure    string   `json:"-" gorm:"column:breaker_last_failure;type:varchar(64);default:''"`
	BreakerHP             float64  `json:"-" gorm:"column:breaker_hp"`
	BreakerTripCount      int      `json:"-" gorm:"column:breaker_trip_count;default:0"`
	BreakerRecentRequests float64  `json:"-" gorm:"column:breaker_recent_requests;default:0"`
	BreakerRecentFailures float64  `json:"-" gorm:"column:breaker_recent_failures;default:0"`
	BreakerRecentTimeouts float64  `json:"-" gorm:"column:breaker_recent_timeouts;default:0"`
}

func (ChannelBreakerState) TableName() string {
	return "channel_breaker_states"
}

type ChannelTestConfig struct {
	ChannelID                int     `json:"-" gorm:"column:channel_id;primaryKey"`
	TestCase                 *string `json:"test_case" gorm:"type:text"`
	ExpectedAnswer           *string `json:"expected_answer" gorm:"type:text"`
	MaxFirstTokenLatency     *int    `json:"max_first_token_latency" gorm:"column:max_first_token_latency"`
	ScheduledTestInterval    *int    `json:"scheduled_test_interval" gorm:"column:scheduled_test_interval;default:0"`
	MaxRetryAttempts         int     `json:"max_retry_attempts" gorm:"column:max_retry_attempts;default:0"`
	TreatEmptyReplyAsFailure bool    `json:"treat_empty_reply_as_failure" gorm:"column:treat_empty_reply_as_failure"`
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
	channel.DynamicCircuitBreaker = state.DynamicCircuitBreaker
	channel.ToleranceCoefficient = state.ToleranceCoefficient
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
	channel.MaxRetryAttempts = config.MaxRetryAttempts
	channel.TreatEmptyReplyAsFailure = config.TreatEmptyReplyAsFailure
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
		DynamicCircuitBreaker: channel.DynamicCircuitBreaker,
		ToleranceCoefficient:  channel.ToleranceCoefficient,
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
		ChannelID:                channel.Id,
		TestCase:                 channel.TestCase,
		ExpectedAnswer:           channel.ExpectedAnswer,
		MaxFirstTokenLatency:     channel.MaxFirstTokenLatency,
		ScheduledTestInterval:    channel.ScheduledTestInterval,
		MaxRetryAttempts:         channel.MaxRetryAttempts,
		TreatEmptyReplyAsFailure: channel.TreatEmptyReplyAsFailure,
	}
}

func shouldPersistChannelTestConfig(channel *Channel) bool {
	if channel == nil {
		return false
	}
	return channel.TestCase != nil ||
		channel.ExpectedAnswer != nil ||
		channel.MaxFirstTokenLatency != nil ||
		channel.ScheduledTestInterval != nil ||
		channel.MaxRetryAttempts > 0 ||
		channel.TreatEmptyReplyAsFailure
}

func shouldPersistChannelBreakerConfig(channel *Channel) bool {
	if channel == nil {
		return false
	}
	return channel.DynamicCircuitBreaker || channel.ToleranceCoefficient != nil
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

func upsertLegacyChannelFeatureConfigTx(tx *gorm.DB, channel *Channel) error {
	if tx == nil {
		return errors.New("transaction is nil")
	}
	if channel == nil {
		return errors.New("channel is nil")
	}
	if channel.MaxRetryAttempts <= 0 && !channel.TreatEmptyReplyAsFailure {
		return nil
	}
	config := ChannelTestConfig{
		ChannelID:                channel.Id,
		MaxRetryAttempts:         channel.MaxRetryAttempts,
		TreatEmptyReplyAsFailure: channel.TreatEmptyReplyAsFailure,
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"max_retry_attempts",
			"treat_empty_reply_as_failure",
		}),
	}).Create(&config).Error
}

func normalizeLegacyChannelSetting(channel *Channel) {
	if channel == nil || channel.Setting == nil || strings.TrimSpace(*channel.Setting) == "" {
		return
	}

	var setting dto.ChannelSettings
	if err := common.Unmarshal([]byte(*channel.Setting), &setting); err != nil {
		return
	}
	if channel.MaxRetryAttempts <= 0 && setting.MaxRetryAttempts > 0 {
		channel.MaxRetryAttempts = setting.MaxRetryAttempts
	}
	if !channel.TreatEmptyReplyAsFailure && setting.TreatEmptyReplyAsFailure {
		channel.TreatEmptyReplyAsFailure = true
	}
	if !channel.DynamicCircuitBreaker && setting.DynamicCircuitBreaker {
		channel.DynamicCircuitBreaker = true
	}
	if channel.ToleranceCoefficient == nil && setting.ToleranceCoefficient != nil {
		channel.ToleranceCoefficient = setting.ToleranceCoefficient
	}

	var raw map[string]any
	if err := common.Unmarshal([]byte(*channel.Setting), &raw); err != nil {
		return
	}
	delete(raw, "max_retry_attempts")
	delete(raw, "treat_empty_reply_as_failure")
	delete(raw, "dynamic_circuit_breaker")
	delete(raw, "tolerance_coefficient")
	normalized, err := common.Marshal(raw)
	if err != nil {
		return
	}
	channel.Setting = common.GetPointer(string(normalized))
}

func normalizeLegacyChannelSettingIfExternalTablesExist(tx *gorm.DB, channel *Channel) {
	if tx == nil || !tx.Migrator().HasTable(&ChannelBreakerState{}) || !tx.Migrator().HasTable(&ChannelTestConfig{}) {
		return
	}
	normalizeLegacyChannelSetting(channel)
}

func saveChannelExternalFieldsTx(tx *gorm.DB, channel *Channel) error {
	normalizeLegacyChannelSettingIfExternalTablesExist(tx, channel)
	state := channel.breakerStateRecord()
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"dynamic_circuit_breaker",
			"tolerance_coefficient",
		}),
	}).Create(&state).Error; err != nil {
		return err
	}
	return saveChannelTestConfigTx(tx, channel)
}

func deleteChannelExternalFieldsTx(tx *gorm.DB, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Where("channel_id in ?", ids).Delete(&ChannelBreakerState{}).Error; err != nil {
		return err
	}
	if err := tx.Where("channel_id in ?", ids).Delete(&ChannelTestConfig{}).Error; err != nil {
		return err
	}
	return nil
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
			"dynamic_circuit_breaker",
			"tolerance_coefficient",
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

type legacyChannelSettingRow struct {
	Id      int     `gorm:"column:id"`
	Setting *string `gorm:"column:setting"`
}

type legacyChannelFeatureConfig struct {
	ChannelID                int      `gorm:"column:channel_id;primaryKey"`
	MaxRetryAttempts         int      `gorm:"column:max_retry_attempts"`
	TreatEmptyReplyAsFailure bool     `gorm:"column:treat_empty_reply_as_failure"`
	DynamicCircuitBreaker    bool     `gorm:"column:dynamic_circuit_breaker"`
	ToleranceCoefficient     *float64 `gorm:"column:tolerance_coefficient"`
}

func (legacyChannelFeatureConfig) TableName() string {
	return "channel_feature_configs"
}

func channelColumnName(name string) string {
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return fmt.Sprintf(`"%s"`, name)
	}
	return fmt.Sprintf("`%s`", name)
}

func migrateLegacyChannelSettingExternalFields() error {
	if !DB.Migrator().HasColumn(&Channel{}, "setting") {
		return nil
	}
	var rows []legacyChannelSettingRow
	if err := DB.Model(&Channel{}).
		Select("id", "setting").
		Where("setting IS NOT NULL AND setting != ''").
		Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if row.Setting == nil || strings.TrimSpace(*row.Setting) == "" {
			continue
		}

		channel := &Channel{Id: row.Id, Setting: row.Setting}
		normalizeLegacyChannelSetting(channel)
		if !shouldPersistChannelBreakerConfig(channel) && !shouldPersistChannelTestConfig(channel) && channel.Setting != nil && *channel.Setting == *row.Setting {
			continue
		}

		err := DB.Transaction(func(tx *gorm.DB) error {
			if shouldPersistChannelBreakerConfig(channel) {
				state := channel.breakerStateRecord()
				if err := tx.Clauses(clause.OnConflict{
					Columns: []clause.Column{{Name: "channel_id"}},
					DoUpdates: clause.AssignmentColumns([]string{
						"dynamic_circuit_breaker",
						"tolerance_coefficient",
					}),
				}).Create(&state).Error; err != nil {
					return err
				}
			}
			if channel.MaxRetryAttempts > 0 || channel.TreatEmptyReplyAsFailure {
				if err := upsertLegacyChannelFeatureConfigTx(tx, channel); err != nil {
					return err
				}
			}
			if channel.Setting == nil {
				return tx.Model(&Channel{}).Where("id = ?", row.Id).Update("setting", nil).Error
			}
			return tx.Model(&Channel{}).Where("id = ?", row.Id).Update("setting", *channel.Setting).Error
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func migrateLegacyChannelFeatureConfigTable() error {
	if !DB.Migrator().HasTable(&legacyChannelFeatureConfig{}) {
		return nil
	}

	var configs []legacyChannelFeatureConfig
	if err := DB.Find(&configs).Error; err != nil {
		return err
	}
	for _, config := range configs {
		if config.ChannelID == 0 {
			continue
		}
		channel := &Channel{
			Id:                       config.ChannelID,
			MaxRetryAttempts:         config.MaxRetryAttempts,
			TreatEmptyReplyAsFailure: config.TreatEmptyReplyAsFailure,
			DynamicCircuitBreaker:    config.DynamicCircuitBreaker,
			ToleranceCoefficient:     config.ToleranceCoefficient,
		}

		err := DB.Transaction(func(tx *gorm.DB) error {
			if shouldPersistChannelBreakerConfig(channel) {
				state := channel.breakerStateRecord()
				if err := tx.Clauses(clause.OnConflict{
					Columns: []clause.Column{{Name: "channel_id"}},
					DoUpdates: clause.AssignmentColumns([]string{
						"dynamic_circuit_breaker",
						"tolerance_coefficient",
					}),
				}).Create(&state).Error; err != nil {
					return err
				}
			}
			if channel.MaxRetryAttempts > 0 || channel.TreatEmptyReplyAsFailure {
				return upsertLegacyChannelFeatureConfigTx(tx, channel)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	return DB.Migrator().DropTable(&legacyChannelFeatureConfig{})
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
		sql := buildLegacyExternalColumnUpsertSQL("channel_breaker_states", insertCols, selectCols)
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
		sql := buildLegacyExternalColumnUpsertSQL("channel_test_configs", insertCols, selectCols)
		if err := DB.Exec(sql).Error; err != nil {
			return err
		}
	}

	if err := migrateLegacyChannelSettingExternalFields(); err != nil {
		return err
	}
	if err := migrateLegacyChannelFeatureConfigTable(); err != nil {
		return err
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

func buildLegacyExternalColumnUpsertSQL(table string, insertCols []string, selectCols string) string {
	tableName := channelColumnName(table)
	insertColsSQL := strings.Join(quoteColumns(insertCols), ", ")
	base := fmt.Sprintf("INSERT INTO %s (channel_id, %s) SELECT %s FROM %s",
		tableName,
		insertColsSQL,
		selectCols,
		channelColumnName("channels"),
	)
	// SQLite needs a WHERE clause here to disambiguate SELECT ... ON CONFLICT.
	base += " WHERE true"
	assignments := make([]string, 0, len(insertCols))
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		for _, col := range insertCols {
			quoted := channelColumnName(col)
			assignments = append(assignments, fmt.Sprintf("%s = VALUES(%s)", quoted, quoted))
		}
		return base + " ON DUPLICATE KEY UPDATE " + strings.Join(assignments, ", ")
	}
	for _, col := range insertCols {
		quoted := channelColumnName(col)
		assignments = append(assignments, fmt.Sprintf("%s = excluded.%s", quoted, quoted))
	}
	return base + " ON CONFLICT (channel_id) DO UPDATE SET " + strings.Join(assignments, ", ")
}
